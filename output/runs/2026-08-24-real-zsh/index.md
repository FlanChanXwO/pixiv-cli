# Pixiv CLI real-zsh E2E run

- Run ID: `2026-08-24-real-zsh`
- Source worktree: `/private/tmp/pixiv-cli-pixiv-e2e-fixes`
- Shell workspace: `/private/tmp/pixiv-cli-e2e-shell-20260821`
- Scope: Pixiv CLI only; FANBOX and account-mutating operations are excluded.
- Evidence contract: one logical Pixiv CLI invocation per case directory, with `report.md`, raw `stdout.txt`, and raw `stderr.txt`.

## Status

Task 14 establishes and proves the evidence runner with a non-sensitive `pixiv --version` smoke case. Task 15 will populate the complete read-only, authentication-diagnostic, download, error-path, and targeted repair coverage.

## Cases

| Group | Case | Result | Purpose |
| --- | --- | --- | --- |
| baseline | `version` | PASS | Prove direct execution of the built Pixiv CLI from the independent shell workspace and separate raw stdout/stderr capture. |
