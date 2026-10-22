# v1.2.1 — 2026-10-22

## Fixed

- Upgrade fsnotify to v1.10.1 to fix concurrent Windows account-store watcher access reported by the race detector. ([#153](https://github.com/FlanChanXwO/pixiv-cli/pull/153))

## Security

- Raise the required Go toolchain from 1.27.1 to 1.27.3 for source builds, CI and release binaries. Go 1.27.3 addresses CVE-2026-111943 by rejecting certificate chains containing specific untrusted intermediate certificates in `crypto/x509`; this upgrade also includes the Go 1.27.2 security fixes. ([#158](https://github.com/FlanChanXwO/pixiv-cli/pull/158), [Go security announcement](https://groups.google.com/g/golang-announce/c/66pojueaQek), [Go release history](https://go.dev/doc/devel/release#go1.27.2))

**Full Changelog**: [v1.2.0...v1.2.1](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.2.0...v1.2.1)
