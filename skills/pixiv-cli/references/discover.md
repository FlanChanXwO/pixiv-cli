# Discovery playbooks

Chains for finding works and artists. All commands below assume the preflight
in SKILL.md already ran. Verify flags with `pixiv <cmd> --help`.

## Find works by keyword/tag

```
pixiv search "初音ミク" --limit 10 --json
```

- Default search target is partial tag match (`--target tag-partial`); switch
  to `--target tag-exact` or `--target title-caption` when the user asks for
  exact tags or title/caption search. Use `--period day|week|month` to limit
  the time range, or `--sort date_desc|date_asc` to choose the order.
- `search` does not have a `--tag` flag. Supply tag text as the required `WORD`;
  reserve `--tag` for `user bookmarks` (filter) and `bookmark add` (repeatable
  bookmark tag).
- Sort defaults to `date_desc`; `date_asc` is the only other supported value.
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
- Local filters skip leading empty upstream batches until the first non-empty
  logical batch or true end. `--limit N` fills filtered results across batches;
  `--limit 0` walks all filtered results. Do not invent request caps.
- There is no like-count field; do not treat bookmark totals as likes.
- Artwork JSON/text include the stable page URL
  `https://www.pixiv.net/artworks/{id}` as the first field/line.

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
pixiv ranking --mode week --date 2026-07-01 --limit 10
pixiv ranking --mode week_r18 --limit 10
pixiv recommended illust --limit 10
pixiv recommended all --limit 5
```

- `ranking` supports `day`, `day_male`, `day_female`, `week`, `week_original`,
  `week_rookie`, `month`, `day_manga`, `week_manga`, `month_manga`,
  `week_rookie_manga`, `day_r18`, `day_male_r18`, `day_female_r18`,
  `week_r18`, and `week_r18g`. The first seven work anonymously through the
  Web fallback; the final nine require authentication. Never substitute a
  failed extended mode with `day`.
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
