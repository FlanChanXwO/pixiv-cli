# Discovery playbooks

Chains for finding works and artists. All commands below assume the preflight
in SKILL.md already ran. Verify flags with `pixiv <cmd> --help`.

## Find works by keyword/tag

```
pixiv search "初音ミク" --limit 10 --json
```

- Default search target is partial tag match (`--search-target
  partial_match_for_tags`); switch to `exact_match_for_tags` or
  `title_and_caption` when the user asks for exact tags or title search.
- Sort defaults to `date_desc`. `popular_desc` may require Pixiv Premium on the
  account — if it errors, report the real API message; do not retry silently.
- Stable filters include `--rating sfw|r18|r18g|mature|all`, `--type
  all|illust-and-ugoira|illust|manga|ugoira`, `--ai-mode all|exclude|only`,
  `--aspect-ratio all|landscape|portrait|square`, `--resolution
  all|high|medium|low`, and exact `--tool` names. `comics` remains an alias for
  `manga`; only pass restricted ratings when the user explicitly asks.
- With a token, App API performs content-type, resolution, aspect-ratio, tool,
  and AI-exclusion filtering. Rating and AI-only are applied by the public SDK
  to App result batches; `AIType==2` is AI-generated.
- Tool names are dynamic. Run authenticated `pixiv search-options "WORD"
  --json`, then pass the returned name exactly. Do not hard-code a list.
- High, medium, and low resolution require both dimensions to be `>=3000`,
  `1000..2999`, and `<=999`, respectively. Bookmark-count filtering is not
  available.
- Anonymous Web search accepts only its reliable filters. R18/R18G/mature and
  `search-options` require authentication; never add a Cookie workaround or
  report an authentication failure as an empty result.
- Need page 2+: `--page N` (1-based) with a positive `--limit`.

## From a search hit to full detail

Extract `id` fields from the search JSON, then:

```
pixiv detail 129543211 --json
```

Detail includes tags, dimensions, page count, bookmark/view counts, AI flag,
App-provided drawing tools, and author info — prefer one `detail` call over
re-searching.

## Explore an artist

```
pixiv user detail 11
pixiv user artworks 11 --limit 20
pixiv user following 11 --limit 20
```

- `user detail USER_ID` requires the ID (no self-default). `--json` gives the
  full stable profile envelope.
- `user artworks` / `bookmarks` / `following` default to the current account
  when USER_ID is omitted.
- Only the ID is accepted. If the user gives a name/URL: extract the numeric ID
  from a `pixiv.net/users/<id>` URL, or search works first and take the author
  ID from results.

## Rankings and recommendations

```
pixiv ranking --mode day --limit 10
pixiv ranking --mode weekly --date 2026-07-01 --limit 10
pixiv recommended illust --limit 10
pixiv recommended all --limit 5
```

- `ranking` works anonymously (web fallback); R-18 modes need authentication.
- `recommended` always needs authentication and a kind. `all` = four
  independent streams (illust/manga/novel/user), each with its own pagination
  cursor — treat them as four lists, not one.

## Curate: bookmarks and follows (write ops)

```
pixiv bookmark add 129543211
pixiv follow add 11
```

State the target ID in one line before executing (SKILL.md operation tiers).
`remove` variants are symmetrical. These need authentication; on an anonymous
session report that a login is required instead of attempting fallback.
