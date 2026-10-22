# v1.2.1 — 2026-10-22

## Fixed

- Upgrade fsnotify to v1.10.1 to fix concurrent Windows account-store watcher access reported by the race detector. ([#153](https://github.com/FlanChanXwO/pixiv-cli/pull/153))

## Security

- Raise the required Go toolchain from 1.27.1 to 1.27.3 for source builds, CI and release binaries. Go 1.27.3 addresses CVE-2026-111943 by rejecting certificate chains containing specific untrusted intermediate certificates in `crypto/x509`; this upgrade also includes the Go 1.27.2 security fixes. ([#158](https://github.com/FlanChanXwO/pixiv-cli/pull/158), [Go security announcement](https://groups.google.com/g/golang-announce/c/66pojueaQek), [Go release history](https://go.dev/doc/devel/release#go1.27.2))

## Documentation

- Reorganize the bilingual documentation around tables and diagrams, split installation, authentication, MCP gallery and maintainer guides into focused pages, and correct quick-start commands. ([#154](https://github.com/FlanChanXwO/pixiv-cli/pull/154))
- Add a dynamic contributor gallery and links to the bilingual READMEs. ([#156](https://github.com/FlanChanXwO/pixiv-cli/pull/156))

## Maintenance

- Restore the root LICENSE to the plain MIT text for GitHub license detection; bundled third-party notices remain distributed separately. ([#155](https://github.com/FlanChanXwO/pixiv-cli/pull/155))
- Update the PR dependency checklist for the Go-only project. ([#157](https://github.com/FlanChanXwO/pixiv-cli/pull/157))

**Full Changelog**: [v1.2.0...v1.2.1](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.2.0...v1.2.1)
