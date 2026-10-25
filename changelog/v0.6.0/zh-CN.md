# v0.6.0 — 2026-07-24

## 破坏性变更

- 持久化数据改放在当前用户的 `.pixiv-cli` 目录；如果账号、配置或日志放在旧位置，需要移动过去。（[`352a2f0`](https://github.com/FlanChanXwO/pixiv-cli/commit/352a2f0)）

## 新增

- CLI 与 MCP 新增需要登录的小说搜索和用户搜索，并提供日文 SDK 与 MCP 参考页。（[`2f414ac`](https://github.com/FlanChanXwO/pixiv-cli/commit/2f414ac), [`60bd6b4`](https://github.com/FlanChanXwO/pixiv-cli/commit/60bd6b4)）

## 修复

- 代理资源下载恢复正常，分页提示与实际页大小一致，桌面 OAuth 登录完成后会显示结果。（[#29](https://github.com/FlanChanXwO/pixiv-cli/pull/29)）

**完整变更**：[v0.5.0...v0.6.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v0.5.0...v0.6.0)

<!-- internal:
  [`7d75073`](https://github.com/FlanChanXwO/pixiv-cli/commit/7d75073)
  [`82c18ad`](https://github.com/FlanChanXwO/pixiv-cli/commit/82c18ad)
  [`b0c6ad1`](https://github.com/FlanChanXwO/pixiv-cli/commit/b0c6ad1)
-->
