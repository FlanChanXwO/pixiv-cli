# v0.4.2 — 2026-07-19

## Added

- Added `scripts/install.sh` and PowerShell-free `scripts/install.cmd`. They select the current OS/architecture archive from the latest stable release, verify the published SHA-256 and staged binary, and perform a non-admin per-user installation. Both scripts are published and signed as fixed release assets.
- Added `pixiv auth import` / `auth export` and public SDK auth-bundle APIs for hidden TTY/raw-stdin token import, single-token or versioned-bundle export, and atomic offline restore.
- Illustration search gained rating, type, AI, aspect-ratio, resolution, and drawing-tool filters; the public SDK adds `SearchIllustFilters`, `SearchIllustOptions`, and `Illust.Tools`, while authenticated `pixiv search-options WORD` exposes available tools.
- MCP `search_illust` gained the matching filters and `search_illust_options`; typed upstream and local-state error classification now reports safe transport/configuration causes without exposing URLs, hosts, certificates, or credentials.
- Added the English `pixiv-cli` product skill and reorganized public documentation into English, Simplified Chinese, and Japanese entry points with maintainers' material under `docs/maintainers/`.

## Changed

- **Breaking:** removed `pixiv auth add`, `pixiv auth token`, and `--token` without aliases. Use `auth import`; only `auth export [UID]` and `auth export --all` without `--output` may intentionally write a secret to stdout.
- Auth import reports redacted `{user_id,username,status}` account records and has a fixed bundle schema. Offline bundle restore merges by UID, preserves a non-empty local default, rejects token/proxy combinations, and never refreshes or calls the network.
- With a refresh token, searches always use App API and never fall back to Web. Anonymous Web search applies only reliable filters; R18/R18G/mature content and dynamic search options explicitly require login.
- `pixiv search --r18` remained a deprecated alias for `--rating r18` in this release; it no longer edits the search word and conflicts with a different explicit rating.

## Fixed

- Preserved CRLF for `install.cmd`, built Linux release assets on Ubuntu 22.04, and gate-checked GNU symbol requirements so Linux amd64/arm64 assets do not exceed the public `GLIBC_2.35` baseline.
- Made bundle JSON decoding canonical-key-only; corrected AI semantics (`AIType == 2`); redacted legacy MCP operation diagnostics; normalized unsafe download-derived filename extensions; emitted all MCP tags; and stabilized known ranking-mode titles.
- Removed hidden 60-second HTTP-client timeouts in favor of caller-context lifetime, unified configured transport handling, and made config/auth writes durable and atomic with typed commit outcomes and platform-appropriate permission/ACL handling.
- Preserved real business errors for MCP download failures, rejected invalid random-download counts rather than silently changing them, and retained safe error classification for refresh-token initialization and execution failures.

## Security

- Installer and updater paths accept only exact official GitHub release sources and fail before replacement on missing, duplicate, malformed, or mismatched checksums and on failed staged-binary checks. PATH changes remain opt-in and no installer reads Pixiv auth state.
- Secret export is local-only and secret output is restricted to the documented raw-token or bundle forms. File export uses dedicated owner-only handling; restore reports whether a write was committed, not committed, or unresolved.
- Hardened Rust/native evidence provenance, macOS OAuth helper temporary-source handling, graph input path validation, ugoira ZIP frame allocation limits, proxy redaction, and release-workflow SemVer and secret-reference policy parsing.
