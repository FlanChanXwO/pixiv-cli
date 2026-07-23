# v0.4.3 — 2026-07-19

## Fixed

- Linux Homebrew release validation no longer uses `/var/tmp`, avoiding `EINVAL` that could prevent the staging formula from installing. Linux uses a runner-private temporary directory; macOS and the public formula path are unchanged.
