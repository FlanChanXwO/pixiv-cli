# Goal 2 任务清单

每个普通任务必须按 TDD tracer bullet 实施，并依次经过 spec review、quality review。每个任务完成后填写“实际/证据/风险”。

## T01 — MCP refresh_token 保留真实打开错误

- 状态：未完成
- 范围：P1-1。区分可判定的未设置 token 与 context、代理、配置、client factory 等失败；返回脱敏但真实的错误，不把所有失败伪装成缺 token。
- 验收：MCP public tool 测试先 RED；无 token 提示保持兼容，其他错误包含安全真因且不泄露凭据；`go test ./internal/mcpserver -count=1`。
- 实际：
- 证据：
- 风险/下一步：

## T02 — 补齐 application token-sensitive 用例覆盖

- 状态：未完成
- 范围：P1-2。通过 fake public SDK/config store 覆盖 `LoginService.Start/Complete`、`AccountService.Remove/Use`、`ConfigService.Get/Set/Unset` 的成功与关键错误传播。
- 验收：测试只观察 application 公共接口；覆盖 PKCE session handoff、账号选择/删除后的默认 UID、nil store 与依赖错误；`go test ./internal/application -cover -count=1`。
- 实际：
- 证据：
- 风险/下一步：

## T03 — 安全分类上游 transport 失败

- 状态：未完成
- 范围：P2-3。为 DNS、TLS、代理拒连、连接拒绝/重置等增加不含 URL/host 的稳定 transport 子类，并映射到公开 typed error。
- 验收：protocol/public SDK 测试逐类 RED→GREEN；context cancellation/deadline 的 `errors.Is` 保持；未知 transport 仍安全；无 token/URL 泄露。
- 实际：
- 证据：
- 风险/下一步：

## C01 — 集中检查 1（T01–T03）

- 状态：未完成
- 检查：错误透明性、脱敏、MCP result/error observability、application coverage、全量 test/vet/race 聚焦、文档/changelog 是否同步。
- 实际：
- 证据：
- 风险/下一步：

## T04 — 分类本地 snapshot/auth/config 真因

- 状态：未完成
- 范围：P2-4。替换 `localSnapshotError(operation, _ error)` 的无差别抹除；保留文件不存在、权限、JSON/TOML 解析、代理 URL、账号不一致等安全分类，同时不暴露 token 或文件内容。
- 验收：`OpenDefault` 公共操作测试覆盖代表性本地失败，operation/code/cause 可诊断且 secret-safe。
- 实际：
- 证据：
- 风险/下一步：

## T05 — 统一 CLI 下载 composition root 并补 bootstrap 纯逻辑测试

- 状态：未完成
- 范围：P2-5 与 P2-13。CLI 不再内联 `download.NewManager`；经 application service/factory 与 bootstrap production wiring 注入；覆盖 runtime config/proxy override 和 factory 错误。
- 验收：CLI/MCP 共享受控构造链；无协议反向依赖；相关 application/bootstrap/CLI 测试 RED→GREEN。
- 实际：
- 证据：
- 风险/下一步：

## T06 — 移除 MCP 推荐下载静默 count 钳制

- 状态：未完成
- 范围：P2-6。为默认 count 提供明确产品语义；超出有依据范围时返回显式参数错误，不静默改为 20；同步 MCP 文档和 changelog。
- 验收：count 缺省、非正数、合法值、超限值均有 public tool 测试；无合法请求被静默截断。
- 实际：
- 证据：
- 风险/下一步：

## C02 — 集中检查 2（T04–T06）

- 状态：未完成
- 检查：composition boundary、错误真因、无据限制、CLI/MCP 契约、全量 test/vet、文档和 changelog。
- 实际：
- 证据：
- 风险/下一步：

## T07 — 删除已证实死代码和诱导性 token 字段

- 状态：未完成
- 范围：P2-7 与 P3 dead-code 清单。删除无生产调用方的 `download.Manager.Enqueue` 后台队列及其固定并发/吞错/不可取消链；逐项验证并删除 `malformedError`、`transportError`、cursor helpers、appapi helpers/不可达 auth branch、`RuntimeConfig.RefreshToken` 或记录仍可达证据。
- 验收：调用图无生产引用；删除后相关包与全量测试通过；配置 schema/序列化不出现 refresh token。
- 实际：
- 证据：
- 风险/下一步：

## T08 — 审计更新 SemVer fail-closed 策略并拆出模块

- 状态：未完成
- 范围：P2-8 与 P3 semver 拆分。核对 release workflow 是否已对所有受信发布入口强制 SemVer；若完整则保留 strict reject、补 fail-closed 依据和跨 verifier 测试，若不完整才实现安全 skip + 可观测诊断。将解析/比较从 `releases.go` 拆到 `semver.go`，保持合法版本排序和 prerelease 策略。
- 验收：混合合法/非法 tag、全非法、stable/prerelease 与 workflow tag policy 均有测试；策略选择有代码注释和文档证据；不把网络/签名失败伪装成无更新。
- 实际：
- 证据：
- 风险/下一步：

## T09 — 加固 config/auth 持久化原子性与 durability

- 状态：未完成
- 范围：P3 一致性。`config.toml` 使用同目录临时文件、权限、Sync、原子替换；`auth.json` 在 rename 前 Sync，并按平台安全处理目录 durability。
- 验收：临时目录集成测试覆盖成功、写/Sync/rename 失败时旧文件保留、权限不放宽、无临时残留；Windows 行为不伪造 POSIX 保证。
- 实际：
- 证据：
- 风险/下一步：

## C03 — 集中检查 3（T07–T09）

- 状态：未完成
- 检查：死代码证据、更新 fail-open/fail-closed 边界、文件原子性/权限/掉电风险、race、全量 test/vet、回滚路径。
- 实际：
- 证据：
- 风险/下一步：

## T10 — 决定并统一 HTTP client timeout 策略

- 状态：未完成
- 范围：P3 timeout 一致性。先形成 ADR，决定默认由 context/显式 client 控制，或为 60s 提供可配置、可观测的客观依据；随后让 webapi 不再裸用 `http.DefaultClient`，统一 App/OAuth/resource/Web 策略，流式资源不得被整请求固定 timeout 误伤。
- 验收：client 构造测试证明显式 client 优先、默认策略一致、context 语义保持；不新增无据 timeout。
- 实际：
- 证据：
- 风险/下一步：

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
