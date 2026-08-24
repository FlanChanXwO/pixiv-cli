# Goal 1 Tasks

执行规则：每轮只处理第一个未完成且未阻塞的 task；每个代码 task 必须实际完成 Red → Green → Refactor。完成后填写“实际完成、验证证据、剩余风险、下一步”，如有代码改动则提交与 task 编号关联的 commit。集中检查 task 不计入下一组三个实现 task。

## Task 1：配置 schema、运行时快照与 Sensitive 写入契约

- 状态：已完成
- 目标：新增 reverse-search provider/pixiv-only/SauceNAO key 配置、环境覆盖、默认 TOML 和 stdin-only secret setter；修复 Sensitive 环境覆盖提示。
- 验收：先确认配置/CLI 测试 Red，再使 provider enum/default、`SAUCENAO_API_KEY` 优先级、redacted get、argv 拒绝、非 TTY stdin 写入和无环境值泄漏测试通过。
- 实际完成：新增 `[reverse_search]` 的 `provider` / `pixiv_only` 默认配置、运行时快照字段与 `SAUCENAO_API_KEY` 环境覆盖；provider 仅接受 `saucenao`、`ascii2d-color`、`ascii2d-bovw`、`all`。新增 Sensitive SauceNAO key，`config set` 拒绝 argv 明文并只从非 TTY stdin 读取，`config get` 无论是否设置均固定输出 `<redacted>`；配置 mutation 与环境覆盖提示不再携带 Sensitive 环境值。
- 验证证据：逐切片实际确认 Red：运行时字段缺失导致编译失败、未知 provider 未被拒绝、环境未覆盖文件、默认 TOML 缺节、argv secret 被接受、stdin secret 被拒绝、mutation 泄漏环境值、override note 显示值、unset secret 显示 `<unset>`；分别最小实现后转 Green。最终 `go test ./internal/config/settings ./internal/cli/commands/config -count=1` 与 `go test ./internal/config/... ./internal/cli/commands/config -count=1` 通过，`git diff --check` 通过；提交 hook 的 `gofmt` 与 `go test ./...` 通过并生成 commit `d4a1254`。
- 剩余风险：本 task 只建立配置与安全写入契约；key 的 provider 使用、CLI/MCP composition 注入及全链路泄漏 canary 属后续 Task 3、7–9。测试内使用的假 secret 仅为固定测试数据。
- 下一步：Task 2 建立 reverse-search 顶层领域契约与单次私有载荷快照，继续逐切片 Red → Green → Refactor。

## Task 2：领域契约与单次私有载荷快照

- 状态：已完成
- 目标：建立 `internal/services/reversesearch` 顶层类型、错误分类、Searcher/Facade seam，以及文件/URL 单次临时快照与 SHA-256。
- 验收：先确认 source tests Red，再证明常规文件、符号链接目标、HTTP(S)、userinfo 拒绝、重定向复核、私网允许、一次抓取、0600 临时文件、重开 reader、取消和清理。
- 实际完成：新增 `internal/services/reversesearch` 顶层 `Provider` / `Request` / `Response` / `Searcher`、稳定安全错误分类，以及带必需 preflight 的 Facade seam。Facade 在读取 source 前完成 provider preflight，只加载一个私有快照，并在成功/失败后清理。Loader 支持跟随符号链接后的常规文件与 HTTP(S)，流式复制一次并同时计算 SHA-256；快照为 0600（Unix），可独立重开 reader，路径保持私有且 Close 幂等。HTTP(S) 禁止 userinfo，首 URL、每次重定向和最终响应均复核；保留调用方 redirect policy 与 net/http 默认十跳规则，允许环回/私网，不增加超时、重试、大小上限或地址段限制。context 取消在请求与复制阶段均保持整体取消并清理部分文件。
- 验证证据：实际 Red 包括：顶层包/领域类型不存在、错误分类不存在、file loader 不存在、HTTP URL 被当成本地文件、畸形显式 HTTP 回退为文件错误、URL-like 合法文件名被误判、Facade seam 不存在、preflight 无法在载荷前失败、复制期取消被误分类为 `source_read_failed`；均逐切片最小实现后转 Green。覆盖常规文件、符号链接、非常规文件拒绝、SHA-256、0600、源变更隔离、reader 重开、幂等清理、环回 URL 单次抓取、userinfo/scheme/redirect/final URL、非 2xx 脱敏、请求期与复制期取消、Facade 成功/失败/preflight 生命周期。`go test ./internal/services/reversesearch -count=1`、`go test -race ./internal/services/reversesearch -count=1`、`go test ./internal/services/... -count=1`、`go test ./...`、`git diff --check` 全部通过；commit hook 的 gofmt 与全仓测试通过，提交 `69caa31`。
- 剩余风险：provider 结果领域结构将在 Task 3–5 按真实 fixture 逐步扩展；Facade preflight 已为 SauceNAO 缺 key 的载荷前失败建立硬顺序，但具体凭据与协议行为尚未实现。Windows 依赖 `os.CreateTemp` 的用户私有临时文件 ACL 语义，Unix 精确 0600 已有断言。
- 下一步：Task 3 以 fixture TDD 实现 SauceNAO provider adapter，首先固定缺 key 时 preflight 在任何载荷读取前失败。

## Task 3：SauceNAO provider adapter

