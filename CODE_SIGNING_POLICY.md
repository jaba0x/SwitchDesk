# Code signing policy

Free code signing provided by [SignPath.io](https://about.signpath.io), certificate by [SignPath Foundation](https://signpath.org).

## What gets signed

The Windows program, `SwitchDesk.exe`, in the `SwitchDesk-<version>-windows-x64.zip` of each [release](https://github.com/jaba0x/SwitchDesk/releases). Nothing else is signed with this certificate.

## How a release is built and signed

1. A release is built by GitHub Actions from the source in this repository, with the workflow in [`.github/workflows/build.yml`](.github/workflows/build.yml). Nothing is built or uploaded by hand.
2. The unsigned `SwitchDesk.exe` goes from that workflow straight to SignPath as a signing request.
3. Every signing request is approved by hand by an approver (below) before it's signed.
4. The signed file is published on the release page together with `SHA256SUMS.txt` and a signed GitHub build attestation, which anyone can check with `gh attestation verify <file> --repo jaba0x/SwitchDesk`.

## Team

| Role | Who |
|---|---|
| Authors and committers | [Jaba Macharashvili](https://github.com/jaba0x) |
| Reviewers | [Jaba Macharashvili](https://github.com/jaba0x) |
| Approvers | [Jaba Macharashvili](https://github.com/jaba0x) |

Changes from other people come in as pull requests and are reviewed before they're merged. All members use multi-factor authentication for GitHub and SignPath.

## Privacy

SwitchDesk runs on your own computer. It won't transfer information to other networked systems unless you ask it to. See the [privacy policy](PRIVACY.md) for exactly what it connects to and when.
