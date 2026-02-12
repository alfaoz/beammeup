# beammeup

beammeup is a local CLI cockpit that SSHes into your VPS and sets up a browser-usable proxy exit.

live: [beammeup.pw](https://beammeup.pw)

## what beammeup is

beammeup is install-only. there is no hosted control plane.

you run beammeup on your own machine, and it configures services on your VPS over SSH.

## core concepts

### ships
A **ship** is a local saved profile in:

- `~/.beammeup/ships/*.ship`

A ship stores:

- target host/IP
- SSH user/port
- default protocol (`http` or `socks5`)
- HTTP mode (`auto` or `sidecar`)
- proxy port and firewall preference

A ship never stores SSH passwords.

### hangars
A **hangar** is the remote beammeup-managed setup on that ship's server.

A hangar can include:

- SOCKS5 proxy (microsocks)
- HTTP proxy (managed squid or isolated sidecar)
- remote metadata at `/etc/beammeup/hangar.json`

wizard terms:

- `destroy hangar` = remove beammeup-managed remote config
- `abandon ship` = remove local `.ship` file only

## install

```bash
curl -fsSL https://beammeup.pw/install.sh | bash
```

then run:

```bash
beammeup
```

## interactive cockpit (default)

```bash
beammeup
```

beammeup opens a persistent menu loop:

- if you have no ships, onboarding creates one
- select ship -> ship cockpit
- launch/hangar/edit/abandon actions
- all screens support back navigation

password behavior:

- prompted once per ship per app session
- cached in memory only
- never written to disk

## protocols and HTTP modes

### SOCKS5
- lightweight and reliable
- best when your client supports SOCKS5 auth properly

### HTTP
for browser-extension compatibility.

HTTP modes:

- `auto` (default): beammeup may manage `/etc/squid/squid.conf` when safe
- `sidecar`: isolated HTTP service that does **not** overwrite existing system squid config

## HTTP conflict wizard

if beammeup detects an existing non-beammeup squid config, it does not overwrite it.

in TUI, you get a conflict wizard with options to:

- switch to SOCKS5 fallback
- create isolated HTTP sidecar
- cancel

## non-interactive CLI

beammeup keeps scriptable flags for automation.

### list ships

```bash
beammeup --list-ships
```

### configure SOCKS5

```bash
beammeup \
  --host 203.0.113.10 \
  --ssh-user root \
  --protocol socks5 \
  --proxy-port 18080 \
  --action configure
```

### configure HTTP sidecar (safe with existing squid)

```bash
beammeup \
  --host 203.0.113.10 \
  --ssh-user root \
  --protocol http \
  --http-mode sidecar \
  --proxy-port 18181 \
  --action configure
```

### show inventory

```bash
beammeup --ship myship --show-inventory
```

### destroy hangar

```bash
beammeup --ship myship --action destroy --yes
```

## updater

```bash
beammeup --self-update
```

or auto-update before run:

```bash
beammeup --auto-update
```

## supported target VPS

currently focused on Debian/Ubuntu with:

- root SSH access
- `apt-get`
- `systemd`

## release builds

build release archives:

```bash
scripts/build-release.sh
```

artifacts:

- `dist/beammeup_darwin_arm64.tar.gz`
- `dist/beammeup_darwin_amd64.tar.gz`
- `dist/beammeup_linux_amd64.tar.gz`
- `dist/beammeup_linux_arm64.tar.gz`
- `dist/version.txt`

## license

mit
