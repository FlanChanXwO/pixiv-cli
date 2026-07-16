# Goal 2 任务清单

每个普通任务必须按 TDD tracer bullet 实施，并依次经过 spec review、quality review。每个任务完成后填写“实际/证据/风险”。

## T01 — MCP refresh_token 保留真实打开错误

- 状态：完成
- 范围：P1-1。区分可判定的未设置 token 与 context、代理、配置、client factory 等失败；返回脱敏但真实的错误，不把所有失败伪装成缺 token。
- 验收：MCP public tool 测试先 RED；无 token 提示保持兼容，其他错误包含安全真因且不泄露凭据；`go test ./internal/mcpserver -count=1`。
- 实际：`refresh_token` 现把 open/factory 错误分为取消、超时、public `*sdk.Error` 和未知初始化/配置/代理错误；未知错误固定脱敏，open 阶段的 unauthorized 不再冒充缺 token。只有 `client.Refresh` 返回 typed `sdk.ErrUnauthorized` 才保留原缺 token 提示；其他 public SDK 错误保留安全 code/operation/backend。成功输出、tool/structured shape、`IsError` 和日志语义不变；`CHANGELOG.md` 已记录用户可见修复。
- 证据：第一条 public in-memory MCP tracer 在旧代码上实际 RED：factory canary error 得到 `错误：未设置 refresh token。请先使用 set_refresh_token 工具设置 token。`，而预期为脱敏 SDK 初始化失败；随后逐项完成 8 个 RED→GREEN 场景。`go test ./internal/mcpserver -run 'TestRefreshToken' -count=1 -v`、`go test ./internal/mcpserver -count=1`、`go test -race ./internal/mcpserver -run 'TestRefreshToken' -count=1`、`go vet ./internal/mcpserver`、gofmt 与 `git diff --check` 全部通过。proxy username/password、URL、token 和配置路径 canary 均未泄漏。独立 spec review APPROVE；quality review 无 Critical/Important，仅要求修正 changelog 的过度声称，修后窄复审 APPROVE。
- 风险/下一步：受信任的自定义 `SDKClient` 若违反 public SDK 约定，在 `Refresh` 返回任意非 typed error，仍沿用既有详情文本；生产 public SDK 只返回安全 typed error，本 task 未扩大该兼容边界。下一任务 T02 只补 application 聚焦测试，不改变业务行为。

## T02 — 补齐 application token-sensitive 用例覆盖

- 状态：已完成
- 范围：P1-2。通过 fake public SDK/config store 覆盖 `LoginService.Start/Complete`、`AccountService.Remove/Use`、`ConfigService.Get/Set/Unset` 的成功与关键错误传播。
- 验收：测试只观察 application 公共接口；覆盖 PKCE session handoff、账号选择/删除后的默认 UID、nil store 与依赖错误；`go test ./internal/application -cover -count=1`。
- 实际：新增 `login_test.go`、`config_test.go` 并扩充 `account_test.go`；只经 application 公共方法与 public SDK/config fake 覆盖 Login Start/Complete、Account Remove/Use、Config Get/Set/Unset。三组 characterization 测试首次均为 GREEN，未伪造行为 RED；未改生产代码。application 语句覆盖率由 29.6% 提升至 70.4%。
- 证据：规格审查与质量审查均 APPROVE；`go test ./internal/application -run 'Test(LoginService|AccountService(Remove|Use)|ConfigService)' -count=1 -v`、`go test ./internal/application -cover -count=1`、`go test -race ./internal/application -count=1`、`go vet ./internal/application`、`gofmt -d ...`、`git diff --check` 全部通过；质量审查另以 `go test ./internal/application -count=20` 验证稳定性。
- 风险/下一步：聚焦测试不执行真实 OAuth 网络交换；真实 PKCE 一次性与交换语义继续由 public SDK 外部测试覆盖。fake 嵌入 nil `SDKClient`，若未来误调用未覆写方法会 fail-loud。下一任务 T03 处理 transport typed error 安全分类。

## T03 — 安全分类上游 transport 失败

- 状态：已完成
- 范围：P2-3。为 DNS、TLS、代理拒连、连接拒绝/重置等增加不含 URL/host 的稳定 transport 子类，并映射到公开 typed error。
- 验收：protocol/public SDK 测试逐类 RED→GREEN；context cancellation/deadline 的 `errors.Is` 保持；未知 transport 仍安全；无 token/URL 泄露。
- 实际：`protocol.Transport` 现仅依据标准库 typed/wrapped cause，把非 context 传输失败分类为 `dns`、`tls`、`proxy`、`connection_refused`、`connection_reset` 或 `unknown`；`proxyconnect` 优先于其内层拒连 errno。公开 SDK 新增 `TransportKind` 与 `Error.TransportKind`，顶层 code/sentinel、operation/backend/status/retryable、fallback/timeout/retry 语义不变；`Error()` 只渲染白名单枚举，非法公开字段值归一为 `unknown`。取消/deadline 继续以 `errors.Is` 判断且不设置子类；`Download` operation remap 保留 kind。`docs/sdk.md` 与 `[Unreleased]` 已同步。
- 证据：DNS、TLS、proxyconnect、ECONNREFUSED、ECONNRESET、unknown 均实际逐项 RED→GREEN；TLS record header/alert/certificate wrapper 初始实际落入 unknown，补 typed 分类后 GREEN；Download remap 初始实际丢失 kind，修后 GREEN；context 回归首次即 GREEN。规格审查发现任意公开 `TransportKind` 会被 `Error()` 回显，外部 canary 实际 RED 显示完整恶意 URL/token，白名单修复后窄复审 APPROVE；质量审查无 Critical/Important/Minor 并 APPROVE。`go test ./... -count=1`、`go test -race ./pixiv ./internal/pixiv/protocol -count=1`、`go vet ./pixiv ./internal/pixiv/protocol`、gofmt、`git diff --check` 全部通过；protocol 通过 Windows/Linux amd64 交叉编译，质量审查另通过 20 次重复测试及 Linux/Windows/FreeBSD/WASM 编译。
- 风险/下一步：标准库未提供 typed cause 的 HTTPS proxy CONNECT 非 200 文本错误仍安全归为 `unknown`；刻意不解析可能包含地址或凭据的错误文本。下一任务 C01 集中复查 T01–T03 的错误透明性、脱敏、覆盖率与全量门禁。

## C01 — 集中检查 1（T01–T03）

