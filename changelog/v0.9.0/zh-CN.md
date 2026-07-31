# v0.9.0 — 2026-07-31

## 破坏性变更

- 统一 v0.9 的命令与自动化能力面：使用 `pixiv timeline latest|following` 取代 `feed`；迁移到改名后的 MCP timeline 工具；SDK 绘图工具发现改用 `SupportedDrawingTools()`；移除 SDK 与项目日志和 MCP 账号/配置工具；远程登录改为一次性桌面 handoff，不再提供设备、配对和复制 callback 表单。本版本还新增 APNG ugoira 输出、交互式下载进度、更丰富的列表筛选、静态绘图工具目录、规范下载去重和更安全的账号池行为。 ([#40](https://github.com/FlanChanXwO/pixiv-cli/pull/40))

## 修复

- 让已发布的 ClawHub Skill 与 Homebrew Formula 部署可从审核、待展示卡片和不可用 beta Formula 状态中正常恢复。 ([`0915746`](https://github.com/FlanChanXwO/pixiv-cli/commit/09157462ab6e36da209eae6973a5fc60c7c1bb8a), [`2a6e9f2`](https://github.com/FlanChanXwO/pixiv-cli/commit/2a6e9f2c6f6967c8698928f2c37a9f8bc461f183), [`384a3e8`](https://github.com/FlanChanXwO/pixiv-cli/commit/384a3e825c5a966c524e5d974e7480c18610928a), [`3e67b28`](https://github.com/FlanChanXwO/pixiv-cli/commit/3e67b286e72c3052a86d3f14675b53c0b0ce7c48), [`73b30c2`](https://github.com/FlanChanXwO/pixiv-cli/commit/73b30c24dbee98bbcc7d8b82d1b2d679cde4519e), [`b09f161`](https://github.com/FlanChanXwO/pixiv-cli/commit/b09f16140600644b48d3393217d52398ff10c6f0))

## 文档

- 引入可审计的双语发布说明，完善 AI agent 安装与账号说明，并提供本地化贡献模板；同时区分 SkillHub 与 ClawHub 的安装路径。 ([`66d5fd7`](https://github.com/FlanChanXwO/pixiv-cli/commit/66d5fd705f1e5aebf6413b5ced270c40670a8527), [`92261fa`](https://github.com/FlanChanXwO/pixiv-cli/commit/92261faf3f2ca8fd7afde6776ea882d0686c2b48), [`ea02b29`](https://github.com/FlanChanXwO/pixiv-cli/commit/ea02b29fcf3c14f6c6edabb94aa92b3d09c4eefd), [#37](https://github.com/FlanChanXwO/pixiv-cli/pull/37), [#38](https://github.com/FlanChanXwO/pixiv-cli/pull/38))

## 维护

- 将打包二进制 smoke job 名称改为稳定文本，不再包含 GitHub 表达式占位符。 ([`b8daaa1`](https://github.com/FlanChanXwO/pixiv-cli/commit/b8daaa1c4de024d369410fcabf18041b631e4f11))

**完整变更**：[v0.8.0...v0.9.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v0.8.0...v0.9.0)
