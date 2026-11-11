# Switches

## The switch list

The **«** next to the SwitchDesk name folds the sidebar down to its icons, which leaves more room for wide tables and the network map. **»** opens it again.

![Switch list](images/switches.png){ .shot }

Every switch with its address, site, tags, model, software, firmware status, CPU and memory, ports up, spanning-tree role, problem count and when it was last read.

**Sort** by clicking any column header; click again to reverse. Addresses sort by number, so `10.0.0.20` comes before `10.0.0.132`. **Columns** above the list chooses which columns are shown; Serial, Boot ROM and Uptime are off until you turn them on. **Drag a header** onto another to move that column, and **drag the edge** of a header to make it wider or narrower. **Reset columns** in the Columns menu puts everything back. The header row stays in view while you scroll, and all of this is remembered on your computer.

Uptime is shown the same short way for every switch, such as `64d 19h`; hover it for what the switch itself said.

**Tags** are your own labels for switches, such as `core`, `access` or `building A`. Add them in a switch's **Edit** dialog, separated by commas. Click a tag in the list to show only the switches that have it, then select them all for a bulk action.

**Search** matches name, address, site, tags, model, software, boot ROM and serial, and also states: `online`, `offline`, `disabled`, `problem`. Words combine, so `acc offline` finds offline switches with "acc" in their name. Press ++slash++ to jump to the box.

**Select switches** with the checkboxes (the header one selects everything shown). A bar appears with actions for all of them:

- **Collect now** reads them immediately.
- **Update firmware** opens the [update dialog](firmware.md#update-switches).
- **Disable** pauses them: no checks, no SSH, no alerts. They keep their last data and stay greyed out until you **Enable** them again. Handy for a switch that's being replaced or is on the bench.
- **Delete** removes them from SwitchDesk with their saved configs. Nothing on the switch itself changes.

The small buttons at the end of each row open the switch's web interface or an SSH session, and do the same for a single switch: update (when there's something newer), disable or enable, and delete.

## The switch page

Click a switch to open it. The header shows its address, family, model, software, uptime and when it was read. **Collect now**, **Disable** and **Edit** are on the right.

### Overview

![Switch overview](images/switch-overview.png){ .shot }

The port faceplate, this switch's problems, system facts with CPU and memory meters, the firmware banks, a checklist of management access (SSH, Telnet, HTTP/HTTPS, SNMP, manager password, time sync, syslog, RADIUS/TACACS+), spanning-tree summary, the stack members with their status (on a stacked or VSF switch) and hardware status. With **Show front panel photos** on in Settings, a photo of the model sits under the faceplate, for the models the NetBox device type library has a photo of.

**S, M, L and XL** under the port panel make it larger, which helps on 48-port switches and stacks. The size is remembered.

To use a front panel photo of your own, for a model the NetBox library has no picture of, put it in the `hardware` folder inside the data folder, named after the part number: `JL256A.png` or `JL256A.jpg`. It's used instead of the library's photo when **Show front panel photos** is on.

### Spanning tree

![Spanning tree](images/switch-stp.png){ .shot }

Bridge and root MAC and priority, this switch's role, topology changes, and every port's role, state and cost.

### Ports

![Ports](images/switch-ports.png){ .shot }

A large faceplate laid out like the hardware: copper ports, SFP ports and module ports (`A1`…) are separate blocks, and each member of a stacked switch (2930M, 3810, VSF on 2930F and 6300) gets its own row. Which ports are SFP cages comes from the model's hardware list (from the NetBox device type library), so an empty SFP cage is drawn as one. Then its colour key, and a table with each port's name, status (Up, Down or Disabled), speed, type and VLAN membership (`20u` is untagged in VLAN 20, `30t` tagged in VLAN 30). The name is the one set on the switch: `name "..."` on AOS-S, `description` on AOS-CX. Hover a port on any faceplate to see it too, for example "Port 3 · Printer-2nd-floor".

Switches with transceivers also get an **Optics** table: Rx and Tx power in dBm (one value per lane on QSFP), temperature, model and serial. On AOS-CX it shows the optic's own warning range and judges against it. AOS-S doesn't report those levels, so its recent optical alarms are shown instead. A port that's down with no light shows as *Not connected*, and DAC cables show *No DOM*. The readings come from `show interface dom detail` on AOS-CX and `show interfaces transceiver detail` on AOS-S.

### VLANs

The switch's VLANs, with editing. See [VLANs](vlans.md).

### Port access

On AOS-S and AOS-CX switches, the **Port access** tab manages who may connect to a port:

- **RADIUS servers:** add and remove them. The shared secret is never shown on screen, in the command preview or in the transcript. It can't contain spaces, double quotes or question marks, because the switch CLI treats a typed `?` as a request for help.
- **802.1X and MAC authentication:** turn them on or off per port, or for the whole switch, with the client limit and re-authentication period. Shift-click selects a range of ports.
- **Clients that don't log in:** on AOS-S you pick their VLAN. On AOS-CX you create local roles (a single VLAN, or a trunk with tagged VLANs) and choose per port which role applies after a login, after a rejected login, while RADIUS can't be reached, and before a login.
- **Connected clients (AOS-CX):** port, MAC address, user, IP address, login method, status, role and VLAN, read at each collection or with **Refresh**.

Ports that link to another switch or an access point are marked, and you're warned before turning on a login there, since that can cut the link and SwitchDesk's own connection with it. As with VLANs, you see the exact commands before anything is sent. Port access was contributed by [Arne Naessens](https://github.com/ArneNaessens).

### Neighbors

![Neighbors](images/switch-neighbors.png){ .shot }

Everything LLDP and CDP see: local port, neighbor name, its port and address, capabilities and description. Neighbors that are also in SwitchDesk link to their own page.

### Config

![Config compare](images/config-compare.png){ .shot }

The running config, with search. On the left are saved copies: SwitchDesk keeps one every time the config changes (20 per switch by default). Pick **Compare with…** to see what changed between two versions, and **Save as file** to export one.

### Logs

The recent log, with major and error events highlighted.

### Console

![Console](images/console.png){ .shot }

Run `show`, `ping` and `traceroute` commands on the switch. Anything else is refused, so the console can't change configuration. ++arrow-up++ brings back earlier commands.

### Raw output

The exact output of every command SwitchDesk ran in the last reading. If something on the other tabs looks wrong, this shows what the switch actually said.

## Clients

**Clients** in the sidebar lists every device the switches have learned, once each, at the port it's plugged into: its MAC address, the switch, the port with its description, the VLAN, and how many switches learned the address on the way. Edge ports are highlighted; an address only seen on uplinks (a device behind a switch SwitchDesk doesn't manage) shows the uplink it was found on.

Type in the box to filter by MAC address (any part of it), switch name, port, port description or VLAN. Click an address, or type one and press **Find path**, to see every switch that learned it, with the edge port first and each uplink named after the switch it leads to.

Address tables are read with every collection, so a device that has been quiet for longer than the switch's MAC age time may be missing until it sends something.

## Opening the switch's web interface

The **Web** button at the top of a switch's page, and the globe icon on each row of the switch list, open the switch's own web interface in your browser: `https://` and its address, or `http://` for an Instant On switch read over plain HTTP. Instant On switches managed from the cloud have no web button; use the Instant On portal for those.

## Opening a switch in your SSH client

The **SSH** button at the top of a switch's page, and the terminal icon on each row of the switch list, open the switch in a terminal of your own, for longer sessions with tabs, logging and your own colours.

Pick the program under **Settings → SSH client**. The list shows the clients SwitchDesk knows for your kind of computer, finds them in their usual install folders or on the PATH, and marks the ones that aren't installed. Under the list you see the exact command it will run.

| Computer | Clients in the list |
|---|---|
| Windows | Windows Terminal, PuTTY, KiTTY, MobaXterm, SecureCRT, OpenSSH in a console window |
| macOS | Terminal, iTerm2, Ghostty, WezTerm, SecureCRT |
| Linux | GNOME Terminal, Konsole, WezTerm, xterm |

**Automatic**, the default, uses what the computer already has: on macOS the app registered for `ssh://` links (usually Terminal), on Windows Windows Terminal if it's there and otherwise OpenSSH in a console window, on Linux the default terminal.

For anything else, choose **Custom command** and write the command line. These are filled in for each switch:

| Placeholder | Becomes |
|---|---|
| `{host}` | The switch's address |
| `{port}` | Its SSH port |
| `{user}` | The user name of its login |
| `{dest}` | `user@host` (just the host when there's no user) |
| `{url}` | `ssh://user@host:port` |
| `{name}` | The name you gave the switch |

Some examples:

| Client | Command |
|---|---|
| Windows Terminal with your own profile | `wt.exe -p "Network" ssh -p {port} {dest}` |
| PuTTY with a saved session for its settings | `putty.exe -load "Switches" -ssh {dest} -P {port}` |
| Termius or any app that opens ssh:// links (macOS) | `open {url}` |
| A script of your own | `C:\Tools\open-ssh.cmd {host} {port} {user}` |

Put a path with spaces in double quotes. The command is run directly, not through a shell, and SwitchDesk never passes the switch's password on: the client asks for it, or uses its own keys and saved sessions. Instant On switches read through the cloud or their web interface have no SSH button.