- 状态：已完成（发现 Important 修复项）
- 检查：错误透明性、脱敏、MCP result/error observability、application coverage、全量 test/vet/race 聚焦、文档/changelog 是否同步。
- 实际：集中复核 T01–T03 的 commit range、公开错误契约、MCP 结果/日志、application 测试边界、文档与跨层依赖。T01 open/factory、T02 public application tests、T03 typed transport 分类均符合各自验收；legacy MCP `IsError=false`/success log 的既知 observability 债仍按计划留给 T13。独立审计发现 1 个 Important：`client.Refresh` 返回非 context、非 unauthorized、非 `*sdk.Error` 的任意错误时，fallback 仍以 `%v` 写入 MCP 输出，可能泄露 proxy userinfo、完整 URL、token 或配置路径；这是 T01 脱敏范围内的遗漏，已插入 F01。
- 证据：独立 checkpoint reviewer 逐项复核并以该 Important 暂拒 APPROVE；主线程源码复核确认 `internal/mcpserver/server.go` 的 unknown Refresh 分支仍格式化 raw error。`go test ./... -count=1`、`go vet ./...`、`go test -race ./internal/mcpserver ./internal/application ./internal/pixiv/protocol ./pixiv -count=1`、`go test ./internal/application -cover -count=1`（70.4%）、`sh scripts/build.sh`、开发 binary `version --json`、聚焦 refresh/transport tests、base..HEAD `git diff --check` 全部通过；CLI/MCP 未新增对协议子包的直接 import，未发现 debug residue，构建产物已清理且工作树恢复干净。
- 风险/下一步：C01 的测试/构建门禁通过，但 unknown Refresh 原始错误泄露尚未关闭，因此不得把 T01–T03 组合标为无风险。下一轮先执行 F01 的 public MCP canary RED→GREEN，并经过 spec/quality review；T04 必须等 F01 关闭后再开始。

## F01 — 关闭 C01 的 MCP Refresh unknown-error 泄露

- 状态：已完成
- 来源：C01 Important。`client.Refresh` 的 unknown raw error fallback 仍把 `%v` 暴露给 MCP 调用方。
- 范围：只把非 context、非 typed unauthorized、非安全 `*sdk.Error` 的 Refresh 错误改为固定、可操作且脱敏的提示；不得改变 open/factory 分支、typed SDK 诊断、missing-token 兼容文案、成功输出、legacy structured/text shape、`IsError` 或日志语义。
- 验收：public in-memory MCP test 以包含 proxy username/password、完整 URL、token、Cookie 和配置路径的 raw canary 实际 RED→GREEN；输出及日志均不含 canary；refresh 聚焦、mcpserver 全包、race、vet 与全仓测试通过；独立 spec/quality review APPROVE。
- 实际：`client.Refresh` 的 unknown fallback 不再格式化任意 `err`，改为固定可操作文案“Token刷新失败。请检查 refresh token 是否有效，以及网络连接或代理设置。”；context、deadline、typed unauthorized、安全 `*sdk.Error`、open/factory、成功分支与 legacy result/log shape 均未改。新增 public in-memory MCP canary test 同时检查 text content、structured output 和实际非空注入日志；`[Unreleased]` 现准确说明 unknown Refresh execution error 不再回显原始详情。
- 证据：只加测试时旧实现实际 RED，输出完整包含 proxy username/password、地址、query token、Cookie、refresh token 与配置路径；移除 `%v` 后 GREEN，所有 canary 在 Content、StructuredContent 和日志中均不可见。独立 spec review 与 quality review 均 APPROVE且无 finding；质量审查另通过聚焦测试 20 次与 race 5 次。主线程复验 `go test ./internal/mcpserver -run '^TestRefreshToken' -count=1 -v`、mcpserver 全包、聚焦 race、vet、gofmt、`git diff --check` 和 `go test ./... -count=1` 全部通过。
- 风险/下一步：公开 `*sdk.Error` 仍按 public SDK 的稳定安全诊断契约输出；任意外部 adapter 主动伪造其公开字段属于受信注入边界。C01 的 Important 已关闭；下一任务 T04 分类本地 snapshot/auth/config 真因。

## T04 — 分类本地 snapshot/auth/config 真因

- 状态：已完成
- 范围：P2-4。替换 `localSnapshotError(operation, _ error)` 的无差别抹除；保留文件不存在、权限、JSON/TOML 解析、代理 URL、账号不一致等安全分类，同时不暴露 token 或文件内容。
- 验收：`OpenDefault` 公共操作测试覆盖代表性本地失败，operation/code/cause 可诊断且 secret-safe。
- 实际：公开 SDK 新增 additive `LocalStateKind` 与 `auth_malformed`、`config_malformed`、`permission_denied`、`not_found`、`invalid_proxy`、`account_mismatch`、`unavailable`、`unknown` 八个稳定子类；顶层 `CodeInvalidArgument`/sentinel、operation、backend、user ID 与 retryable 语义保持兼容。default/account/login 的 path、auth、config、proxy 边界使用私有 stage marker 保留分类上下文，公开错误只携带固定脱敏 cause；filesystem permission/not-exist 优先，其他 `PathError`、`LinkError`、`SyscallError` 与 wrapped `syscall.Errno` 归为 unavailable，解析/schema 错误仍按 auth/config malformed 分类。两处 OAuth identity mismatch 均保留 OAuth backend 与所选 user ID 并标为 account mismatch；Download operation remap 保留 kind。非法公开 kind 只输出 `unknown`，缺失的可选 auth/config 文件仍正常加载为空状态；SDK 文档与 `[Unreleased]` 已同步。
- 证据：malformed config 首先因公开字段缺失实际 compile-RED，malformed auth 与 invalid proxy 先实际落入 unknown 后逐项 GREEN；account mismatch 原先 kind 为空、Download remap 原先丢 kind，均实际 RED→GREEN。质量审查进一步复现非 permission `LinkError`/`SyscallError`/wrapped errno 被误标 malformed，以及 hostile `https_proxy` 覆盖 fixture 并进入 OAuth transport；新增聚焦测试后全部 GREEN，invalid-proxy 用本地 OAuth atomic counter 证明请求数为 0，handler goroutine 也不再调用 `Fatal`。独立 spec review APPROVE；quality review 提出的两个 Important 和一个 Minor 经原实现代理修复后窄复审 APPROVE。主线程复验 T04 聚焦集、hostile lower/upper proxy 环境、`go test ./pixiv -count=1`、`go test -race ./pixiv -count=1`、`go vet ./pixiv`、Windows/Linux amd64 CGO-disabled 交叉编译、`go test ./... -count=1`、gofmt、pre-commit 配置存在性与 `git diff --check` 全部通过。
- 风险/下一步：真实 filesystem permission 在跨平台 public 测试中不稳定，分类优先级使用标准库 typed synthetic error 验证；config/auth/proxy/mismatch 主路径由 public `OpenDefault`/账号操作覆盖。私有 marker 内部仍包装 raw error，但规格审查逐条确认所有生产调用路径在公开返回或日志前都经安全映射。下一任务 T05 只处理 CLI download composition root 与 bootstrap 纯逻辑测试。

