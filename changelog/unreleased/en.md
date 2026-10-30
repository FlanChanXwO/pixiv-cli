# Unreleased

> Release-prep workspace. Audit the target tag range, then write the next bilingual notes directly. Every PR or direct commit in the audit must appear in both languages. Internal-only changes are listed in a trailing `internal` comment, which is hidden from the rendered Release body.

## Breaking changes

- `pixiv mcp` no longer accepts the `mcp_gallery_cache_ttl` and `mcp_gallery_concurrency` settings; remove them from `config.toml`. ([#168](https://github.com/FlanChanXwO/pixiv-cli/pull/168))

## Added

- `pixiv whoami` shows the current local Pixiv account and its cached Premium status without network access; `--json` matches an `auth list --json` account.
- MCP adds the `pixiv_show_artwork` tool, and the Gallery layout adapts to phones and embedded panels. ([#167](https://github.com/FlanChanXwO/pixiv-cli/pull/167))

## Changed

- CLI text output uses readable account identities, status labels, single-line lists without headings, and detail cards. Empty results print `No results.` on stderr with exit code 0. For commands that support `--json` or `--ndjson`, scripts parsing text should use those options; their output is unchanged.
- `pixiv fanbox download` keeps stdout empty on success and no longer prints saved paths. This command has no `--json` or `--ndjson` option.

- JSON logs restore ERROR command-failure records. Debug logs add the SDK error creation site and safe typed network details, including OAuth token exchange failures.

- CLI logs on stderr use `[HH:MM:SS.mmm] [operation] [LEVEL] [parent/file:line]: message key=value` with terminal-aware colors, quoted control characters and indented multiline values; JSON logs include `source`. Default text logging keeps one error line.
- MCP search filters are preserved when a bookmark lookup partially fails, and the partial result is reported instead of dropping the filters or the successful items. ([#165](https://github.com/FlanChanXwO/pixiv-cli/pull/165))


## Fixed

- CLI text output escapes upstream titles, names, comments and tags onto a single line; FANBOX creator and post commands report output write failures.
- CLI failures explain authentication, invalid arguments, upstream HTTP status, timeout and cancellation with a next step. Local enum arguments are checked before opening an account.
- `pixiv auth list --json` returns `"accounts": []` when no accounts are saved.

**Full Changelog**: _Added when the next version is tagged._

<!-- internal:
  [#230](https://github.com/FlanChanXwO/pixiv-cli/pull/230)
  [#231](https://github.com/FlanChanXwO/pixiv-cli/pull/231)
  [#228](https://github.com/FlanChanXwO/pixiv-cli/pull/228)
  [#229](https://github.com/FlanChanXwO/pixiv-cli/pull/229)
  [#227](https://github.com/FlanChanXwO/pixiv-cli/pull/227)
  [#218](https://github.com/FlanChanXwO/pixiv-cli/pull/218)
  [#220](https://github.com/FlanChanXwO/pixiv-cli/pull/220)
  [#222](https://github.com/FlanChanXwO/pixiv-cli/pull/222)
  [#223](https://github.com/FlanChanXwO/pixiv-cli/pull/223)
  [#224](https://github.com/FlanChanXwO/pixiv-cli/pull/224)
  [#225](https://github.com/FlanChanXwO/pixiv-cli/pull/225)
  [#226](https://github.com/FlanChanXwO/pixiv-cli/pull/226)
  [#177](https://github.com/FlanChanXwO/pixiv-cli/pull/177)
  [#183](https://github.com/FlanChanXwO/pixiv-cli/pull/183)
  [#185](https://github.com/FlanChanXwO/pixiv-cli/pull/185)
  [#159](https://github.com/FlanChanXwO/pixiv-cli/pull/159)
  [#160](https://github.com/FlanChanXwO/pixiv-cli/pull/160)
  [#161](https://github.com/FlanChanXwO/pixiv-cli/pull/161)
  [#162](https://github.com/FlanChanXwO/pixiv-cli/pull/162)
  [#163](https://github.com/FlanChanXwO/pixiv-cli/pull/163)
  [#164](https://github.com/FlanChanXwO/pixiv-cli/pull/164)
  [#166](https://github.com/FlanChanXwO/pixiv-cli/pull/166)
  [#170](https://github.com/FlanChanXwO/pixiv-cli/pull/170)
  [#171](https://github.com/FlanChanXwO/pixiv-cli/pull/171)
  [#172](https://github.com/FlanChanXwO/pixiv-cli/pull/172)
  [#173](https://github.com/FlanChanXwO/pixiv-cli/pull/173)
  [#174](https://github.com/FlanChanXwO/pixiv-cli/pull/174)
  [#175](https://github.com/FlanChanXwO/pixiv-cli/pull/175)
  [#176](https://github.com/FlanChanXwO/pixiv-cli/pull/176)
  [#178](https://github.com/FlanChanXwO/pixiv-cli/pull/178)
  [#179](https://github.com/FlanChanXwO/pixiv-cli/pull/179)
  [#180](https://github.com/FlanChanXwO/pixiv-cli/pull/180)
  [#181](https://github.com/FlanChanXwO/pixiv-cli/pull/181)
  [#182](https://github.com/FlanChanXwO/pixiv-cli/pull/182)
  [#184](https://github.com/FlanChanXwO/pixiv-cli/pull/184)
  [#186](https://github.com/FlanChanXwO/pixiv-cli/pull/186)
  [#187](https://github.com/FlanChanXwO/pixiv-cli/pull/187)
  [#188](https://github.com/FlanChanXwO/pixiv-cli/pull/188)
  [#189](https://github.com/FlanChanXwO/pixiv-cli/pull/189)
  [#190](https://github.com/FlanChanXwO/pixiv-cli/pull/190)
  [#191](https://github.com/FlanChanXwO/pixiv-cli/pull/191)
  [#192](https://github.com/FlanChanXwO/pixiv-cli/pull/192)
  [#193](https://github.com/FlanChanXwO/pixiv-cli/pull/193)
  [#194](https://github.com/FlanChanXwO/pixiv-cli/pull/194)
  [#195](https://github.com/FlanChanXwO/pixiv-cli/pull/195)
  [#196](https://github.com/FlanChanXwO/pixiv-cli/pull/196)
  [#197](https://github.com/FlanChanXwO/pixiv-cli/pull/197)
  [#198](https://github.com/FlanChanXwO/pixiv-cli/pull/198)
  [#199](https://github.com/FlanChanXwO/pixiv-cli/pull/199)
  [#200](https://github.com/FlanChanXwO/pixiv-cli/pull/200)
  [#202](https://github.com/FlanChanXwO/pixiv-cli/pull/202)
  [#203](https://github.com/FlanChanXwO/pixiv-cli/pull/203)
  [#204](https://github.com/FlanChanXwO/pixiv-cli/pull/204)
  [#205](https://github.com/FlanChanXwO/pixiv-cli/pull/205)
  [#206](https://github.com/FlanChanXwO/pixiv-cli/pull/206)
  [#207](https://github.com/FlanChanXwO/pixiv-cli/pull/207)
  [#208](https://github.com/FlanChanXwO/pixiv-cli/pull/208)
  [#209](https://github.com/FlanChanXwO/pixiv-cli/pull/209)
  [#210](https://github.com/FlanChanXwO/pixiv-cli/pull/210)
  [#211](https://github.com/FlanChanXwO/pixiv-cli/pull/211)
  [#212](https://github.com/FlanChanXwO/pixiv-cli/pull/212)
  [#213](https://github.com/FlanChanXwO/pixiv-cli/pull/213)
  [#214](https://github.com/FlanChanXwO/pixiv-cli/pull/214)
  [#215](https://github.com/FlanChanXwO/pixiv-cli/pull/215)
  [#216](https://github.com/FlanChanXwO/pixiv-cli/pull/216)
  [#217](https://github.com/FlanChanXwO/pixiv-cli/pull/217)
  [#219](https://github.com/FlanChanXwO/pixiv-cli/pull/219)
  [#221](https://github.com/FlanChanXwO/pixiv-cli/pull/221)
-->
