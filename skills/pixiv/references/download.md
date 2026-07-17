# Download workflows

Downloads write to disk — always run the checklist first.

## Pre-download checklist

1. `pixiv config get download_path` — confirm the target directory with the
   user (default `./downloads`, resolved against the *current working
   directory*, so state your cwd when confirming).
2. Confirm the item count. For multi-ID or large batches, restate the list
   before running.
3. Only override with `--download-path DIR` / `--filename-template T` when the
   user asked for a specific location or naming; these flags never persist.

## Single and multi-page artworks

```
pixiv download 129543211
pixiv download 129543211 130000001 130000002
```

- Multi-page works: every page is downloaded automatically (`_p0`, `_p1`, ...).
- Filename template default: `{author} - {title}_{id}`. Placeholders: `{id}`,
  `{title}`, `{author}`, `{author_id}`. Persist a new default with
  `pixiv config set filename_template "..."` (confirm first — config write).

## Ugoira (animated works)

- The CLI downloads the frame zip and encodes GIF + APNG with a built-in Rust
  encoder. No ffmpeg or any external tool is needed.
- Encoding large ugoira takes noticeable time. Do NOT impose your own timeout
  or kill the process because it "seems slow" — wait for completion or a real
  error.

## Batch from a search/user listing

Chain source-side, don't scrape human output:

```
pixiv user artworks 11 --limit 5 --json > "$TMPDIR/works.json"
# extract ids (jq if available, otherwise Grep/Read on the temp file)
pixiv download <id...>
```

Confirm the final ID list and count with the user before the `download` step.

## Reporting results

- Per-ID outcomes: report which IDs succeeded and which failed, with the real
  error for each failure. Never summarize failures away as "done".
- Anonymous sessions can download public works via web fallback; restricted or
  R-18 works may fail — surface the real API error, don't retry blindly.