## T05 — 统一 CLI 下载 composition root 并补 bootstrap 纯逻辑测试

- 状态：已完成
- 范围：P2-5 与 P2-13。CLI 不再内联 `download.NewManager`；经 application service/factory 与 bootstrap production wiring 注入；覆盖 runtime config/proxy override 和 factory 错误。
- 验收：CLI/MCP 共享受控构造链；无协议反向依赖；相关 application/bootstrap/CLI 测试 RED→GREEN。
- 实际：application 新增 protocol-free `DownloadClient`、稳定下载 DTO、manager/factory port、request 与 `DownloadService`；service 将同一 SDK operation snapshot、context、IDs、下载路径和文件名模板交给注入 manager，并对 missing/typed-nil client、factory、manager fail-loud。`internal/download` 以 alias 复用 application client port 与 DTO，保持旧内部名称、MCP source compatibility 和 CLI JSON shape。CLI 已删除 `internal/download` import，只保留参数/runtime/flag override、`SDK.OpenOperation`、service 调用和 presenter；bootstrap 私有 `newDownloadManager` 成为唯一 production `download.NewManager` 调用点，供 CLI service、MCP runtime 与 MCP snapshot factory 共用。`LoadRuntimeConfig` 和 proxy override 纯逻辑测试已补齐，架构文档同步 DownloadService 职责；无用户行为变化，未记 changelog。
- 证据：application port 首先因类型不存在实际 compile-RED，factory sentinel 随后 GREEN；missing factory、nil client、nil manager 分别从 panic/错误行为 RED 转为明确错误，CLI `Services.Download` 缺失也实际 compile-RED 后经 public `RunContext` GREEN。质量审查发现 typed-nil interface 仍可穿透保护：typed-nil client 曾调用 factory 且返回 nil，typed-nil manager曾触发 panic canary；增加只检查 nil-able reflect kind 的 helper 后两项 RED→GREEN，且 download 重复 client method set 改为 application alias，窄复审 APPROVE。runtime defaults、file/env priority、malformed TOML、proxy nil/empty/nonempty 与 production factory是既有正确行为的 characterization，首次 GREEN；旧 commit 可复现 CLI direct import/NewManager，当前 AST guard 与 import/`rg` 门禁 GREEN。独立 spec review APPROVE；quality review 修复后 APPROVE。主线程复验 application/bootstrap/CLI 聚焦测试、真实 TLS/proxy download 开启与显式清空两支、相关五包普通/race/vet、`go test ./... -count=1`、application Linux/Windows amd64 CGO-disabled 测试二进制编译（ELF/PE）、pre-commit、gofmt 与 `git diff --check` 全部通过；bootstrap statement coverage 为 17.7%（评审基线 10.2%）。
- 风险/下一步：bootstrap/CLI 的 CGO-disabled 异构编译仍会被仓库既有 `ugoira_rust_stub.go` 主动拒绝，要求 CGO、目标 Rust staticlib 和 linker；application port 已完成 Linux/Windows compile，本次 native 全仓/五包门禁通过。T19 仍负责剩余 bootstrap production factory/评审矩阵复扫；下一任务 T06 只处理 MCP 推荐下载 count 的静默钳制。

## T06 — 移除 MCP 推荐下载静默 count 钳制

- 状态：已完成
- 范围：P2-6。为默认 count 提供明确产品语义；超出有依据范围时返回显式参数错误，不静默改为 20；同步 MCP 文档和 changelog。
- 验收：count 缺省、非正数、合法值、超限值均有 public tool 测试；无合法请求被静默截断。
- 实际：`download_random_from_recommendation.count` 改为可选整数：省略或 `null` 默认 5，显式值仅接受 1..20；0、负数和大于 20 的值在打开 SDK、请求推荐或创建/调用下载 manager 前返回稳定参数错误，不再静默改写。20 约束单次请求作品数，依据是多作品与每作品多页/多文件会放大下载工作及同一 structured response 的文件元数据，不截断单作品文件；推荐不足时使用实际可用项。delivery 校验仍优先，count 错误保留规范化 delivery，错误输出显式提供空 `items/files` 以满足 public output schema。MCP schema、canonical tool 文档、README、架构说明与 changelog 已同步，并澄清 random tool 当前不附加 ImageContent。
- 证据：旧实现对显式 `count=0` 实际打开 SDK、请求推荐并下载默认 5 项，对 `count=21` 实际下载 20 项，形成两条真实 public-tool RED；最小实现后转 GREEN。负数、缺省、`null`、合法 1、最大 20、推荐不足、schema optional/default/range 均有 public in-memory MCP 测试；随机选择断言集合与数量而非顺序。spec review 发现 `count=0 + image_content` 的 structured delivery 漂移，补测先 RED 后修复；组合测试又暴露非法 delivery 被 `items:null` output-schema 错误遮蔽，补为显式空数组后 GREEN。独立 spec review APPROVE；quality review 发现上限依据误称 random 会附加 ImageContent，收窄代码注释与文档后窄复审 APPROVE。主线程复验 `TestDownloadRandom` 单次与 20 次、MCP package/race/vet、`go test ./...`、pre-commit、gofmt 与 `git diff --check` 全部通过。
- 风险/下一步：为保持既有 legacy wire 兼容，参数错误仍是 `IsError=false` 的文本与 structured result；T13 负责统一 legacy 错误可观测性。random tool 的 `delivery=image_content` 当前不附加 ImageContent，已在 canonical 文档明确，本任务未扩展该既有能力。下一任务 C02 集中复核 T04–T06 的 composition boundary、错误真因、限制依据、CLI/MCP 契约与全量门禁。

## C02 — 集中检查 2（T04–T06）

