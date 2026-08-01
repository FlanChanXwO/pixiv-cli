# Unreleased

> Release-prep staging area. Feature PRs provide their category and summary in the PR template; the approved release-prep plan groups those sources into the next versioned notes.

## Breaking changes

- Removed the standalone `pixiv filter` command. Apply `--filter EXPR` directly to visual list commands or `pixiv download`.
- Replaced CLI `--ugoira-format` with `--ugoira-mode gif|apng|zip|frames`.

## Added

- Added safe typed illustration expressions, automatic NDJSON for piped visual lists, download archives, metadata sidecars, directory templates, retry controls, request pacing, Ugoira ZIP/frames output, open-ended page ranges, public-bookmark and illustration-series download sources, and SOCKS proxy URIs.
