---
name: pixiv-cli-release-notes
description: Manage pixiv-cli feature PR release-note metadata, bilingual release-prep notes, historical GitHub Release synchronization, and an approved version release. Use whenever work is moving from a completed feature branch toward a PR, merge, changelog, tag, GitHub Release, SkillHub, or ClawHub publication; use it even when the request only mentions “prepare a release” or “update release notes”.
---

# pixiv-cli release notes and release workflow

Use this Skill for the repository release lifecycle. Read `docs/maintainers/development.md` before acting; it is the durable policy reference.

## Authorization boundary

Offer a SemVer recommendation after reviewing the merged scope. A recommendation is information, not permission.

Before creating a release-prep PR, merging it, creating or pushing a tag, dispatching release publication, or running `sync-history --apply`, obtain one explicit message in the current session that names:

- the exact version;
- the intended commit/tag range or historical versions; and
- the expected impact, including breaking changes when applicable.

Record that authorization in the release-prep PR description or the task summary. A general request to publish, an issue, or another contributor's merge does not supply this confirmation.

## Feature PR contract

1. Inspect the affected CLI, MCP, SDK, configuration, docs, and product Skill surfaces.
2. Run the repo-local `pixiv-cli-review` Skill and focused tests. Repair release-note metadata, links, formatting, version metadata, or workflow-policy failures when the evidence is clear. Pause for a decision on behavior, security, or scope disputes.
3. Ensure the PR body contains exactly one declaration from `.github/PULL_REQUEST_TEMPLATE.md`:

   ```text
   <!-- release-note
   category: Added|Changed|Fixed|Security|Documentation|Maintenance|None
   breaking: true|false
   summary: One user-facing release summary.
   none_reason: Required only for None.
   -->
   ```

4. Confirm the required Quality check passes. Request a GitHub review only when the caller explicitly provides `--reviewer USER_OR_TEAM`; otherwise use the repository's required CI and any existing review requirements.
5. Create and monitor the feature PR, then merge only with the caller's approval and the repository gate satisfied.

Feature PRs carry the declaration and documentation changes. They leave `changelog/unreleased/` untouched; final bilingual prose belongs to the later release-prep PR.

## Prepare a release

1. Fetch tags and audit the candidate range:

   ```bash
   go run ./scripts/releasenotes audit \
     --repo FlanChanXwO/pixiv-cli \
     --from vPREVIOUS \
     --to COMMIT_OR_TAG \
     --output /tmp/pixiv-cli-release-audit.json
   ```

   Inspect direct-commit exceptions, missing declarations, first external contributors, and the recommended bump. Historical direct commits use their real commit links; never infer a PR number.

2. Present the proposed version, grouped user outcomes, source range, contributor list, and breaking-change assessment. Wait for the explicit authorization described above.
3. Create a reviewed JSON plan with bilingual text and source URLs. A source can appear once in a grouped outcome; all sources must be GitHub PR or commit links for this repository. Example:

   ```json
   {
     "entries": [
       {
         "category": "Added",
         "english": "Add APNG downloads.",
         "zh_cn": "新增 APNG 下载。",
         "sources": ["https://github.com/FlanChanXwO/pixiv-cli/pull/42"]
       }
     ]
   }
   ```

4. Preview the generated files, review them, then write the release-prep change explicitly:

   ```bash
   go run ./scripts/releasenotes prepare \
     --version X.Y.Z --previous vPREVIOUS --date YYYY-MM-DD \
     --plan /tmp/pixiv-cli-release-plan.json \
     --audit /tmp/pixiv-cli-release-audit.json

   go run ./scripts/releasenotes prepare \
     --version X.Y.Z --previous vPREVIOUS --date YYYY-MM-DD \
     --plan /tmp/pixiv-cli-release-plan.json \
     --audit /tmp/pixiv-cli-release-audit.json --apply
   ```

5. Run the offline validation and documentation tests, create the release-prep PR, and monitor required CI/reviews:

   ```bash
   go run ./scripts/releasenotes validate \
     --version X.Y.Z --dir changelog/vX.Y.Z --previous vPREVIOUS \
     --audit /tmp/pixiv-cli-release-audit.json
   go test ./scripts/documentation -count=1
   ```

## Tag and publication

After the authorized release-prep PR is merged, confirm the default branch contains its merge commit. Create and push only the authorized `vX.Y.Z` tag. Monitor the Release workflow through GitHub CLI until GitHub Release, assets, SkillHub, and ClawHub handoffs report their final status. Verify:

- the GitHub Release body renders the tag's English and Simplified Chinese notes through `scripts/releaseassets finalize`;
- every expected asset and checksum/signature artifact is present;
- SkillHub and ClawHub report the matching product-skill publication or a documented unchanged-skill skip.

Report links and exact evidence. A publication failure outside release-note metadata, mapping, format, version metadata, or workflow policy requires a pause with the failing evidence.

## Historical releases

Use a dry-run for each historical version first:

```bash
go run ./scripts/releasenotes sync-history \
  --repo FlanChanXwO/pixiv-cli --version X.Y.Z --dir changelog/vX.Y.Z
```

With the current-session authorization for the named historical versions, add `--apply`. Existing Releases receive an exact-body update only. A missing historical Release is created with its existing tag and no assets; verify the returned body equals the local render.