- 状态：已完成
- 目标：实现必填 key、multipart JSON API、固定字段、结果/quota 解析、Pixiv ref 提取和安全错误映射。
- 验收：先确认 fixtures Red，再覆盖正常/空结果、status error、非 2xx、非法/缺失 JSON，以及 key/source/body 不进入错误或 diagnostics。
- 实际完成：新增 `ProviderClient`、`ProviderResponse`、`Match` 与 `Quota` 领域契约，并实现 `internal/services/reversesearch/saucenao` adapter。preflight 在读取快照前拒绝缺失 key；搜索以流式 multipart POST 固定发送 `api_key`、`output_type=2`、`db=999` 和单个 `file`，不增加超时、重试、缓冲上限或 `numres`。adapter 严格解析响应 status、四项 quota、相似度、索引信息、标题、作者、Pixiv artwork/user ID 与外链，支持 API 数字的 JSON number/string 表示；空结果合法返回。非 2xx、API status、畸形/缺失 JSON 和请求失败均映射为稳定安全错误，context 取消保留原始错误，且上游 body、key、source 与 transport 原始错误链不会外泄。
- 验证证据：按切片实际确认 Red：包与构造器不存在、结果领域类型/Search 不存在、API status 被误分类、空对象被当成功、非 Pixiv `author_name` 未映射、畸形数字通过 unwrap 泄漏、transport 回显 multipart 导致 key/source 泄漏，以及无 results 的 status error 被误判为 malformed；逐项最小实现后全部转 Green。fixture 覆盖成功、空结果、API status、非 2xx、非法/尾随/缺失 JSON、数字 string、Pixiv ref、作者回退、外链、multipart 精确字段和取消。`go test ./internal/services/reversesearch/saucenao -count=1`、`go test -race ./internal/services/reversesearch/saucenao -count=1`、`go test ./internal/services/reversesearch/... -count=1`、`go test ./internal/services/... -count=1`、`go test ./...` 与 `git diff --check` 全部通过；commit hook 的 gofmt 与全仓测试通过，提交 `6599dec`。
- 剩余风险：SauceNAO 官方 API 文档直连及临时使用 `127.0.0.1:7890` 代理均返回 HTTP 403，因此固定字段以目标中已确认的契约和本地 fixture 为准；真实上游/凭据联网验证留待 Task 10。当前 adapter 尚未接入 Facade 聚合，属于 Task 5；本 task 未使用真实 key、真实图片或 FlareSolverr。
- 下一步：Checkpoint 1 集中审查 Tasks 1–3 的需求偏离、安全边界、生命周期、错误映射、依赖变化与调试残留；若发现问题，仅追加独立修复 task。

## Checkpoint 1：集中检查-debug 循环（Tasks 1–3）

- 状态：已完成
- 检查：需求偏离、配置持久化与脱敏、临时文件生命周期、URL 安全、provider 错误、类型检查、相关测试、调试残留、依赖变化。
- 发现问题时：在文件末尾追加独立修复 task，下一轮先处理第一个未完成修复 task。
- 实际检查：按 `code-review-expert` 对 Tasks 1–3 的 3 个提交（15 文件、1727 新增/38 删除）完成配置、source/snapshot、Facade、SauceNAO adapter、错误链和测试的逐文件审查。配置默认值、provider enum、私有持久化、stdin-only secret、环境快照与脱敏实现符合计划；快照流式复制、0600、一次抓取、重开与清理主路径正确；SauceNAO 固定 multipart、结构校验和敏感错误清洗主路径正确。未发现依赖、public SDK、调试输出或生产文档提前变更。发现 4 个需在后续集成前修复的 P1 边界问题，按职责归并为追加 Task 13、14，本检查轮不顺手改代码。
- 验证证据：Git 范围为 `d4a1254^..6599dec`，工作区业务文件干净；`git diff --check` 通过，`go.mod`、`go.sum` 与 `sdk/` 在该范围无 diff，调试残留检索仅命中测试 canary。`go vet ./internal/config/settings ./internal/cli/commands/config ./internal/services/reversesearch/...`、`go test -race ./internal/services/reversesearch/... -count=1`、`go test ./... -count=1` 和 `sh scripts/build.sh` 全部通过，构建产物为 `build/pixiv`。精读证据：`source.go` 仅将 `http://`/`https://` 前缀识别为 URL，且在 `Stat` 前调用阻塞式 `os.Open`；`client.go` 在判断 HTTP status 前优先返回 multipart writer 错误，并允许 `strconv.ParseFloat` 接受字符串 `NaN`/`Inf`。
- 剩余风险：P1：稳定 FIFO source 会在常规文件校验前阻塞 `os.Open`，context 无法中断；带显式 HTTP(S) scheme 但缺 `//` 的现有同名文件可被当成本地文件，与“显式 scheme 永远走 URL、非法 URL 不回退”冲突。P1：SauceNAO 提前返回非 2xx 且上传 writer 同时失败时会被误报为 `provider_failed`；字符串 `NaN`/`Inf` similarity 会进入领域对象并在后续 JSON 编码失败，而非在 adapter 映射为 malformed。当前测试/race/build 均未覆盖这些条件，因此绿色门禁不能证明这些契约。
- 下一步：优先执行追加 Task 13，以 TDD 修复 source 分类和非普通文件阻塞；随后执行 Task 14，再回到 Task 4。

## Task 4：ascii2d 会话、上传与结果解析

