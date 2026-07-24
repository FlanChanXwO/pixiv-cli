# 未发布

## 修复

- 自动 SkillHub 发布现在由成功结束的 Release workflow 触发，而非 GitHub Release event；后者由 `github.token` 创建时不会递归触发 Actions。
- 确认 SkillHub 社区提交时现在识别其 `reviewStatus` 返回字段，避免已成功发布被误报为失败。
