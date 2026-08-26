# Discovery playbooks

Chains for finding works and artists. All commands below assume the preflight
in SKILL.md already ran. Verify flags with `pixiv <cmd> --help`.

## Find works by keyword/tag

```
pixiv search "初音ミク" --type artwork --limit 10 --json
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
- `--content-type all|illust-and-ugoira|illust|manga|ugoira`, `--ai-mode
  all|exclude|only`, `--aspect-ratio all|landscape|portrait|square`,
  `--resolution all|high|medium|low`, and exact `--draw-tool` names are
  artwork-only filters. `--type` selects the entity route; it is not an
  artwork subtype flag.
- `--bookmark-min N` / `--bookmark-max N` are non-negative inclusive public
  bookmark-count bounds. Use `--bookmark-strategy auto|local|best_effort|server`
  only when the user explicitly asks for bookmark-count filtering. `auto`
  currently uses the local candidate filtering path; `local` has the same
  filtering semantics. `best_effort` preserves the upstream candidate bounds
  and marks the result partial when local completeness cannot be established.
  `server` fails explicitly because this branch has no reliable server-filter
  evidence. Never call bookmark count a like count, and do not describe a
  candidate page as a complete site-wide result.
- `--rating` is retained only as a compatibility diagnostic. Any non-empty
  value is rejected because the v1 App API search contract has no verified
  rating field; it is not a local filter. The same unsupported-field rule
  applies when a flag is not valid for the selected entity.
- Drawing-tool names use the fixed catalog for this CLI version. Choose an exact
  value from the [CLI reference](../../../docs/en/cli-reference.md#drawing-tool-catalog);
  a unique one-edit spelling correction is shown in the validation error.
- Search requires an authenticated local account. `tag-title-caption` and
  bookmark-count filtering are App-only in this branch. Never add a Cookie
  workaround or report an authentication failure as an empty result.
- Need page 2+: `--page N` (1-based) with a positive `--limit`.
- Local filters skip leading empty upstream batches until the first non-empty
  logical batch or true end. `--limit N` fills filtered results across batches;
  `--limit 0` walks all filtered results. Do not invent request caps.
- There is no like-count field; do not treat bookmark totals as likes.
- Artwork JSON/text include the stable page URL
  `https://www.pixiv.net/artworks/{id}` as the first field/line.

## Reverse-search an image

Use the same `search` command with a local regular file or an HTTP(S) image URL:

```bash
pixiv search ./image.png --provider saucenao --json
pixiv search https://your-image-url.example/image.png --provider all --ndjson
```

- An explicit `http:`/`https:` scheme always selects reverse-search mode. An
  invalid URL is an error and must not be retried as a keyword. Other values
  select image mode only when they resolve to an existing regular file; other
  text remains a keyword.
- Providers are `saucenao`, `ascii2d-color`, `ascii2d-bovw`, and `all`.
  `reverse_search_provider` defaults to `saucenao`; `--provider` is a one-call
  override. `reverse_search_pixiv_only` defaults to true and controls whether
  non-Pixiv evidence stays in `results`.
- Image mode accepts provider/output/proxy flags only. Do not add keyword
  filters, `--type`, `--limit`, `--page`, or `--trending-tags`.
- SauceNAO and ascii2d are third-party upload services. The CLI snapshots the
  source once and returns only its kind/hash, so use an image and URL that the
  user is authorized to share. Provider retention/caching follows provider
  policy; ascii2d accepts JPEG/PNG/WEBP and has a provider-specific 10 MB limit.
- JSON returns `input`, provider summaries, raw provider evidence, canonical
  `records`, provider errors, and `partial`. Piped/explicit NDJSON emits only
  canonical records. Reverse-search artwork records use generic `type=artwork`,
  not `illust`, because subtype is not established; external-only hits are not
  records. With `all`, one successful and one failed provider is a successful
  partial result; a single-provider or all-provider failure is non-zero.
- Never echo the source, API key, temporary path, CSRF/redirect values, or
  upstream response body. The MCP version has the same provider result shape
  but permits private files and private/loopback/link-local URLs only for a
  trusted local MCP client; see the MCP reference for that trust model.

## Find novels by keyword/tag

```
pixiv search "初音ミク" --type novel --limit 10 --json
```

- The canonical route uses `--type novel` and supports only
  `--search-by tag-partial|tag-exact|title-caption`, `--sort
  date_desc|date_asc`, and `--period day|week|month`. The legacy
  `pixiv novel search` route remains a compatibility route with the same basic
  fields.
- `--rating`, `--min-text-length`, `--max-text-length`, and `--original-only`
  are not part of the v1 novel-search contract. Do not send them or treat them
  as local filters.
- Novel search uses App authentication and follows the same logical
  `--limit`/`--page` rules as other paged lists. Never infer a request cap.
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
pixiv user novels 11 --limit 20
pixiv user bookmarks 11 --tag "初音ミク" --limit 20
pixiv user following 11 --limit 20
pixiv user followers 11 --limit 20
pixiv user related 11 --limit 20
pixiv user blocked 11 --limit 20
```

To discover a user by name instead of starting from an artwork, use:

```
pixiv user search "NAME" --limit 20 --json
```

- User search uses the authenticated App user-search operation. This v1 branch
  has no anonymous or `related_illust_authors` fallback; an account or an
  explicit authentication error is required.

- `user detail USER_ID` requires the ID (no self-default). `--json` gives the
  full stable profile envelope.
- `user artworks` / `bookmarks` / `following` default to the current account
  when USER_ID is omitted.
- `user detail` accepts only the ID. If the user gives a `pixiv.net/users/<id>`
  URL, extract its numeric ID. For a name, use `user search` while
  authenticated. Do not substitute an artwork search or label its authors as a
  username-search result.

## Rankings and recommendations

```
pixiv ranking --mode day --limit 10
pixiv ranking --mode week --date 2026-07-01 --limit 10
pixiv ranking --mode week_r18 --limit 10
pixiv recommended --type artwork --limit 10
pixiv recommended --type all --limit 5
```

- `ranking` supports `day`, `day_male`, `day_female`, `week`, `week_original`,
  `week_rookie`, `month`, `day_manga`, `week_manga`, `month_manga`,
  `week_rookie_manga`, `day_r18`, `day_male_r18`, `day_female_r18`,
  `week_r18`, and `week_r18g`. The final nine require authentication; never
  substitute a failed extended mode with `day`.
- `recommended` always needs authentication and a kind. Use the typed
  `--type` flag; for `all`, inspect the actual output shape and keep the
  returned categories separate rather than assuming one flat list.

## Curate: bookmarks and follows (write ops)

```
pixiv bookmark add 129543211
pixiv follow add 11
pixiv bookmark list --type artwork --limit 20
pixiv bookmark tags --limit 20
pixiv bookmark detail 129543211 --json
```

State the target ID in one line before executing (SKILL.md operation tiers).
`remove` variants are symmetrical. These need authentication; on an anonymous
session report that a login is required instead of attempting fallback.
