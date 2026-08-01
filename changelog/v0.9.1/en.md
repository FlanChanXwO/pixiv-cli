# v0.9.1 — 2026-08-01

## Fixed

- Restore release validation for historical commits explicitly attributed by the audit, so valid release preparation is not rejected. ([#42](https://github.com/FlanChanXwO/pixiv-cli/pull/42))
- Require the product Skill version to match the CLI release before protected release jobs run, so the matching SkillHub and ClawHub publication can proceed. ([#44](https://github.com/FlanChanXwO/pixiv-cli/pull/44))

## Maintenance

- Restore the audited workflow-dispatch recovery path used to verify release-notes policy for immutable release tags. ([#43](https://github.com/FlanChanXwO/pixiv-cli/pull/43))

**Full Changelog**: [v0.9.0...v0.9.1](https://github.com/FlanChanXwO/pixiv-cli/compare/v0.9.0...v0.9.1)
