#!/usr/bin/env bash
set -euo pipefail

# Guardrail: prevent committing real/public IPv4 addresses.
# We allow:
# - loopback: 127.0.0.0/8
# - private: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
# - documentation ranges: 192.0.2.0/24, 198.51.100.0/24, 203.0.113.0/24
# - 0.0.0.0 (bind-all)

ipv4_re='([0-9]{1,3}\\.){3}[0-9]{1,3}'

fail=0
while IFS= read -r file; do
  # Skip binaries (best-effort): if file contains NUL, ignore it.
  if LC_ALL=C grep -q $'\\x00' "$file" 2>/dev/null; then
    continue
  fi

  # Extract candidate IPv4s, then filter out allowed ranges.
  while IFS= read -r ip; do
    case "$ip" in
      127.*|10.*|192.168.*|0.0.0.0) continue ;;
      172.1[6-9].*|172.2[0-9].*|172.3[0-1].*) continue ;;
      192.0.2.*|198.51.100.*|203.0.113.*) continue ;;
    esac

    echo "[check-no-public-ips] ERROR: public IP found: $ip in $file" >&2
    fail=1
  done < <(LC_ALL=C rg -o -N "\\b$ipv4_re\\b" "$file" 2>/dev/null | sort -u)
done < <(git ls-files)

exit "$fail"

