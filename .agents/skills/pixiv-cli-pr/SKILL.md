---
name: pixiv-cli-pr
description: Prepare, update, or verify a pixiv-cli pull request using its current template, trusted verification policy, reviewed diff, and head-specific check results. Use for contributor handoff or PR readiness; merging, live /test execution, and releases are separate actions.
---

# Prepare a pixiv-cli PR

## Verify scope

Read `AGENTS.md`, the request/issue, branch status, and the diff against the intended base. Preserve unrelated changes and verify the remote repository before pushing. Use [the review workflow](../pixiv-cli-review/SKILL.md) and [test workflow](../pixiv-cli-test/SKILL.md); record actual outcomes and blockers.

Before staging and again before publishing, inspect `git diff --name-status BASE...HEAD`, `git diff --cached --name-status`, and any unstaged/untracked files, substituting the verified base. Every included path must implement the requested result or maintain a directly affected contract. Stage explicit paths rather than the whole workspace. Leave working notes, generated evidence, exploratory scripts, draft proposals, and unrelated cleanup out of the commit; keep required durable documentation in its established owner. A planning document belongs only when the requested deliverable is that document, not merely because it helped implementation.

Apply [test admission](../pixiv-cli-test/SKILL.md#decide-whether-test-code-must-change) to every test/fixture change. Temporary `tmp/test/` evidence stays local; describe actual verification in the PR instead of committing the probe. Review the final diff for accidental files even when ignore rules hide them from ordinary status.

## Write the body

Read `.github/PULL_REQUEST_TEMPLATE.md` from the current target branch. Copy its literal headings and required checklist; the metadata parser currently recognizes their bilingual spelling, so do not translate or invent headings. Write concise English content under them, or the language explicitly requested for the PR.

State actual changes and why. List commands that ran and their outcomes; separate unrun/native/live checks. Check a required item only when true, and explain an unmet condition rather than checking it to satisfy automation. Do not invent release-note fields, version decisions, historical problems, or unchanged non-features.

Use an ordinary code fence for evidence commands. A `commands` fence is a declaration for on-demand verification, not a transcript: include it only when useful and authorized. Inspect `scripts/pr/command-whitelist.txt`; commands and controlled pipelines are parsed as data, not executed by a shell. Never put secrets, private targets, or state-changing account commands in a public PR.

Validate a prepared non-secret body file from the repository root:

```bash
go run ./scripts/prmeta --body-file BODY_PATH --cli pixiv
```

Replace `BODY_PATH` with the actual file. This checks local metadata/declared-command syntax, not remote trust, API success, or the outcome of a `/test` run. Keep draft files outside tracked sources unless they are requested artifacts.

## Publish and inspect

When authorized, push the dedicated branch and create/update the PR against the verified base. Do not force-push, merge, change protections, or release implicitly. Read checks and review threads on the **current head SHA**, not an earlier green revision. Use [the CI workflow](../pixiv-cli-ci/SKILL.md) for failures.

Include the head's commit statuses as well as check runs. Platform/Container worker runs execute under the trusted base ref; follow the smoke Check Run details URL instead of treating the absence of a PR-head workflow run as missing smoke evidence. Keep skipped checks distinct from executed passes.

`/test` can make real service requests; posting it is a separate action with its own scope. A repeated request may reuse or supersede evidence according to the current workflow; inspect its actual dispatch/result rather than promising a new run from the comment alone.

Deliver the PR URL, commit, actual validation, and pending blockers. Distinguish a review-ready draft from a merge-ready PR; do not mark required checks successful because their workflow exists.
