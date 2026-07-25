# Discovery playbooks

Chains for finding works and artists. All commands below assume the preflight
in SKILL.md already ran. Verify flags with `pixiv <cmd> --help`.

## Find works by keyword/tag

```
pixiv search "初音ミク" --limit 10 --json
```

- Default search field is partial tag match (`--search-by tag-partial`); switch
  to `--search-by tag-exact`, `--search-by title-caption`, or authenticated
  `--search-by tag-title-caption` when the user asks for exact tags, title/caption,
  or tags-plus-title/caption search. Use `--period day|week|month|half-year|year`
  to limit the time range, or use inclusive `--start-date YYYY-MM-DD` and/or
  `--end-date YYYY-MM-DD` (never together with `--period`).
- For a reliable boolean tag query, use exact tags: `pixiv search "tagA tagB"
  --search-by tag-exact` requires both tags, while `pixiv search "tagA OR tagB"
  --search-by tag-exact` accepts either tag. `OR` is uppercase; literal `AND`
  is not an operator. Partial tag search also accepts the verified uppercase
  `OR`, but it can match partial, alias, or translated tags and is not a strict
  exact-tag AND. `title-caption` and `tag-title-caption` have no boolean-tag contract.
- `search` does not have a `--tag` flag. Supply tag text as the required `WORD`;
  reserve `--tag` for `user bookmarks` (filter) and `bookmark add` (repeatable
  bookmark tag).
- Sort defaults to `date_desc`; `date_asc` is the only other supported value.
- Stable filters include `--rating sfw|r18|r18g|mature|all`, `--type
  all|illust-and-ugoira|illust|manga|ugoira`, `--ai-mode all|exclude|only`,
  `--aspect-ratio all|landscape|portrait|square`, `--resolution
  all|high|medium|low`, exact `--draw-tool` names, and App-only, Pixiv Premium-only
  `--bookmark-min N` / `--bookmark-max N` public bookmark-count bounds. Only pass restricted
  ratings or bookmark bounds when the user explicitly asks; say that Pixiv Premium is required
  for bookmark bounds, and never call bookmark count a like count. For a saved account, bookmark bounds use a cached
  self-profile membership check (24h by default) and a non-Premium account is blocked before search; do not refresh
  that cache unless the user explicitly asks for `pixiv auth refresh`.
- Tool names are dynamic. Run authenticated `pixiv search-options "WORD"
  --json`, then pass the returned name exactly. Do not hard-code a list.
- Anonymous Web search accepts only its reliable filters. R18/R18G/mature,
  `tag-title-caption`, bookmark bounds, and `search-options` require authentication; bookmark
  bounds also require Pixiv Premium. Never add a Cookie workaround or report an authentication
  failure as an empty result.
- Need page 2+: `--page N` (1-based) with a positive `--limit`.
- Local filters skip leading empty upstream batches until the first non-empty
  logical batch or true end. `--limit N` fills filtered results across batches;
  `--limit 0` walks all filtered results. Do not invent request caps.
- There is no like-count field; do not treat bookmark totals as likes.
- Artwork JSON/text include the stable page URL
  `https://www.pixiv.net/artworks/{id}` as the first field/line.

## Find novels by keyword/tag

```
pixiv novel search "初音ミク" --rating sfw --min-text-length 1000 --limit 10 --json
```

- `novel search` requires App authentication and never falls back to anonymous Web search.
- It supports `--search-by tag-partial|tag-exact|title-caption`, `--sort
  date_desc|date_asc`, `--period day|week|month`, and `--rating
  sfw|r18|r18g|mature|all`.
- `--min-text-length` and `--max-text-length` are non-negative character bounds;
  `0` disables a bound, and a non-zero maximum cannot be below the minimum.
  Use `--original-only` only when the user explicitly wants original novels.
- Rating, text length, and original-only are verified from returned novel fields.
  They may therefore skip upstream batches; pagination follows the same logical
  `--limit`/`--page` rules as illustration search. Never infer a request cap.
- Novel JSON includes `url`, `x_restrict`, `text_length`, and `is_original`.

## Pagination and completeness

- A search without an explicit `--limit` returns one logical upstream batch. It
  is a sample, never evidence that no further matches exist.
- `--limit N` continues across upstream batches until it has N filtered results
  or reaches the current end. If it returns exactly N, say “found N matching
  works” or “first N matches”; never say “only N matches exist”. If it returns
  fewer than N, the current query reached its end and that fact may be stated.
- When the user asks for a specific number of candidates, request that number
  with `--limit N`; do not stop at a smaller first batch. To continue a prior
  bounded search, keep WORD, search fields, filters, and sort identical, then
  increment `--page N` (1-based). `--page` always requires a positive
  `--limit`.
- Only use `--limit 0` for an explicit request to enumerate every result the
  current search can return. State that this is an exhaustive traversal of the
  current API search result, not an unsupported claim about a permanent global
  corpus.

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

To discover a user by name instead of starting from an artwork, use:

```
pixiv user search "NAME" --limit 20 --json
```

- Authenticated results use Pixiv's App user search and report
  `source: "app_search"`.
- Anonymous fallback reports `source: "related_illust_authors"`; it is a
  deduplicated list of authors from illustration search, not username search.
  State that distinction before claiming a name search is complete.

- `user detail USER_ID` requires the ID (no self-default). `--json` gives the
  full stable profile envelope.
- `user artworks` / `bookmarks` / `following` default to the current account
  when USER_ID is omitted.
- `user detail` accepts only the ID. If the user gives a `pixiv.net/users/<id>`
  URL, extract its numeric ID. For a name, prefer `user search` when authenticated;
  otherwise search works and take the author ID from results while labeling the
  anonymous source correctly.

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
