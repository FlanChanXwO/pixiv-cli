# v0.8.0 — 2026-07-28

## 破坏性变更

- 公开 Go SDK 采用新的两层下载 API。常见场景使用 `Download` 或 `DownloadAll`；需要选择路径、命名、页码、质量或并发时，使用带 `DownloadOptions` 的 `DownloadWith` 或 `DownloadAllWith`。原始 `Download(ctx, ResourceRef, path)` 更名为 `DownloadResource`。
- SDK 构造器改为职责明确的入口：本地默认配置使用 `OpenDefault()`，定制本地状态使用 `OpenDefaultWith(OpenDefaultOptions)`，access-token client 使用 `NewClient(NewClientOptions)`；原先混合的 `Options` 类型已移除。
- 配置 API 已强类型化：`GetConfig`、`SetConfig`、`UnsetConfig` 使用 `ConfigKey`、`ConfigInput` 与 `ConfigValue`，CLI 文本只在边界解析。敏感 relay secret 不能通过通用配置 API 读取或写入。
- `pixiv download` 现在接收 `SRC...` 并提供 `--concurrency`。MCP `download` 使用 `src` 或 `srcs`，并支持 `concurrency`；原先分开的 PID/URL 输入字段已移除。

## 新增

- 下载可接收作品 ID、官方 Pixiv 作品 URL 或受策略允许的 CDN URL。新手默认使用 `./downloads`、文档化的命名模板与 `2 × GOMAXPROCS` 自动并发；正数并发值会严格按指定值执行。
- 批量下载结果保持来源输入顺序，并逐项报告是否已启动、结果、缓存状态与错误。直链 CDN 下载保留 URL 文件名，并拒绝页码、质量或自定义模板等仅适用于作品的选项。
- SDK、CLI 与 MCP 下载现在提供持久 `.pixiv-cache` 元数据重验证、原子替换、安全的 `Range` + `If-Range` 续传和 Ugoira ZIP 输入缓存；资源结果会暴露缓存状态。
- 新增 `ParseUserReference`，与作品引用解析对称，同时不削弱 ID 型 API 的类型安全。
- macOS、Windows 与桌面 Linux client 新增跨机器 `auth login` callback relay 配置。它使用按需持久化的 `pixiv://` handler、严格 callback 白名单、一次性 secret/state 校验、可选 TLS，并在配置或启动 HTTP 明文 relay 时明确提示风险；macOS 会恢复已有的旧 handler，并在没有旧 handler 时报告关联状态。

## 修复

- 恢复跨机器 `auth login` 的浏览器成功/失败页面。client handler 会打开一次性、无敏感信息的 relay 结果页，等待服务器真实 OAuth exchange 结果。