- 状态：已完成（发现 P1 修复项）
- 检查：composition boundary、错误真因、无据限制、CLI/MCP 契约、全量 test/vet、文档和 changelog。
- 实际：集中复核 `2e1f9ca..d2be109` 的 T04–T06 生产代码、公开测试和文档。T04 的 typed-first 本地状态分类、白名单错误渲染、Download remap 与脱敏边界，T05 的 application 下载 port、bootstrap 唯一 production wiring、typed-nil fail-loud、CLI JSON/proxy/runtime 行为，以及 T06 的 count/default/range/delivery/限制依据均符合各自契约；没有新增 timeout、retry、fallback 或单作品文件截断。独立 checkpoint review 发现 1 个 P1：typed `downloadOut` schema 要求 `items/files` 为 array，但 direct `download` 的 no-ID、非法 delivery、manager/build/read-file 失败，以及 random 的 SDK-open、推荐失败、空推荐、manager/build 失败仍返回 nil slices（random 多数还返回 nil result），会把真正错误遮蔽为 output-schema 错误；部分 random 错误还把已规范化 `image_content` 重置为 `local_path`。已插入 F02。
- 证据：生产 `download.NewManager` 经 `rg` 仅剩 `internal/bootstrap/bootstrap.go` 一处；`go list`/import 检索确认 CLI/MCP/application 未直连 appapi/webapi/oauth/resource。go-sdk v0.8.0 `mcp/server.go` 明确对所有 typed handler output marshal 后执行 `applySchema`，nil slice 序列化为 `null`；T06 的 invalid-delivery public test 已以同一机制实际 RED 为 `items: type null, want array`，现代码审计证明其余列举分支仍有相同形状。独立 reviewer `REQUEST_CHANGES` 且除该项外无 finding。主线程与 reviewer 通过 `git diff --check 2e1f9ca..HEAD`、T04/local-state 聚焦重复测试、T05 application/bootstrap/CLI/download 测试、`TestDownloadRandom` 20 次、相关六包 race、`go test ./... -count=1`、`go vet ./...`、`sh scripts/build.sh`、开发 binary `version --json` 和 `pre-commit run --all-files`；application/bootstrap coverage 分别为 73.5%/17.7%，构建产物已清理且 worktree 恢复干净。
- 风险/下一步：P1 未关闭前，下载参数错误、认证/推荐失败、空推荐或本地下载失败仍可能只暴露 schema 错误，因此不得把 C02 组合标为无风险。下一轮先执行 F02：用 public in-memory MCP tracer bullets 覆盖 direct/random 全部失败与空结果类别，统一显式空数组、legacy-compatible result 和规范化 delivery；T07 必须等 F02 关闭后再开始。真实 Pixiv 联网 e2e 未启用；默认 e2e package 已通过。T05 结构变化造成的两份知识图谱滞后按 T20 集中重建。

## F02 — 修复 MCP 下载错误被 output schema 遮蔽

- 状态：已完成
- 来源：C02 P1。typed `downloadOut` 的 `items/files` 必须是 JSON array，现有多个失败/空结果分支返回 nil slices，导致 go-sdk output validation 覆盖真正错误。
- 范围：为 direct `download` 与 `download_random_from_recommendation` 的所有参数错误、SDK/推荐失败、空推荐、manager/download 失败、结果整理失败和 image-content 读取失败统一构造显式空 `items/files` 与 `downloadResult(out)`；完成 delivery 规范化后保留规范化值，尚未规范化或非法 delivery 继续使用 `local_path`。保持现有安全错误文本、legacy `IsError=false`、日志语义和成功输出；不得给 random 新增 ImageContent，也不得顺手处理 T13 observability。
- 验收：public in-memory MCP tracer bullets 逐类实际 RED→GREEN，至少覆盖 direct no-ID/invalid-delivery/download/build/read-file 与 random SDK-open/recommendation/empty/download/build；每个分支返回原有业务错误而非 `validating tool output`，structured `items/files` 是空数组，delivery 正确，失败前不发生不应有的下游调用；MCP package/race/vet、全仓测试、文档/changelog 审核和独立 spec/quality review 通过。
- 实际：新增 `emptyDownloadResult`，让 direct `download` 与 `download_random_from_recommendation` 的参数、SDK/推荐、空推荐、下载、结果整理和文件读取失败统一返回显式空 `items/files`、原业务文本与正确 `delivery`；非法或尚未规范化的 delivery 使用 `local_path`，其余保留规范化值。成功路径、legacy `IsError=false`、日志语义与 random 不附加 ImageContent 的行为未改变。同步更新 README（英文）、`docs/mcp-tools.md` 与 CHANGELOG。
- 证据：10 个 public in-memory MCP tracer tests 均先实际 RED，错误为 typed output 把 `items:null` 判为非 array 并覆盖业务错误，随后 GREEN；覆盖 direct no-ID/invalid-delivery/manager/build/read-file 与 random SDK-open/recommendation/empty/manager/build。既有 random 非法 count/delivery 测试首次即 GREEN，未伪造 RED。补充 `downloadCalls` 断言后，前置失败确认零次下载调用，manager/build/read 失败确认恰好一次调用。独立 spec 复审与独立 quality review 均 APPROVE、无 P0/P1/P2；后者另验证 release、无部分 ImageContent 泄露及无范围外变更。主线程通过 focused `TestDownload`、`-count=20`、MCP package、MCP race、`go vet ./...`、`go test ./... -count=1`、`pre-commit run --all-files` 与 `git diff --check`。
- 风险/下一步：文件系统错误 fixture 会按运行平台动态生成期望文案，但尚未在六个平台实际执行；留待 T21 跨平台矩阵验证。失败仍保持 legacy `IsError=false`，按既定 T13 单独处理。真实 Pixiv 联网 e2e 不属于本修复且未启用。下一轮开始 T07；T05/F02 后的知识图谱滞后仍按 T20 集中重建。

## T07 — 删除已证实死代码和诱导性 token 字段

