# Settings

![Settings](images/settings.png){ .shot }

## Language

The interface language. **Automatic** follows your computer's language when SwitchDesk has it. Languages marked *draft* still need a review by a native speaker. **Translation files** opens the folder where you can add your own; see [Translating](translating.md).

## Credentials

Shared username and password sets. Add as many as you need and pick one per switch. Editing a set and leaving the password empty keeps the old one. A set that's still in use can't be removed.

## Monitoring

| Setting | Default | What it does |
|---|---|---|
| Status check | SSH port | How SwitchDesk tells whether a switch is up between reads. See below. |
| Check every | 30 seconds | How often the status check runs. Not used when the check is off. |
| Read switches over SSH every | 15 minutes | How often each switch is logged in to and read. 0 means only when you click **Collect now**. |
| Switches read at the same time | 4 | Up to 16. Higher is faster on big networks. |
| Config copies kept per switch | 20 | Older copies are deleted. |
| Warn above CPU % | 80 | |
| Warn above memory % | 90 | |
| Don't flag default SNMP communities | off | For networks where `public` is expected. |

### Status check

- **SSH port**: opens a TCP connection to each switch's SSH port (or web port for Instant On) and closes it. Fast and works through most VPN and zero trust setups, but the switch logs every check as an SSH connection.
- **Ping (ICMP)**: pings each switch with the computer's own `ping` command. No SSH connections at all, but ICMP has to reach the switches. Many zero trust tunnels only carry TCP, so switches can show offline there.
- **Off**: no background checks. A switch's status only changes when it's read: on the **Read switches over SSH every** schedule, or when you click **Collect now**. A switch that can't be reached is marked offline; one that answers but refuses the login still counts as online. With the read interval at 0 as well, SwitchDesk only connects when you ask it to.

## Automatic firmware updates

Off, copy only, or install inside a maintenance window. See [Firmware updates](firmware.md#automatic-updates).

## App password

Optional, and off unless you turn it on. Useful when other people can use the same computer account, or the data folder sits on a shared or synced drive.

With an app password:

- SwitchDesk asks for it when it starts. Until it's entered, nothing is read or checked.
- The saved switch passwords can't be read without it, not even from the data folder. The key that encrypts them (`secret.key`) is replaced by `secret.lock`, the same key encrypted with your password (Argon2id + AES-GCM).
- **Lock the window after** locks SwitchDesk after the idle time you pick. **Lock now** locks it straight away. While the window is locked, checks and reads keep running in the background.

There's no way to recover a forgotten password. The lock screen offers **Forgot the password?**, which opens SwitchDesk without it: your switches, settings, config copies and map stay, but every saved password is cleared and has to be entered again under **Credentials**.

**Change password** and **Turn off** both ask for the current password. Moving or copying the data folder keeps the password, and an `.sdbak` export has its own passphrase as before.

## Data

The data folder, export and import. See [Your data](data.md).

## About

The version you run, a link to [clihero.com](https://clihero.com) and the GitHub repository, and **Check for updates**.
