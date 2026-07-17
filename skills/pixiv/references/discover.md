# Discovery playbooks

Run the preflight in `../SKILL.md` and verify flags with
`pixiv <command> --help`.

## Search and filter works

```text
pixiv search "初音ミク" --limit 10 --json
pixiv search "初音ミク" --type illust-and-ugoira --ai-mode exclude
pixiv search "背景" --resolution high --aspect-ratio landscape
pixiv search-options "背景" --json
```

- Prefer App-side filters: content type, resolution, aspect ratio, tool, and
  AI exclusion. Rating and AI-only are applied by the public SDK over App
  results; pagination continues when a positive limit needs more matches.
- Tool names are dynamic. Query `search-options WORD` while authenticated,
  then pass the returned value exactly to `--tool`.
- `--rating r18|r18g|mature` requires App authentication. Do not attempt a Web
  workaround or add a Cookie. `--rating all` in anonymous mode only means the
  content visible to that anonymous session.
- `--ai-mode only` checks `AIType==2`; `exclude` excludes that value.
- Bookmark-count filtering is unavailable.

## Inspect a search result

Extract an `id` from JSON, then request stable detail instead of re-searching:

```text
pixiv detail 129543211 --json
```

Artwork models include dimensions, rating/AI metadata, author, tags, and the
App-provided `tools` list when present.

## Explore an artist and recommendations

```text
pixiv user detail 11 --json
pixiv user artworks 11 --limit 20
pixiv recommended all --limit 5
```

User and personalized recommendation operations require authentication.
`recommended all` returns four independently paginated streams: illustration,
manga, novel, and user.