- 状态：已完成
- 目标：实现 cookie jar、CSRF、一次上传、严格 Location/hash、color/bovw HTML 解析和 provider-specific 媒体/10 MB 校验。
- 验收：先确认 fixtures Red，再覆盖 JPEG/PNG/WEBP、边界大小、无效格式、缺失关键结构、selector 漂移、同源验证，以及 color/bovw 共享一次上传。
- 实际完成：新增 `internal/services/reversesearch/ascii2d` adapter 与顶层 `ASCII2DClient` / `ASCII2DSession` 注入端口。每次 Upload 都创建独立 cookie jar，读取首页严格匹配 `file_upload` multipart POST form 的 CSRF，再以流式 multipart 仅上传一次；JPEG/PNG/WEBP 使用真实媒体嗅探和对应 `.jpg` / `.png` / `.webp` 文件名。上传不自动跟随重定向，只接受同 scheme/host/effective-port、无 query/fragment、精确 `/search/color/{32位小写hex}` 的 Location；首页和结果页的自动重定向也限制为同源，并保留调用方 redirect policy 与 net/http 默认十跳规则。上传所得 session 可并发读取 color/bovw，二者共享 hash/cookie 且不重复外传图片。HTML parser 复用既有 `x/net/html`，按 ascii2d 的首个查询 `.item-box`、后续 `.info-box` / `.detail-box` / links / source 关键结构映射 rank、标题、作者、来源和外链；合法空结果返回非 nil 空 slice，结构缺失或 selector 漂移显式返回安全 malformed 错误。未增加依赖、重试、provider 总超时、统一载荷限制或 fallback。
- 验证证据：TDD 首轮测试因 ascii2d 包无生产文件而构建 Red；最小实现后覆盖格式、边界、CSRF/cookie、Location/hash、结果解析和共享上传转 Green。自审追加的并发上传测试在旧的 client 共享 jar 下实际出现一次 CSRF/cookie mismatch，改为每次上传独立 session 后 Green；扩展名测试实际得到三种 `filename="image"` Red，加入媒体对应扩展名后 Green；首页/结果跨源重定向测试在旧实现下均成功导致 `err=nil` / `CodeUnknown`，加入同源 redirect guard 后 Green。fixture 还覆盖缺 form/CSRF/Location、wrong route、非法 hash、附带 query、跨源 Location、缺 query item、`.detail-box` 漂移、缺作品链接、非 2xx 脱敏、unsupported provider、合法空结果以及 color/bovw 并发只上传一次。官方 `https://ascii2d.net/readme` 确认 JPEG/PNG/WEBP 与 10 MB；一次只读 GET 证明 Go 默认 User-Agent 返回 200。按用户授权临时启动既有本地 FlareSolverr 容器：color/bovw 路由均返回 200，但公开示例 hash 已失效；复用浏览器会话的 multipart 仍被 Cloudflare 返回 403，因此未伪造真实上传成功，也未反复提交。容器已恢复原先停止状态，探测临时文件已精确清理。`go vet ./internal/services/reversesearch/...`、`go test -race ./internal/services/reversesearch/... -count=1`、`go test ./internal/services/... -count=1`、`go test ./... -count=1` 与 `git diff --cached --check` 全部通过；commit hook 的 gofmt 与全仓测试通过，提交 `ef0dcfe`。
- 剩余风险：10 MB 按计划固定为 `10 * 1024 * 1024` bytes；仅当 ascii2d snapshot 超过该值时以 `invalid_source` 拒绝，目的和影响范围已在常量中文注释及精确边界测试中固定，SauceNAO-only 路径不受影响。JPEG/PNG/WEBP 限制同样只在 ascii2d Upload 前触发。由于当前真实 multipart 被 Cloudflare 403 阻断，本 task 的非空结果 DOM 依据当前上游关键 selector 契约与公开 wrapper 行为构造 fixture，尚未由本机真实新 hash 复核；任何关键漂移会显式 malformed，不会静默丢结果，真实兼容性留给 Task 10 opt-in e2e。当前 adapter 只建立端口和 session，provider 编排/partial/canonical 仍属于 Task 5。
- 下一步：Task 5 以 TDD 实现 Facade provider 选择、`all` 并发、ascii2d session 共享、固定排序、canonical Pixiv ref/evidence、pixiv-only 与 partial/all-failed 语义。

## Task 5：Facade 聚合、规范化与 partial 语义

- 状态：已完成
- 目标：实现 provider 选择、`all` 并发、固定排序、strict Pixiv ref、canonical 去重/evidence 合并、pixiv-only、provider summary/error 和 aggregate error。
- 验收：先确认聚合测试 Red，再证明单 provider、partial、全部失败、缺 Sauce key + ascii 成功、context 取消、跨 provider 分数不换算及 deterministic 输出。
- 实际完成：扩展顶层稳定领域 envelope，新增有序 `providers`、canonical/non-canonical `results`、安全 `provider_errors` 与 `partial`，并实现可注入 SauceNAO/ascii2d 端口的 `Aggregator` 作为既有 Facade 的 `PayloadSearcher`。单 provider 在 source 读取前执行 preflight；`all` 则刻意把各 provider preflight 留在并发分支内，使缺 SauceNAO key 能成为 partial 而不阻断 ascii2d。`all` 同时运行 SauceNAO 与 ascii2d 分支，ascii2d 仅上传一次并并发查询 color/bovw；发布顺序固定为 SauceNAO、color、bovw，每个 provider 内按 rank 稳定排序，与完成先后和 similarity 大小无关。仅从正数显式 artwork ID，或严格 HTTPS `www.pixiv.net/artworks/{id}` / `users/{id}` URL 建立 canonical ref；不接受 userinfo、端口、query、fragment、额外 path、lookalike host、HTTP、前导零或标题/作者猜测。作品身份优先于作者身份；相同 `(type,id)` 按首次出现合并多条 evidence，保留各 provider 原始 similarity 而不跨源换算。`pixiv-only` 只过滤最终 results，不伪造 provider result_count。单 provider 失败返回完整安全 envelope 和非零错误；`all` 至少一成功/一失败时返回 `partial=true` 且 nil error，全部失败返回 `all_providers_failed`，context 取消/截止始终丢弃 provider partial envelope并作为整体错误返回。未知/未分类 provider 错误统一清洗为安全 `provider_failed`，classified error 即使被 `errors.Join` 包裹也只发布审查过的消息。
- 验证证据：严格按纵向 TDD 执行。首个单 SauceNAO envelope 测试因 `AggregatorDependencies`、领域类型与输出字段不存在而构建 Red，最小实现后 Green；单 provider failure 初次得到 nil providers，补充安全失败 envelope 后 Green；单 ascii2d 初次返回 `provider not configured`，实现一次上传/选定模式后 Green；缺 key + ascii 成功的 `all` 初次返回 invalid provider，加入两分支并发与共享 ascii session 后 Green；canonical 合并初次只保留显式 ID 的一个结果，加入 strict URL parser、provider/rank 顺序与 evidence map 后 Green；含 `UserID` 的 artwork URL 初次被错误归为 user 77，调整作品 URL 优先级后 Green；`errors.Join` 初次把第二条私密 diagnostic 拼进返回文本，改为提取第一个稳定领域 Error 后 Green。测试覆盖单 SauceNAO、单 color/bovw、preflight 不读 source、provider branch 和两 ascii mode 的真实重叠屏障、缺 key partial、单 mode partial、全部失败、整体取消、固定 provider/rank 顺序、跨 provider 分数不换算、canonical 合并、pixiv-only 与严格 URL 正反例。按 `code-review-expert` 完成 SOLID、安全、竞态、错误与删除候选审查，无 P0/P1；唯一 P2 是 `all` 延迟 preflight 意图不显式，已补中文注释，无删除候选。`go vet ./internal/services/reversesearch/...`、`go test -race ./internal/services/reversesearch/... -count=1`、`go test ./internal/services/... -count=1`、`go test ./... -count=1` 与 `git diff --cached --check` 全部通过；commit hook 的 gofmt 与全仓测试通过，提交 `e67e21f`。
- 剩余风险：聚合器信任 Task 3/4 provider adapter 已验证的 `Match` 数值/结构；它只稳定排序 rank，不新增 rank、结果数、内存、超时或重试限制。`Result` 当前不直接依赖 `internal/shared/record`，因此 envelope 的 `records` 投影仍未加入，严格按计划留给 Task 6；CLI/MCP wire 投影与 partial exit/`isError` 语义分别属于 Task 7/8。两个并发重叠测试使用 5 秒纯内存 watchdog 仅防止测试死锁，生产代码没有该超时或对应限制。
- 下一步：Task 6 以 TDD 新增受校验的 identity Record constructor，并使 download 与 artwork bookmark add/remove 按正数 ID 接受通用 `type:"artwork"`，保持其他 action 类型边界不变。

