---
name: pixiv-cli-ci
description: Diagnose pixiv-cli GitHub Actions failures and verify current-head quality, platform, PR-verification, native/browser, and release evidence. Use for CI readiness, failed or stalled runs, or an explicitly authorized rerun/cancel/dispatch; default to read-only evidence gathering.
---

# Diagnose pixiv-cli CI

Read `AGENTS.md`, the exact changed workflow, and its invoked script/tool. Workflow YAML owns ordering, inputs, permissions, and conditions; `ci/platforms.json` plus `tools/platformmatrix` own platform selection. Do not infer executable policy from a historical plan or a skill's old command example.

## Identify the failure

Use the connected GitHub tools or an already available authenticated `gh`. Establish repository, PR/head SHA, workflow, run ID, attempt, event, job, and failing step. Read the first causal error and relevant surrounding log, not just the final failure line. Never display tokens, account stores, signed URLs, or raw secret-bearing responses.

Useful read-only commands, with verified placeholders, are:

```bash
gh pr checks PR_NUMBER
gh run view RUN_ID --json name,event,headBranch,headSha,status,conclusion,jobs,url
gh run view RUN_ID --log-failed
gh run view --job JOB_ID --log
```

Classify deterministic code/contract failure, missing environment, upstream/network failure, cancellation, or genuine infrastructure flakiness. A long-running job is not automatically hung. Gather bounded relevant output, but preserve complete logs where needed instead of truncating source evidence.

## Select the owning checks

Use [pixiv-cli-test](../pixiv-cli-test/SKILL.md) for local reproduction. `ci.yml`/`platform-smoke.yml` use the change classifier and aggregate gates; do not change classification or skip conditions to make this run green. Run relevant safely independent downstream phases if fail-fast hid their results.

`pr-metadata.yml` handles trusted metadata; `pr-verification.yml` handles explicit `/test` requests using trusted parser/runner code and an exact PR head. Keep write-capable control jobs separate from untrusted PR execution. Never execute a PR body or title as shell code, expose privileged credentials to it, or treat a returned artifact as trusted without the existing identity checks.

`native-evidence.yml` and `browser-evidence.yml` are credential-free evidence providers; synthetic browser data does not prove real user-profile access. `homebrew-prepublish-verify.yml` is a read-only rehearsal, not a deployment workflow. Native linking and real API tests remain different evidence classes.

## Operate only the authorized action

Before rerun, cancel, dispatch, or repair, state the exact run/workflow/target and why. A changed source or documented infrastructure hypothesis may justify a rerun; repeated deterministic failure does not. Preserve the original failure and attempt identity even after recovery succeeds.

`release.yml` is tag-triggered and has no manual dispatch input. Its verified immutable artifacts precede the single `release-approval` boundary. Independent Homebrew, container, SkillHub, and ClawHub publishers consume a completed Release handoff; `publish-dockerhub.yml` publishes both GHCR and Docker Hub. Manual publisher recovery uses the original `release_run_id`, not a guessed tag or current main. ClawHub also exposes `verify_only` for a non-republishing review check. Inspect live YAML before dispatching.

Do not move a tag, rebuild different bytes under an old release identity, bypass approval, invent a `deploy` input, or enable production secrets to rescue a check. Use [release preparation](../pixiv-cli-release-notes/SKILL.md) for release-specific work.

## Report

State the causal finding, exact run/attempt/SHA, evidence, relevant local results, and remaining checks. Distinguish API/tool access failure from workflow failure. Never call a skipped matrix, stale-head run, fixture, or pending publisher a completed release.
