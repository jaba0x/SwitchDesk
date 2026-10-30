<p align="center">
  <img src="docs/banner.png" alt="SwitchDesk" width="820">
</p>

<p align="center">
  A desktop app for Aruba switches. It reads each switch over SSH and shows what's down, what's misconfigured and how the switches connect to each other.
</p>

<p align="center">
  <img alt="macOS" src="https://img.shields.io/badge/macOS-universal-1b212b?style=flat-square&logo=apple&logoColor=white">
  <img alt="Windows" src="https://img.shields.io/badge/Windows-10%20%7C%2011-1b212b?style=flat-square&logo=windows&logoColor=white">
  <img alt="Go" src="https://img.shields.io/badge/Go-1.23+-1b212b?style=flat-square&logo=go&logoColor=white">
  <img alt="Wails" src="https://img.shields.io/badge/Wails-v2-1b212b?style=flat-square">
  <img alt="Version" src="https://img.shields.io/badge/version-0.9.9-4cc38a?style=flat-square">
</p>

<p align="center">
  <a href="https://jaba0x.github.io/SwitchDesk/"><b>Website</b></a> ·
  <a href="https://jaba0x.github.io/SwitchDesk/docs/"><b>Documentation</b></a> ·
  <a href="https://github.com/jaba0x/SwitchDesk/releases/latest"><b>Download</b></a> ·
  <a href="https://github.com/jaba0x/SwitchDesk/discussions"><b>Discussions</b></a>
</p>

<p align="center">
  <img src="docs/dashboard.png" alt="SwitchDesk dashboard" width="900">
</p>

## Why

Every morning I want to know the same things about the switch network: is everything up, did anything change overnight, is spanning tree still sane, and which port goes where. Clicking through web UIs one switch at a time is slow, and a full NMS is a lot to run for a few dozen switches. SwitchDesk reads the switches over plain SSH and keeps everything on your machine. It started with a few dozen switches and is used on networks of several hundred.

## Features

### Dashboard
Online and offline counts, a problem list sorted by severity, and every switch drawn as a faceplate with one LED per port. Green is up, grey is down, a slashed outline is disabled, amber means spanning tree is blocking it.

### Problems it catches

| Area | Checks |
|---|---|
| Reachability | Switch offline, SSH login failing, SSH host key changed |
| Spanning tree | STP disabled, topology changes since last check, root picked by default priority, more than one root in the network |
| Security | Telnet on, plain HTTP management on, default SNMP community, no manager password |
| Management | No NTP, no syslog, local accounts only, unsaved config |
| Hardware | Fan, power supply and temperature faults, high CPU or memory |
| Optics | SFP/QSFP receive power out of range, against the optic's own levels (AOS-CX) or its recent alarms (AOS-S) |
| Logs | Major and error events in the recent log |

### Every switch, in detail

<p align="center">
  <img src="docs/switch.png" alt="Switch page" width="900">
</p>

System facts, a management-access checklist, spanning tree (bridge, root, port roles and states), ports with their configured names and optical levels for SFPs, VLANs, LLDP and CDP neighbors, logs, the running config, and the raw output of every command it ran. Config copies are saved whenever the config changes, and any two can be compared side by side. A console runs `show`, `ping` and `traceroute`, and nothing else.

### VLANs without the CLI
Create, rename and delete VLANs, and set port membership by clicking ports through untagged, tagged and off. Before anything is sent you see the exact commands. Sending stops at the first line the switch rejects, and `write memory` only runs when every line before it went through.

A network-wide matrix shows which switch carries which VLAN and flags names that don't match between switches. From there you can rename, add or remove a VLAN on many switches at once, and review every switch's commands before anything is sent.

### Port access (802.1X and MAC authentication)
On AOS-S and AOS-CX, add or remove RADIUS servers and turn 802.1X and MAC authentication on or off per port, with the client limit and re-authentication period. On AOS-S you pick the VLAN for clients that don't log in. On AOS-CX you create local roles with their VLAN and choose per port which role a client gets after a login, after a rejected login, while RADIUS can't be reached and before a login. AOS-CX also lists the connected clients from `show port-access clients detail`: port, MAC address, user, IP address, login method, status, role and the VLAN the client was put in, read at each collection or with Refresh. Ports that link to another switch or an access point are marked, and you're warned before turning on a login there. Shared secrets never appear on screen, in the command preview or in the transcript.

### Network map

