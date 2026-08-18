# Unreleased

> Release-prep workspace. Audit the target tag range, then write the next bilingual notes directly. Every PR or direct commit in the audit must appear in both languages.

## Configuration and diagnostics

- Restored unified `[logging].level`/`[logging].format` configuration with `PIXIV_LOG_LEVEL` and
  `PIXIV_LOG_FORMAT` overrides; `debug` diagnostics remain stderr-only and MCP stdout stays JSON-RPC.
- `pixiv config` now manages the logging, download directory, request pacing, proxy, and account-pool keys
  from one schema; first-run `config.toml` is generated from schema metadata and never overwrites an existing file.
