# Privacy policy

SwitchDesk is a desktop program. It has no account, no server of its own and no telemetry: it doesn't collect, send or sell anything about you, your computer or your network.

Everything it stores (switch list, encrypted credentials, collected data, config backups, settings) stays in its data folder on your computer.

## What it connects to

| Where | When | What is sent |
|---|---|---|
| Your switches | When you add them, and on the schedule you set in Settings | Your SSH, web or SNMP login for that switch, and the commands it runs |
| GitHub (`api.github.com`) | Every few hours to look for a new version. Turn it off in **Settings → Check for new versions automatically**; **Check for updates** still works when you click it | An ordinary request for the latest release, with SwitchDesk's version in the user agent. Nothing about you or your switches |
| GitHub (`raw.githubusercontent.com`) | Only with **Show front panel photos** on, once per switch model | The photo's file name |
| HPE Aruba Instant On portal | Only for Instant On sites you add from the cloud | Your Instant On sign-in, to read those sites |
| A download link you paste | Only when you add firmware from a link | An ordinary download request |

The SSH client, browser and terminal that the SSH and Web buttons open are your own programs; SwitchDesk only starts them with the switch's address.

## Questions

Email [jaba@clihero.com](mailto:jaba@clihero.com) or open an [issue](https://github.com/jaba0x/SwitchDesk/issues).
