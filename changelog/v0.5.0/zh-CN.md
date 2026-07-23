# v0.5.0 — 2026-07-22

## 新增

- CLI/MCP 操作摘要写入用户 state 目录下 `pixiv/logs` 的按日 JSONL（默认保留 7 天，仅清理可识别的历史日志文件）；终端默认不再输出日志痕迹。日志只记录脱敏操作摘要，不含 token、查询串、绝对路径、上游 body 或原始错误。目录创建/轮转/清理失败时静默继续。仅上游不可用、上游错误、响应畸形、限流等特殊非认证故障会在 CLI 错误中提示日志目录；登录失败与 token 过期不提示。
- MCP `download` / `download_random_from_recommendation` 支持 `pages` 与 `quality`，与 CLI 共用下载选项。
- 下载新增 `--pages`（1-based，支持 `1,3-5`）与 `--quality original|regular|small|thumb|mini`；默认仍下载全部原图。Ugoira 对派生质量或页选择返回 unsupported。public SDK 暴露 `ParsePageSpec`/`DownloadQuality`/`DownloadOptions`。
- 所有作品模型、CLI JSON/文本与 MCP 结构化/文本输出增加作品页 `url`（`https://www.pixiv.net/artworks/${id}`），JSON 为 public Illust 首字段，文本输出放在每件作品第一行。
- 公开 SDK 的 `UgoiraMetadata` 新增成对且非空的 `download_url`/`download_quality`（`medium|original`）；`zip_urls.original` 仅在真正取得 original ZIP 时输出。下载器、CLI 与 MCP 统一使用该已验证资源。
- 插画排行榜扩展为 16 个 App API mode：新增四个 manga 与五个 R18 mode；CLI `--mode`、MCP 稳定标签和 SDK 常量同步支持，新增 mode 明确要求认证。

## 变更

- CLI terminal prompts, OAuth completion pages, log-directory hints, and fixed help examples now use English. Artwork metadata and user-supplied query text remain unchanged.
- Breaking: MCP fixed status, error, and display text now uses English. Structured output, Pixiv metadata, and user-supplied text remain unchanged.
- Breaking: `pixiv search --search-target`, `--target`, `--duration`, and `--tool` have been removed without aliases. Use `--search-by tag-partial|tag-exact|title-caption`, `--period day|week|month`, and `--draw-tool`; the user-facing `--limit` default no longer exposes the internal `-1` sentinel.
- Breaking: `--download-path` and `--filename-template` are now accepted only by `pixiv download`. All other data, user, bookmark, and follow commands reject the previously ignored flags instead of silently accepting a no-op.
- Breaking: CLI/MCP 诊断日志改写用户 state 目录文件，不再默认输出到 stderr；MCP stdout 仍仅用于 JSON-RPC。
- Breaking: MCP 下载仅返回本地 `path`/`file_uri`/`mime_type`/页号/大小；移除 `delivery=image_content` 内嵌图片与 `get_thumbnail_base64` 工具。Agent 应使用宿主本地附件能力发送文件；宿主不支持时仅分享作品 URL。
- Breaking: 移除 MCP 旧 wire 字段 `search_r18`、`user_id_to_check`、`max_bookmark_id`、`offset`、`include_thumbnail`；列表与搜索统一使用规范字段 `user_id`、`rating`、`page`/`limit`。
- Breaking: 移除 CLI 兼容入口 `--ai-type`、`--r18`、`--profile`、`--offset` 与 `search --type comics`；请分别使用 `--ai-mode`、`--rating r18`、`--uid`、`--page`/`--limit` 与 `--type manga`。

## 修复

- 修复 CLI 命令与 MCP 会话结束后未关闭按日 JSONL 日志文件的问题；Windows 现可在命令返回后清理用户 state 临时目录，不再依赖进程退出释放文件句柄。
- 修复认证态作品详情、分页和 ugoira metadata 在 App API 成功后仍访问匿名 Web 补全、致使 R18 作品遭遇 403/404 的问题。认证路径现只使用 App 数据；多页直接读取 `meta_pages`，单页派生规范页面，缺失或不一致明确返回上游响应错误，不伪装 partial result。
- 修复 App API 幂等 JSON 读取限流恢复：仅首次 HTTP 429 且 `Retry-After` 有效时按调用方 context 等待并重试一次；无效 header、第二次 429、写操作和资源下载均保留真实错误且不重放，安全 info 日志不含 URL、header 或凭据。
- 修复 macOS `pixiv://` helper 在后台吞掉 loopback 最终响应、导致浏览器停留在 Pixiv 白色 relay 页的问题。helper 现打开本地桥接页，并在 OAuth 真正完成后显示居中的最终成功/失败页；callback 仅短暂位于 URL fragment，提交前即从地址栏和历史中清除，失败页不泄露敏感原因；CLI 成功提示前增加一个空行。
- 搜索在本地筛选产生连续空上游批次时，CLI/MCP 会补拉到首个非空逻辑批次；`--limit N`/`limit` 填满逻辑结果，`--limit 0`/`limit=0` 遍历全部，`--page`/`page` 按过滤后结果分页。
- App 作品 AI 字段优先读取 `illust_ai_type`，并兼容旧 `ai_type`；本地 AI 判定仍固定 `AIType==2`。
