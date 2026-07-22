# Download workflows

Downloads write to disk — always run the checklist first.

## Pre-download checklist

1. `pixiv config get download_path` — confirm the target directory with the
   user (default `./downloads`, resolved against the *current working
   directory*, so state your cwd when confirming).
2. Confirm the exact ID list and item count immediately before each
   `pixiv download` invocation. Approval is single-use and never carries over
   to another download command.
3. Only override with `--download-path DIR` / `--filename-template T` when the
   user asked for a specific location or naming; these flags never persist.

## Single and multi-page artworks

```
pixiv download 129543211
pixiv download 129543211 130000001 130000002
pixiv download 129543211 --pages 1,3-5 --quality regular
```

- Multi-page works: every page is downloaded by default (`_p0`, `_p1`, ...).
- `--pages` is 1-based (`1,3-5` closed ranges, de-duplicated, natural order).
  A missing selected page fails explicitly rather than silently skipping.
- `--quality` for static images: `original` (default), `regular` (longest side
  1200), `small` (540), `thumb` (250×250 center crop), `mini` (48×48 center
  crop). Preserve the upstream JPEG/PNG format and alpha channel.
- Ugoira keeps the GIF/APNG flow; page selection or a non-original quality
  returns unsupported. With authentication, Pixiv may expose only a verified
  medium ZIP; this is still the legitimate download resource and must never be
  described as original. Do not add a Web/Cookie workaround to obtain another
  variant.
- Filename template default: `{author} - {title}_{id}`. Placeholders: `{id}`,
  `{title}`, `{author}`, `{author_id}`. Persist a new default with
  `pixiv config set filename_template "..."` (confirm first — config write).

## Animated works

- Animated downloads may take noticeable time. Do NOT impose your own timeout
  or kill the process because it "seems slow" — wait for completion, user
  cancellation, or a real error.

## Batch from a search/user listing

Chain from JSON rather than scraping human output:

1. Create an environment-safe, uniquely named temporary file through the
   agent runtime's temp-file facility or `mktemp`; do not construct a
   predictable pathname or place it in a project/root directory.
2. Redirect one bounded listing command's JSON stdout to that file, then check
   its exit status before reading or parsing it. On failure, report stderr and
   do not continue with partial or empty data.
3. Inspect the successful response's actual JSON shape before choosing a
   selector. Extract `id` only from the array/object that demonstrably contains
   artwork records. Never recursively collect every field named `id`, because
   author, user, and other nested records can also have IDs.
4. Validate artwork IDs, deduplicate them while preserving order, then show the
   exact list and count to the user. Run `pixiv download ...` only after a new,
   explicit confirmation for that invocation.
5. Remove the temporary file after parsing or on failure.

## Reporting results

- Per-ID outcomes: report which IDs succeeded and which failed, with the real
  error for each failure. Never summarize failures away as "done".
- Anonymous sessions can download public works via web fallback; restricted or
  R-18 works may fail — surface the real API error, don't retry blindly.
