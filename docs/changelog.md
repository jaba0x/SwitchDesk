# Changelog

## 1.0.1

- Fixed: on AOS-S, ports that belong to a trunk (listed as `13-Trk3` or `1/A1-Trk1`) were left out of the port list and the faceplate. They're back, and the port's tooltip names its trunk.
- Fixed: fiber ports reported as `1000SX`, `1000LX` or `100FX` were drawn as copper ports.
- Dual-personality (combo) ports, such as 21-24 on a 2930M 24G, stay together in one block whatever is plugged in, and the gaps between the copper, SFP and module blocks are wider.

Thanks to [@glepping](https://github.com/glepping) for the output from a 2930M stack ([#14](https://github.com/jaba0x/SwitchDesk/issues/14)).

## 1.0.0

- **Languages.** Pick the interface language under **Settings → Language**, or let SwitchDesk follow your computer. English and a German draft come with this release, and translations are plain JSON files: put one in the data folder's `translations` folder and it shows up in the list, no building needed. Every language is welcome, see [Translating](translating.md). Asked for by [@RashbaNor](https://github.com/RashbaNor) in [discussion #5](https://github.com/jaba0x/SwitchDesk/discussions/5).
- **Optional app password.** Off unless you turn it on in **Settings → App password**. SwitchDesk then asks for it at start and reads nothing until it's entered. The key for the saved switch passwords is stored encrypted with it (Argon2id + AES-GCM), so they can't be read from the data folder without the password. The window can lock after an idle time, or with **Lock now**. A forgotten password can be dropped from the lock screen, which clears the saved switch passwords but keeps everything else. Suggested by [@BarracudaGDU](https://github.com/BarracudaGDU) in [#13](https://github.com/jaba0x/SwitchDesk/issues/13).
- **Faceplates look like the hardware.** Each member of a stacked switch gets its own row, and SFP and module ports (`A1`…) are separate blocks with a gap. Suggested by [@glepping](https://github.com/glepping) in [#14](https://github.com/jaba0x/SwitchDesk/issues/14).
- Fixed: on stacked AOS-S switches (2930M, 3810, VSF), the module ports such as `1/A1` were missing, and port ranges like `1/1-1/24` weren't understood, so VLAN membership on stacks was read wrong ([#14](https://github.com/jaba0x/SwitchDesk/issues/14)).
- Fixed: the firmware table's last column covered another one in a narrow window; the table now scrolls.

## 0.9.10

- **Port access.** A new tab on AOS-S and AOS-CX switches manages RADIUS servers, 802.1X and MAC authentication per port and switch-wide, the VLAN for clients that don't log in (AOS-S), and local roles (AOS-CX). AOS-CX switches also list their connected clients. Two new checks: port authentication with no RADIUS server, and 802.1X set on ports but off for the switch. Contributed by [Arne Naessens](https://github.com/ArneNaessens) in [#15](https://github.com/jaba0x/SwitchDesk/pull/15).
- Fixed: the VLANs page crashed when a switch had no VLAN list, for example after a failed read. Fixed by [Arne Naessens](https://github.com/ArneNaessens) in [#16](https://github.com/jaba0x/SwitchDesk/pull/16).
- Hardening: names read from a switch's config (RADIUS hosts, roles, ports, MAC addresses) no longer go into inline JavaScript on the Port access tab, and a RADIUS secret can't contain `?`, which the switch CLI would take as a request for help.

## 0.9.9

- Fixed: with a few hundred switches, the window could stay empty after a restart once every switch had been read ([#7](https://github.com/jaba0x/SwitchDesk/issues/7)). Measured with 284 fully read switches:
  - The window no longer waits for the saved data. It opens at once and says "Loading saved data… 120 of 284 switches" while the rest comes in.
  - Saved data loads about twice as fast. The raw command output, the bulk of each switch's file, is now read only when you open that switch's **Raw output** tab.
  - The dashboard and the network map are about 15 times faster to build (0.16 s instead of 2–3 s). Matching up links between switches used to compare every link with every other.
  - If the first load is ever slow, the window says "Loading the dashboard…" instead of staying empty.

  Thanks to [@Y4nnickB](https://github.com/Y4nnickB) for the report and the screenshot.

## 0.9.8

- Fixed: switches whose hostname has a character such as `+`, `&`, `/`, a space or an umlaut couldn't be read. SwitchDesk didn't recognise their prompt and stopped with "no CLI prompt within 20s". Reported by [@MarcelW85](https://github.com/MarcelW85) and [@new-it](https://github.com/new-it) in [#10](https://github.com/jaba0x/SwitchDesk/issues/10).

## 0.9.7

- **Port names.** The names set on the switch now show in the ports table and when you hover a port on any faceplate, for example "Port 3 · Printer-2nd-floor". SwitchDesk reads `name "..."` from the AOS-S config and the full `description` from AOS-CX. Names appear after the next read of each switch. Requested by [@MarcelW85](https://github.com/MarcelW85) in [#9](https://github.com/jaba0x/SwitchDesk/issues/9).

## 0.9.6

- **Disabled ports look different from ports that are down.** On the faceplates a disabled port is now an outline with a slash, and a key explains the colours. The ports table has one Status column (Up, Down or Disabled), and port counts read like "12/48 · 3 disabled". Contributed by [Arne Naessens](https://github.com/ArneNaessens) in [#8](https://github.com/jaba0x/SwitchDesk/pull/8).
- Fixed: AOS-S switches that name their time server with `ntp server-name`, such as the J9774A and J9775A, were reported as having no NTP server. Contributed by [Arne Naessens](https://github.com/ArneNaessens) in [#12](https://github.com/jaba0x/SwitchDesk/pull/12), reported by [@new-it](https://github.com/new-it) in [#11](https://github.com/jaba0x/SwitchDesk/issues/11).

## 0.9.5

- Fixed: with a big firmware folder, or files in it that aren't switch images, the window could stay blank at startup. SwitchDesk read every file in the folder before the window could load, and read files it didn't recognise again on every start. Now it starts from the saved list of images and looks at the folder in the background. Files it can't recognise are remembered and skipped until they change. Thanks to [@aw-glaronia](https://github.com/aw-glaronia) for finding that clearing the firmware folder helped, and to [@Y4nnickB](https://github.com/Y4nnickB) for the report ([#7](https://github.com/jaba0x/SwitchDesk/issues/7)).

## 0.9.4

- Fixed: clicking a switch that has no problems at all did nothing on the Switches page (it just jumped to the top). The switch's problem list arrived empty in a form the page didn't expect. The dashboard had the same weakness on a network with no problems. Thanks to [@aw-glaronia](https://github.com/aw-glaronia) for the report and the error details ([#4](https://github.com/jaba0x/SwitchDesk/issues/4)).

## 0.9.3

- **Change a VLAN on many switches at once.** The VLANs page has **Edit** and **Delete** on every row and a **New VLAN** button. Rename a VLAN everywhere, add it to switches, or remove it, then review the exact commands for each switch before anything is sent. Contributed by [Arne Naessens](https://github.com/ArneNaessens) in [#6](https://github.com/jaba0x/SwitchDesk/pull/6). Thank you!
- VLAN changes show up right away instead of after the next read of the switch.
- When a page can't be shown, SwitchDesk now says so, with details you can copy into an issue, instead of showing nothing ([#4](https://github.com/jaba0x/SwitchDesk/issues/4), [#7](https://github.com/jaba0x/SwitchDesk/issues/7)).
- With many switches, starting up could take longer than the window waited for, which left it empty. It now waits up to 30 seconds and shows "Starting SwitchDesk…" meanwhile ([#7](https://github.com/jaba0x/SwitchDesk/issues/7)).

## 0.9.2

This release fixes security issues found in a code review and by GitHub code scanning (CodeQL). Please update.

### Security

| Severity | Issue | Who could exploit it |
|---|---|---|
| High | Switch IDs in an imported `.sdbak` were used as file paths | Someone who gets you to import a crafted export |
| Medium | Imported switch IDs were shown in the window without escaping | Someone who gets you to import a crafted export |
| Medium | Firmware file names could add commands to the switch's `copy` command | Someone who gets a crafted file into your firmware folder (macOS/Linux) |
| Medium | AOS-CX firmware upload accepted any HTTPS certificate | Someone on the network between you and the switch |
| Medium | Instant On (web interface) login accepted any certificate and could be downgraded to plain text | Someone on the network between you and the switch |
| Medium | Test connection ignored the saved SSH host key | Someone on the network between you and the switch |

In detail:

- **Imports.** IDs in an imported `.sdbak` are checked, and replaced when they aren't in the form SwitchDesk uses. Before, a crafted ID such as `../../folder` let the app read, write or delete files outside the data folder, and markup in an ID could run script in the window. Files already on disk are checked too. In merge mode, an imported credential is matched to one of yours only when both the username and the password match, so an imported switch can't borrow your password. Imported settings are kept within the normal ranges.
- **HTTPS certificates are pinned.** AOS-CX firmware uploads and Instant On sign-ins now remember the certificate a switch shows the first time, the same way SSH host keys are remembered. If it changes, SwitchDesk stops before sending the password and shows a warning. **Trust new key** on the switch page clears it.
- **No plaintext downgrade.** Once an Instant On switch has offered encrypted login, a later request for the password in plain text is refused.
- **Test connection** on a saved switch checks the stored SSH host key, so it doesn't send the password to a switch with a different key.
- **Firmware file names.** Files copied into the firmware folder by hand get a safe name if theirs has spaces, quotes, line breaks or other unusual characters. Every value in the switch's `copy tftp` command is checked as well.
- **Smaller fixes.** The Instant On session path and redirects have to stay on the switch itself. The Download update button only opens this project's release pages. File names and links no longer go into inline JavaScript in the window.

### What you'll notice

- After updating, each AOS-CX and Instant On switch learns its HTTPS certificate on its next connection. You don't need to do anything.
- If a switch is later reset or replaced, its page shows "The switch's identity changed" until you click **Trust new key**.
- A firmware image you copied in by hand with an unusual name may now show up under a slightly different name.

To report a security problem, see [SECURITY.md](https://github.com/jaba0x/SwitchDesk/blob/main/SECURITY.md).

## 0.9.1

- Pages use the full width of the window. The switch list no longer scrolls sideways on wide screens.
- Instant On 1930 (web interface): a reading no longer stops with "the web session has ended" when the switch answers a page it doesn't have, such as PoE on the 8-port JL680A. SwitchDesk checks the session is still alive and carries on. PoE isn't read from models without PoE.
- Instant On: the sign-in also works when the switch sets its session cookie without a path.
- When a session really does end, the message now says which page was refused and what the switch said.

## 0.9.0

- New **Status check** setting under Settings → Monitoring: **SSH port** (what earlier versions did), **Ping (ICMP)** or **Off**. The SSH port check opens a connection to every switch every 30 seconds, and switches log each one as an SSH connection. Ping avoids that; Off stops background checks completely, and a switch's status then comes from its reads. Old settings keep the SSH port check.

## 0.8.3

- The window opens maximised.
- The dashboard's problems list has a quick filter: type to match the problem, switch name or details (press `/` to jump there, Esc to clear), and pick a category. It works together with the severity chips and shows how many of the problems match.

## 0.8.2

- Delete and Remove buttons work in the Mac app. They asked for confirmation with a browser dialog that the app's window doesn't show, so the answer was always "cancel". Confirmation now uses SwitchDesk's own dialog, for switches, firmware images, credentials and replacing data on import.

## 0.8.1

- **Instant On (web interface)** is back in the switch type list, in both Add switch and Edit switch. Picking it switches the dialog to the web interface: port 443 and the switch's web login. Editing an Instant On switch that was added for SSH opens it this way too, so saving moves it to the web interface.

## 0.8.0

- Optical levels for SFP, SFP+ and QSFP transceivers on AOS-S and AOS-CX: Rx and Tx power in dBm (per lane on QSFP), temperature, model and serial, in a new **Optics** table on the Ports tab and in the port LED tooltips.
- New problems: **Optical power alarm** and **Optical power warning**. On AOS-CX they use the levels the transceiver itself reports. AOS-S doesn't report levels, so its recent optical alarms are shown as a warning. Unused SFPs in ports that are down are ignored.

## 0.7.0

- Instant On switches in local management mode can be added. They have no SSH, so SwitchDesk signs in to the switch's web interface with its admin login, the same encrypted login the web page uses, and signs out after each reading. In **Add switch**, pick **Instant On (local)**; in **Add many**, pick **Instant On (web interface)**.
- They show model, firmware, ports with PoE and loops, spanning tree, LLDP neighbors for the map, VLANs, fans, power, temperature and logs.
- New problems for them: a clock that's out (with the SNTP server that never answered), and no loop protection when spanning tree is off too.
- Instant On switches added over SSH earlier get a **Use web interface** button on their page.
- AOS-S: "Unsaved changes" no longer shows when the switch says its running configuration hasn't changed since the last save.

## 0.6.0

- Instant On switches managed from the cloud can be added. They have no SSH, so SwitchDesk reads them from the Instant On portal with your Instant On account. In **Add switch**, pick **Instant On cloud**, sign in, and choose switches from every site the account can see.
- They show online/offline status, ports with PoE draw and loops, portal alerts, firmware status and PoE budget. The devices on their ports put them on the network map.
- Instant On accounts are kept with your other credentials, encrypted, and move with an export.
- The map merges a link reported from both ends even when one end doesn't name its port.

## 0.5.3

- The network map no longer draws links between branches that are only connected through routers. A neighbor is now matched to a switch by its chassis MAC first. A matching IP address or name only counts if the MAC agrees, or if the other switch reports the link back. Branches that reuse the same private subnets used to get false links this way.
- "Different spanning-tree roots" is only raised for switches joined by layer 2 links. Routed branches each have their own root, and that's no longer flagged.

## 0.5.2

- AOS-CX firmware upload over HTTPS works with switches whose self-signed certificate has a negative serial number, such as the 6100. Before, the upload failed with "x509: negative serial number" and fell back to TFTP.
- The job log lists exactly which web server settings were changed for the upload.

## 0.5.1

- Firmware updates on AOS-CX upload the image over HTTPS to the switch's REST API, so the switch no longer has to reach your computer over TFTP. The web server is turned on in the management VRF for the upload if it was off, and turned off again afterwards.
- Firmware updates on AOS-CX no longer hang at "Continue (y/n)?". AOS-CX needs Enter after the answer; SwitchDesk now presses it, and switches that act on the single key (AOS-S) still get only the key.
- A TFTP copy that the switch never starts, or that stops moving, now fails after two minutes with the reason, instead of waiting until the switch logs the session out.
- Telnet and plain HTTP are checked by trying TCP ports 23 and 80 directly. A config that couldn't be read, or an Instant On switch, no longer shows a Telnet warning that isn't true. The switch page shows what was seen on each port.

## 0.5.0

- Firmware updates on AOS-S upload the image over SSH (SFTP or SCP), so they work over a VPN or zero trust access where the switch can't reach your computer. `ip ssh filetransfer` is turned on for the upload and the previous settings are restored afterwards.
- TFTP stays as the fallback and for AOS-CX. A failed TFTP copy now says when the switch couldn't reach your computer.

## 0.4.0

- A yellow bar tells you when a newer SwitchDesk is on GitHub; **Settings → Check for updates** checks on demand.
- About section and author credit.

## 0.3.0

- Firmware updates: library of images (add files or download links), available updates per switch, primary and secondary banks, one-click update with reboot or copy only, update jobs with logs.
- Automatic firmware updates: copy to the spare bank, or install inside a maintenance window.
- Built-in TFTP server for switches to download images from.

## 0.2.0

- Switch list: checkboxes with bulk collect, disable, enable and delete; per-row buttons; search by name, address, model or state.
- Disabled switches: paused without losing their data.
- Choose where the data folder lives; move or copy it.
- Export and import between computers with a passphrase-protected `.sdbak` file.

## 0.1.0

- First release: dashboard with problems and port faceplates, switch pages (system, spanning tree, ports, VLANs, neighbors, config, logs, console, raw output), VLAN editing with review, VLAN matrix, LLDP/CDP network map, config copies with compare.
- AOS-CX, AOS-S and Instant On over SSH.