## Task 6：通用 Record identity 与 action 管道兼容

- 状态：已完成
- 目标：新增受校验的 identity Record constructor；支持 `type:"artwork"`，使 download 和 artwork bookmark action 按 ID 消费。
- 验收：先确认 Record/action tests Red，再证明 artwork/user identity、非法 ID/type/url 拒绝、download/bookmark 接受 artwork、其他 action 类型边界不变。
- 实际完成：在 `internal/shared/record` 新增 `NewIdentityRecord`，仅构造正数 ID 的 `artwork` / `user` canonical identity，并要求 URL 与类型和 ID 对应的精确 HTTPS Pixiv URL 一致；输出只含稳定字符串 `id`、`type`、`url`。通用 NDJSON parser 保持原有兼容策略，不因 identity 构造器而收紧未知外部 Record。download 与 bookmark add/remove 的既有视觉作品 allowlist 仅新增 `artwork`，继续复用共享管道的正数 ID 解析；`illust`、`manga`、`ugoira` 保持可用，`user`、`novel` 仍被作品 action 拒绝，follow 仍只接受 `user`。按固定架构决策，reverse-search service 继续不依赖 shared Record；Task 7/8 的 CLI/MCP adapter 将使用本构造器投影 envelope `records`。
- 验证证据：identity 首轮聚焦测试因 `record.NewIdentityRecord` 不存在而编译 Red；最小实现后 artwork/user 正例以及零/负 ID、空/未知/subtype type、空 URL、HTTP、错误 host/route/ID 和 query 反例全部 Green。action 首轮测试实际显示 download/bookmark allowlist 缺 `artwork`，且管道返回失败；只新增该类型后，download、bookmark_add、bookmark_remove 均将 Record `"42"` 解析为 `int64(42)` 调用，原 subtype 正例与非作品/follow 负例通过。`go vet ./internal/shared/record ./internal/cli/commands/pixiv/download ./internal/cli/commands/pixiv/bookmark ./internal/cli/commands/pixiv/follow`、对应聚焦测试、对应 `go test -race`、`git diff --cached --check` 与 `go test ./... -count=1` 全部通过；commit hook 的 gofmt 与全仓测试通过，提交 `959414e`。
- 剩余风险：identity constructor 刻意只接受精确 canonical URL，不接受大小写、短域名、query 或其他等价变体；这是反向搜图 canonical ref 的稳定输出边界，不改变外部 NDJSON Record 的宽松保留语义。反向搜图 `records` 尚未进入 CLI/MCP wire envelope，因为 adapter 尚未实现；分别由 Task 7/8 消费本构造器完成，不能为提前输出而破坏 service 领域边界。
- 下一步：Checkpoint 2 集中检查 Tasks 4–6 的 ascii2d 协议证据、限制作用域、并发/partial/canonical 与 Record action 边界；发现问题只追加独立修复 task。

## Checkpoint 2：集中检查-debug 循环（Tasks 4–6）

- 状态：已完成
- 检查：ascii2d 协议证据、10 MB 限制作用域、并发竞态/取消、partial 状态机、canonical 顺序、Record 兼容与所有相关测试。
- 发现问题时：追加独立修复 task，不在检查轮顺手扩展功能。
- 实际检查：按 `code-review-expert` 对 Tasks 4–6 的提交范围 `ef0dcfe^..959414e`（14 文件、2120 新增/9 删除）完成逐文件审查。ascii2d 的 cookie/CSRF 会话、上传 Location/hash、媒体嗅探、结果 DOM、同源 redirect 与错误清洗职责集中且无新增依赖；10 MiB 和 JPEG/PNG/WEBP 限制只在 ascii2d `Upload` 前生效。Aggregator 的 Sauce/ascii 两主分支、ascii color/bovw 两 mode 并发写入独立 outcome，并经 WaitGroup 后按 SauceNAO/color/bovw 与 rank 稳定发布；取消、partial、all-failed、pixiv-only、canonical 优先级和 evidence 合并主状态机正确。Identity constructor 不收紧外部 NDJSON parser，download/bookmark 只增加 `artwork`，follow 仍为 user-only。无 public SDK、依赖、调试残留或提前文档变更。发现 2 个合入 CLI/MCP 前必须修复的 P1 错误边界，追加 Task 15、16，本检查轮未改业务代码。
- 验证证据：官方 `https://ascii2d.net/readme` 当前直连 HTML 明确写明支持 10 MB 以内的一般 JPEG/PNG/WEBP，和 provider-specific 常量/测试一致，无需启动 FlareSolverr。确定性临时 RoundTripper 在 POST 后关闭 request body并同时返回带非法 hash 的 302，当前 `Upload` 实际输出 `code=provider_failed error=could not upload image to ascii2d`；根因是 `client.go` 先检查 writer error、后验证 Location，违反非法 hash 必须 malformed 的固定契约，临时 probe 已删除。`aggregator.go` 的 `safeProviderFailure` 对 classified error 直接返回原对象；现有测试用含私密 cause 的 `wantErr` 并以 `require.ErrorIs(err, wantErr)` 证明该对象仍在公开错误链，当前测试只检查 `Error()` 文本，未覆盖 unwrap 泄漏。`go vet` 覆盖 reverse-search/Record/三个 action 包通过；对应 `go test -race` 全部通过；`go test ./... -count=1`、`sh scripts/build.sh`、提交范围 `git diff --check` 与 go.mod/go.sum/sdk 无 diff 检查全部通过，构建产物为 `build/pixiv`。
- 剩余风险：P1：ascii2d 畸形 3xx 与并发 writer error 同时发生时错误分类依赖本地上传错误，掩盖上游 Location/hash 漂移；需按 HTTP status → Location/hash → writer error 的权威顺序分类，同时继续同步 writer goroutine。P1：单 provider classified failure 的安全文本虽已清洗，但 cause 可被后续 diagnostics、logging 或错误遍历取出，可能包含 key/source/body；需复制 code/message 为无 cause 的发布错误，并保持 context cancellation 原样。真实 ascii2d 非空 DOM 仍未通过 Cloudflare 下的新上传 hash 复核，这是已知外部兼容性风险，继续留给 Task 10 opt-in e2e，不伪造为本 checkpoint 的通过项。
- 下一步：优先执行追加 Task 15，以 TDD 修复 ascii2d upload 响应/写入错误优先级；随后执行 Task 16 清洗 aggregator classified error chain，再回到 Task 7。

