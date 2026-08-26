# 原始输入与已确认计划

使用 goal-mode 技能初始化这份计划，但是不要执行

<proposed_plan>
# SauceNAO / ascii2d 以图搜图集成（修订版）

## 总结

原计划需要实质修订。保留 `pixiv search SOURCE` 自动识别、SauceNAO/ascii2d、多 provider 聚合、CLI/MCP 输出和 partial 成功；修正 v1 架构边界、输出协议、Record 类型、配置入口和文档路径。

不新增 public SDK，也不修改 `sdk/pixiv`。

## 接口与架构

- 新增 `internal/services/reversesearch` 顶层 Facade，并在 `AGENTS.md` 和双语架构文档中加入仅限该能力的例外：CLI/MCP 可依赖顶层 Facade/契约，但不得导入其 SauceNAO/ascii2d 协议子包。
- 内部契约采用：
  - `Provider`: `saucenao | ascii2d-color | ascii2d-bovw | all`
  - `Request`: `Source`、`Provider`、`PixivOnly`
  - `Searcher.Search(context.Context, Request) (Response, error)`
- `APIKey`、HTTP client、cookie jar 和代理属于构造依赖，不进入每次 `Request`。CLI 按 `--proxy/--no-proxy` 构造查询服务；MCP 使用启动时的代理快照。
- CLI 保留自动识别：
  - 明确带 HTTP(S) scheme 的输入始终进入图片模式；URL 非法时显式失败，不回退关键词。
  - 其他输入仅在跟随符号链接后为现有常规文件时进入图片模式。
  - 图片模式只接受 `--provider`、现有输出参数和代理参数；显式搜索过滤、类型、分页或 trending 参数均报 usage error。
- MCP 新增 `reverse_search`，输入仅为必填 `source` 和可选 `provider`。
- 配置新增：
  - `reverse_search_provider`，默认 `saucenao`
  - `reverse_search_pixiv_only`，默认 `true`
  - `saucenao_api_key`，由 `SAUCENAO_API_KEY` 覆盖
- 删除原计划中的 CLI/MCP `all_results`；需要非 Pixiv 命中时通过 `reverse_search_pixiv_only=false` 统一控制。

## Provider、载荷与输出

