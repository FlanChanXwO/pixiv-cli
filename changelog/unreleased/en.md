# Unreleased

> Release-prep workspace. Audit the target tag range, then write the next bilingual notes directly. Every PR or direct commit in the audit must appear in both languages. Internal-only changes are listed in a trailing `internal` comment, which is hidden from the rendered Release body.

## Breaking changes

- Rewrite the Go SDK around account-bound references and objects: create Pixiv clients with `pixiv.New(CredentialStore, Options)` and FANBOX clients with `fanbox.New(sessionID, Options)`, replacing `Open`/`OpenWith`/`NewWith`. Load metadata through reference `Get` methods, call operations on the returned objects, and pass `ctx` to every network operation. Pixiv refreshes credentials through revision-checked persistence; callers own account selection and storage. Lists use query/account-bound `sdk.Page[T]`, `Next(ctx)` and opaque cursors; decoded objects/pages must regain their client binding before further operations. Configure logging through the current `slog` default logger instead of constructor logger options. Handle the unified `sdk.Error` / `Reason` and preserved error chain; replace generic resource wrappers with product `Image`/`File` operations. Use independent `sdk/saucenao` and `sdk/ascii2d` packages for image search, and `sdk/download` for selected Pixiv pages and partial download reports. Module and import roots remain unchanged.

## Changed

- Local accounts are now stored in `auth.json`. When initializing the account file, the first account-using command automatically imports an existing `pixiv-cli.db`; a valid current-format `auth.json` is preserved without merging the database; the old database is retained unchanged, including its modification time.
- `pixiv config get/set/unset` now uses TOML paths, such as `download.path` in place of `download_path`. It accepts the declared configuration fields, including service network, login and default-account settings. Existing file paths stay the same, and obsolete file keys remain ignored.
- New `config.toml` files use a template with English comments and defaults. Existing files are preserved.
- `pixiv config set reverse_search.saucenao_api_key VALUE` accepts the API key as an argument. `config get` prints its effective value, including an environment override, directly instead of `<redacted>`.
- Simplify Ugoira GIF/APNG encoding with standard-library codecs; output images may have subtle visual differences.
- `config.toml` values must use the declared TOML type. Quoted numbers and booleans, such as `check_enabled = "true"`, are no longer converted and now fail with the line and key to fix; remove the quotes.
- MCP `next_cursor` values issued by earlier versions are rejected; restart the query with its original arguments. A cursor now stays on the account that produced its first batch, and HTTP continuation requires the same account selection.
- MCP Gallery records identify images with readable objects (`artwork_id`, `page_index`, `quality`, or `user_id`) instead of opaque strings.
- FANBOX commands and tools reject `api.fanbox.cc` and `downloads.fanbox.cc` URLs instead of reading the host as a creator ID.
- CLI and MCP error messages for upstream, validation and pagination failures are reworded.

## Fixed

- Ugoira GIF conversion now fails before encoding when its frames would need more than 1 GiB of memory, instead of exhausting it.
- A canceled ascii2d search no longer waits indefinitely for an unresponsive FlareSolverr to close its session, and an oversized FlareSolverr response is rejected.
- Pixiv media URLs that name a non-default port on a media host are rejected.
- `pixiv fanbox tags` and the `fanbox_creator_tags` MCP tool no longer fail with `missing FANBOX tag list`; they read the tag list FANBOX currently returns.