## Task 7：CLI `pixiv search` 自动图片模式

- 状态：已完成
- 目标：在现有 search owner 中实现 URL/常规文件智能识别、provider 覆盖、flags 冲突、proxy 构造、human/JSON/NDJSON 和 partial warning。
- 验收：先确认 CLI tests Red，再证明普通关键词完全保持、非法 URL 不回退、图片模式不打开 Pixiv SDK/账号 DB、输出与 exit code 契约正确。
- 实际完成：`pixiv search SOURCE` 现在把明确 HTTP(S) scheme 始终送入反向搜图，把跟随符号链接后的现有常规文件送入反向搜图，其他输入继续走普通关键词搜索；非法 HTTP(S) 不回退关键词。新增 `--provider` 校验与覆盖、`--proxy/--no-proxy` 传输覆写、图片模式搜索过滤/类型/分页/trending 冲突校验，以及独立的 production assembly 按每次调用构造代理化 Facade。CLI JSON 输出完整 `input/providers/results/records/provider_errors/partial` envelope，human 输出安全摘要，显式或非 TTY NDJSON 只输出 canonical records；partial 结果写 stderr warning，单 provider/全失败保留 envelope 并返回错误。JSON 配置解析走独立 runtime 端口，图片模式不初始化 Pixiv SDK 或账号数据库。
- 验证证据：首条 URL 测试先因 `Dependencies.ReverseSearch`/请求类型缺失编译 Red；provider flag、输出 envelope、partial、过滤冲突与 root 隔离测试也分别先确认旧行为失败，再逐切片转 Green。`go test ./internal/cli/commands/pixiv/search ./internal/cli ./internal/services/reversesearch/assembly -count=1`、`go vet ./internal/cli/commands/pixiv/search ./internal/cli ./internal/services/reversesearch/...`、对应 `go test -race`、`go test ./... -count=1` 与 `sh scripts/build.sh` 全部通过；审查覆盖源分类、代理/配置、记录投影、stdout/stderr、错误链边界和 CLI/provider import 边界，未发现 P0/P1。工作树将在提交后保持仅有本目标提交。
- 剩余风险：真实 SauceNAO/ascii2d 网络兼容性仍按 Task 10 的显式 opt-in e2e 观察；MCP tool、架构/secret 回归和用户文档属于后续 Task 8–12，当前 CLI 端口已为其保留顶层 Facade 契约。生产 provider 缺 key、上游失败和 source 错误继续按 service 的安全错误分类传播，不自动重试或 fallback。
- 下一步：Task 8 新增 MCP `reverse_search` tool 与启动时运行时注入。

## Task 8：MCP `reverse_search` tool 与运行时注入

- 状态：已完成
- 目标：按 repo-local MCP tool 规范新增 tool package、封闭 input/output schema、Facade 注入和 structured result/error。
- 验收：先确认 MCP tests Red，再证明 source/provider schema、配置默认、records/results、partial `isError=false`、全失败 `isError=true`、JSON-RPC stdout 与敏感信息安全。
- 实际完成：新增 `internal/mcpserver/pixiv/tools/reverse_search`，输入 schema 只开放必填 `source` 与可选 provider enum，输出固定为 `input/providers/results/records/provider_errors/partial` envelope。MCP runtime 增加启动时注入的 `ReverseSearchPorts`，由 CLI composition root 在 stdio 启动时一次构造 Facade，固定代理、SauceNAO key、provider 默认值和 pixiv-only；单次 MCP input 只能覆盖 provider。结果只投影 canonical artwork/user Record，不回显原始 source；partial 保持 `isError=false`，执行/配置/全 provider 失败保留 structured envelope 并设置 `isError=true`，错误文本只发布稳定 code/通用摘要。
- 验证证据：先把 `reverse_search` 加入精确注册集合，旧实现实际因缺少 tool 失败；随后新增 schema/端口测试，旧端口字段缺失导致编译 Red。Green 后 `go test ./internal/mcpserver/pixiv -count=1`、`go test -race ./internal/mcpserver/pixiv -count=1`、`go vet ./internal/mcpserver/pixiv ./internal/cli`、`go test ./internal/cli ./internal/mcpserver/pixiv -count=1` 和 `git diff --check` 全部通过。聚焦测试覆盖 source/provider schema、启动默认、records/results、partial、全失败/未配置 structured envelope、source/key/body 脱敏与 JSON-RPC stdout-only；对应代码将在本 task 提交。
- 剩余风险：真实 SauceNAO/ascii2d 网络兼容性仍由 Task 10 的显式 opt-in e2e 观察；架构静态边界、跨链路 secret canary 和用户文档属于 Task 9–12。MCP 继续沿已确认的可信本机信任模型允许服务层处理本地文件和私网 URL。
- 下一步：Task 9 补 reverse-search Facade 例外的架构规则、provider import 静态边界和 secret 回归。

