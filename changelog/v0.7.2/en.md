# v0.7.2 — 2026-07-25

## Fixed

- Make automatic SkillHub publication work for recovered Releases: after the GitHub Release and Homebrew deployment succeed, the downstream workflow receives and revalidates the exact immutable release tag instead of treating its `main` head branch as a version.
- Publish the updated `pixiv-cli` product skill as independent SkillHub version 0.7.1. Its version changes because the Skill content changed, not merely because the CLI has a new release.
