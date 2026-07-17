---
name: pixiv
description: Operate Pixiv through the pixiv-cli binary—search and filter illustrations, inspect details and users, manage bookmarks/follows, and download artworks. Verify current syntax with `pixiv <command> --help`.
---

# Pixiv CLI Skill

Use this skill when operating the `pixiv` binary. Treat `--help` and the project
README as the command contract; this skill focuses on safe orchestration and
semantic traps.

## Preflight

1. Run `pixiv version --json` once. If it fails, report the real installation
   or execution error.
2. Run `pixiv auth list --json`. It lists configured accounts and never prints
   tokens, but does not prove their credentials are valid. Use the networked
   `pixiv auth check [UID] --json` when validation is needed. If no configured
   account is available, only the documented anonymous Web fallback remains.

## Hard rules

1. Never directly handle, print, echo, or persist a raw refresh/access token
   outside the CLI-managed account store. `auth add` and `auth login` may store
   credentials through the supported CLI path; never inspect `auth.json`.
2. Never pass a literal token via `--token` or `--refresh-token`; direct the
   user to interactive `pixiv auth add` when importing one.
3. Never accept or forward `PHPSESSID` or another browser Cookie. The CLI uses
   App API refresh tokens and does not convert them into Web sessions.
4. Run interactive `pixiv auth login` only when the user explicitly asks and
   is present to complete browser OAuth.

## Operation tiers

| Tier | Commands | Behavior |
| --- | --- | --- |
| Read | `search`, `search-options`, `detail`, `ranking`, `recommended`, `user *`, `config get/path`, `auth list/check`, `version`, `update --check` | Execute freely. |
| Write | `bookmark add/remove`, `follow add/remove` | State the target ID before executing. |
| Disk | `download` | Confirm target directory and item count; read `references/download.md`. |
| Account/config | `auth use/remove`, `config set/unset`, actual `update` | Ask for explicit confirmation each time. |

## Search contract

```text
pixiv search "WORD" --limit 10 --json
pixiv search "WORD" --rating sfw --type illust --ai-mode exclude
pixiv search "WORD" --resolution high --aspect-ratio landscape --tool "CLIP STUDIO PAINT"
pixiv search-options "WORD" --json
```

- `--type`: `all|illust-and-ugoira|illust|manga|ugoira`; `comics` remains a
  compatibility alias for `manga`.
- `--ai-mode`: `all|exclude|only`. Pixiv `AIType==2` means AI-generated.
  Deprecated `--ai-type` maps `0=exclude`, `1=only`, `2=all` and conflicts
  with an explicit `--ai-mode`.
- `--aspect-ratio`: `all|landscape|portrait|square`.
- `--resolution`: `all|high|medium|low`; both dimensions must be `>=3000`,
  `1000..2999`, or `<=999`, respectively.
- `--tool` is an exact upstream tool name. Obtain dynamic choices with
  authenticated `search-options`; do not invent or hard-code tool lists.
- No bookmark-count search filter exists.

With a refresh token, search stays on App API and an App error is final. App
filters resolution, aspect, tool, content type, and AI exclusion; the public
SDK filters rating and AI-only results. Without a token, Web fallback performs
only reliable filters. `r18`, `r18g`, `mature`, and `search-options` require
authentication and must not be presented as empty anonymous results.

## Output and pagination

- Always use a positive `--limit N` for bounded list requests. No `--limit`
  means one upstream batch; `--limit 0` follows all pages and should only be
  used when the user explicitly wants everything.
- `--page` is 1-based and requires a positive `--limit`; `--offset` is
  deprecated.
- Use human output for small display-only results and `--json` for field
  extraction or command chaining. Keep MCP stdout reserved for JSON-RPC.

## Routing

| Task | Read |
| --- | --- |
| Search, filter, inspect artists and recommendations | `references/discover.md` |
| Download single, batch, multi-page, or ugoira works | `references/download.md` |
| Authentication, fallback, network, or empty-result diagnosis | `references/troubleshooting.md` |