- 状态：已完成
- 范围：P2-7 与 P3 dead-code 清单。删除无生产调用方的 `download.Manager.Enqueue` 后台队列及其固定并发/吞错/不可取消链；逐项验证并删除 `malformedError`、`transportError`、cursor helpers、appapi helpers/不可达 auth branch、`RuntimeConfig.RefreshToken` 或记录仍可达证据。
- 验收：调用图无生产引用；删除后相关包与全量测试通过；配置 schema/序列化不出现 refresh token。
- 实际：删除 `download.Manager.Enqueue`，并连同仅服务该入口的固定容量 5 semaphore、`context.WithoutCancel` goroutine、`downloadOne` 吞错链一起移除；MCP 内部 `DownloadManager` method set 与测试 fake 同步收窄。删除无调用的 `malformedError`、`transportError`、无 source cursor wrappers、`getMapped` 和 `baseHeaders`。GET/POST 重试现共用只识别 typed HTTP 401/403 的 `isAuthAPIResponse`，保留认证 refresh/replay 并删除不可达字符串匹配 fallback。删除无读写方、从未进入 setting specs/TOML 的 `RuntimeConfig.RefreshToken`；auth storage、环境变量、SDK request/options、OAuth、CLI 与 MCP 的合法 token 能力保持不变。纯内部清理无用户可见行为变化，因此未改 changelog/文档；知识图谱留待 T20 重建。
- 证据：`TestRuntimeConfigSurfaceExcludesRefreshToken` 在旧字段存在时真实 RED（runtime DTO JSON surface 含 `RefreshToken`），删除字段后 GREEN；该测试同时用含 `[auth].refresh_token` 的 TOML fixture 验证值不进入 runtime DTO，并检查 aliases/setting surface 不提供 token 配置。GET typed 401/403 恰好 refresh 一次并总请求两次、non-auth 502 正文含 `oauth unauthorized invalid_grant` 仍只请求一次且不 refresh/replay，均为既有正确行为的首次 GREEN characterization，未伪造 RED。精确全仓检索确认全部目标符号、`.Enqueue(`、semaphore、`downloadOne` 与 `context.WithoutCancel` 零残留，source-aware cursor 调用仍完整。独立 spec review 与 quality review 均 APPROVE、无 finding；主线程通过聚焦测试（appapi 20 次）、五包 race、`go vet ./...`、`go test ./... -count=1`、九文件 gofmt、`git diff --check` 和精确调用图门禁。
- 风险/下一步：RuntimeConfig 当前不参与生产 JSON 序列化，JSON 仅作为稳定可枚举的 DTO surface 测试手段；TOML 输入与 alias 另有独立断言。真实 Pixiv 联网 e2e 未启用，本任务不改网络 endpoint/fallback。未单独运行会生成范围外 build 产物的 `scripts/build.sh`，全仓测试已完成当前平台编译链接；跨平台矩阵留待 T21。下一轮执行 T08；结构删除后的两份知识图谱继续按 T20 集中更新。

## T08 — 审计更新 SemVer fail-closed 策略并拆出模块

- 状态：已完成
- 范围：P2-8 与 P3 semver 拆分。核对 release workflow 是否已对所有受信发布入口强制 SemVer；若完整则保留 strict reject、补 fail-closed 依据和跨 verifier 测试，若不完整才实现安全 skip + 可观测诊断。将解析/比较从 `releases.go` 拆到 `semver.go`，保持合法版本排序和 prerelease 策略。
- 验收：混合合法/非法 tag、全非法、stable/prerelease 与 workflow tag policy 均有测试；策略选择有代码注释和文档证据；不把网络/签名失败伪装成无更新。
- 实际：审计确认仓库内唯一 Release 创建点是 `.github/workflows/release.yml`：tag push 与必填 `workflow_dispatch.release_tag` 共用 `RELEASE_TAG`，validate/test build/production build 均校验同一 SemVer tag，publish 再以同一值判定 channel。故保留 selected-channel strict fail-closed：stable 仍先忽略 GitHub 标记的 prerelease，显式 prerelease 则验证它们；当前通道出现非 SemVer published tag 被视为发布入口绕过或 policy 漂移，返回含 tag 的错误而不静默选择旧版本。SemVer 类型及 parse/validate/render/compare、build metadata、无界数字比较已从 `releases.go` 原样移至 `semver.go`。release workflow 把 validator 拆为固定位置、只含精确命令的 canonical bash step，policy 用既有 canonical-step 校验拒绝条件、软失败和额外命令；ADR 0008 记录取舍并链接既有开发门禁。未改变网络、HTTP、cache、签名、安装或 checker 错误语义；既有行为与内部重构不记 changelog。
- 证据：首个跨 verifier tracer 把 validator 改为 `... || true` 时，旧 policy 实际返回 nil（RED），改为精确 command 检查后 GREEN。质量复审又证明旧多命令 step 会接受 `if:false`、`continue-on-error:true` 与 `if false; then ... fi` 三种 bypass；新增 table mutation 后三项均真实 RED，拆出 canonical dedicated step 后 GREEN，固定版本、`|| true` 与额外命令也均被拒绝。mixed valid+invalid、多个 all-invalid、stable/prerelease、build metadata、超 `uint64` SemVer 数字均经 public `GitHubReleaseClient.Check` 覆盖；新增 selector cases 为既有 strict 行为的首次 GREEN characterization，错误含非法 tag 且 result 无 Release。spec review 首轮要求 mixed case 补 nil Release 断言，修后窄复审 APPROVE；quality review 的 P1 policy bypass 修复后窄复审 APPROVE。主线程通过 selector 与 policy 聚焦测试各 20 次、`sh scripts/test-release-workflow.sh`、update/releaseworkflow race、`go vet ./...`、`go test ./... -count=1`、gofmt、`git diff --check`、唯一创建入口检索，以及新 `semver.go` 与 HEAD 原实现块逐字等价检查。
- 风险/下一步：本任务证明的是入库 workflow 与本地 policy；远端 Environment protection、tag protection 和实际 GitHub runner 配置仍须在 T21 远端终审/六平台 CI 复核。真实发布没有被触发，既有 tag/Release 均未改动。release workflow 结构与 SemVer 文件拆分会使两份知识图谱滞后，仍按 T20 集中重建。下一轮执行 T09 的 config/auth 持久化原子性与 durability。

## T09 — 加固 config/auth 持久化原子性与 durability

- 状态：已完成
- 范围：P3 一致性。`config.toml` 使用同目录临时文件、权限、Sync、原子替换；`auth.json` 在 rename 前 Sync，并按平台安全处理目录 durability。
- 验收：临时目录集成测试覆盖成功、写/Sync/rename 失败时旧文件保留、权限不放宽、无临时残留；Windows 行为不伪造 POSIX 保证。
- 实际：`config.toml` 与 `auth.json` 现共用 `internal/utils/files.WritePrivateFile`：在目标同目录以随机名 staging，设置平台对应权限，完整写入、file `Sync`、关闭后才原子替换。Unix-like 替换后同步父目录；Windows 首次创建使用不覆盖目标的 `MoveFileEx`，既有目标使用带同目录唯一 recovery backup 的 `ReplaceFileW`，不伪造 POSIX directory fsync 或 DACL 保证。Windows 1177 部分完成会恢复旧 target；恢复失败则显式保留 old backup 与 new source，避免清理逻辑造成二次数据丢失；已提交后的 backup 清理错误仍继续执行并合并 parent durability。Unix-like 主动使用 `0700`/`0600`；Windows 首次创建继承父 ACL、替换既有目标保留目标 ACL，文档与测试均不再把 mode bits 当作 ACL 证据。同步更新 README、architecture、development 与 CHANGELOG。
- 证据：1177 恢复成功用例先因缺少 outcome/recovery 模型真实 compile-RED，再 GREEN；Windows 生产接线先 cross-compile RED，再 GREEN。写失败、file Sync 失败、replace unchanged、短写、首次创建替换失败、1177 恢复成功/失败、已提交 backup 清理失败与 parent sync 合并共 10 个状态/失败用例连续运行 20 次通过；config `set/unset` 与 auth `Save/Load` 集成测试验证数据、Unix-like 权限及正常路径无临时残留。独立 spec review APPROVE；quality review 首轮发现 Windows 1176/1177 数据丢失与 ACL 过度承诺，修复后 P1 窄复审闭合，P2 文档/旧测试残留再修后最终 APPROVE。主线程通过相关三包普通/race/vet、Windows/Linux amd64 三包交叉编译、`go test ./... -count=1`、gofmt、`git diff --check` 与 `pre-commit run --all-files`。
- 风险/下一步：Win32 错误码分类已由 Windows build-tag 测试覆盖并交叉编译，但本轮未在真实 Windows 文件系统注入 1176/1177；留待 T21 六平台矩阵复核。1177 自动恢复也失败时，为保护数据会刻意残留 recovery backup 与 source temp，此状态不适用“无临时残留”，需按组合错误人工恢复。Windows 主动配置私有 DACL 不在本任务范围，当前保证已明确限定。下一轮执行 C03；结构变化后的两份知识图谱仍按 T20 集中重建。

