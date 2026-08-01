# v0.9.1 — 2026-08-01

## 修复

- 恢复对审计中明确标注来源的历史提交的发布校验，避免有效的发布准备被错误拒绝。 ([#42](https://github.com/FlanChanXwO/pixiv-cli/pull/42))
- 在受保护的发布任务运行前校验产品 Skill 版本与 CLI release 一致，使对应版本能够发布到 SkillHub 和 ClawHub。 ([#44](https://github.com/FlanChanXwO/pixiv-cli/pull/44))

## 维护

- 恢复用于核验不可变 release tag 发布说明策略的、经审计的 workflow_dispatch 恢复路径。 ([#43](https://github.com/FlanChanXwO/pixiv-cli/pull/43))

**完整变更**：[v0.9.0...v0.9.1](https://github.com/FlanChanXwO/pixiv-cli/compare/v0.9.0...v0.9.1)
