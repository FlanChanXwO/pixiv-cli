# v0.7.1 — 2026-07-25

## 变更

- 插画收藏数筛选现在会预检已保存账号缓存的 Pixiv 高级会员状态（默认 24 小时）；非会员在本地被拒绝，避免 Pixiv 静默忽略筛选边界。可在 `config.toml` 设置 `[premium] status_cache_ttl`，或执行 `pixiv auth refresh [UID] [--all]` 强制刷新 OAuth 凭据和会员缓存。
- 简化账号文本反馈：登录成功输出紧凑安全账号摘要 `✓ uid:UID username:NAME`，`auth list` 用 `*` 与 `✓`/`-` 本地状态符号替代 `token:yes/no`。
- 首次普通 CLI 命令会生成缺失的 `config.toml`，其中只含常用设置；高级设置保持省略直到显式配置，且绝不覆盖已有文件。
- 本地按日操作日志改为紧凑的 Spring/SLF4J 风格，包含时间、级别、PID、业务组件、仓库相对调用点和操作；空字段及仅表示本地的 backend/status 会省略。

## 修复

- 修复 macOS 浏览器 OAuth callback：临时 `pixiv://` helper 现在读取与活动 CLI loopback listener 相同的私有端点文件；下次登录会自动重编译既有 helper。
- 自动 SkillHub 发布现在由成功结束的 Release workflow 触发，而非 GitHub Release event；后者由 `github.token` 创建时不会递归触发 Actions。
- 确认 SkillHub 社区提交时现在识别其 `reviewStatus` 返回字段，避免已成功发布被误报为失败。
- 若 `skills/pixiv-cli/` 与上一个 release tag 相比没有变化，则跳过 SkillHub 提交；Skill 的 frontmatter 版本独立于 CLI Release 版本，仅在 Skill 实际变化时递增。
