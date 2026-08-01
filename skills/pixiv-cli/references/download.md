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
3. Only override with `--download-path DIR` / `--filename-template T` /
   `--directory-template T` / `--concurrency N` when the
   user asked for a specific location or naming; these flags never persist.

## Single and multi-page artworks

```
pixiv download 129543211
pixiv download 129543211 130000001 130000002
pixiv download https://www.pixiv.net/artworks/129543211
pixiv download 129543211 --pages 1,3-5,8- --quality regular
pixiv download 129543211 --concurrency 8
pixiv download 129543211 --archive downloaded.sqlite --write-metadata
```

- Multi-page works: every page is downloaded by default (`_p0`, `_p1`, ...).
- `--pages` is 1-based (`1,3-5` closed ranges and `8-` open ranges,
  de-duplicated, natural order).
  A missing selected page fails explicitly rather than silently skipping.
- `--quality` for static images: `original` (default), `regular` (longest side
  1200), `small` (540), `thumb` (250×250 center crop), `mini` (48×48 center
  crop). Preserve the upstream JPEG/PNG format and alpha channel.
- `--ugoira-mode` accepts `gif` (default), `apng`, `zip`, or `frames`. `zip`
  keeps the verified source ZIP; `frames` writes a frame directory and timing
  manifest. Page selection or a non-original quality returns unsupported.
  With authentication, Pixiv may expose only a verified
  medium ZIP; this is still the legitimate download resource and must be
  described with its verified quality.
- Filename template default: `{author} - {title}_{id}`. Filename and relative
  directory templates support `{id}`, `{title}`, `{author}`, `{author_id}`,
  `{date}`, `{tags}`, and `{num}`; `{num}` is 0-based and suppresses the
  default multi-page suffix. Persist a new default with
  `pixiv config set filename_template "..."` (confirm first — config write).
- `--archive FILE` stores an artwork ID in a SQLite archive only after all
  requested outputs and sidecars succeed. `--write-metadata` atomically writes
  one JSON sidecar per artifact, containing public artwork metadata and its
  relative output path. Neither option writes credentials.
- Resource downloads retry retryable failures three times by default, with
  1/2/4-second backoff. `--retries` and `--retry-delay` explicitly override
  this policy; a valid upstream `Retry-After` takes precedence.

## Direct Pixiv URLs and whole-user downloads

`detail` accepts an artwork ID or only a current official artwork page URL:
`https://pixiv.net/artworks/{id}` or `https://www.pixiv.net/artworks/{id}`.
`download` accepts those plus resource-policy-allowed CDN URLs,
`https://pixiv.net/users/{id}`, `https://pixiv.net/users/{id}/artworks`,
`https://pixiv.net/users/{id}/bookmarks/artworks`, and
`https://pixiv.net/user/{id}/series/{series_id}` (the `www` host and an optional
locale, query, or fragment are also valid).

```
pixiv download https://www.pixiv.net/users/12345678
pixiv download https://www.pixiv.net/en/users/12345678/artworks
pixiv download https://www.pixiv.net/users/12345678/bookmarks/artworks
pixiv download https://www.pixiv.net/user/12345678/series/42
```

- A user URL uses App OAuth to walk every `illust`, `manga`, and `ugoira` in
  upstream page order; novels are outside the download set, with no implicit
  count, page, retry, or timeout limit.
- Only the listed `pixiv.net` / `www.pixiv.net` HTTPS paths are accepted. Short
  links, old URL shapes, novels, FANBOX, Pixivision, Sketch, other hosts, and
  other paths fail locally before the SDK or downloader opens.
- Downloads preserve CLI argument order and de-duplicate expanded artwork IDs
  by first appearance within one invocation. Use `--archive FILE` for explicit
  cross-run de-duplication. They persist ETag/Last-Modified metadata and resumable partials in
  `.pixiv-cache`; only an `If-Range` validator match may resume a partial file.

## Animated works

- Animated downloads may take noticeable time. Do NOT impose your own timeout
  or kill the process because it "seems slow" — wait for completion, user
  cancellation, or a real error.

## Batch from a search/user listing

Use the shared NDJSON record protocol rather than text parsing or a temporary
JSON collector merely to make a pipeline convenient:

```bash
pixiv search "landscape" --limit 20 \
  --filter 'bookmarkCount >= 1000 and xRestrict == 0' \
  | pixiv download --ugoira-mode apng
```

Before the final command, inspect the selected records and state the exact
record type, IDs/count, and destination to the user. `download` consumes every
visual `illust`/`manga`/`ugoira` record from stdin; it does not download novel
or user records. An incompatible record is a visible stderr diagnostic. Use
`--on-error fail-fast` to stop at the first bad record, or the default `skip`
only when the user explicitly accepts continuing over invalid records.

`--filter EXPR` is compiled before network or file work. It supports public
illustration fields such as `bookmarkCount`, `viewCount`, `xRestrict`, `tags`,
and `tools`; use `any(tags, # in ["A", "B"])` or
`all(tools, # in ["Photoshop"])` for collections. Ordinary list flags and the
expression are combined with AND. A CDN URL has no illustration metadata, so
it rejects `--filter` rather than guessing.

## Reporting results

- Downloads are actions: successful CLI stdout is empty. Never parse a
  download report from stdout. Inspect the requested local directory to report
  produced files; errors are safe stderr diagnostics and make the command
  non-zero. Cancellation stops immediately.
- Per-target outcomes: report which references succeeded and which failed.
  Never summarize failures away as "done".
- Anonymous sessions can download public works via web fallback; restricted or
  R-18 works may fail — surface the real API error, don't retry blindly.
