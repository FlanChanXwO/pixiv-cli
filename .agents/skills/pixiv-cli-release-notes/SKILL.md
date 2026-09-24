---
name: pixiv-cli-release-notes
description: Prepare pixiv-cli bilingual versioned release notes, audit source coverage, coordinate an authorized version release, or recover its independent publishers. Use for release preparation and historical Release synchronization, not routine feature PR metadata.
---

# Prepare a pixiv-cli Release

Read `AGENTS.md`, current `release.yml` and the relevant publisher YAML, and the release sections of `docs/en/maintainers/development.md`. Executable workflows own current inputs and ordering. SemVer advice is not authorization to create a release-prep PR, merge, tag, publish, or rewrite history.

## Prepare an approved version

1. Confirm the version, prior tag, target commit/range, and allowed actions. Inspect current tags/releases rather than guessing the next version.
2. Use `scripts/cmd/releasenotes audit` to collect merged PRs, direct commits, authors, and first-time contributors for the exact range. Keep its non-secret audit output in a temporary location, not committed source. Read the tool's current help for arguments.
3. Write `changelog/vX.Y.Z/en.md` and `zh-CN.md` directly in the release-prep change. Use the documented non-empty section order, matching source sets, inline PR/commit links, contributors, and compare footer. Pixiv requires coverage of every in-range PR/direct commit; internal work belongs in `Maintenance`.
4. Update the changelog navigation and release metadata that actually changes. Preserve `skills/pixiv-cli/SKILL.md` publisher fields and align its version with the release contract. Do not publish maintenance skills as the product skill, or change old release notes as unrelated cleanup.
5. Run the release-note tool's validation with the same audit plus the relevant local release/test checks. After release-prep merge, re-audit and validate the exact final commit before creating the authorized tag. Missing attribution or an API failure blocks that step.

Ordinary PRs have changes, verification, and a checklist; no release classification fields or automatically generated changelog fragment are required. `changelog/unreleased/` is not a mandatory feature-PR editing target.

## Release and publishers

`release.yml` is tag-push only. Validate, tests, native production archives, and verified containers are tied to the immutable source. Prepared artifacts precede `release-approval`; publisher credentials are scoped separately. Do not dispatch a nonexistent manual release input or replace source under an existing tag.

GitHub Release, GHCR, Homebrew, Docker Hub, SkillHub, and ClawHub are not one atomic transaction. Read the current graph: independent publishers consume the completed Release handoff and original artifact identity. Manual publisher recovery uses `release_run_id`. Inspect their results independently; one successful channel does not prove another succeeded. Use ClawHub `verify_only` only for its documented non-republishing check.

Verify public Release notes/assets, signatures/checksums and channel outcomes that the authorized release requires. Real SDK, native-library, browser-host, and installer evidence must be obtained through their actual workflows; mocks and local workflow tests cannot replace them. Do not add extra approval stages or bypass the existing single final approval.

## History and handoff

Run history synchronization as a dry-run for explicitly identified versions first. `sync-history --apply` requires explicit approval; it does not authorize new tags or replacement assets. Confirm the resulting remote body against local bilingual rendering.

Report the exact version, tag/commit, Release run, per-channel state, and outstanding evidence. Never print signing keys, deploy keys, account credentials, or private logs. Leave incomplete publication clearly incomplete.