## C03 — 集中检查 3（T07–T09）

- 状态：已完成（发现 P1/P2/P3 修复项）
- 检查：死代码证据、更新 fail-open/fail-closed 边界、文件原子性/权限/掉电风险、race、全量 test/vet、回滚路径。
- 实际：集中复核 `0f92eb5..447b568` 的 T07–T09 生产代码、测试、发布 workflow 与文档。T07 的目标死符号及不可达 auth 字符串 fallback 已删除，source-aware cursor 与合法 refresh-token 入口、typed 401/403 refresh/replay 保持；T08 的唯一仓库内 Release 创建入口、canonical SemVer validator、selected-channel strict fail-closed 与 policy bypass 防线闭合；T09 的 config/auth 私有 writer 对同目录 staging、完整写入、file Sync、关闭、替换、Unix-like `0700`/`0600`、目标目录 fsync 及 Windows private recovery/ACL 限定成立。集中检查同时发现三项遗漏：P1 是 public `ReplaceFile`/`ReplaceFileWithBackup` 丢弃 Windows 1177 restore-fail 的 `preserveSource` outcome，resource、ugoira、updater 与 release cache 的错误清理都会删除新 artifact；P2 是首次 `MkdirAll` 新建配置目录后未同步该目录在外层父目录中的新 entry；P3 是 `docs/architecture.md` 仍描述 T07 已删除的 `Enqueue` 与固定并发 5 semaphore。已插入 F03，阻断 T10。
- 证据：目标 dead symbol 精确 `rg` 零命中；`RuntimeConfig` 无 token 字段而 auth store、环境变量、SDK options、CLI/MCP 合法入口仍在；appapi GET/POST 仅以 typed HTTP 401/403 触发 refresh/replay。仓库生产 `gh release create` 仅在 `.github/workflows/release.yml`，tag push/必填 dispatch 共用 `RELEASE_TAG`，validate/build/production/publish 与 ADR 0008 一致；selector、超 `uint64` SemVer 与 workflow mutation tests 通过。P1 由 `replace_windows.go` 丢弃 outcome 到 `resource.go`、`ugoira_encoder.go`、`installer.go` 与 `releases.go` 的 cleanup 数据流直接证明；其中 release cache defer 的 `if removeErr := os.Remove(temporaryPath); ...` 会无条件先执行 init，named `err` 只控制是否合并 remove error，不能保护 source。P2 由 `files.go` 只调用 `syncParent(filepath.Dir(path))`、没有同步新建目录外层父 entry 直接证明。独立 checkpoint auditor REQUEST_CHANGES，独立 spec reviewer确认三项均属于 C03/T07–T09。主线程通过 T07 五包、T08 selector 20 次、release workflow、T09 十类故障状态 20 次、Windows/Linux 交叉编译、`go test ./... -count=1`、`go test -race ./... -count=1`、`go vet ./...`、`sh scripts/test-release-workflow.sh`、`sh scripts/build.sh` 与开发 binary `version --json`；构建产物已清理，worktree 恢复干净。`git show` 确认 T07 `9b2a5f4`、T08 `8a614d0`、T09 `447b568` 分属 dead-code、release、storage 三个提交，生产文件无交叉依赖；整体回滚以逆提交顺序最稳妥，单独回滚文档/`tasks.md` 冲突须人工保留较新事实。
- 风险/下一步：在 F03 完成前，极端 Windows 1177 且自动恢复失败会让目标路径缺失、旧内容仅留 recovery backup，并被部分调用方继续删除新 source；首次成功创建 config/auth 目录也没有完整的掉电持久性证据。下一轮必须先执行 F03：以 public/caller-visible recovery contract 逐个 RED→GREEN，补新建目录链 fsync 顺序/失败测试并删除旧架构陈述；经过 spec/quality 窄复审和全量门禁后才能开始 T10。F03 将建立在 T09 的 outcome/durability 模型上，未来若必须撤销 T09，应先撤销 F03、再撤销 `447b568`；这会重新暴露原 config/auth 掉电风险，只是紧急回滚路径而不是等价修复。T08 与 T07 的生产改动可再按 `8a614d0`、`9b2a5f4` 逆序撤销，且不得移动不可变 v0.3.0 tag。真实 Windows 故障注入仍留 T21，知识图谱仍按 T20 集中重建。

## F03 — 关闭 C03 的 Windows recovery 与首次目录 durability 缺口

