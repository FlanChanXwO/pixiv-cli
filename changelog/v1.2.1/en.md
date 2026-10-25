# v1.2.1 — 2026-10-22

## Changed

- The documentation is reorganized into focused pages with tables and diagrams, including installation, authentication, the MCP gallery, and maintainer guides; quick-start commands are corrected, and the READMEs link to a contributor gallery. ([#154](https://github.com/FlanChanXwO/pixiv-cli/pull/154), [#156](https://github.com/FlanChanXwO/pixiv-cli/pull/156))
- The root `LICENSE` is plain MIT text so GitHub detects the license. Third-party notices remain available through `pixiv --licenses`. ([#155](https://github.com/FlanChanXwO/pixiv-cli/pull/155))

## Fixed

- A race condition in the Windows account-store file watcher is fixed (fsnotify updated to v1.10.1), so long-running CLI and MCP sessions no longer hit concurrent access errors. ([#153](https://github.com/FlanChanXwO/pixiv-cli/pull/153))

## Security

- The required Go toolchain is now 1.27.3 for source builds and release binaries. Go 1.27.3 rejects certificate chains with specific untrusted intermediates in `crypto/x509` (CVE-2026-111943) and also includes the Go 1.27.2 fixes. If you build from source, upgrade your toolchain. ([#158](https://github.com/FlanChanXwO/pixiv-cli/pull/158))

**Full Changelog**: [v1.2.0...v1.2.1](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.2.0...v1.2.1)

<!-- internal:
  [#157](https://github.com/FlanChanXwO/pixiv-cli/pull/157)
-->