- URL 响应或本地文件只流式复制一次到权限受限的临时文件，同时计算 SHA-256；各 provider 从同一快照重新打开 reader。结束时关闭并清理，不回显路径、原始 URL或文件内容。
- HTTP(S) 禁止 userinfo，并在每次重定向和最终响应重新校验协议。按已确认的信任模型，CLI/MCP 均允许任意可读常规文件及私网、环回、链路本地 URL；文档必须将 MCP 标为仅适合可信本机客户端。
- 不设置统一大小上限、固定总超时或重试。ascii2d 单独验证 JPEG/PNG/WEBP 与官方 10 MB 限制；该限制不影响 SauceNAO-only 请求。[ascii2d 官方说明](https://ascii2d.net/readme)
- SauceNAO：
  - API key 必填，缺失时在读取/上传载荷前返回 `missing_credential`。
  - multipart 上传固定 `output_type=2`、`db=999`，不开放高级参数。
  - 解析 rank、similarity、index、标题、作者、显式 Pixiv ID/用户 ID、外链和非敏感 quota；任何错误不得包含 key、source 或原始响应体。
- ascii2d：
  - 使用独立 cookie jar 获取首页 CSRF。
  - 一次 POST `/search/file`，只从同源预期 Location 提取严格 hash。
  - color/bovw 共享上传 hash，再并行抓取两个结果页。
  - 缺失 CSRF、Location、hash 或关键结果结构时返回 `malformed_upstream_response`，不静默跳过。
- `all` 固定按 `saucenao`、`ascii2d-color`、`ascii2d-bovw` 排序；SauceNAO 与 ascii2d 分支并发，ascii2d 两种模式共享上传。context 取消始终中止整体操作，不转换为 partial。
- JSON/MCP envelope 固定为：
  - `input`: 仅 `kind`、`sha256`
  - `providers`: 有序 `{name,status,result_count,quota?}`
  - `results`: 可选 canonical Pixiv ref，加一个或多个 provider evidence
  - `records`: canonical Pixiv 管道实体
  - `provider_errors`: `{provider,code,message}`
  - `partial`: 仅在至少一个成功且至少一个失败时为 true
- 只从显式 Pixiv ID 或严格 `/artworks/{id}`、`/users/{id}` URL 建立 canonical ref；不根据标题或作者猜测。同一 `(type,id)` 按首次出现去重并合并 evidence，跨 provider 分数不比较。
- Provider 无法可靠判断 artwork 子类型，因此作品 Record 使用准确的通用 `type:"artwork"`；新增受校验的 identity Record constructor，并让 download、bookmark add/remove 接受该类型。用户仍使用 `type:"user"`。
- CLI JSON 与 MCP structured output返回完整 envelope；人类输出展示过滤后的结果；非 TTY/显式 NDJSON 仅输出 `records`。
- 单 provider 失败或全部 provider 失败：CLI 非零，MCP 保留 envelope 且 `isError=true`。`all` partial：CLI exit 0 并写安全 warning，MCP `isError=false`。

## 配置、安全与文档

- `[reverse_search]` 默认配置写入 provider/pixiv-only；SauceNAO key 仅在用户设置后写入私有 TOML。
- `config get saucenao_api_key` 始终输出 `<redacted>`；`config set saucenao_api_key` 拒绝 argv value，只接受非 TTY stdin。
- 修复通用环境覆盖提示：所有 Sensitive 设置只说明“由环境覆盖”，不得打印环境值。
- 图片模式解析 JSON 输出配置时不得初始化 Pixiv SDK 或打开账号数据库。
- 文档说明图片会上传至第三方、URL 会先在本地抓取、MCP 的文件外传/SSRF 权限、partial 语义和 NDJSON 限制。SauceNAO 会短期保存上传图片，URL 查询可能缓存更久。[SauceNAO 隐私与条款](https://saucenao.com/legal.html)
- 更新目标分支实际存在的文件：英/中文 README、CLI/MCP reference、`docs/{en,zh-CN}/maintainers/{architecture,development}.md`、`skills/pixiv-cli/`、`changelog/unreleased/{en,zh-CN}.md`。
- 不再引用已不存在的日文文档、旧 `docs/maintainers/*` 路径或 `.github/PULL_REQUEST_TEMPLATE.md` release-note 流程。

## 测试计划

- 严格 TDD：每项行为先运行聚焦测试确认 Red，再完成 Green/Refactor。
- Provider fixtures：
  - SauceNAO multipart、固定字段、正常/空结果、quota、API status、非 2xx、非法/缺失 JSON、脱敏。
  - ascii2d cookie/CSRF、一次上传、Location/hash、color/bovw HTML、10 MB 边界、格式校验、selector 漂移和共享上传。
- Facade/source：
  - URL/文件只生成一次快照、SHA-256、临时权限和清理。
  - URL scheme/userinfo/重定向复核，以及已确认允许的私网与任意常规文件行为。
  - deterministic provider 顺序、并发取消、canonical 去重、evidence 合并、pixiv-only、partial 和全部失败。
- CLI/MCP/config：
  - 自动识别与关键词保留、参数冲突、provider 配置/覆盖、JSON/human/NDJSON。
  - `artwork` Record 到 download/bookmark 的管道兼容。
  - MCP schema、structured error、partial `isError`。
  - key 的 stdin-only 写入、环境优先级，以及 stdout/stderr/diagnostics/JSON-RPC 全链路不泄密。
- 真实网络 e2e 仅在 `PIXIV_REVERSE_SEARCH_E2E=1` 下运行，SauceNAO 还要求 key；它用于观察上游兼容性，不作为默认发布门禁。
- 验证顺序：目标 provider/Facade 测试 → CLI/MCP/config/Record 回归 → 架构与 secret 测试 → `go test ./...` → `sh scripts/build.sh`。不新增依赖，复用现有 `golang.org/x/net/html`。
</proposed_plan>