## Task 9：架构规则、静态边界与 secret 回归

- 状态：已完成
- 目标：为 reverse-search 顶层 Facade 例外建立精确架构规则，禁止 CLI/MCP 导入 provider 子包，并补全跨层泄漏 canary。
- 验收：先确认 architecture/secret tests Red，再证明 public SDK inventory 不变、provider import 禁令生效、source/key/body/CSRF/Location 不跨错误/日志/输出边界。
- 实际完成：在 `AGENTS.md`、中英文维护者架构文档中明确 reverse-search 是唯一跨常规 public SDK 边界的例外：生产组装只允许 `internal/cli/root.go` 依赖 `internal/services/reversesearch/assembly`，CLI commands 与 MCP 只依赖顶层 `internal/services/reversesearch` 契约；新增 `internal/architecture` AST 静态测试禁止 CLI/MCP/Record 层导入 provider 子包；新增 public SDK inventory SHA-256 pin，固定 `sdk`、`sdk/pixiv`、`sdk/fanbox` 导出符号清单未被本目标修改；CLI 根命令与 MCP failure canary 覆盖 source、API key、上游 body、CSRF 与 Location 私密值，确认它们不进入错误、stdout/stderr 或 structured envelope。
- 验证证据：首个 architecture Red 运行 `go test ./internal/architecture -run '^TestReverseSearchBoundaryExceptionIsDocumented$' -count=1 -v`，在规则尚未写入三份文档时因缺少约束短语失败；完成后 `go test ./internal/architecture -count=1 -v`、`go test ./scripts/internal/publicapi -run 'TestRepositoryPublicAPIInventoryIsPinned|TestInventoryCollectsOnlyExportedPackageSymbols' -count=1 -v`、CLI/MCP secret canary 均通过。随后 `go test ./internal/architecture ./scripts/internal/publicapi ./internal/cli/commands/pixiv/search ./internal/cli ./internal/mcpserver/pixiv -count=1`、对应 `go test -race`（architecture/publicapi/search/mcp）、`go vet` 和 `git diff --check` 全部通过；未新增依赖，public SDK inventory hash 保持 `ed45ee60aba67e2a657174325e9796451a6ef88f4161dc643ad97368f5e7eb31`。
- 剩余风险：静态 import gate 只覆盖其声明的 CLI commands、MCP、shared Record 目录，生产 assembly 仍按规则允许留在 `internal/cli/root.go`；真实 provider 网络兼容性与 opt-in e2e 仍待 Task 10，用户文档与 release note 仍待 Task 11–12。本 task 不增加超时、重试、截断或 fallback。
- 下一步：Checkpoint 3 集中检查 Tasks 7–9。

## Checkpoint 3：集中检查-debug 循环（Tasks 7–9）

- 状态：已完成
- 检查：CLI 兼容、MCP schema/runtime、静态架构、stdout/stderr/JSON-RPC、代理、账号 DB 隔离、race/类型/聚焦回归。
- 发现问题时：追加修复 task并按顺序处理。
- 实际检查：按 `pixiv-cli-review` 对 `ce03802^..4cfc4d4`（Tasks 7–9，18 个文件）逐项检查 CLI owner/production assembly、MCP tool/schema/runtime、Facade/provider import 边界、Record 投影、代理与启动快照、Pixiv SDK/账号 DB 隔离、错误/诊断/stdout/JSON-RPC 和敏感信息；未发现 P0/P1/P2，未追加修复 task。额外的 same-package 清单命令暴露出维护文档既有基线与当前仓库目录不一致，但本范围新增的 `internal/architecture` 使用 external test package，不新增该类偏差。
- 验证证据：`go test ./... -count=1` 全部通过；`go vet ./internal/cli ./internal/cli/commands/pixiv/search ./internal/mcpserver/pixiv ./internal/services/reversesearch/... ./internal/architecture ./scripts/internal/publicapi` 通过；Task 9 的聚焦 `go test`、race、public SDK inventory pin、AST import gate、secret canary 与 `git diff --check` 通过；Task 7/8/9 提交钩子均实际运行 gofmt 与全仓测试，当前 HEAD 为 `4cfc4d4`。
- 剩余风险：真实 SauceNAO/ascii2d 上游兼容性仍未在默认门禁验证，按 Task 10 的显式 opt-in e2e 观察；用户文档、产品 skill、未发布说明和最终构建交付仍待 Task 11–12。MCP 的本地文件/私网 URL 能力继续只适合可信本机客户端，未新增隐藏限制或 fallback。
- 下一步：Task 10 新增默认跳过、显式启用的真实网络 e2e 与 fixture 维护入口。

## Task 10：真实网络 e2e opt-in 与 fixture 维护入口

- 状态：未开始
- 目标：新增默认跳过的真实 SauceNAO/ascii2d 兼容性 e2e，使用显式环境变量且不记录 source/key；明确它不属于普通 CI 门禁。
- 验收：默认测试不访问网络；启用条件、缺 key 行为和安全输出有测试。若当前环境具备明确凭据/测试源则记录真实结果，否则标注外部阻塞，不伪造通过。
- 实际完成：
- 验证证据：
- 剩余风险：
- 下一步：

## Task 11：英/中文用户与维护者文档、产品 skill

- 状态：未开始
- 目标：更新 README、CLI reference、MCP tools、双语 architecture/development 和 `skills/pixiv-cli/`。
- 验收：覆盖识别规则、provider/config、secret stdin、第三方上传与保存、MCP 文件外传/SSRF 信任模型、partial、pixiv-only、NDJSON/generic artwork；链接和现有 locale 路由检查通过。
- 实际完成：
- 验证证据：
- 剩余风险：
- 下一步：