- 状态：已完成
- 来源：C03 P1/P2/P3。公共替换 API 丢失 unresolved recovery outcome；首次创建目录未同步外层 parent entry；架构文档残留已删除后台队列。
- 范围：让 `files.ReplaceFile` 与 `ReplaceFileWithBackup` 的调用方可稳定识别“必须保留 source”的恢复状态，并修正 resource、ugoira、updater 与 release cache 的清理所有权。Windows 1177 restore fail 必须同时保留 old backup 与 new source，1177 restore success 恢复旧 target 并清理临时 source，1175/1176/普通失败仍保持旧 target 且不留无用临时文件，成功路径不变。Unix-like 首次创建一层或多层目录时，在最终 file rename 后按可证明顺序同步目标目录及每个新建目录的外层 parent entry；既有目录不新增无依据同步，任何 post-commit sync/cleanup 失败 fail-loud 且不伪装回滚。删除 architecture 的 `Enqueue`/semaphore 陈述，不扩展到 T10 timeout 或其他文件写入协议。
- 验收：按 TDD vertical slices，至少由 caller-visible 测试先 RED 覆盖 resource download、ugoira publish、Windows updater 与 release cache 的 preserve-source cleanup；platform-neutral recovery tests 与 Windows build-tag classifier/交叉编译覆盖 public contract。Unix-like 测试先 RED 覆盖新建单层/多层目录的 fsync 顺序、既有目录仅同步目标目录、各 sync failure 的组合错误与已提交状态；普通失败/成功无临时残留、权限不放宽。相关 package/race/vet、Windows/Linux 交叉编译、全量 test/race/vet、build、pre-commit 和独立 spec/quality review 通过。
- 实际：`internal/utils/files` 新增 `ReplacementSourcePreservationError` marker 与 `MustPreserveReplacementSource`，Windows 1177 的 target restore 也失败时把原 replace/restore cause 包装成支持 `errors.As`、`errors.Is`、`Unwrap` 的 typed error，错误文本不加入 source/target 路径。resource download、ugoira publish、release installer 与 GitHub release cache 的 per-call/per-instance cleanup 只在该 marker 可达时移交 source 所有权并保留恢复材料；普通替换错误仍清理 source，成功与 installer `.old` 语义不变。`WritePrivateFile` 在调用开始记录缺失目录链，replacement committed 后按 leaf→root 同步 target directory 及每个新目录的外层 parent；所有同步均继续尝试并以 `errors.Join` 返回，未提交路径不执行 durability sync，Windows helper 仍明确 no-op。architecture 删除 `Enqueue`/固定 semaphore 旧描述；按 docs 路由只在 architecture 记录内部边界，并在 `[Unreleased]` 合并记录用户可感知的数据保留与首次目录 durability 修复。
- 证据：resource 与 ugoira 首条 caller artifact tests 均先因缺少 per-call seam 真实 compile-RED，再 GREEN；installer 测试先实际 RED 为 staged source 被 defer 删除后的 `os.ErrNotExist`，再 GREEN；release cache 测试先 compile-RED（缺 per-client replacer），再 GREEN。Unix-like 单层新目录测试先实际 RED，观测同步序列只有 `[leaf]` 而预期 `[leaf, outer parent]`，修复后 GREEN；多层顺序、既有目录仅 target、三层 sync failure 全部尝试/`errors.Join`、wrapped marker 与普通 cleanup 在通用实现后首次 GREEN，均如实作为 characterization。独立 spec review 与 quality review 均 APPROVE；spec 另确认 resource 既有外部测试已覆盖普通 ReplaceFile 失败的零临时残留，quality 核对 1175/1176/1177、committed cleanup、relative/root/symlink/concurrent-create 与四处 named/defer 语义。主线程将新增 recovery/durability tests 各运行 20 次，通过四包 normal/race/vet、Windows/Linux amd64 的 files/update/pixiv 交叉编译、`go test ./... -count=1`、`go test -race ./... -count=1`、`go vet ./...`、`sh scripts/build.sh` 与开发 binary `version --json`；构建及交叉编译产物已清理。
- 风险/下一步：真实 Windows 文件系统仍未注入 1177 + restore-fail；当前证据是共享状态模型、Windows classifier/build-tag、caller artifact tests 与交叉编译，真实故障矩阵仍留 T21。`internal/download` 异构编译继续受仓库既有 CGO/Rust staticlib/目标 linker 门禁限制，本机 build/race/vet 已覆盖本 diff。unresolved recovery 会刻意留下 old backup 与 new source，错误返回后调用方或人工流程负责恢复，不能按普通临时残留清理。下一轮开始 T10；结构变化后的两份知识图谱仍按 T20 集中重建。

## T10 — 决定并统一 HTTP client timeout 策略

- 状态：已完成
- 范围：P3 timeout 一致性。先形成 ADR，决定默认由 context/显式 client 控制，或为 60s 提供可配置、可观测的客观依据；随后让 webapi 不再裸用 `http.DefaultClient`，统一 App/OAuth/resource/Web 策略，流式资源不得被整请求固定 timeout 误伤。
- 验收：client 构造测试证明显式 client 优先、默认策略一致、context 语义保持；不新增无据 timeout。
- 实际：ADR 0010 采纳“context/显式 client 控制总生命周期”：移除 App API、OAuth 与 operation proxy snapshot 的固定 60 秒 `http.Client.Timeout`；Web API 与 resource constructor 不再复用全局可变的 `http.DefaultClient`。public `pixiv.NewClient` 在调用方未注入时创建一个专用、零整请求 timeout 的 client，并将同一指针贯穿 App、Web、resource 与账号/OAuth 动态路径；显式 `Options.HTTPClient` 的指针、timeout、transport、jar 与 redirect policy 保持不变。`OpenDefault` 仍按 snapshot 配置克隆 `http.DefaultTransport` 并应用显式代理，只移除 total timeout，不改变标准 transport 的 dial、TLS handshake、idle connection 等阶段策略。resource stream 返回后继续由请求 context 控制 body 读取；未新增 timeout 配置、重试或 fallback。SDK、架构、文档索引与 changelog 已同步。
- 证据：shared/proxy、App、OAuth 的零 timeout tests 在旧实现上分别因 `1m0s` 真实 RED，Web/resource tests 因别名 `http.DefaultClient` 真实 RED，public `NewClient` test 因默认 `HTTP client is nil` 真实 RED；最小实现后全部 GREEN。显式 client identity/timeout、各 adapter 的 context cancellation 与 resource 返回 response 后取消 context 的 body read 属既有语义 characterization，首次 GREEN；既有公开 transport tests 另覆盖 `context.DeadlineExceeded` 的 `errors.Is` 链。生产 Pixiv 路径精确检索无残留 `http.DefaultClient`、`SetTimeout(60...)` 或 `Timeout: 60...`。独立 spec review 与 quality review 均 APPROVE；quality reviewer 将 streaming test 连续运行 50 次。实现代理与主线程通过六包聚焦测试、相关 race/vet、`go test -race ./...`、`go test ./...`、`go vet ./...`、`sh scripts/build.sh`、pre-commit、Linux/Windows amd64 changed-package 交叉编译、gofmt 与 `git diff --check`。
- 风险/下一步：未对真实 Pixiv 长时间资源流执行超过 60 秒的联网 wall-clock E2E；零 timeout 构造断言、context body cancellation 与既有资源回归直接覆盖隐藏 total timeout 的代码路径。调用方若需要操作级 deadline，必须通过每次调用的 context 或显式 client 表达；默认策略仍保留 Go transport 的阶段性保护。下一轮只执行 T11；两份知识图谱继续按 T20 集中重建。

## T11 — 复用分页遍历并收敛 Web page-size 常量

