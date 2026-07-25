# v0.7.2 — 2026-07-25

## 修复

- 修复恢复发布后的自动 SkillHub 提交：GitHub Release 和 Homebrew 部署均成功后，下游 workflow 会接收并重新验证精确的不可变 release tag，不再将其 `main` head branch 当作版本。
- 将更新后的 `pixiv-cli` 产品 Skill 作为独立的 SkillHub 版本 0.7.1 提交。该版本只因 Skill 内容变化而递增，不会仅跟随 CLI 发版而变化。
