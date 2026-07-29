# Download workflows

Downloads write to disk — always run the checklist first.

## Pre-download checklist

1. `pixiv config get download_path` — confirm the target directory with the
   user (default `./downloads`, resolved against the *current working
   directory*, so state your cwd when confirming).
2. Confirm the exact targets and item scope immediately before each
   `pixiv download` invocation. A user URL means every visual work by that
   user, with no implicit limit; approval is single-use and never carries over
   to another download command.
3. Only override with `--download-path DIR` / `--filename-template T` / `--concurrency N` when the
   user asked for a specific location or naming; these flags never persist.

## Single and multi-page artworks

```
pixiv download 129543211
pixiv download 129543211 130000001 130000002
pixiv download https://www.pixiv.net/artworks/129543211
pixiv download 129543211 --pages 1,3-5 --quality regular
pixiv download 129543211 --concurrency 8
```

- Multi-page works: every page is downloaded by default (`_p0`, `_p1`, ...).
- `--pages` is 1-based (`1,3-5` closed ranges, de-duplicated, natural order).
  A missing selected page fails explicitly rather than silently skipping.
- `--quality` for static images: `original` (default), `regular` (longest side
  1200), `small` (540), `thumb` (250×250 center crop), `mini` (48×48 center
  crop). Preserve the upstream JPEG/PNG format and alpha channel.
- `--ugoira-format` accepts `gif` (default) or `apng`; it controls only Ugoira
  conversion. Page selection or a non-original quality returns unsupported.
  With authentication, Pixiv may expose only a verified
  medium ZIP; this is still the legitimate download resource and must never be
  described as original. Do not add a Web/Cookie workaround to obtain another
  variant.
- Filename template default: `{author} - {title}_{id}`. Placeholders: `{id}`,
  `{title}`, `{author}`, `{author_id}`. Persist a new default with
  `pixiv config set filename_template "..."` (confirm first — config write).

## Direct Pixiv URLs and whole-user downloads

`detail` accepts an artwork ID or only a current official artwork page URL:
`https://pixiv.net/artworks/{id}` or `https://www.pixiv.net/artworks/{id}`.
`download` accepts those plus resource-policy-allowed CDN URLs, `https://pixiv.net/users/{id}` and
`https://pixiv.net/users/{id}/artworks` (the `www` host and an optional locale,
query, or fragment are also valid).

```
pixiv download https://www.pixiv.net/users/12345678
pixiv download https://www.pixiv.net/en/users/12345678/artworks
```

- A user URL walks every `illust`, `manga`, and `ugoira` in upstream page order;
  it does not download novels and does not add an implicit count, page, retry, or
  timeout limit. It requires App OAuth; never add a Cookie, WebView, anonymous
  fallback, redirect, or HTML-scraping workaround.
- Only the listed `pixiv.net` / `www.pixiv.net` HTTPS paths are accepted. Short
  links, old URL shapes, novels, FANBOX, Pixivision, Sketch, other hosts, and
  other paths fail locally before the SDK or downloader opens.
- Downloads preserve CLI argument order. They do not create a database, history,
  or cross-run de-duplication; repeated inputs are intentionally processed again.
  They do persist ETag/Last-Modified metadata and resumable partials in
  `.pixiv-cache`; only an `If-Range` validator match may resume a partial file.

## Animated works

- Animated downloads may take noticeable time. Do NOT impose your own timeout
  or kill the process because it "seems slow" — wait for completion, user
  cancellation, or a real error.

## Batch from a search/user listing

Use the shared NDJSON record protocol; never scrape human output or add a
temporary JSON collector merely to make a pipeline convenient:

```bash
pixiv search "landscape" --ndjson --limit 20 \
  | pixiv filter --min-bookmarks 1000 \
  | pixiv download --ugoira-format apng
```

Before the final command, inspect the selected records and state the exact
record type, IDs/count, and destination to the user. `download` consumes every
visual `illust`/`manga`/`ugoira` record from stdin; it does not download novel
or user records. An incompatible record is a visible stderr diagnostic. Use
`--on-error fail-fast` to stop at the first bad record, or the default `skip`
only when the user explicitly accepts continuing over invalid records.

## Reporting results

- Downloads are actions: successful CLI stdout is empty. Never parse a
  download report from stdout. Inspect the requested local directory to report
  produced files; errors are safe stderr diagnostics and make the command
  non-zero. Cancellation stops immediately.
- Per-target outcomes: report which references succeeded and which failed.
  Never summarize failures away as "done".
- Anonymous sessions can download public works via web fallback; restricted or
  R-18 works may fail — surface the real API error, don't retry blindly.
