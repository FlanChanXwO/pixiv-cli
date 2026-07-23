# v0.2.0 — 2026-07-13

## 新增

- 新增公开 Go SDK；提供具体 `*pixiv.Client`、稳定模型、类型化错误、opaque cursor、账号/config 与受策略限制的资源流访问。
- 新增 `pixiv user artworks/bookmarks/following [USER_ID]`；新增 `bookmark add/remove` 与 `follow add/remove`。
- MCP 新增 `user_artworks`、分页用户列表、收藏/关注写操作及 structured output。
- 新增可注入的 `slog` 诊断日志与 `log_level`/`log_format` 配置，支持 `PIXIV_LOG_LEVEL`/`PIXIV_LOG_FORMAT` 覆盖。
- 新增 Linux quality gate 与六平台已打包 binary smoke；它们离线验证 CLI、config 与 MCP stdio，不使用 Pixiv 凭据或真实上游网络。

## 变更

- 列表 CLI 改用 `--limit` 和逻辑 `--page`；`--offset` 已废弃。CLI/MCP 不暴露 SDK cursor。
- 有 refresh token 时 App API 失败不再自动回落 Web；Web 仅用于无 token 的匿名白名单读操作和明确 metadata enrichment。
