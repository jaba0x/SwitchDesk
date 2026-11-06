---
hide:
  - navigation
---

# SwitchDesk

SwitchDesk is a desktop app for Aruba switches. It logs in to each switch over SSH, reads its state, and puts the whole network on one screen: which switches are up, what's misconfigured, how spanning tree looks, which VLANs live where, how everything is cabled together, and which switches have newer firmware waiting.

It runs on macOS and Windows and keeps all data on your own computer.

![SwitchDesk dashboard](images/dashboard.png){ .shot }

[Download the latest release](https://github.com/jaba0x/SwitchDesk/releases/latest){ .md-button .md-button--primary }
[Install guide](install.md){ .md-button }

## What it does

**One screen for the morning check.** Online and offline counts, every problem across the network sorted by severity, and each switch drawn as a faceplate with one LED per port. Read more on the [dashboard](dashboard.md) page.

**Every switch in detail.** System facts, management access, spanning tree, ports, VLANs, LLDP and CDP neighbors, logs, the running config with saved copies you can compare, and a read-only console. See [Switches](switches.md).

**Where is this device?** Type a MAC address under Clients and get the switch and port it's plugged into, with the uplinks along the path told apart from the edge port. See [Switches](switches.md#clients).

**VLANs without the CLI.** Click ports to make them untagged, tagged or off, see the exact commands before anything is sent, and compare VLANs across all switches in one matrix. See [VLANs](vlans.md).

**A network map that draws itself.** Built from LLDP and CDP, laid out top-down from the spanning-tree root, with blocked and down links marked. See [Network map](network-map.md).

**Firmware updates.** See which switches have something newer in your firmware library, update one or many with a click, or let SwitchDesk do it on its own in a maintenance window. See [Firmware updates](firmware.md).

**Yours to move.** Choose where the data lives, or move everything to another computer with one passphrase-protected file. See [Your data](data.md).

## Supported switches

| Family | Models | Support |
|---|---|---|
| AOS-CX | 6000, 6100, 6200, 6300, 6400, 8xxx | Full |
| AOS-S / ProCurve | 2530 (YA/YB), 2540, 2930F/M, 3810, 5400R | Full |
| Instant On | 1830, 1930, 1960 | Read-only, through the switch's web interface (local management) or the Instant On portal (cloud management). See [Instant On switches](instant-on.md). |

All the switches need is SSH, plus LLDP if you want them on the map.
