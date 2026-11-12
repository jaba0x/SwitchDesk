# Security

## Reporting a vulnerability

Please don't open a public issue or discussion for a security problem.

Report it privately through GitHub instead: open the [Security tab](https://github.com/jaba0x/SwitchDesk/security) and click **Report a vulnerability**. Only you and I can see the report, and we can talk it through there. If that doesn't work for you, email jaba@clihero.com.

Tell me the version you tested, what an attacker could do, and how to reproduce it. A proof of concept helps a lot.

I'll reply within a few days. Once a fix is out, the release notes credit you by name unless you'd rather stay anonymous.

## Supported versions

Fixes go into the latest release. SwitchDesk tells you in the app when a newer version is out, so please update before reporting.

## Scope

SwitchDesk runs on your own computer and talks to your switches over SSH, HTTPS and, during some firmware updates, TFTP. These are in scope:

- how credentials are stored and encrypted, in the data folder and in `.sdbak` exports
- SSH host key checking and the handling of switch output
- the firmware library, downloads and the built-in TFTP server
- the Instant On portal and web interface sign-in
- the update check against GitHub releases

Bugs in the switches' own firmware belong with HPE Aruba Networking.

## Antivirus warnings

Some antivirus and EDR products flag `SwitchDesk.exe`. As far as I can tell these are heuristic false positives, and here is how to check that for yourself rather than taking my word for it.

**Where the files come from.** Every release is built by GitHub Actions from the source in this repository, with the workflow in [`.github/workflows/build.yml`](.github/workflows/build.yml). Nothing is built or uploaded by hand.

**Checking a download.** From 1.0.15 on, each release has `SHA256SUMS.txt` and a signed build attestation. With the [GitHub CLI](https://cli.github.com):

```sh
gh attestation verify SwitchDesk-1.0.15-windows-x64.zip --repo jaba0x/SwitchDesk
```

It confirms the file was built by this repository's workflow, from which commit. You can also build it yourself: `wails build -platform windows/amd64` gives the same program.

**Why it gets flagged.** SwitchDesk isn't code-signed yet, so it has no publisher reputation. And several things it does on purpose look like what malware does to a heuristic engine:

- it opens connections to many hosts and ports (it checks switches on 22, 23, 80 and 443)
- it runs an SFTP and a TFTP server for firmware updates
- it starts other programs (your SSH client, your browser)
- it loads the WebView2 runtime the way all Wails apps do
- it's written in Go, whose single large executables are often flagged by machine-learning engines

**If your antivirus blocks it,** please open an issue with the product, the detection name and the version, and send the file to the vendor as a false positive. Code signing is planned so that this goes away.
