# beammeup

persistent terminal cockpit for setting up your own http/socks5 exit node on a vps.

live: [beammeup.pw](https://beammeup.pw)

## install

```bash
curl -fsSL https://beammeup.pw/install.sh | bash
```

then run:

```bash
beammeup
```

## model

- `ship` = local profile in `~/.beammeup/ships/*.ship`
- `hangar` = remote beammeup configuration on that server

wizard terms:

- `destroy hangar` = remove remote beammeup-managed proxy setup
- `abandon ship` = delete local `.ship` profile only

ship files store host/protocol defaults and never store ssh passwords.

## features (v2)

- go runtime (no gum/dialog dependency)
- persistent cockpit loop with back navigation
- onboarding flow when no ships exist
- launch flow that detects `online | missing | drift`
- HTTP conflict wizard when existing Squid config is detected
- isolated HTTP sidecar mode (`--http-mode sidecar`) that never overwrites `/etc/squid/squid.conf`
- one hangar can manage both http and socks5 configs
- in-memory ssh password cache per session only
- non-interactive flags kept for automation parity

## interactive flow

```bash
beammeup
```

- create/select ships
- launch ship
- inspect hangar
- configure/repair
- rotate credentials
- destroy hangar (optional abandon ship prompt)

## non-interactive examples

list ships:

```bash
beammeup --list-ships
```

configure http:

```bash
beammeup \
  --host 203.0.113.10 \
  --ssh-user root \
  --protocol http \
  --proxy-port 18181 \
  --action configure
```

configure isolated http sidecar (safe with existing squid):

```bash
beammeup \
  --host 203.0.113.10 \
  --ssh-user root \
  --protocol http \
  --http-mode sidecar \
  --proxy-port 18181 \
  --action configure
```

show current socks5 setup from saved ship:

```bash
beammeup --ship rpsvps --protocol socks5 --action show
```

destroy hangar non-interactively:

```bash
beammeup --ship rpsvps --action destroy --yes
```

## updater

```bash
beammeup --self-update
```

or auto-update before run:

```bash
beammeup --auto-update
```

## release builds

build release archives for supported targets:

```bash
scripts/build-release.sh
```

artifacts:

- `dist/beammeup_darwin_arm64.tar.gz`
- `dist/beammeup_darwin_amd64.tar.gz`
- `dist/beammeup_linux_amd64.tar.gz`
- `dist/beammeup_linux_arm64.tar.gz`
- `dist/version.txt`

## supported target vps

currently focused on debian/ubuntu with:

- root ssh access
- `apt-get`
- `systemd`

## license

mit
