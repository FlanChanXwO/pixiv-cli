# v0.4.5 — 2026-07-20

## Fixed

- Linux Homebrew hosted staging verification now runs a real local staging-tap `brew install` inside a short-lived, digest-pinned `homebrew/brew` container. It avoids the hosted Linuxbrew `Resource` cleanup `EINVAL` while keeping a read-only formula mount, no secrets, a local-only tap, and an exact installed-version check. The container is discarded with `--rm`; macOS and end-user Homebrew installs are unchanged. The GitHub prepublish workflow passed this check on all four Homebrew platform/architecture combinations using published v0.4.4 assets.