<p align="center">
  <img src="docs/map.png" alt="Network map" width="900">
</p>

Built from LLDP and CDP and laid out top-down from the spanning-tree root. Drag switches wherever you like and the layout is kept. Blocked links are dashed amber, down links red. Firewalls, access points and switches you don't manage get their own shapes. Phones and hosts stay hidden until you turn them on. Hover a switch to see port numbers on its links, double-click to open it.

### Status checks
Between full reads, SwitchDesk checks that each switch is still up, and Settings decides how. By default it opens a TCP connection to port 22 and closes it straight away. That works over most VPN and zero trust setups, but the switch logs every check as an SSH connection, which fills the log on a busy network. Switch the check to ping and no SSH connections are made, as long as ICMP reaches the switches. Turn it off and a switch's status only changes when it's read. With the read interval at 0 as well, SwitchDesk connects only when you click **Collect now**.

### Switch list
Search by name, address, site, model or software, or by state: `offline`, `disabled`, `problem`. Words combine, so `acc offline` finds offline access switches. Press `/` to jump to the search box. Tick switches to collect, disable, enable or delete them together, or use the small buttons at the end of each row. A disabled switch keeps its last data but isn't checked, read or alerted on until you enable it again.

### Firmware updates
Each switch shows the version it runs, what's in its primary and secondary bank, and whether your firmware library has something newer for its platform. **Update** copies the image into the spare bank (the running version stays in the other one as a fallback), sets it as the boot default, saves the config and reboots, then checks that the switch came back on the new version. Pick several switches and they're updated one at a time. Or choose **copy only** and reboot whenever suits you.

On AOS-S, SwitchDesk uploads the image to the switch over SSH (SFTP, or SCP on older firmware). The switch never has to connect back to your computer, so this works over a VPN or zero trust access. For the upload it turns on `ip ssh filetransfer`, which also switches off the switch's TFTP client and server, and afterwards it puts those settings back the way they were.

On AOS-CX, which has no SFTP or SCP server, the image goes over HTTPS to the switch's REST API, again from your computer to the switch. If the web server is off in the management VRF, SwitchDesk turns it on for the upload and off again afterwards.

Only if the upload is refused does the switch pull the image from SwitchDesk over TFTP (UDP 69). For that the switch has to be able to reach your computer.

HPE only offers firmware downloads after you sign in, so you fill the library yourself. Add `.swi` files you downloaded, or paste a download link from the HPE Networking Support Portal and SwitchDesk fetches it. Images are matched to switches by platform code (WC, YA, KB, FL, PL…), read from the file name or the image itself.

**Automatic mode** (Settings) can copy new firmware to every switch's spare bank as soon as it lands in the library, or also install it by rebooting one switch at a time inside a maintenance window you set.

## Supported switches

| Family | Models | Support |
|---|---|---|
| AOS-CX | 6000, 6100, 6200, 6300, 6400, 8xxx | Full |
| AOS-S / ProCurve | 2530 (YA/YB), 2540, 2930F/M, 3810, 5400R | Full |
| Instant On | 1830, 1930, 1960 | Read-only. Local management: read through the switch's web interface with its admin login (ports, PoE where the model has it, spanning tree, LLDP, VLANs, hardware, logs). Cloud management: read from the Instant On portal with your Instant On account. Changes stay on the switch's web page or in the Instant On app. |

AOS-CX and AOS-S switches only need SSH, plus LLDP if you want the map. Instant On switches need nothing on the switch. Locally managed ones are read through their web interface; cloud-managed ones through the Instant On portal, with an account that has two-step verification turned off (a separate view-only account works best).

If your model isn't listed or reads wrong, open an [issue](https://github.com/jaba0x/SwitchDesk/issues/new) with the model number, its firmware version and the output of `show version`. When a reading fails, paste the message SwitchDesk shows too. It names the command or page that failed.

## Install

