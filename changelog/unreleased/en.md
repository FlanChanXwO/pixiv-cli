# Unreleased

> Release-prep workspace. Audit the target tag range, then write the next bilingual notes directly. Every PR or direct commit in the audit must appear in both languages. Internal-only changes are listed in a trailing `internal` comment, which is hidden from the rendered Release body.

## Changed

- Local accounts are now stored in `auth.json`. The first run of an account-using command automatically imports an existing `pixiv-cli.db`; the old database is retained unchanged, including its modification time.
- `pixiv config get/set/unset` now uses TOML paths, such as `download.path` in place of `download_path`. It accepts the declared configuration fields, including service network, login and default-account settings. Existing file paths stay the same, and obsolete file keys remain ignored.
- New `config.toml` files use a template with English comments and defaults. Existing files are preserved.
- `pixiv config set reverse_search.saucenao_api_key VALUE` accepts the API key as an argument. `config get` prints its effective value, including an environment override, directly instead of `<redacted>`.
- Simplify Ugoira GIF/APNG encoding with standard-library codecs; output images may have subtle visual differences.
