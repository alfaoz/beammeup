# beammeup

ship your browser traffic through your own vps.

live at **[beammeup.pw](https://beammeup.pw)**

## what it offers

a local install-only cli that:
- sets up authenticated `http` or `socks5` proxy on your vps
- supports inventory, show, configure/repair, rotate credentials, and preflight checks
- stores reusable ship profiles in `~/.beammeup/ships/*.ship`
- never stores ssh passwords in ship files
- runs a full `gum`-based tui wizard for interactive use

### what are ships?

ships are saved connection profiles for your target vps.
each `.ship` file keeps things like host, ssh user/port, protocol, and proxy port so you can relaunch quickly without retyping everything.
ssh passwords are prompted at runtime and are not written into ship files.

## install

```bash
curl -fsSL https://beammeup.pw/install.sh | bash
```

then:

```bash
beammeup
```

installer behavior:
- installs `gum` automatically when possible (brew/apt/dnf/yum/pacman/zypper/apk)
- falls back to direct `gum` binary install when package manager install is unavailable

## how it works

beammeup runs on your machine and sshes into your vps. it configures proxy services remotely. 

there is no hosted control plane. no `scotty`. install-only.

wizard terminology:
- `destroy hangar`: remove beammeup configuration from the server
- `abandon ship`: delete local `.ship` profile only

## supported targets

currently tested for debian/ubuntu vps with root ssh access.

requirements on target:
- `apt-get`
- `systemd`
- internet access for package install

## quick usage

interactive wizard:

```bash
beammeup
```

list saved ships:

```bash
beammeup --list-ships
```

use a saved ship:

```bash
beammeup --ship my-vps
```

non-interactive example:

```bash
beammeup \
  --host 203.0.113.10 \
  --ssh-user root \
  --protocol http \
  --action configure \
  --proxy-port 18181
```

## updates

manual:

```bash
beammeup --self-update
```

auto-update before run:

```bash
beammeup --auto-update
```

## contributing

issues and prs are welcome.

if you report a security issue, include repro steps, impacted flow, and expected behavior.

## license

mit