## Task 12：双语未发布说明与目标级验证

- 状态：未开始
- 目标：更新 `changelog/unreleased/{en,zh-CN}.md`，运行全部聚焦测试、全仓测试和构建，修复仅归属于本目标的问题。
- 验收：两种语言覆盖同一用户可感知变更；目标包测试、architecture/secret 回归、`go test ./...`、`sh scripts/build.sh` 有实际结果；无意外依赖或机器文件进入 diff。
- 实际完成：
- 验证证据：
- 剩余风险：
- 下一步：

## Checkpoint 4：集中检查-debug 循环（Tasks 10–12）

- 状态：未开始
- 检查：真实 e2e 隔离、fixture 可维护性、文档/skill/发布说明一致性、全仓测试/构建、git diff、敏感信息与机器文件。
- 发现问题时：追加修复 task；所有新增 task 完成后才可进入终审。
- 实际检查：
- 验证证据：
- 剩余风险：
- 下一步：

## Final Review：目标终审

- 状态：未开始
- 范围：重新全量阅读 `input.md`、`plan.md`、`tasks.md`；审查用户体验、代码边界、并发/取消、安全/隐私、错误语义、配置、Record 管道、MCP JSON-RPC、测试、构建、文档、release note 与回滚方案。
- 验收：不存在已知高风险问题；所有未阻塞 task/修复 task 已完成；阻塞项有事实依据；最终验证证据完整；按项目规则完成代码审查并处理阻塞 finding。
- 实际检查：
- 验证证据：
- 剩余低风险事项：
- 完成结论：

## 追加修复任务区

集中检查发现问题时，从此处开始追加，使用连续编号并保留同样的状态/证据/风险/下一步字段。

## Task 13：修复 source scheme 分类与非普通文件阻塞

- 来源：Checkpoint 1，P1。
- 状态：已完成
- 目标：保证任何显式 HTTP(S) scheme 都进入 URL 校验且非法 URL 不落入文件路径；在可能阻塞的读取前拒绝稳定的 FIFO/设备/socket 等非普通文件，同时在打开后再次校验实际句柄以约束 TOCTOU。
- 验收：先新增测试并实际确认 Red，覆盖现有同名 `http:opaque`/`https:opaque` 文件仍返回 `invalid_source`、普通冒号文件仍可读取、FIFO 在没有 writer 时不阻塞并返回 `source_not_regular_file`、符号链接普通目标继续成功、打开前后类型变化不被作为普通文件复制；不得用固定超时掩盖阻塞根因。
- 实际完成：source 分类改为大小写不敏感识别完整 `http:` / `https:` scheme，任何此类输入均先进入既有 URL 校验，因而现有同名畸形 URL 文件不再被打开；普通 `art:work.png` 等非 HTTP(S) 冒号文件仍按现有常规文件规则工作。文件路径在 `os.Open` 前先用跟随符号链接的 `os.Stat` 拒绝目录、FIFO、设备和 socket 等非普通目标，避免稳定 FIFO 无 writer 时阻塞；打开后继续对实际句柄 `Stat`，并通过内部可控 opener 建立确定性 TOCTOU characterization，证明预检查后变成目录的对象不会生成快照。生产路径未增加超时、重试、轮询、大小限制或依赖。
- 验证证据：第一个 Red 中，现有 `http:opaque` 文件被成功当作文件读取，测试得到 `err=nil` / `CodeUnknown`；修改 scheme 分流后，与普通冒号文件及既有 URL 拒绝用例共同转 Green。第二个 Red 使用真实无 writer FIFO 直接调用公开 `Loader.Load`，旧实现被 Go 测试进程 watchdog 捕获并由栈证明阻塞在 `os.Open`；加入打开前类型校验后，同一测试不依赖测试内超时即可立即返回 `source_not_regular_file`。打开后类型复核属于既有 Green 行为，本轮通过在预检查与打开之间把常规文件替换成目录的 characterization 固定，并确认没有快照残留。`go vet ./internal/services/reversesearch/...`、`go test -race ./internal/services/reversesearch/... -count=1`、`go test ./internal/services/... -count=1`、`go test ./... -count=1` 和 `git diff --check` 全部通过；commit hook 的 gofmt 与全仓测试通过，提交 `3e2cb47`。
- 剩余风险：打开前 `Stat` 与实际 `Open` 仍不是单个原子操作；若可信本机上的其他进程恰好在两者之间把常规路径替换为 FIFO，底层阻塞式 `os.Open` 仍可能等待。打开后的类型变化不会被复制，稳定非普通目标已在打开前拒绝；彻底消除该极窄本地竞态需要平台特定的 nonblocking/openat 策略，不在本 task 已确认范围内，也不以固定超时掩盖。
- 下一步：Task 14。

## Task 14：修复 SauceNAO HTTP 优先级与非有限数值

