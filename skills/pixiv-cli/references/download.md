# Download workflows

Downloads write to disk — always run the checklist first.

## Pre-download checklist

1. Run `pixiv config get download_path` and confirm the target directory with
   the user. The default `./downloads` is resolved against the current working
   directory, so state that directory when confirming it.
2. Confirm the exact targets and item scope immediately before each
   `pixiv download` invocation. A user URL means every visual work in the
   selected upstream listing; approval is single-use and never carries over to
   another download command.
3. Use only the current download options: `--download-path` or its `--output`
   / `-o` alias, `--filename-template`, `--pages`, `--quality`,
   `--ugoira-mode`, `--on-error`, and the command-scoped proxy flags. Unknown
   options fail locally; the v1 CLI does not publish archive, metadata,
   concurrency, retry, or directory-template download flags.

## Single and multi-page artworks

```
pixiv download 129543211
pixiv download 129543211 130000001 130000002
pixiv download https://www.pixiv.net/artworks/129543211
pixiv download 129543211 --pages 1,3-5,8- --quality regular
pixiv download 129543211 --ugoira-mode apng --output ./downloads
```

- Multi-page works download every page by default (`_p0`, `_p1`, ...).
- `--pages` is 1-based. It accepts comma-separated numbers, closed ranges such
  as `1,3-5`, and open ranges such as `8-`; selected pages are de-duplicated in
  natural order. A missing selected page is an explicit error rather than a
  silent skip.
- `--quality` accepts `original` (default), `regular`, `small`, `thumb`, and
  `mini` for static images. Invalid values fail before the download request.
- `--ugoira-mode` accepts `gif` (default) or `apng`. Other containers are not
  part of the current CLI contract and fail validation.
- `--output DIR` and `--download-path DIR` name the same destination. If both
  are provided, they must be identical. `--filename-template` accepts the
  placeholders shown by `pixiv download --help`: `{id}`, `{title}`, `{author}`,
  `{author_id}`, `{date}`, `{tags}`, and `{num}`.

## Direct Pixiv URLs and whole-user downloads

`download` accepts an artwork ID, an official artwork page URL, a user URL, a
user artwork-list URL, a public user-artwork-bookmarks URL, or a
resource-policy-allowed CDN URL:

```
pixiv download https://www.pixiv.net/users/12345678
pixiv download https://www.pixiv.net/en/users/12345678/artworks
pixiv download https://www.pixiv.net/users/12345678/bookmarks/artworks
```

- A user URL and its `/artworks` form use App OAuth to traverse every `illust`,
  `manga`, and `ugoira` returned by the upstream listing. Novels are outside
  this download set; the CLI adds no implicit count, page, or timeout limit.
- The bookmarks URL expands public artwork bookmarks. It does not download
  novel bookmarks or claim that the returned works are a complete private
  bookmark view.
- Only the listed `pixiv.net` / `www.pixiv.net` HTTPS page paths are accepted.
  Novel pages, artwork-series pages, FANBOX, Pixivision, Sketch, short links,
  other hosts, and other paths fail before the downloader opens them. A direct
  CDN source is accepted only when the SDK resource policy validates it.
- Artwork IDs expanded from user or bookmark URLs are de-duplicated by first
  appearance within the invocation. There is no CLI archive option for
  cross-run de-duplication.

## Animated works

Animated downloads may take noticeable time. Do not impose an arbitrary timeout
or kill the process merely because it is slow — wait for completion, user
cancellation, or a real error.

## Batch from a search or listing

Use the shared NDJSON record protocol rather than text parsing or a temporary
JSON collector merely to make a pipeline convenient:

```bash
pixiv search "landscape" --type artwork --limit 20 --ndjson \
  | pixiv download --ugoira-mode apng --on-error fail-fast
```

Before the final command, inspect the selected records and state the exact
record type, IDs/count, and destination to the user. `download` consumes every
visual `illust`/`manga`/`ugoira` record from stdin; it does not download novel or
user records. An incompatible record is a visible diagnostic. Use
`--on-error fail-fast` when the user wants the first invalid record to stop the
pipeline; keep the default `skip` only when the user accepts continuing over
invalid records.

## Reporting results

- Downloads are actions: successful CLI stdout is empty. Never parse a download
  report from stdout. Inspect the requested local directory to report produced
  files; errors are safe stderr diagnostics and make the command non-zero.
- Report which references succeeded and which failed. Never summarize failures
  away as “done”. Cancellation stops immediately.
- Downloads require an authenticated local account. Restricted or R-18 works
  may fail; surface the actual API error and do not add a Cookie workaround or
  silently switch to a Web path.
