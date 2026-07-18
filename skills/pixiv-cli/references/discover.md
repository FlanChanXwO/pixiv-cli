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
- `search` does not have a `--tag` flag. Supply tag text as the required `WORD`;
  reserve `--tag` for `user bookmarks` (filter) and `bookmark add` (repeatable
  bookmark tag).
- Sort defaults to `date_desc`. `popular_desc` may require Pixiv Premium on the
  account — if it errors, report the real API message; do not retry silently.
- Stable filters include `--rating sfw|r18|r18g|mature|all`, `--type
  all|illust-and-ugoira|illust|manga|ugoira`, `--ai-mode all|exclude|only`,
  `--aspect-ratio all|landscape|portrait|square`, `--resolution
  all|high|medium|low`, and exact `--tool` names. Only pass restricted ratings
  when the user explicitly asks.
- Tool names are dynamic. Run authenticated `pixiv search-options "WORD"
  --json`, then pass the returned name exactly. Do not hard-code a list.
- Anonymous Web search accepts only its reliable filters. R18/R18G/mature and
  `search-options` require authentication; never add a Cookie workaround or
  report an authentication failure as an empty result.
- Need page 2+: `--page N` (1-based) with a positive `--limit`.

## From a search hit to full detail

Extract `id` fields from the search JSON, then:

```
pixiv detail 129543211 --json
```

Inspect the returned fields before selecting values for a follow-up action;
prefer one `detail` call over re-searching the same work.

## Explore an artist

```
pixiv user detail 11
pixiv user artworks 11 --limit 20
pixiv user bookmarks 11 --tag "初音ミク" --limit 20
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
- `recommended` always needs authentication and a kind. For `all`, inspect the
  actual output shape and keep the returned categories separate rather than
  assuming one flat list.

## Curate: bookmarks and follows (write ops)

```
pixiv bookmark add 129543211
pixiv follow add 11
```

State the target ID in one line before executing (SKILL.md operation tiers).
`remove` variants are symmetrical. These need authentication; on an anonymous
session report that a login is required instead of attempting fallback.