- 来源：Checkpoint 1，P1。
- 状态：已完成
- 目标：即使服务端提前响应并导致 multipart writer 失败，非 2xx 仍稳定映射为 `upstream_http_status`；拒绝会破坏 JSON envelope 的非有限 similarity。
- 验收：先新增测试并实际确认 Red，使用可控 transport 证明 response 与 writer error 同时发生时 status 分类优先且无 goroutine 泄漏；字符串 `NaN`、`Inf`、`-Inf` 均映射 `malformed_upstream_response`，有限合法 similarity 保持不变，key/source/body/error chain 继续脱敏。
- 实际完成：SauceNAO `Search` 在收到 response 后仍同步等待 multipart writer 完成，先保留 context 整体取消语义，再优先依据非 2xx 返回 `upstream_http_status`；只有 2xx response 才将 writer error 映射为 upload `provider_failed`。这样既不遗留 writer goroutine，也不让服务端已明确返回的 HTTP 状态被本地 pipe error 覆盖；response body 在所有返回路径继续关闭且非 2xx body 不读取。`flexibleFloat` 在 `ParseFloat` 后拒绝 `NaN` 和正负无穷，仅要求数值有限，不新增 similarity 范围、截断或其他无依据限制；解码失败继续由既有安全 malformed 边界清除原始 cause。
- 验证证据：第一个 Red 使用可控 transport 主动关闭 request body、同时返回 429，旧实现得到 `provider_failed`；调整顺序后得到 `upstream_http_status`，并观测 response body 已关闭，key、source、上游 body 均不在完整 error chain。writer result 在分类前被同步接收，因此测试返回即证明 writer 已完成发送，不存在该 goroutine 的未消费结果。第二个 Red 中，字符串 `NaN`、`Inf`、`-Inf` 均被旧实现接受并返回成功（`err=nil` / `CodeUnknown`）；加入有限性校验后全部映射为 `malformed_upstream_response`，既有 `91.23` fixture 仍成功。`go vet ./internal/services/reversesearch/...`、`go test -race ./internal/services/reversesearch/... -count=1`、`go test ./internal/services/... -count=1`、`go test ./... -count=1` 和 `git diff --check` 全部通过；commit hook 的 gofmt 与全仓测试通过，提交 `80d5729`。
- 剩余风险：本 task 使用确定性 transport 模拟标准 `net/http` 提前响应/关闭上传流语义，未访问真实 SauceNAO；真实服务与代理组合的兼容性仍由 Task 10 opt-in e2e 观察。上游可能返回的其他非标准 similarity 字符串会按同一 malformed 契约显式失败，不静默清洗。
- 下一步：Task 4 实现 ascii2d 会话、单次上传、严格结果定位与解析。

## Task 15：修复 ascii2d upload 响应分类优先级

- 来源：Checkpoint 2，P1。
- 状态：已完成
- 目标：当 ascii2d 上传响应与 multipart writer error 同时存在时，先按权威 HTTP status 和 Location/hash 契约分类，再处理仅影响有效成功响应的本地写入失败；仍同步回收 writer goroutine。
- 验收：先新增确定性 transport 测试并实际确认 Red；非 3xx 始终为 `upstream_http_status`，3xx 缺失/跨源/错误 route/非法 hash 始终为 `malformed_upstream_response`，即使 writer 同时失败；只有 Location/hash 有效但上传写入失败时为 `provider_failed`。覆盖 response body 关闭、writer 完成、context cancellation 以及 token/source/Location/body/error chain 脱敏，不增加 timeout、重试或 fallback。
- 实际完成：调整 ascii2d 上传结果处理顺序：先同步等待 multipart writer 结果并保留 context cancellation，再按非 3xx HTTP status、3xx Location/hash、有效 Location 下的 writer error 依次分类。新增确定性 RoundTripper 测试，覆盖非 3xx、缺失/跨源/错误 route/非法 hash、合法 Location + writer error，以及取消场景；所有错误继续只发布稳定安全消息，不回显 source、token、Location、响应体或 writer cause。
- 验证证据：在 `HEAD` 临时 worktree 只应用新增测试时，非法 Location 用例实际 Red（旧实现均返回 `provider_failed`）；当前实现后 `go test ./internal/services/reversesearch/ascii2d -run 'TestUpload(Prioritizes|ReportsWriter|PreservesCancellation)' -count=1 -v`、`go test -race ./internal/services/reversesearch/ascii2d -count=1`、`go vet ./internal/services/reversesearch/...`、`go test ./internal/services/reversesearch/... -count=1`、`go test ./... -count=1`、`sh scripts/build.sh` 和 `git diff --check` 全部通过。构建产物为 `build/pixiv`；代码审查未发现 P0/P1/P2、依赖变化、架构越界或删除候选；已按 Task 15 提交。
- 剩余风险：真实 ascii2d 新 hash 仍受上游 Cloudflare/反爬影响，真实兼容性继续由 Task 10 的显式 opt-in e2e 观察；本 task 使用确定性 transport，不伪造真实网络通过。生产代码未增加 timeout、重试、载荷限制或 fallback。
- 下一步：Task 16 清洗 aggregator classified provider error chain，完成后回到 Task 7。

## Task 16：清洗 aggregator classified provider error chain

- 来源：Checkpoint 2，P1。
- 状态：已完成
- 目标：单 provider failure 只向 adapter 发布稳定 code/message，不保留 classified error 的私密 cause 或 joined diagnostics；context cancellation 继续原样返回。
- 验收：先扩展错误链 canary 并实际确认 Red，证明 key/source/body 私密 cause 当前可经 `errors.Is/As/Unwrap` 到达；修复后 envelope 和返回错误仍保留原稳定 code/message，但完整公开错误链不含原 error/cause/joined detail。all-provider safe errors与未分类错误行为保持不变。
- 实际完成：`safeProviderFailure` 对 classified `*Error` 只复制稳定 code/message，创建 cause 为 nil 的新安全错误；因此单 provider 返回的 error 与 structured `ProviderError` 保持原分类/文本，但不再暴露 provider cause 或 `errors.Join` 的其他诊断。取消仍在进入清洗前由 `cancellationError` 原样返回，未分类错误和 all-provider aggregate 语义保持不变。新增错误链 canary 检查 `errors.Is`、`errors.As` 和 `errors.Unwrap`。
- 验证证据：先仅修改 canary 并实际 Red：旧实现中 `errors.Is(err, wantErr)` 为 true；修复后单 provider、all-provider、取消测试通过。`go test -race ./internal/services/reversesearch/... -count=1`、`go vet ./internal/services/reversesearch/...`、`go test ./internal/services/... -count=1`、`go test ./... -count=1`、`sh scripts/build.sh` 和 `git diff --check` 全部通过；构建产物为 `build/pixiv`。
- 剩余风险：该边界依赖各 provider 以 `reversesearch.NewError` 发布的 message 已经过安全审查；本 task 不改变上游 adapter 的错误映射，也不新增重试、超时或 fallback。Task 7/8 仍需在 CLI/MCP adapter 层验证 structured output、stdout/JSON-RPC 全链路脱敏。
- 下一步：Checkpoint 3 集中检查 Tasks 7–9；在此之前执行 Task 7。
