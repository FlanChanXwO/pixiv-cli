# v0.9.0 — 2026-07-31

## 破坏性变更

- `pixiv feed` 改为 `pixiv timeline latest|following`；MCP timeline 工具改名；SDK 绘图工具发现改用 `SupportedDrawingTools()`；SDK 与项目日志入口、MCP 账号/配置工具被移除；远程登录改为在桌面端确认一次，不再使用设备配对和复制回调表单。（[#40](https://github.com/FlanChanXwO/pixiv-cli/pull/40)）

## 新增

- 新增动图 APNG 输出、交互式下载进度、更丰富的列表筛选、静态绘图工具目录和作品去重下载。（[#40](https://github.com/FlanChanXwO/pixiv-cli/pull/40)）

## 修复

- ClawHub skill 与 Homebrew Formula 在遇到审核、待展示卡片或 beta Formula 暂不可用时也能发布或恢复。（[`0915746`](https://github.com/FlanChanXwO/pixiv-cli/commit/0915746), [`2a6e9f2`](https://github.com/FlanChanXwO/pixiv-cli/commit/2a6e9f2), [`384a3e8`](https://github.com/FlanChanXwO/pixiv-cli/commit/384a3e8), [`3e67b28`](https://github.com/FlanChanXwO/pixiv-cli/commit/3e67b28), [`73b30c2`](https://github.com/FlanChanXwO/pixiv-cli/commit/73b30c2), [`b09f161`](https://github.com/FlanChanXwO/pixiv-cli/commit/b09f161)）

**完整变更**：[v0.8.0...v0.9.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v0.8.0...v0.9.0)

<!-- internal:
  [`ea02b29`](https://github.com/FlanChanXwO/pixiv-cli/commit/ea02b29)
  [`92261fa`](https://github.com/FlanChanXwO/pixiv-cli/commit/92261fa)
  [`66d5fd7`](https://github.com/FlanChanXwO/pixiv-cli/commit/66d5fd7)
  [#38](https://github.com/FlanChanXwO/pixiv-cli/pull/38)
  [#37](https://github.com/FlanChanXwO/pixiv-cli/pull/37)
  [#41](https://github.com/FlanChanXwO/pixiv-cli/pull/41)
  [`b8daaa1`](https://github.com/FlanChanXwO/pixiv-cli/commit/b8daaa1)
-->