Download the latest build from **[Releases](https://github.com/jaba0x/SwitchDesk/releases/latest)**.

### macOS

Get `SwitchDesk-…-macos-universal.zip` (runs on Apple Silicon and Intel), unzip it and drag **SwitchDesk** into Applications. The app isn't notarized by Apple, so the first time right-click it and choose **Open**, or run:

```sh
xattr -dr com.apple.quarantine /Applications/SwitchDesk.app
```

### Windows

Get `SwitchDesk-…-windows-x64.zip`, unzip it and run `SwitchDesk.exe`. If SmartScreen warns about an unknown publisher, choose **More info → Run anyway**. WebView2 is already part of Windows 10 and 11.

### Build it yourself

With Go 1.23+ installed (on a Mac also the Xcode command line tools):

```sh
git clone https://github.com/jaba0x/SwitchDesk.git
cd switchdesk
./scripts/build-mac.sh          # macOS: builds, installs to /Applications and opens it
.\scripts\build-windows.ps1     # Windows: builds build\bin\SwitchDesk.exe
```

### Staying up to date

SwitchDesk checks GitHub for a newer release when it starts and every few hours after. When one is out, a yellow bar at the top of the window links to the download. **Settings → Check for updates** does the same on demand.

### Releases

The **build** workflow publishes a release with both downloads attached. Push a tag like `v0.2.0`, or run the workflow from the Actions tab and enter a version.

## Getting started

1. **Add a switch** with its address and credentials. **Test connection** shows the hostname and switch family before you save.
2. **Add many** takes one address per line, `name, address` pairs, or ranges such as `10.0.1.20-10.0.1.40`.
3. The first read starts right away. After that, each switch's SSH port is checked every 30 seconds and each switch is re-read every 15 minutes. In Settings you can change both, check with ping instead, or turn the checks off.

## Your data

Everything stays on your computer, in one folder.

| OS | Default folder |
|---|---|
| macOS | `~/Library/Application Support/SwitchDesk` |
| Windows | `%AppData%\SwitchDesk` |

- `inventory.json` holds switches, settings and map positions. Passwords are encrypted with AES-GCM using a key in `secret.key`, readable only by your user.
- `snapshots/` keeps the last reading of each switch, so the app opens with data. It loads in the background, so the window opens straight away even with hundreds of switches.
- `backups/` keeps a copy of a switch's running config each time it changes.

To use another folder, go to Settings → Data → Change. You can move the data to another disk or a synced drive, copy it there, or open a folder that already has SwitchDesk data. The key moves with the folder, so the same folder works from any computer that can reach it.

To move to another computer, use Settings → Data → Export. It writes a single `.sdbak` file. Passwords inside are re-encrypted with a passphrase you pick (Argon2id + AES-GCM), so the file is safe to carry around and opens on any machine. On the other computer, Import either adds the file's switches to the ones already there or replaces everything with the file's contents.

SSH host keys are pinned on first connect. If a switch later presents a different key, SwitchDesk stops connecting to it until you confirm on the switch page.

## Documentation

The full guide is at **[jaba0x.github.io/SwitchDesk/docs](https://jaba0x.github.io/SwitchDesk/docs/)**: install, first steps, every screen, firmware updates, moving your data, and troubleshooting. It's built with MkDocs from the `docs/` folder. The landing page at [jaba0x.github.io/SwitchDesk](https://jaba0x.github.io/SwitchDesk/) is plain HTML in `web/`. Both are published on every push.

## Development

```sh
go test ./internal/...
wails dev
```

The UI is plain HTML, CSS and JavaScript in `frontend/dist` with no build step. To work on it in a normal browser with sample data, open `frontend/dev/preview.html`.

```
internal/sshx    SSH shell sessions (prompts, paging, escape codes, legacy ciphers) and SFTP/SCP uploads
internal/aruba   command sets per family, output parsers, VLAN and port-access change builders
internal/core    polling, problem rules, network map
internal/store     inventory, encrypted credentials, snapshots, config copies
internal/firmware  image library, version matching, built-in TFTP server
internal/cxrest    firmware upload to AOS-CX over its REST API
internal/instanton Instant On portal sign-in and reads
internal/ionlocal  Instant On web interface sign-in and reads (local management)
```

The parsers are tested against sample output from AOS-S and AOS-CX. When a switch prints something they misread, the **Raw output** tab shows exactly what came back. That output makes a good test case.

## Author

<a href="https://clihero.com">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/clihero-logo-white.png">
    <img src="docs/clihero-logo.png" alt="clihero.com" width="220">
  </picture>
</a>

Built by **Jaba Macharashvili**. More of my infra, homelab and hardware projects are on [clihero.com](https://clihero.com).

## Credits

Thanks to [Arne Naessens](https://github.com/ArneNaessens) for mass VLAN editing, the disabled-port display and the `ntp server-name` fix, and to everyone who reports issues.

Built with [Wails](https://wails.io). Fonts are [IBM Plex](https://github.com/IBM/plex), under the SIL Open Font License.
