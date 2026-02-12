package remote

// Script is uploaded and executed on the target VPS.
const Script = `#!/usr/bin/env bash
set -euo pipefail

log() {
  printf '[remote] %s\n' "$*" >&2
}

die() {
  printf '[remote] ERROR: %s\n' "$*" >&2
  exit 1
}

is_valid_port() {
  local port="$1"
  [[ "$port" =~ ^[0-9]+$ ]] || return 1
  (( port >= 1 && port <= 65535 ))
}

generate_secret() {
  local charset="$1"
  local len="$2"
  local token=""
  while [[ "${#token}" -lt "$len" ]]; do
    token+="$(LC_ALL=C tr -dc "$charset" </dev/urandom | head -c "$len" || true)"
  done
  printf '%s' "${token:0:len}"
}

read_env_value() {
  local file="$1"
  local key="$2"
  if [[ ! -f "$file" ]]; then
    return 1
  fi
  grep -m1 "^${key}=" "$file" | cut -d= -f2- || true
}

service_defined() {
  local unit="$1"
  systemctl cat "$unit" >/dev/null 2>&1
}

service_active() {
  local unit="$1"
  if systemctl is-active --quiet "$unit" 2>/dev/null; then
    printf '1'
  else
    printf '0'
  fi
}

get_public_ip() {
  local ip
  ip="$(curl -4fsS https://api.ipify.org 2>/dev/null || true)"
  if [[ -z "$ip" ]]; then
    ip="$(curl -4fsS https://ifconfig.me 2>/dev/null || true)"
  fi
  if [[ -z "$ip" ]]; then
    ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
  fi
  if [[ -z "$ip" ]]; then
    ip="UNKNOWN"
  fi
  printf '%s' "$ip"
}

find_squid_auth_helper() {
  local candidate
  for candidate in \
    /usr/lib/squid/basic_ncsa_auth \
    /usr/lib64/squid/basic_ncsa_auth \
    /usr/lib/squid3/basic_ncsa_auth
  do
    if [[ -x "$candidate" ]]; then
      printf '%s' "$candidate"
      return 0
    fi
  done
  return 1
}

port_in_use() {
  local port="$1"
  if command -v ss >/dev/null 2>&1; then
    ss -ltn "( sport = :$port )" | tail -n +2 | grep -q .
    return $?
  fi
  if command -v netstat >/dev/null 2>&1; then
    netstat -ltn 2>/dev/null | awk '{print $4}' | grep -qE "[:.]${port}$"
    return $?
  fi
  return 1
}

ensure_port_available() {
  local desired="$1"
  local current="$2"
  if [[ -n "$current" && "$desired" == "$current" ]]; then
    return 0
  fi
  if port_in_use "$desired"; then
    die "Port $desired is already in use."
  fi
}

ensure_requirements() {
  [[ -f /etc/os-release ]] || die "Cannot detect distro (/etc/os-release missing)."
  . /etc/os-release

  case "${ID:-}" in
    ubuntu|debian)
      ;;
    *)
      die "Unsupported distro: ${ID:-unknown}. v2 supports Debian/Ubuntu only."
      ;;
  esac

  (( EUID == 0 )) || die "This installer must run as root."
  command -v apt-get >/dev/null 2>&1 || die "apt-get is required."
  command -v systemctl >/dev/null 2>&1 || die "systemd is required."
}

ensure_packages() {
  local install_needed=0
  local pkg
  local log_file="/tmp/beammeup-install.log"

  for pkg in "$@"; do
    if ! dpkg -s "$pkg" >/dev/null 2>&1; then
      install_needed=1
      break
    fi
  done

  if [[ "$install_needed" -eq 0 ]]; then
    return 0
  fi

  : >"$log_file"
  log "Installing packages: $*"

  if ! DEBIAN_FRONTEND=noninteractive apt-get update >>"$log_file" 2>&1; then
    tail -n 50 "$log_file" >&2 || true
    die "apt-get update failed."
  fi

  if ! DEBIAN_FRONTEND=noninteractive apt-get install -y "$@" >>"$log_file" 2>&1; then
    tail -n 50 "$log_file" >&2 || true
    die "apt-get install failed."
  fi
}

apply_firewall_rule() {
  local port="$1"
  FIREWALL_NOTE="No firewall update applied (port may already be reachable)."

  if [[ "$NO_FIREWALL_CHANGE" -eq 1 ]]; then
    FIREWALL_NOTE="Skipped firewall changes by request."
    return
  fi

  if command -v ufw >/dev/null 2>&1; then
    local ufw_state
    ufw_state="$(ufw status 2>/dev/null | head -n 1 || true)"
    if [[ "$ufw_state" == "Status: active" ]]; then
      if ufw allow "${port}/tcp" >/dev/null 2>&1; then
        FIREWALL_NOTE="Opened TCP ${port} via UFW."
      else
        FIREWALL_NOTE="UFW active, but failed to open TCP ${port}."
      fi
      return
    fi
  fi

  FIREWALL_NOTE="Firewall not modified. Open TCP ${port} manually if blocked."
}

cleanup_firewall_rule() {
  local port="$1"
  [[ -n "$port" ]] || return 0
  is_valid_port "$port" || return 0

  if command -v ufw >/dev/null 2>&1; then
    local ufw_state
    ufw_state="$(ufw status 2>/dev/null | head -n 1 || true)"
    if [[ "$ufw_state" == "Status: active" ]]; then
      ufw delete allow "${port}/tcp" >/dev/null 2>&1 || true
    fi
  fi
}

BEAM_DIR="/etc/beammeup"
SOCKS_ENV="${BEAM_DIR}/microsocks.env"
SOCKS_SERVICE="beammeup-microsocks.service"
SOCKS_SERVICE_FILE="/etc/systemd/system/${SOCKS_SERVICE}"
HTTP_ENV="${BEAM_DIR}/http.env"
HTTP_HTPASSWD="${BEAM_DIR}/http.htpasswd"
SQUID_CONF="/etc/squid/squid.conf"
SQUID_BACKUP="/etc/squid/squid.conf.beammeup.bak"
HANGAR_META="${BEAM_DIR}/hangar.json"

SOCKS_EXISTS=0
SOCKS_ACTIVE=0
SOCKS_PORT=""
SOCKS_USER=""
SOCKS_PASS=""

HTTP_EXISTS=0
HTTP_ACTIVE=0
HTTP_PORT=""
HTTP_USER=""
HTTP_PASS=""
HTTP_MANAGED=0
HTTP_LEGACY=0

HANGAR_STATUS="missing"
METADATA_EXISTS=0

load_socks_state() {
  SOCKS_EXISTS=0
  SOCKS_ACTIVE=0
  SOCKS_PORT=""
  SOCKS_USER=""
  SOCKS_PASS=""

  if [[ -f "$SOCKS_ENV" || -f "$SOCKS_SERVICE_FILE" ]]; then
    SOCKS_EXISTS=1
  fi

  SOCKS_PORT="$(read_env_value "$SOCKS_ENV" PROXY_PORT || true)"
  SOCKS_USER="$(read_env_value "$SOCKS_ENV" PROXY_USER || true)"
  SOCKS_PASS="$(read_env_value "$SOCKS_ENV" PROXY_PASS || true)"

  if service_defined "$SOCKS_SERVICE"; then
    SOCKS_EXISTS=1
    SOCKS_ACTIVE="$(service_active "$SOCKS_SERVICE")"
  fi
}

load_http_state() {
  HTTP_EXISTS=0
  HTTP_ACTIVE=0
  HTTP_PORT=""
  HTTP_USER=""
  HTTP_PASS=""
  HTTP_MANAGED=0
  HTTP_LEGACY=0

  HTTP_PORT="$(read_env_value "$HTTP_ENV" PROXY_PORT || true)"
  HTTP_USER="$(read_env_value "$HTTP_ENV" PROXY_USER || true)"
  HTTP_PASS="$(read_env_value "$HTTP_ENV" PROXY_PASS || true)"

  if [[ -f "$HTTP_ENV" ]]; then
    HTTP_EXISTS=1
    HTTP_MANAGED=1
  fi

  if [[ -f "$SQUID_CONF" ]]; then
    if grep -q "managed by beammeup" "$SQUID_CONF"; then
      HTTP_EXISTS=1
      HTTP_MANAGED=1
    elif grep -q "beammeup-proxy" "$SQUID_CONF"; then
      HTTP_EXISTS=1
      HTTP_LEGACY=1
    fi

    if [[ -z "$HTTP_PORT" ]]; then
      HTTP_PORT="$(awk '/^http_port[[:space:]]+/ {print $2; exit}' "$SQUID_CONF" 2>/dev/null || true)"
    fi
  fi

  if [[ -z "$HTTP_USER" && -f "$HTTP_HTPASSWD" ]]; then
    HTTP_USER="$(awk -F: 'NR==1 {print $1}' "$HTTP_HTPASSWD" 2>/dev/null || true)"
  fi

  if service_defined "squid.service"; then
    if [[ "$HTTP_EXISTS" == "1" ]]; then
      HTTP_ACTIVE="$(service_active "squid.service")"
    fi
  fi
}

write_hangar_metadata() {
  local status="$1"
  local notes="$2"
  mkdir -p "$BEAM_DIR"
  cat >"$HANGAR_META" <<EOF_META
{
  "version": "1",
  "updated_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "status": "${status}",
  "notes": "${notes}",
  "http": {
    "exists": ${HTTP_EXISTS},
    "active": ${HTTP_ACTIVE},
    "port": "${HTTP_PORT}",
    "user": "${HTTP_USER}"
  },
  "socks5": {
    "exists": ${SOCKS_EXISTS},
    "active": ${SOCKS_ACTIVE},
    "port": "${SOCKS_PORT}",
    "user": "${SOCKS_USER}"
  }
}
EOF_META
  chmod 600 "$HANGAR_META"
}

reconcile_hangar_status() {
  METADATA_EXISTS=0
  if [[ -f "$HANGAR_META" ]]; then
    METADATA_EXISTS=1
  fi

  local any_exists=0
  if [[ "$SOCKS_EXISTS" == "1" || "$HTTP_EXISTS" == "1" ]]; then
    any_exists=1
  fi

  if [[ "$any_exists" == "0" ]]; then
    if [[ "$METADATA_EXISTS" == "1" ]]; then
      HANGAR_STATUS="drift"
    else
      HANGAR_STATUS="missing"
    fi
    return
  fi

  if [[ "$SOCKS_EXISTS" == "1" && "$SOCKS_ACTIVE" != "1" ]]; then
    HANGAR_STATUS="drift"
  elif [[ "$HTTP_EXISTS" == "1" && "$HTTP_ACTIVE" != "1" ]]; then
    HANGAR_STATUS="drift"
  else
    HANGAR_STATUS="online"
  fi

  if [[ "$METADATA_EXISTS" == "0" ]]; then
    write_hangar_metadata "$HANGAR_STATUS" "reconstructed metadata from managed config"
    METADATA_EXISTS=1
  fi
}

print_inventory() {
  load_socks_state
  load_http_state
  reconcile_hangar_status

  if [[ "$METADATA_EXISTS" == "1" ]]; then
    write_hangar_metadata "$HANGAR_STATUS" "inventory refresh"
  fi

  printf 'BM_PUBLIC_IP=%s\n' "$(get_public_ip)"

  printf 'BM_SOCKS_EXISTS=%s\n' "$SOCKS_EXISTS"
  printf 'BM_SOCKS_ACTIVE=%s\n' "$SOCKS_ACTIVE"
  printf 'BM_SOCKS_PORT=%s\n' "$SOCKS_PORT"
  printf 'BM_SOCKS_USER=%s\n' "$SOCKS_USER"
  printf 'BM_SOCKS_PASS=%s\n' "$SOCKS_PASS"

  printf 'BM_HTTP_EXISTS=%s\n' "$HTTP_EXISTS"
  printf 'BM_HTTP_ACTIVE=%s\n' "$HTTP_ACTIVE"
  printf 'BM_HTTP_MANAGED=%s\n' "$HTTP_MANAGED"
  printf 'BM_HTTP_LEGACY=%s\n' "$HTTP_LEGACY"
  printf 'BM_HTTP_PORT=%s\n' "$HTTP_PORT"
  printf 'BM_HTTP_USER=%s\n' "$HTTP_USER"
  printf 'BM_HTTP_PASS=%s\n' "$HTTP_PASS"

  printf 'BM_HANGAR_STATUS=%s\n' "$HANGAR_STATUS"
  printf 'BM_METADATA_EXISTS=%s\n' "$METADATA_EXISTS"
}

emit_result() {
  local protocol="$1"
  local port="$2"
  local user="$3"
  local pass="$4"
  local action="$5"
  local note="$6"

  printf 'BM_RESULT_PROTOCOL=%s\n' "$protocol"
  printf 'BM_RESULT_HOST=%s\n' "$(get_public_ip)"
  printf 'BM_RESULT_PORT=%s\n' "$port"
  printf 'BM_RESULT_USER=%s\n' "$user"
  printf 'BM_RESULT_PASS=%s\n' "$pass"
  printf 'BM_RESULT_ACTION=%s\n' "$action"
  printf 'BM_RESULT_FIREWALL_NOTE=%s\n' "${FIREWALL_NOTE:-}"
  printf 'BM_RESULT_NOTE=%s\n' "$note"
}

run_preflight() {
  ensure_requirements
  load_socks_state
  load_http_state

  local chosen_port="${PROXY_PORT:-}"
  local current_port=""

  if [[ "$PROTOCOL" == "socks5" ]]; then
    current_port="$SOCKS_PORT"
    if [[ -z "$chosen_port" ]]; then
      chosen_port="${SOCKS_PORT:-1080}"
    fi
  else
    current_port="$HTTP_PORT"
    if [[ -z "$chosen_port" ]]; then
      chosen_port="${HTTP_PORT:-18181}"
    fi
  fi

  is_valid_port "$chosen_port" || die "Invalid proxy port: $chosen_port"
  ensure_port_available "$chosen_port" "$current_port"

  printf 'BM_PREFLIGHT=OK\n'
  printf 'BM_PREFLIGHT_PROTOCOL=%s\n' "$PROTOCOL"
  printf 'BM_PREFLIGHT_PORT=%s\n' "$chosen_port"
}

apply_socks() {
  ensure_requirements
  ensure_packages microsocks curl

  mkdir -p "$BEAM_DIR"

  if ! id -u beammeup >/dev/null 2>&1; then
    useradd --system --no-create-home --shell /usr/sbin/nologin beammeup
  fi

  load_socks_state
  load_http_state

  local existed="$SOCKS_EXISTS"
  local desired_port="${PROXY_PORT:-${SOCKS_PORT:-1080}}"
  local final_user="$SOCKS_USER"
  local final_pass="$SOCKS_PASS"
  local note=""

  is_valid_port "$desired_port" || die "Invalid proxy port: $desired_port"
  ensure_port_available "$desired_port" "$SOCKS_PORT"

  if [[ -z "$final_user" || "$ROTATE_CREDENTIALS" -eq 1 ]]; then
    final_user="beam$(generate_secret 'a-z0-9' 5)"
  fi
  if [[ -z "$final_pass" || "$ROTATE_CREDENTIALS" -eq 1 ]]; then
    final_pass="$(generate_secret 'A-Za-z0-9' 20)"
  fi

  local microsocks_bin
  microsocks_bin="$(command -v microsocks || true)"
  [[ -n "$microsocks_bin" ]] || die "microsocks binary not found after install."

  cat >"$SOCKS_ENV" <<EOF_ENV
PROXY_PORT=$desired_port
PROXY_USER=$final_user
PROXY_PASS=$final_pass
EOF_ENV
  chmod 600 "$SOCKS_ENV"

  cat >"$SOCKS_SERVICE_FILE" <<EOF_UNIT
[Unit]
Description=Beammeup SOCKS5 Proxy
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=beammeup
Group=beammeup
EnvironmentFile=$SOCKS_ENV
ExecStart=$microsocks_bin -i 0.0.0.0 -p \${PROXY_PORT} -u \${PROXY_USER} -P \${PROXY_PASS}
Restart=always
RestartSec=2
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=strict
LimitNOFILE=32768

[Install]
WantedBy=multi-user.target
EOF_UNIT
  chmod 644 "$SOCKS_SERVICE_FILE"

  systemctl daemon-reload
  systemctl enable --now "$SOCKS_SERVICE"
  if ! systemctl is-active --quiet "$SOCKS_SERVICE"; then
    journalctl -u "$SOCKS_SERVICE" -n 50 --no-pager >&2 || true
    die "SOCKS5 service failed to start."
  fi

  apply_firewall_rule "$desired_port"

  if [[ "$ROTATE_CREDENTIALS" -eq 1 ]]; then
    note="Credentials rotated."
  fi

  load_socks_state
  load_http_state
  reconcile_hangar_status
  write_hangar_metadata "$HANGAR_STATUS" "updated socks5"

  emit_result "SOCKS5" "$desired_port" "$final_user" "$final_pass" \
    "$( [[ "$existed" == "1" ]] && echo updated || echo created )" "$note"
}

apply_http() {
  ensure_requirements
  ensure_packages squid apache2-utils curl

  mkdir -p "$BEAM_DIR"

  if [[ -f "$SQUID_CONF" ]] && ! grep -q "managed by beammeup" "$SQUID_CONF"; then
    if ! grep -q "beammeup-proxy" "$SQUID_CONF"; then
      die "Existing non-beammeup Squid config detected at $SQUID_CONF. Refusing to overwrite."
    fi
  fi

  load_http_state
  load_socks_state

  local existed="$HTTP_EXISTS"
  local desired_port="${PROXY_PORT:-${HTTP_PORT:-18181}}"
  local final_user="$HTTP_USER"
  local final_pass="$HTTP_PASS"
  local note=""

  is_valid_port "$desired_port" || die "Invalid proxy port: $desired_port"
  ensure_port_available "$desired_port" "$HTTP_PORT"

  if [[ -z "$final_user" || "$ROTATE_CREDENTIALS" -eq 1 ]]; then
    final_user="beamhttp$(generate_secret 'a-z0-9' 4)"
  fi

  if [[ -z "$final_pass" || "$ROTATE_CREDENTIALS" -eq 1 ]]; then
    final_pass="$(generate_secret 'A-Za-z0-9' 20)"
    if [[ "$HTTP_LEGACY" == "1" && "$ROTATE_CREDENTIALS" -eq 0 ]]; then
      note="Legacy HTTP setup detected. Password regenerated because existing password cannot be recovered."
    elif [[ "$ROTATE_CREDENTIALS" -eq 1 ]]; then
      note="Credentials rotated."
    fi
  fi

  local auth_helper
  auth_helper="$(find_squid_auth_helper || true)"
  [[ -n "$auth_helper" ]] || die "Could not locate Squid basic_ncsa_auth helper."

  cat >"$HTTP_ENV" <<EOF_ENV
PROXY_PORT=$desired_port
PROXY_USER=$final_user
PROXY_PASS=$final_pass
EOF_ENV
  chmod 600 "$HTTP_ENV"

  htpasswd -bc "$HTTP_HTPASSWD" "$final_user" "$final_pass" >/dev/null
  chown proxy:proxy "$HTTP_HTPASSWD" 2>/dev/null || true
  chmod 640 "$HTTP_HTPASSWD"

  if [[ -f "$SQUID_CONF" && ! -f "$SQUID_BACKUP" ]]; then
    cp "$SQUID_CONF" "$SQUID_BACKUP"
  fi

  cat >"$SQUID_CONF" <<EOF_SQUID
# managed by beammeup
http_port $desired_port

acl SSL_ports port 443
acl Safe_ports port 80
acl Safe_ports port 443
acl Safe_ports port 1025-65535
acl CONNECT method CONNECT

http_access deny !Safe_ports
http_access deny CONNECT !SSL_ports

auth_param basic program $auth_helper $HTTP_HTPASSWD
auth_param basic realm beammeup-proxy
auth_param basic credentialsttl 8 hours
acl authenticated proxy_auth REQUIRED

http_access allow authenticated
http_access deny all

forwarded_for delete
request_header_access X-Forwarded-For deny all
request_header_access Via deny all

cache deny all
access_log stdio:/var/log/squid/access.log
cache_log /var/log/squid/cache.log
coredump_dir /var/spool/squid
pid_filename /run/squid.pid
EOF_SQUID

  squid -k parse
  systemctl daemon-reload
  systemctl enable --now squid
  systemctl restart squid

  if ! systemctl is-active --quiet squid; then
    journalctl -u squid -n 50 --no-pager >&2 || true
    die "HTTP proxy (Squid) failed to start."
  fi

  apply_firewall_rule "$desired_port"

  load_http_state
  load_socks_state
  reconcile_hangar_status
  write_hangar_metadata "$HANGAR_STATUS" "updated http"

  emit_result "HTTP" "$desired_port" "$final_user" "$final_pass" \
    "$( [[ "$existed" == "1" ]] && echo updated || echo created )" "$note"
}

show_setup() {
  ensure_requirements
  load_socks_state
  load_http_state

  if [[ "$PROTOCOL" == "socks5" ]]; then
    [[ "$SOCKS_EXISTS" == "1" ]] || die "SOCKS5 setup not found."
    FIREWALL_NOTE=""
    emit_result "SOCKS5" "${SOCKS_PORT:-}" "${SOCKS_USER:-}" "${SOCKS_PASS:-}" "show" ""
    return
  fi

  [[ "$HTTP_EXISTS" == "1" ]] || die "HTTP setup not found."
  FIREWALL_NOTE=""
  local note=""
  if [[ -z "$HTTP_PASS" ]]; then
    note="Password is not retrievable from legacy setup. Use rotate action to issue a new password."
  fi
  emit_result "HTTP" "${HTTP_PORT:-}" "${HTTP_USER:-}" "${HTTP_PASS:-}" "show" "$note"
}

destroy_hangar() {
  ensure_requirements
  load_socks_state
  load_http_state

  local removed_any=0
  local note_parts=()

  FIREWALL_NOTE=""

  if [[ "$SOCKS_EXISTS" == "1" ]]; then
    if service_defined "$SOCKS_SERVICE"; then
      systemctl disable --now "$SOCKS_SERVICE" >/dev/null 2>&1 || true
    fi
    cleanup_firewall_rule "${SOCKS_PORT:-}"
    rm -f "$SOCKS_ENV" "$SOCKS_SERVICE_FILE"
    removed_any=1
    note_parts+=("SOCKS5 removed")
  fi

  if [[ "$HTTP_EXISTS" == "1" ]]; then
    cleanup_firewall_rule "${HTTP_PORT:-}"
    if service_defined "squid.service"; then
      systemctl disable --now squid >/dev/null 2>&1 || true
    fi
    rm -f "$HTTP_ENV" "$HTTP_HTPASSWD"

    if [[ -f "$SQUID_BACKUP" ]]; then
      cp "$SQUID_BACKUP" "$SQUID_CONF"
      note_parts+=("restored squid backup")
      if service_defined "squid.service"; then
        systemctl enable --now squid >/dev/null 2>&1 || true
      fi
    elif [[ -f "$SQUID_CONF" ]] && (grep -q "managed by beammeup" "$SQUID_CONF" || grep -q "beammeup-proxy" "$SQUID_CONF"); then
      rm -f "$SQUID_CONF"
      note_parts+=("removed beammeup squid config")
    fi

    removed_any=1
    note_parts+=("HTTP removed")
  fi

  rm -f "$HANGAR_META"
  systemctl daemon-reload

  if [[ "$removed_any" -eq 1 ]]; then
    emit_result "DESTROY" "$(get_public_ip)" "" "" "destroyed" "${note_parts[*]}"
  else
    emit_result "DESTROY" "$(get_public_ip)" "" "" "destroy-noop" "No beammeup configuration detected."
  fi
}

MODE="inventory"
PROTOCOL=""
PROXY_PORT=""
NO_FIREWALL_CHANGE=0
ROTATE_CREDENTIALS=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode)
      MODE="$2"
      shift 2
      ;;
    --protocol)
      PROTOCOL="$2"
      shift 2
      ;;
    --proxy-port)
      PROXY_PORT="$2"
      shift 2
      ;;
    --no-firewall-change)
      NO_FIREWALL_CHANGE=1
      shift
      ;;
    --rotate-credentials)
      ROTATE_CREDENTIALS=1
      shift
      ;;
    *)
      die "Unknown argument: $1"
      ;;
  esac
done

case "$MODE" in
  inventory)
    print_inventory
    ;;
  preflight)
    [[ "$PROTOCOL" == "http" || "$PROTOCOL" == "socks5" ]] || die "--protocol is required for preflight mode."
    run_preflight
    ;;
  show)
    [[ "$PROTOCOL" == "http" || "$PROTOCOL" == "socks5" ]] || die "--protocol is required for show mode."
    show_setup
    ;;
  destroy)
    destroy_hangar
    ;;
  apply)
    [[ "$PROTOCOL" == "http" || "$PROTOCOL" == "socks5" ]] || die "--protocol is required for apply mode."
    if [[ "$PROTOCOL" == "socks5" ]]; then
      apply_socks
    else
      apply_http
    fi
    ;;
  *)
    die "Unknown mode: $MODE"
    ;;
esac
`