- 状态：未完成
- 范围：P3 重复分页与散落 60/50。把 CLI/MCP 等价 cursor/page/limit 遍历下沉 application 深模块；Web API 用具名、按 endpoint 有依据的 page-size 常量。
- 验收：CLI/MCP 输出、重复 cursor 止环和 `limit=0` 语义不变；application 公共 helper 测试先 RED；匿名 Web 请求 wire 值测试通过。
- 实际：
- 证据：
- 风险/下一步：

## T12 — 输出与文件名小项加固

- 状态：未完成
- 范围：P3。下载推导扩展名经过安全规范化；取消 MCP legacy/SDK text 对 tags 的无依据 5 项截断并完整输出；用稳定 label map 替换 deprecated `strings.Title`，保持用户可见文本可读。
- 验收：恶意/异常扩展名、6+ tags、ranking label 的 public behavior 测试 RED→GREEN；无路径遍历或静默信息丢失。
- 实际：
- 证据：
- 风险/下一步：

## C04 — 集中检查 4（T10–T12）

- 状态：未完成
- 检查：timeout 依据、分页无截断、输出兼容、路径安全、全量 test/race/vet、文档同步。
- 实际：
- 证据：
- 风险/下一步：

## T13 — 拆分 MCP server 并修复 legacy 错误 observability

- 状态：未完成
- 范围：P2-10。按注册/认证/下载/legacy/格式化职责拆 `server.go` 与 `sdk_tools.go`；legacy tool 失败仍保持已文档化 result 兼容，但日志/metrics 必须走 `recordToolError` 而不是 success。
- 验收：MCP JSON-RPC/stdout、structured/text、legacy compatibility 不变；失败 observability 聚焦测试先 RED；拆分后文件职责清晰。
- 实际：
- 证据：
- 风险/下一步：

## T14 — 拆分 macOS 登录 helper 安装器

- 状态：未完成
- 范围：P2-9。把内嵌 Swift/Info.plist/swiftc/lsregister 平台安装逻辑从 673 行 controller 移出到 Darwin 专属文件或独立 package；CLI 只保留 OAuth/TTY 编排。
- 验收：Darwin 与非 Darwin build/test 均通过；回调页、helper 安装、手工 fallback 行为不变；不启动受管浏览器/CDP。
- 实际：
- 证据：
- 风险/下一步：

## T15 — 按职责拆分 webapi client

- 状态：未完成
- 范围：P2-11。机械拆分 transport、分页/枚举、DTO、decoder、mapper；对齐 appapi 结构，不改变 endpoint/wire/fallback。
- 验收：文件规模和职责明显收敛；现有 webapi 与 public SDK tests 全绿；匿名 fallback 规则不变。
- 实际：
- 证据：
- 风险/下一步：

## C05 — 集中检查 5（T13–T15）

- 状态：未完成
- 检查：大文件拆分是否纯机械、MCP stdout/observability、跨平台 build、Web fallback、知识图谱待更新清单。
- 实际：
- 证据：
- 风险/下一步：

## T16 — 重新审计 App/Web enrichment 失败策略

- 状态：未完成
- 范围：P2-12。当前 ADR 0006/0009 与外部测试明确要求 Web enrichment 失败时整体失败。用 R-18/登录墙 fixture、当前 App `MetaPages` 完整性和可用的真实 canary 评估是否应改为 partial result；若改，必须先形成 ADR、显式 enrichment 状态与兼容测试；若不改，以充分证据确认这是已采纳设计而非缺陷，并增强 SDK/错误文档。
- 验收：App 成功/Web enrichment 失败、App 失败、匿名路径均有确定性测试；无静默 fallback；策略有 ADR 和用户可观察语义；真实联网可用时补充但不替代 fixture。
- 实际：
- 证据：
- 风险/下一步：

## T17 — 合并 releaseworkflow/nativeevidence YAML policy helper

- 状态：未完成
- 范围：P3 重复 YAML helper。提取内部共享 package，保持 release/native evidence fail-closed 校验、错误文本和测试 mutation coverage。
- 验收：两个 verifier 使用同一 helper；policy tests、release workflow/Rust vendor scripts 全绿；不得弱化 allowlist/secret/tag/staticlib 门禁。
- 实际：
- 证据：
- 风险/下一步：

## T18 — 拆分超大 release verifier 文件

- 状态：未完成
- 范围：P3。按 job/policy/native evidence 职责拆 `scripts/releaseworkflow/main.go` 与 `scripts/nativeevidence/main.go`，测试 helper 也按领域拆分；只做机械迁移和必要共享抽取。
- 验收：所有 verifier 输出与退出语义不变；mutation tests 和 release scripts 全绿；文件职责可导航。
- 实际：
- 证据：
- 风险/下一步：

## C06 — 集中检查 6（T16–T18）

- 状态：未完成
- 检查：公开 SDK 兼容/ADR、fallback 不变量、发布供应链门禁、脚本拆分等价性、全量 test/race/vet/release/Rust。
- 实际：
- 证据：
- 风险/下一步：

## T19 — 覆盖剩余 bootstrap 纯逻辑与评审矩阵复扫

- 状态：未完成
- 范围：P2-13 剩余项及全报告复扫。补 `LoadRuntimeConfig`/proxy override/production factory 的高价值测试；逐条 `rg` 和 coverage 验证所有 finding 已关闭，发现遗漏则追加修复任务。
- 验收：bootstrap 聚焦 coverage 明显覆盖纯逻辑而非只追百分比；finding closure matrix 有 file/test 证据。
- 实际：
- 证据：
- 风险/下一步：

## T20 — 同步文档、changelog 与两份知识图谱

- 状态：未完成
- 范围：汇总所有用户可见错误/行为/安全变化到 `[Unreleased]`；更新 README、SDK、MCP、architecture/development/ADR；重建代码与文档图谱及指纹。
- 验收：文档链接有效；图谱无 duplicate/dangling/unassigned；meta/fingerprint 对齐实现提交；文档不保留已删除符号或旧行为。
- 实际：
- 证据：
- 风险/下一步：

## T21 — 全量终审、PR、六平台 CI 与本地收尾

- 状态：未完成
- 范围：独立最终 spec/quality/security/supply-chain review；全量本地门禁；推送 PR，等待 quality 与六平台 smoke，合入后清理分支/worktree并确认 `main==origin/main`。
- 验收：所有 reviewer APPROVE；`go test ./...`、race、vet、pre-commit、release/Rust scripts、真实认证 canary 通过；PR CI 全绿；v0.3.0 tag 不变。
- 实际：
- 证据：
- 风险/下一步：

## C07 — 最终完成审计

- 状态：未完成
- 检查：逐条对照 `input.md`、附件每个 finding、所有 task 验收和外部状态；证据不足一律追加任务，不以“未发现”替代证明。
- 实际：
- 证据：
- 风险/下一步：
