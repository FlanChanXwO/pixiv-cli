# Goal-3 tasks：完整实施 Goal

修订日期：2026-09-08（Asia/Shanghai）。审查基线：`2167445280f1f6b6ce3ab8f6dbb3d082746b713d`。

## 执行规则与当前入口

Goal-3 保持单一完整 Goal。当前状态与 required_scope 只来自 [能力准入表](capability-admission.md)。表格的 depends_on 是显式依赖；按拓扑顺序执行，不能按旧编号顺序跳过前置任务。

T23A 是本轮用户单独批准的既有生产缺陷修复，不等待未来 vNext endpoint 合约冻结；它只完成搜索续读基础，不完成 T19/T23 的其他 endpoint 或整个 Goal。其实现状态不授予其他能力发布权限。其余生产能力仍须先 T00/T20、对应 contract、T12，再 T39A 与各 owner 实现。

Contract/计划任务以证据与一致性检查为验收；代码任务逐个 Red → Green → Refactor。不得改写历史 inconclusive/not_tested。任何 required capability 失败都使 Goal incomplete，不能通过排除它结束 Goal。

## 任务依赖表

Status 的 verified 表示对应 task 的实现与相关验证完成；各 task 的完成记录、contract 文档和专项报告分别提供证据，T23A 的生产修复证据见分页报告（修复提交 5162685）。pending 不能被历史 evidence 自动提升。

| Task | Owner / responsibility | depends_on | Deliverable / acceptance | Status |
| --- | --- | --- | --- | --- |
| T00 | scope/计划 owner | none | 冻结 required_scope；状态唯一来源、批准记录、完整性验收 | verified |
| T20 | shared semantics | T00 | 冻结 Target kind / Result kind / Subtype；命令级冲突规则 | verified |
| T01 | artwork contract | T00,T20 | 冻结 artwork search/series/latest/ranking/recommended/ugoira 基础 request、DTO、subtype | verified |
| T02 | novel contract | T00,T20 | 冻结 novel search/detail/series/latest/recommended/ranking/follow | verified |
| CHECK-01 | 集中检查-debug（T00/T20/T01/T02） | T02 | audit-only 复查 input/plan 偏离、41 条 required_scope、bookmark list/tags all、历史 evidence、排除 endpoint、T23A 边界、bug/死代码、类型/构建/测试、安全/数据/回滚/文档，并登记修复项 | verified |
| T03 | bookmark contract | T00,T20,CHECK-01 | 冻结两类 list/tags/detail/mutation/subtype 及 list/tags all 聚合契约 | verified |
| T04 | comment contract | T00,T20,T03 | 冻结 artwork/novel comments read/create/reply/stamp/delete、stamps、total | verified |
| T05 | continuation contract | T01,T02,T03,T04 | 冻结 allowlist、query/account/subtype binding、第二页 fixture；复用现有 cursor | verified |
| CHECK-02 | 集中检查-debug（T03/T04/T05） | T05 | audit-only；不执行真实 API、不修改生产代码/CLI/MCP wire、不改 required_scope；复查 bookmark/comment/continuation contract、历史 evidence、T23A 边界、账号/query/subtype binding、第二页 fixture、额外/重复 key、bug/死代码、类型/构建/测试、安全/数据/回滚/文档，并登记修复项 | verified |
| R02 | MCP filter replay 修复 | CHECK-02,T23A | 按 execution attempt 清空 MCP 本地 `seen` 状态；补非零 cursor + 本地 filter + 安全账号池 replay 回归，确保不静默丢记录；不改变 MCP schema | verified |
| R03 | Goal 任务账本与文档 tracking hygiene | CHECK-02 | 对齐 T03/T04 的显式 depends_on 与完成记录；处理 `/goal-*/` 对新增 Goal 文档的忽略/force-add 规则 | verified |
| T06 | error/其他 read contract | T00,T01,T02,T03,T04 | 冻结 user search/detail/relationships、user artworks/novels、recommended users、trending、follow、mypixiv、error、mutation outcome 与脱敏 | verified |
| CHECK-03 | 集中检查-debug（R02/R03/T06） | R02,R03,T06 | audit-only；复查 T06 read/error/mutation contract、R02 replay 修复、R03 tracking、41 条 required_scope、历史 evidence、T23A 边界、bug/死代码、类型/构建/测试、安全/数据/回滚/文档，并登记修复项 | verified |
| R04 | follow restrict 输入校验 | CHECK-03,T06 | 在 `FollowUser` SDK 边界复用 `validateRestrict`；空值仍默认 `public`，未知值以 `InvalidArgument` 在发起请求前拒绝，并补充无网络请求回归 | verified |
| T12 | SDK compatibility | T20,T01,T02,T03,T04,T05,T06,CHECK-03,R04 | 冻结 symbol map、旧 wrapper、named types、旧消费者编译、cursor 版本恢复；默认源码兼容 | verified |
| T39A | CLI/MCP migration | T12 | 冻结 CLI 路由和 MCP tool/input/output compatibility map；旧 JSON 回放清单 | verified |
| T07A | artwork read endpoint owner | T12 | 实现 artwork search、series、ugoira metadata 等 T07 artwork adapter leaf、DTO 与错误映射；保留现有 transport/SDK 边界，不进入 CLI/MCP | verified |
| T07B | novel read endpoint owner | T12 | 实现 novel search、v2 detail、v2 series 等 T07 novel adapter leaf、DTO 与错误映射；禁止回退已 rejected v1 detail/series path | verified |
| T07C | user read endpoint owner | T12,T06 | 实现 user search/detail/artworks/novels/relationships adapter leaf、DTO 与错误映射；bare-ID 的命令 resolver 仍由 T21 负责 | verified |
| T07D | feed/relationship adjunct endpoint owner | T12,T06 | 实现 trending、MyPixiv、follow 所需 adapter leaf、DTO 与错误映射；不把 mutation read-back 或 CLI/MCP 发布门禁提前并入 | verified |
| T07 | read endpoint owners（umbrella） | T12,T07A,T07B,T07C,T07D | 汇总并审计四个子卡的 artwork/novel/user/feed read adapter、DTO、错误映射与 leaf fixture；全部子卡完成后才可标记 verified | verified |
| T08A | artwork bookmark read endpoint owner | T12 | 实现 artwork bookmark list/tags/detail leaf、required/null/empty/error 映射与离线 fixture；不擅自发送未验证的 subtype wire | verified |
| T08B | novel bookmark read endpoint owner | T12 | 实现 novel bookmark list/tags/detail leaf、candidate path snapshot、required/null/empty/error 映射与离线 fixture | pending |
| T08C | artwork bookmark mutation endpoint owner | T12,T08A | 实现 artwork bookmark add/remove endpoint leaf、request/form/error fixture；不把 error-only 2xx 提升为 read-back 成功 | pending |
| T08D | novel bookmark mutation endpoint owner | T12,T08B | 实现 novel bookmark add/remove endpoint leaf、candidate path/request/form/error fixture；不把未验证 wire 或 2xx 提升为发布成功 | pending |
| T08 | bookmark endpoint owners（umbrella） | T12,T08A,T08B,T08C,T08D | 汇总并审计两类 bookmark 的 list/tags/detail/mutation leaf、DTO、错误映射与 fixture；全部子卡完成后才可标记 verified | pending |
| T09A | appapi transport | T12,T04,T06 | 增加响应可解码的窄 form 能力；保留旧 PostForm，不自动重放不确定 mutation | pending |
| T09 | comment endpoint owners | T09A | 按 artwork/novel read/create/reply/stamp/delete 与 stamps leaf 拆卡实现 | pending |
| T10 | ranking/recommended/latest owners | T12 | 按 endpoint leaf 拆卡实现 subtype 与 request/DTO | pending |
| T11 | endpoint continuation owners | T07,T08,T09,T10,T05 | 校验和提取 endpoint allowlist continuation；不持久化 next_url | pending |
| T13 | sdk/pixiv artwork | T07,T11 | 实现 artwork SDK 与 adapter 对照、旧签名兼容 | pending |
| T14 | sdk/pixiv novel | T07,T11 | 实现 novel SDK、series metadata 与 continuation | pending |
| T15 | sdk/pixiv bookmark | T08,T11 | 实现 explicit bookmark SDK，保留 AddBookmark/RemoveBookmark wrapper | pending |
| T16 | sdk/pixiv comment | T09,T11 | 实现 explicit read/create/reply/delete；ID 来源与不确定结果可观测 | pending |
| T17 | sdk/pixiv stamps | T09,T11 | 实现 stamps SDK 与 stamp/text/reply 独立语义 | pending |
| T18 | sdk/pixiv feed | T10,T11 | 实现 ranking/recommended/latest SDK 与 subtype | pending |
| T19 | sdk/pixiv cursor | T13,T14,T15,T16,T17,T18 | 扩展其余 endpoint payload/binding；不重建 envelope | pending |
| T21 | resolver owner | T20,T12,T13,T14 | 复用 ParseURL；record/URL/ID、command-specific conflict、受控 probe | pending |
| T22 | filter owner | T20,T19 | 规范化 rating/content-type filter；不发送未经确认 server rating | pending |
| T23A | pagination + sdk/pixiv + search | none | 本轮获批基础修复：先失败测试、checkpoint、SDK 绑定、CLI/MCP 两调用方；详见分页报告 | verified |
| T23 | pagination/traversal integration | T19,T22,T23A | 将基础续读契约接入其余 endpoint；验证聚合流、过滤及 Skip/Limit/OneBatch | pending |
| T24 | CLI search | T39A,T13,T21,T22,T23 | artwork search 与 subtype；stdin/JSON/NDJSON | pending |
| T25 | CLI novel search | T39A,T14,T21,T22,T23 | novel search canonical route、period 与旧 route | pending |
| T26 | CLI user search/trending | T39A,T06,T13,T23 | user search 与 trending | pending |
| T27 | CLI bookmark | T39A,T15,T21,T22,T23 | list/tags/detail/add/remove；list/tags all；user target 与内容类型分开 | pending |
| T28 | CLI recommended | T39A,T18,T21,T22,T23 | entity/subtype/all 与旧 positional all | pending |
| T29 | CLI timeline | T39A,T18,T21,T22,T23 | following/latest；entity 与 content-type subtype | pending |
| T30 | CLI ranking | T39A,T18,T22,T23 | artwork/novel ranking | pending |
| T31 | CLI detail | T39A,T13,T14,T21 | artwork/novel/user resolver；content endpoint exclusion 的兼容处理 | pending |
| T32 | CLI series | T39A,T13,T14,T21,T23 | artwork/novel series 与 continuation | pending |
| T33 | CLI comment | T39A,T16,T17,T21,T23 | read/create/reply/stamp/delete 的类型与结果语义 | pending |
| T34 | CLI user | T39A,T06,T13,T14,T21,T23 | detail/artworks/novels/relationships | pending |
| T35 | CLI follow | T39A,T06,T21 | user follow/unfollow 与旧 route alias | pending |
| T36 | CLI mypixiv | T39A,T06,T13,T14,T21,T23 | users/works typed validation | pending |
| T37 | MCP read owners | T39A,T13,T14,T15,T16,T17,T18,T21,T22,T23 | 按 tool owner 拆卡，注册/schema/structured errors 与旧请求回放 | pending |
| T38 | MCP mutation owners | T39A,T15,T16,T17,T06,T21 | 按 tool owner 拆卡，access control、read-back/outcome 与旧请求回放 | pending |
| T39B | compatibility audit | T24,T25,T26,T27,T28,T29,T30,T31,T32,T33,T34,T35,T36,T37,T38 | 实施后审计 SDK symbol/CLI alias/MCP wire，与 T39A 冻结表逐项对照 | pending |
| T40 | CLI presentation | T39B | completion/help/deprecated flags 与候选注册检查 | pending |
| T41 | docs/Skill | T39B,T40 | 准备并验证双语 README、CLI/SDK/MCP docs、Skill；未发布文档允许随实现编写 | pending |
| T42 | protocol/SDK regression | T19,T23 | DTO/null/empty、SDK 对照、cursor、旧消费者与无失效 endpoint 请求 | pending |
| T43 | CLI/MCP regression | T39B,T40 | JSON/NDJSON、stdin、skip/fail-fast、旧 schema、stdout 与不可达性 | pending |
| T44 | live regression | T42,T43 | 未来显式隔离账号 read/mutation；本轮不执行，不借用历史成功 | pending |
| T45 | delivery audit | T41,T42,T43,T44 | required_scope 全集 public_ready；go test/build、脱敏、迁移与发布审计 | pending |
| R01 | cursor 完整性与发布回滚 gate | CHECK-01 | 在发布前明确不可信边界的 cursor 完整性策略，并以跨版本回滚/迁移验证证明不会把 cursor 当作鉴权凭据 | pending |

## T00 完成记录

- Owner package / 涉及文件：scope/计划 owner；`goal-3/input.md`、`goal-3/plan.md`、`goal-3/capability-admission.md`、本文件。
- Depends on：无。
- 冻结 contract / fixture：`capability-admission.md` 明确 required=yes 的 41 个 capability 构成不可自动缩减的 `required_scope`；该文件是状态与发布授权唯一来源；`scope_admitted` → `contract_frozen` → `migration_ready` → `public_ready` 的状态门禁、用户批准记录、`bookmark list/tags --type all` 及 typed tag count 约束均已写明。
- Red 测试、命令及当前行为的预期失败：T00 是范围与计划文档核验，不修改生产代码，故无代码 Red 阶段。用 `awk` 审计能力表前置结果为 `capability_rows=41`、`required_yes=41`，且没有 required capability 的异常状态或 required 值。
- Green 命令及验收断言：`go mod download` 成功；worktree 基线 `go test ./...` 通过；能力表审计与 `rg` 关键约束检索通过。所有 required capability 当前仍为 `scope_admitted`，因此完整 Goal 仍保持 incomplete，未将历史 evidence 或 T23A 局部验证误记为发布就绪。
- 公开兼容性影响：无 SDK、CLI、MCP、wire 或依赖变更；仅将已核验的 T00 计划任务标记为 `verified`。
- 回滚前提 / 依赖闭包：这是文档状态提交，无生产依赖；回滚该提交即可恢复 T00 pending，不需要撤回代码或数据。后续任务仍必须依赖 T00，不能因本状态标记跳过 contract/compatibility 门禁。
- 实际结果 / evidence / 风险：T00 已完成。required_scope 未缩减，状态唯一来源和完整性验收已冻结；剩余风险是 41 个 capability 均尚未达到 `public_ready`，按拓扑顺序下一步执行 T20。当前执行视为对既有 Goal-3 待办任务的继续，不改变 `input.md` 的历史原文或 required_scope。

## T20 完成记录

- Owner package / 涉及文件：shared semantics；`goal-3/cli-migration-matrix.md`、`goal-3/plan.md`、本文件。只冻结产品语义，不提前修改 SDK/CLI/MCP 生产实现。
- Depends on：T00 verified。
- 冻结 contract / fixture：迁移矩阵新增 T20 类型语义契约，分别定义 Target kind、Result/entity kind、Subtype 的允许边界；保留 `ReferenceKind` 的 user/user bookmarks/artwork series/novel series 细分；明确 `illustration` → `illust` 的兼容拼写、`Record.Type()` 的非全局语义、结构化 record → ParseURL → 显式类型 ID → 受控 bare-ID probe 顺序，以及 search/detail/series/bookmark/comment/feed 的命令级冲突规则。
- Red 测试、命令及当前行为的预期失败：T20 是 contract freeze 文档任务，无生产代码 Red 阶段。只读核验确认当前没有统一三层类型或共享 resolver，且 bookmark list/tags 尚不支持 user URL + `--type novel`、`all` 和 bare-ID probe；这些是后续 T21/T27 等实现任务，不在 T20 偷渡修复。
- Green 命令及验收断言：`rg`/`nl` 核验了 `sdk/pixiv/reference.go`、`models.go`、`request.go`、`internal/shared/record` 及相关 CLI/MCP 调用方；文档静态检查确认新增锚点、三层定义、命令表、resolver 顺序和冲突错误规则均可检索。未改生产代码，因此沿用上一轮 `go test ./...` 全通过证据；本提交钩子将再次运行相关 Go 检查。
- 公开兼容性影响：无 public SDK symbol、CLI wire、MCP schema、依赖或默认值变化；只为后续 T12/T21/T39A 提供冻结语义，既有兼容 surface 保持原样。
- 回滚前提 / 依赖闭包：文档提交可整体回滚；回滚必须同时撤销迁移矩阵、plan 引用和本任务状态，不能保留实现任务对未冻结语义的依赖。无数据、账号或生产配置变更。
- 实际结果 / evidence / 风险：T20 已完成并保持 Goal incomplete。现有实现与冻结目标之间的差距已显式登记，下一步按拓扑顺序执行 T01；T01/T02/T03/T04 等 contract owner 必须引用本契约并补各自 endpoint fixture，不得据本任务直接宣告 capability contract_frozen。

## T01 完成记录

- Owner package / 涉及文件：artwork contract；`goal-3/upstream-contract-matrix.md`、`goal-3/plan.md`、本文件。只冻结 search/series/latest/ranking/recommended/ugoira 的基础 contract，不实现 endpoint/SDK/CLI/MCP。
- Depends on：T00、T20 verified。
- 冻结 contract / fixture：upstream matrix 新增 T01 operation 表，明确六类 method/path、request required/optional/default、normalized `Artwork`/`UgoiraMetadataDTO`、search/latest/ranking/recommended/series 的 subtype 与 continuation 差异；同步冻结 `ArtworkDTO` optional 字段、required list 的 null/empty、`next_url` 终止/malformed、publish time、cover/pages 与 ugoira archive/frame 安全边界。
- Red 测试、命令及当前行为的预期失败：T01 是 contract freeze 文档任务，无生产代码 Red 阶段。只读核验确认 series 缺独立 live matrix/第二页，recommended 第二页与 subtype 未确认，latest 扩展 subtype 仅 partial；这些未被伪装为已完成。
- Green 命令及验收断言：`nl`/`rg` 逐项核验 `sdk/pixiv` model/DTO/request/operation、六个 artwork endpoint、现有 endpoint/SDK tests 与 Goal-3 evidence；`git diff --check` 和 T01 关键段落静态检索通过。历史 `go test ./...` 通过，且本次提交钩子将再次运行项目 Go 检查。
- 公开兼容性影响：无 production code、public SDK symbol、CLI/MCP wire、依赖或默认值变化；明确保留 `ArtworkKindIllustration="illustration"` 与 `RawKind`，不因 semantic `illust` 重命名既有 API。
- 回滚前提 / 依赖闭包：文档提交需整体回滚 upstream matrix、plan 引用和 T01 状态；没有数据、账号、生产配置或 endpoint path 依赖。后续 T05/T07/T10/T13 只能在本 contract 约束下补实现与证据。
- 实际结果 / evidence / 风险：T01 已完成基础 contract 冻结，相关 capability 仍保持 `scope_admitted`，Goal 继续 incomplete。下一步按拓扑顺序执行 T02；T05 负责补未确认 continuation/subtype evidence，T07/T10/T13 负责实现，不能把 T01 文档当作 public_ready。

## T02 完成记录

- Owner package / 涉及文件：novel contract；`goal-3/upstream-contract-matrix.md`、`goal-3/plan.md`、本文件。只冻结 search/detail/series/latest/recommended/ranking/follow 的目标 contract，不实现 endpoint/SDK/CLI/MCP。
- Depends on：T00、T20、T01 verified；本轮仅沿 tasks 拓扑推进 T02。
- 冻结 contract / fixture：upstream matrix 新增 T02 operation 表，明确 `/v1/search/novel`、`/v2/novel/detail`、`/v2/novel/series`、`/v1/novel/new`、`/v1/novel/recommended`、`/v1/novel/follow`、`/v1/novel/ranking` 的 request、normalized Novel/NovelDTO、series metadata、`offset`/`last_order`/`max_novel_id` continuation、null/empty/error 规则，以及 detail/series v1、novel content App/WebView 的 rejection/exclusion 边界。plan 已链接 T02 anchor；历史 evidence verdict 未改写。
- Red 测试、命令及当前行为的预期失败：T02 是 contract freeze 文档任务，无生产代码 Red 阶段。只读核验确认当前 detail/series 仍是 v1、latest 仍使用 offset、novel ranking 没有生产 owner、search period/series 第二页及 v2 adapter/SDK 证据不足；这些差距保持显式，未被文档伪装成完成。
- Green 命令及验收断言：`sed` 全量复读 `input.md`/`plan.md`/`tasks.md`；`nl`/`rg` 核验 `internal/services/pixiv/endpoint/novel`、`sdk/pixiv`、CLI/MCP owner、strict evidence 和 migration ledger；T02 anchor/七类 operation/拒绝路径均可检索；`git diff --check` 通过；提交钩子将运行 `go test ./...`。
- 公开兼容性影响：无 production code、public SDK symbol、CLI/MCP wire、依赖、endpoint 请求或默认值变化；仅新增 contract 文档、plan 引用、T02 状态，并显式加入下一轮 `CHECK-01` 复查任务。
- 回滚前提 / 依赖闭包：文档提交需整体回滚 upstream matrix、plan 引用、T02 状态和 CHECK-01 排程；没有数据、账号、生产配置或 endpoint path 依赖。后续实现若已引用本节，回滚前必须同步撤销其依赖或先提供兼容修复。
- 实际结果 / evidence / 风险：T02 已完成，novel capability 仍保持 `scope_admitted`，Goal 继续 incomplete。主要剩余风险是 v2 detail/series 尚未接入生产、latest cursor 仍错误使用 offset、ranking owner 缺失及若干 continuation/binding 证据不足；按每三个 task 的 goal-mode 节奏，下一轮先执行 `CHECK-01`，通过后再按顺序进入 T03。

## CHECK-01 完成记录

- Owner package / 涉及文件：集中质量 gate；只读检查 `goal-3/input.md`、`goal-3/plan.md`、`goal-3/tasks.md`、`goal-3/capability-admission.md`、T00/T20/T01/T02 contract/evidence，以及 T23A 受影响的 shared pagination、SDK、CLI/MCP、双语 SDK 文档。新增风险说明位于 `docs/en/sdk.md`、`docs/zh-CN/sdk.md`。
- Depends on：T02 verified；T23A 为本轮已批准且已 verified 的独立基础修复。
- 冻结 contract / fixture：CHECK-01 明确为 audit-only，不执行真实账号 API、不新增依赖、不启动其他 vNext endpoint、不修改 CLI/MCP wire；逐项核对 41 条 required scope、`bookmark list/tags --type all`、历史 evidence 不得提升、已排除 endpoint、T23A 与完整 `logical-pagination` 的边界。T23A 稳定源非 snapshot 限制、账号/query/client binding 和回滚闭包继续以分页报告及 SDK 文档为准。
- Red 测试、命令及当前行为的预期失败：本 task 是集中审计，无生产代码 Red 阶段。审计先确认两项文档门槛：原 CHECK-01 行未逐项列出高风险边界，且 tasks 顶部把所有 verified 过度指向分页报告；同时确认 cursor 编码没有 MAC/签名、已发行 version-2 cursor 回滚到旧 binary 不兼容。这些是非当前发布阻塞的后续 gate，不被伪装为已解决的生产能力。
- Green 命令及验收断言：`go test -race ./internal/shared/pagination ./sdk/pixiv ./internal/cli/commands/pixiv/search ./internal/mcpserver/pixiv/tools/search_illust -count=1`、`go vet ./internal/shared/pagination ./sdk/pixiv ./internal/cli/commands/pixiv/search ./internal/mcpserver/pixiv/tools/search_illust`、`go test ./scripts/tests/documentation -count=1`、`go test ./...`、`sh scripts/build.sh` 均通过；`git diff --check` 及目标 anchor/static audit 通过。复核确认 checkpoint 只保存非 secret continuation state，CLI/MCP 不暴露 opaque SDK cursor，账号/query/operation binding、错误丢弃、取消、重复 cursor、Skip/Limit/OneBatch 和跨批恢复测试均存在。
- 公开兼容性影响：本轮没有改变 SDK/CLI/MCP/wire 行为、endpoint、默认值或依赖；双语 SDK 文档新增安全边界说明。没有把 cursor 当作鉴权凭据，也没有承诺防篡改或实时 snapshot 语义。
- 回滚前提 / 依赖闭包：文档与任务记录可整体回滚；若未来实现 R01，必须同时验证或回滚 shared collector、SDK cursor binding、CLI/MCP 调用方和对应双语文档，不能只撤一个 cursor 版本或 callback。R01 不授权新增 crypto 依赖，具体策略须有证据并保持兼容。
- 实际结果 / evidence / 风险：CHECK-01 已完成，未发现 P0/P1 或需立即修复的生产 bug、死代码、类型/构建/测试回归、鉴权越权或敏感信息泄露。发现的 P2 风险已通过双语文档告知，并登记 `R01` 作为发布前修复/gate；真实 API 稳定性、跨版本已发行 cursor 回滚和不可信边界 tamper resistance 仍未验证。required capabilities 仍全部为 `scope_admitted`，Goal 继续 incomplete；按任务顺序下一轮进入 T03。

## T03 完成记录

- Owner package / 涉及文件：bookmark contract；`goal-3/upstream-contract-matrix.md`、`goal-3/plan.md`、本文件。只冻结 artwork/novel bookmark list、tags、detail、add、remove、subtype 和 `list/tags --type all` 聚合，不实现 endpoint/SDK/CLI/MCP。
- Depends on：T00、T20、CHECK-01 verified；本轮沿 tasks 拓扑推进 T03。
- 冻结 contract / fixture：upstream matrix 新增 T03 operation 表，区分已存在的 artwork leaf、novel bookmark list 和尚未验证的 novel tags/detail/mutation；明确 `public/private` restrict、tag、`max_bookmark_id`/`offset`、detail absent state、mutation read-back/outcome、artwork subtype candidate、all 的 artwork→novel 顺序、统一 Skip/Limit、typed tag count、双流 cursor 和页原子失败。同步冻结 required list 的 null/empty、continuation malformed、namespace/all 拒绝、private access-control 与无匿名 fallback 边界；历史 evidence verdict 未改写。
- Red 测试、命令及当前行为的预期失败：T03 是 contract freeze 文档任务，无生产代码 Red 阶段。只读核验确认当前 `--type all`、typed tag output、aggregate cursor、novel tags/detail/add/delete、mutation access control/read-back/restore 尚不存在或未进入 strict evidence；artwork tags 的 null-as-empty 只作为 legacy 行为登记，不能代替目标 required-list contract。
- Green 命令及验收断言：复读 `input.md`/`plan.md`/`tasks.md`；`nl`/`rg` 核验 `sdk/pixiv` request/model/DTO/operation、两类 bookmark endpoint、CLI/MCP surface、capability admission、migration matrix、strict/mutation evidence；T03 anchor、十二项 leaf/aggregate operation（十项 leaf、两项 aggregate）、all 边界和 rejection 规则均可检索；`git diff --check` 通过；提交钩子将运行 `go test ./...`。
- 公开兼容性影响：无 production code、public SDK symbol、CLI/MCP wire、依赖、endpoint 请求或默认值变化；保留现有单流 `BookmarkTag{Name,Count}`、`AddBookmark`/`RemoveBookmark` 及旧路由，typed aggregate output 和 novel explicit symbols 留给 T12/T15/T27/T37。
- 回滚前提 / 依赖闭包：文档提交需整体回滚 upstream matrix、plan 引用和 T03 状态；没有数据、账号、生产配置或 endpoint path 依赖。后续 all cursor、SDK、CLI/MCP 实现必须作为 aggregate contract、两端 checkpoint、输出原子性和兼容 wrapper 的依赖闭包回滚，不能只撤一个流或一个 mutation leaf。
- 实际结果 / evidence / 风险：T03 已完成基础 contract 冻结，bookmark capabilities 仍全部为 `scope_admitted`，Goal 继续 incomplete。主要剩余风险是 novel tags/detail/mutation 与 artwork tags/subtype 缺少 strict evidence/owner，all 聚合和 private access-control/read-back 尚未实现；按任务顺序下一轮进入 T04。

## T04 完成记录

- Owner package / 涉及文件：comment/stamp contract；`goal-3/upstream-contract-matrix.md`、`goal-3/plan.md`、本文件。只冻结 artwork/novel comments read、text/reply/stamp/delete、stamps read 与 total 元数据，不实现 endpoint/SDK/CLI/MCP。
- Depends on：T00、T20、T03 verified；本轮沿 tasks 拓扑推进 T04，并复用 T03 已冻结的证据分层与错误边界。
- 冻结 contract / fixture：upstream matrix 新增 T04 contract 表，覆盖两类 comments read、八类 comment mutation 变体、stamps read 和两类 total 元数据；明确 `illust_id`/`novel_id`/`comment_id`/`parent_comment_id`/`stamp_id` 的 namespace 与正数边界、parent chain、required/null/empty、date 与 numeric access-control 的 unresolved wire、optional `total`、正 `offset` continuation、创建 ID/read-back/outcome、同账号清理、CLI/MCP 旧 wire 与敏感信息边界。历史 matrix、strict evidence、mutation manifest 的 `rejected`/`inconclusive`/`not_tested` 均保持原样。
- Red 测试、命令及当前行为的预期失败：T04 是 contract freeze 文档任务，无生产代码 Red 阶段。只读核验确认当前仅有 artwork v3 与 novel v2 comments read adapter/SDK、没有 comment mutation/stamps public owner；strict comments live 没有真实第二页/非空 total，artwork date/numeric access-control 与当前 fixture 不一致，novel v3、mutation response ID、body/null-empty 及字段级 stamps schema 尚未验证，不能将历史 200/read-back 直接当作 public contract。
- Green 命令及验收断言：复读 `input.md`/`plan.md`/`tasks.md`；`nl`/`rg` 核验两类 comments adapter/fixture、SDK `CommentPage`/DTO/operation、CLI/MCP read surface、strict/legacy/mutation evidence、T20 类型矩阵和 capability admission；T04 anchor、十三项 contract row（两类 read、八类 mutation、stamps、两类 total）、rejection/outcome/atomicity 边界均可检索；`git diff --check` 通过；提交钩子将运行 `go test ./...`。
- 公开兼容性影响：无 production code、public SDK symbol、CLI/MCP wire、依赖、endpoint 请求或默认值变化；保留 `Comment`/`CommentPage`/`CommentDTO`、`illust_comments`/`novel_comments` 与当前 JSON/NDJSON envelope；新增 mutation/stamps symbol、字段和 tool 留给 T09A/T16/T17/T33/T37/T38 及 T39A。
- 回滚前提 / 依赖闭包：文档提交需整体回滚 upstream matrix、plan 引用和 T04 状态；没有数据、账号、生产配置或 endpoint path 依赖。后续 comment adapter、窄 form transport、SDK、CLI/MCP 及 read-back 实现必须连同 response ID、同账号 execution context、页/错误原子性和兼容 wrapper 作为依赖闭包回滚。
- 实际结果 / evidence / 风险：T04 已完成基础 contract 冻结，comment/stamps capabilities 仍全部为 `scope_admitted`，Goal 继续 incomplete。主要剩余风险是 comments 版本/日期/access-control、第二页与 total 语义、字段级 stamps schema、mutation response ID 与真实隔离清理尚未通过 strict/owner evidence；按任务顺序下一轮进入 T05。

## T05 完成记录

- Owner package / 涉及文件：continuation contract；`goal-3/upstream-contract-matrix.md`、`goal-3/plan.md`、本文件。只冻结 continuation、binding 与第二页 fixture，不实现 endpoint/SDK/CLI/MCP。
- Depends on：T01、T02、T03、T04 verified；本轮沿 tasks 拓扑推进 T05，并复用 CHECK-01 已确认的 cursor 安全边界与 T23A shared pagination 证据。
- 冻结 contract / fixture：upstream matrix 新增 T05 section，按 artwork/novel/user/series/comments/bookmark/tag/aggregate family 冻结 `offset`、`last_order`、`max_illust_id`、`max_novel_id`、`max_bookmark_id` 的 target allowlist 与 recommended `offset=0` presence 规则；明确 query digest、verified account/ephemeral client、subtype/local filter binding，`sdk.Cursor` 不承载 raw `next_url`/token/cookie；登记 live confirmed、synthetic two-page、`pagination_exempt` 与 `inconclusive` 的真实边界，并要求第二页 required list、终止响应、changed-binding、malformed continuation 和重复 cursor fixture。
- Red 测试、命令及当前行为的预期失败：T05 是 contract freeze 文档任务，无生产代码 Red 阶段。只读核验确认 novel latest 当前仍用 `offset` 而 live target 为 `max_novel_id`；CLI/MCP recommended subtype 尚未进入 SDK cursor digest；`SearchNovels`/`SearchUsers` 当前未纳入 identity-scoped binding；多数单 key parser 忽略未知额外 query key，endpoint 不保存跨页重复状态；这些均登记为后续实现/兼容缺口，没有被写成已完成。
- Green 命令及验收断言：复读 `input.md`/`plan.md`/`tasks.md`；逐项核验 `sdk/cursor.go`、`sdk/pixiv/cursor.go`、`sdk/pixiv/ops_*`、endpoint continuation parser、shared pagination/traversal、strict evidence、SDK/CLI/MCP 两页 fixture 与 account-pool replay；确认 T05 anchor、operation allowlist、binding 字段、安全边界和 evidence verdict 均可检索。提交前运行 `git diff --check`、`go test ./scripts/tests/documentation -count=1`、`go test ./...` 与 `sh scripts/build.sh`。
- 公开兼容性影响：无 production code、public SDK symbol、CLI/MCP wire、endpoint 请求、默认值、依赖或 live 数据变更；只补充内部 contract、fixture/evidence 分层与 CHECK-02 排程。现有 cursor、recommended subtype、搜索账号 binding 的行为保持不变。
- 回滚前提 / 依赖闭包：文档提交需整体回滚 upstream matrix、plan 引用、T05 状态和 CHECK-02 排程；没有数据、账号或生产配置依赖。后续 owner 若已引用 T05，回滚前必须同步撤销其未完成的 adapter/SDK/CLI/MCP 依赖，不能只删除 allowlist 文本。
- 实际结果 / evidence / 风险：T05 contract freeze 已完成；confirmed live 两页仅保留 novel follow/recommended、artwork search 四种 selector、artwork latest/ranking；数据受限 bookmark/user case 仍要求 synthetic two-page；novel-new、artwork recommended、novel-series-v2、comments 等继续保留真实 failure/inconclusive。当前 required capabilities 仍全部为 `scope_admitted`，Goal 继续 incomplete。按每三个 task 插入集中检查规则，已登记下一入口 `CHECK-02`，本轮不执行 T06。

## CHECK-02 完成记录

- Owner package / 涉及文件：集中质量 gate；只读审计 `goal-3/input.md`、`goal-3/plan.md`、`goal-3/tasks.md`、`goal-3/capability-admission.md`、T03/T04/T05 contract/evidence、T23A shared pagination/SDK/CLI/MCP、双语 SDK 文档与 `.gitignore`。本轮不调用真实 Pixiv/FANBOX API，不修改生产实现或公开 wire。
- Depends on：T05 verified；复查 T03/T04/T05 与 T23A 的依赖闭包、required_scope、历史 evidence、账号/query/subtype binding、第二页 fixture、回滚和交付 tracking。
- 冻结 contract / audit scope：CHECK-02 只验证“已冻结项”和“明确保留的未决项”是否被正确分层；不把 T05 `verified` 解读为所有 operation continuation 可发布，不把 synthetic two-page、`pagination_exempt`、历史 live evidence 或 T23A 局部修复提升为 capability 完成。审计后 41 条 required capability 必须继续为 `scope_admitted`。
- Red 测试、命令及当前行为的预期失败：本 task 是 audit-only，无生产代码 Red 阶段。审计确认 shared checkpoint、SDK binding、错误丢弃、重复 opaque cursor 与多数 T23A 回归已有证据；同时发现 MCP `CollectWith` 的本地 filter `seen` 只在 zero cursor 重置，账号池从非零 cursor 安全 replay 时可能静默丢记录（`internal/mcpserver/pixiv/internal/runtime/runtime.go:223-238`、`internal/shared/traversal/traversal.go:70-93`、`internal/mcpserver/pixiv/internal/filters/filters.go:161-176`）。现有 replay fixture 均从 zero cursor 开始，不能证明该路径安全（`internal/mcpserver/pixiv/tools/search_illust/bookmark_test.go:60-96`）。
- Green 命令及验收断言：`go test ./... -count=1`、`go vet ./...`、`sh scripts/build.sh`、`git diff --check` 与文档测试均通过；测试/build 通过只证明当前既有路径没有回归，不覆盖上述非零 cursor replay 缺口。静态审计确认 `capability-admission.md` 的 41 条 required capability 全部仍为 `scope_admitted`，没有历史 evidence、T23A 或 T05 状态升级；工作区无敏感/生成物进入 Git，远端交付状态在提交前后另行核验。
- Findings / 修复登记：P1 MCP replay 状态问题登记为 `R02`，下一轮优先修复并补非零 cursor/local-filter/replay 回归；P1 `SearchNovels`/`SearchUsers` account binding 矛盾继续由既有 T12/T18 决策；cursor tamper/rollback 由既有 R01；novel latest、recommended subtype、严格额外 key/跨页重复 continuation、aggregate atomicity 由既有 T10/T11/T18/T19/T23 链路承接。T03 表格漏写 CHECK-01、T04 表格漏写 T03、CHECK-02 验收防误升级措辞和 `/goal-*/` tracking 风险登记为 `R03`，不在本审计轮直接修复。
- 公开兼容性影响：无 production code、public SDK symbol、CLI/MCP wire、endpoint、默认值、依赖、required_scope 或 live 数据变化；仅把 CHECK-02 标记为 verified 并登记 R02/R03。已确认的 T23A 文档仍需以 R02 修复结果收紧，不提前宣称 replay 完整安全。
- 回滚前提 / 依赖闭包：本轮只改 audit ledger/report；回滚需同时撤销 CHECK-02 状态、审计记录和 R02/R03 排程，不涉及运行数据或账号。R02 后续修改必须将 runtime filter 状态、traversal replay、MCP search fixture 作为同一测试/回滚闭包。
- 实际结果 / evidence / 风险：CHECK-02 已完成，发现一项 T23A 范围内 P1 数据遗漏风险，故 Goal 继续 incomplete；下一入口是 R02，不是 T06。`pagination-validation-report.md` 已补充本审计对历史 zero-cursor replay 证据的限定，避免把既有全量测试通过误读为非零 cursor replay 已验证。

## R02 完成记录

- Owner package / 涉及文件：`internal/mcpserver/pixiv/internal/runtime/runtime.go`、`internal/mcpserver/pixiv/internal/runtime/runtime_test.go` 与 `internal/shared/traversal/traversal.go`。runtime 仍持有 MCP 本地 record filter；shared traversal 仅补充已有 opaque cursor 的 From 入口，使非零 continuation 能在同一 execution-attempt 语义下验证。
- Depends on：CHECK-02、T23A verified；未引入新的 endpoint、账号池策略或 MCP wire surface。
- 冻结 contract / fixture：每次 pooled `Execute` 回调代表一个独立 attempt，`seen` 必须在 attempt 开始时清空，不再根据 cursor 是否为零推断生命周期。离线 fixture 使用真实 `pixiv.OpenWith` SDK client：先取得 offset=30 的非零 cursor；首个 attempt 返回通过 `min_views` local filter 的 artwork 200、续到 offset=60 后返回错误；safe replay 从同一 offset=30 重新返回重复的 200 和被 filter 排除的 201。两次 attempt 均断言 `committed=false`，MCP schema 与 opaque cursor wire 不变。
- Red 测试、命令及当前行为的预期失败：`go test ./internal/mcpserver/pixiv/internal/runtime -run TestCollectWithFromResetsLocalFilterOnSafeReplay -count=1 -v` 实际失败，safe replay 后结果为 `[]`，断言期望 1 条；这证明上一 attempt 的 `seen` 状态会静默丢掉 replay 首批记录，而非编译错误或人工推断。
- Green 命令及验收断言：同一 focused test 通过；`go test ./internal/shared/traversal ./internal/mcpserver/pixiv/internal/runtime ./internal/mcpserver/pixiv/tools/search_illust ./internal/mcpserver/pixiv -count=1`、`go test -race ./internal/shared/traversal ./internal/mcpserver/pixiv/internal/runtime ./internal/mcpserver/pixiv/tools/search_illust -count=1`、`go test ./... -count=1`、`go vet ./...`、`sh scripts/build.sh`、`git diff --check` 均通过。结果断言确认 replay 不遗漏、不重复，existing traversal commit-boundary tests 继续通过。
- 公开兼容性影响：没有 MCP schema/input/output、CLI、SDK public symbol、endpoint 请求、默认值、依赖或 token 行为变化；`CollectWith` 的 zero-cursor 调用保持原语义，仅新增 shared internal `CollectWithFrom`/`TraverseWithFrom` 委托入口。
- 回滚前提 / 依赖闭包：需整体回滚 runtime 的 attempt wrapper、shared traversal From 委托入口及其离线 SDK 回归；若后续 owner 使用 From 入口，必须先撤销或迁移这些调用，不能只删除 wrapper。无运行数据、账号或持久配置迁移。
- 实际结果 / evidence / 风险：R02 已 verified，P1 local-filter replay 遗漏已修复并通过 code-review-expert 自审，无新增 P0/P1。真实 API/live second-page、mutation、其余 endpoint continuation 仍未由本 task 覆盖；41 条 required capability 继续为 `scope_admitted`，Goal 保持 incomplete，下一入口是 R03。

## R03 完成记录

- Owner package / 涉及文件：Goal-3 任务账本与仓库 tracking；`.gitignore`、`goal-3/tasks.md`。不修改生产代码、required_scope、公开 API、CLI/MCP wire 或运行数据。
- Depends on：CHECK-02 verified；本轮只处理其登记的 T03/T04 dependency ledger 与 Goal 文档跟踪问题。
- 冻结 contract / fixture：T03 的显式依赖与完成记录均为 `T00,T20,CHECK-01`；T04 的显式依赖与完成记录均为 `T00,T20,T03`。保留通用 `/goal-*/` 忽略，新增 `goal-3` 例外，使当前已批准、已纳入版本控制的 Goal 文档可直接被 Git 发现；未来 `goal-4` 等目录仍保持 ignored，只有明确决定纳入交付时才 force-add。
- Red 测试、命令及当前行为的预期失败：这是 tracking/documentation task，无生产代码 Red 阶段。变更前 `git check-ignore -v --no-index goal-3/r03-tracking-probe.md` 与 `goal-4/r03-tracking-probe.md` 均命中 `.gitignore:17:/goal-*/`；任务表也实际存在 T03/T04 表格依赖与完成记录不一致。
- Green 命令及验收断言：修改后 `git check-ignore -v --no-index goal-3/r03-tracking-probe.md` 不再命中，而 `goal-4/r03-tracking-probe.md` 仍命中通用规则；`rg` 核验 T03/T04 表格与完成记录依赖一致；`go test ./scripts/tests/documentation -count=1`、`git diff --check` 通过。Goal-3 目录中的新增文档可不使用 force-add，未来 Goal 目录的显式 force-add 策略仍可观测。
- 公开兼容性影响：无 production code、SDK/CLI/MCP public surface、endpoint、依赖、required_scope 或状态授权变化；只修正计划 DAG 和仓库忽略边界。
- 回滚前提 / 依赖闭包：整体回滚 `.gitignore` 的 `goal-3` 例外、T03/T04 依赖表和本完成记录即可恢复原 tracking 行为；不涉及业务数据、账号、构建产物或运行配置。
- 实际结果 / evidence / 风险：R03 已 verified。任务表依赖 DAG 无新增环，Goal-3 文档 tracking 策略已明确；41 条 required capability 仍为 `scope_admitted`，Goal 继续 incomplete，下一入口按拓扑进入 T06。

## T06 完成记录

- Owner package / 涉及文件：error/其他 read contract；`goal-3/upstream-contract-matrix.md`、`goal-3/plan.md`、本文件。只冻结 user search/detail/relationships、user artworks/novels、recommended users、trending、MyPixiv、follow mutation、统一错误分类、mutation outcome 与脱敏，不实现 endpoint/SDK/CLI/MCP。
- Depends on：T00、T01、T02、T03、T04 verified；本轮复用 T05 的 continuation/binding 规则、T20 的 Target/Result/Subtype 语义、CHECK-02/R02 的 replay 与错误边界。
- 冻结 contract / fixture：upstream matrix 新增 T06 section。read operation 明确 method/path、required/optional/null/empty、正数 ID、subtype、offset/`offset=0`、verified current identity、private relationship scope 与无匿名 fallback；follow add/remove 明确 form path、写前检查、同账号 read-back 和确定失败/已接受未读回/dispatch 后不确定三类 outcome；错误矩阵冻结 `InvalidArgument`、`InvalidCursor`、`Unauthorized`、`CredentialsExpired`、`Forbidden`、`NotFound`、`ContentUnavailable`、`RateLimited`、`MalformedUpstreamResponse`、`UpstreamError`、`UpstreamUnavailable` 的边界，以及 cursor、SDK error、CLI/MCP structured output 的脱敏规则。历史 live rows 与 user artworks/novels 的 `inconclusive`/`pagination_exempt` 边界未改写。
- Red 测试、命令及当前行为的预期失败：T06 是 contract freeze 文档任务，无生产代码 Red 阶段。只读审计确认 user/trending/MyPixiv/relationship endpoint 与 SDK/MCP fixture 已存在，但这些新增 T06 families 没有 strict live rows；`user-illusts`/`user-novels` 真实第二页未观察；SearchUsers account binding gap 仍由 T05 登记；follow leaf 与 SDK 只保留 status-only `PostForm`，没有 production read-back/outcome evidence；bare-ID probe 不能把 403、网络错误或不确定响应当作换类型依据。
- Green 命令及验收断言：复读 `input.md`/`plan.md`/`tasks.md`；`rg`/`nl` 核验 T06 operation 表覆盖 user read、recommended、trending、MyPixiv、follow mutation、error/redaction、bare-ID 与 evidence boundary；确认历史 strict rows 未被改写、41 条 required capability 仍为 `scope_admitted`；提交前运行 `go test ./scripts/tests/documentation -count=1`、`git diff --check`，提交钩子运行 `go test ./...`。
- 公开兼容性影响：无 production code、public SDK symbol、CLI/MCP wire、endpoint 请求、默认值、依赖或 live 数据变化；保留现有 `SearchUsers`、user detail/relationship、MyPixiv、trending、`FollowUser`/`UnfollowUser` 的源码与 wire 形状，后续 owner 不能把合约冻结误当作发布完成。
- 回滚前提 / 依赖闭包：文档提交需整体回滚 T06 matrix section、plan link、T06 状态和 CHECK-03 排程；没有数据、账号、生产配置或 endpoint path 依赖。后续 T07/T12/T13/T14/T21/T35/T36/T37/T38 若已引用 T06，回滚时必须同步撤销其 read/error/mutation compatibility 依赖，不能只删除 contract 文本。
- 实际结果 / evidence / 风险：T06 已 verified；当前 41 条 required capability 继续为 `scope_admitted`，Goal 保持 incomplete。T06 未新增 live API、未提高任何 capability 状态，未声明 follow read-back、SearchUsers account binding、user/relationship/MyPixiv/trending strict live 或 bare-ID probe 已完成。最近三个 task 为 R02、R03、T06，下一入口按 goal-mode 规则进入 `CHECK-03`，不直接跳到 T12。

## CHECK-03 完成记录

- Owner package / 涉及文件：集中质量 gate；只读复查 `goal-3/input.md`、`goal-3/plan.md`、`goal-3/tasks.md`、`goal-3/capability-admission.md`、T06 upstream contract、R02 runtime/traversal 回归、R03 tracking 规则、历史 evidence 与 T23A 分页报告，并核对 `sdk/pixiv`、follow endpoint、CLI/MCP mutation 调用方。无真实 Pixiv/FANBOX API、无生产实现改动。
- Depends on：R02、R03、T06 verified。
- 冻结 contract / fixture：审计确认 T06 已覆盖 user search/detail/relationships、user artworks/novels、recommended、trending、MyPixiv、follow mutation、错误分类/脱敏与 bare-ID 边界；R02 的 execution-attempt filter reset 和非零 cursor replay 证据保持有效；R03 的 Goal-3 tracking 例外只作用于已批准目录；41 条 required capability 仍全部为 `scope_admitted`。历史 strict/inconclusive/pagination_exempt verdict、T23A 仅修复搜索续读的边界均未升级。
- Red 测试、命令及当前行为的预期失败：这是 audit-only task，无生产代码 Red 阶段。审计发现 follow contract 的一个未登记缺口：T06 要求 `restrict` 只接受 `public|private`、空值默认 `public`，但 `sdk/pixiv.Client.FollowUser` 目前没有调用已有 `validateRestrict`，未知值可继续进入 `/v1/user/follow/add`；该问题登记为 P1 `R04`，要求在 SDK 边界拒绝且不发网络请求。既有 SearchUsers account binding、follow read-back、strict live、bare-ID probe 等未决项仍由既有 owner 承接，未重复伪造为新完成项。
- Green 命令及验收断言：`go test ./scripts/tests/documentation -count=1`、`go test ./... -count=1`、`go vet ./...`、`sh scripts/build.sh`、`go test -race ./internal/shared/traversal ./internal/mcpserver/pixiv/internal/runtime ./internal/mcpserver/pixiv/tools/search_illust -count=1` 与 `git diff --check` 均通过；`awk` 审计结果为 `required_rows=41 scope_admitted_rows=41 non_scope_admitted_required=0`；worktree 在构建后无非预期变更且本地/远端仍一致。全量测试和 build 通过不替代 R04 的待补 Red→Green 回归，也不替代未来 live/mutation gate。
- Findings / 修复登记：`R04`（pending，P1）负责 `FollowUser` 的 `restrict` 本地校验与 no-network 测试；T12 显式依赖 R04，避免 SDK compatibility 在该输入契约未修复前继续推进。R01、T07/T09A/T12/T18/T21/T35/T37/T38 等既有 pending owner 仍按原职责处理 cursor、adapter、read-back、resolver、wire 与发布兼容，不在本轮扩张范围。
- 公开兼容性影响：本轮只更新 Goal-3 ledger，不改变 SDK symbol、CLI/MCP wire、endpoint、默认值、依赖、required_scope、live 数据或运行配置；R04 完成后应仅把已明确非法的 follow restrict 提前分类为 `InvalidArgument`，空值默认 public 的正常路径保持不变。
- 回滚前提 / 依赖闭包：回滚本轮需同时撤销 CHECK-03 verified、R04 表格/登记与 T12 对 R04 的依赖；不涉及业务数据、账号、token、构建产物或生产配置。执行 R04 时需将 `sdk/pixiv/ops_mutation.go`、其聚焦测试和对应 ledger 作为同一闭包验证，不能只删校验而保留测试或依赖声明。
- 实际结果 / evidence / 风险：CHECK-03 已 verified，唯一新增发现为 R04；Goal 继续 incomplete，41 条 required capability 未提升，下一 task 按账本进入 R04，不进入 T12。

## R04 完成记录

- Owner package / 涉及文件：公开 Pixiv SDK mutation；`sdk/pixiv/ops_mutation.go`、`sdk/pixiv/ops_mutation_test.go`。只补 `FollowUser` 的输入边界校验，不改变 follow endpoint、PostForm transport、read-back/outcome 或 CLI/MCP wire。
- Depends on：CHECK-03、T06 verified。
- 冻结 contract / fixture：空 `restrict` 仍先规范化为 `RestrictPublic`；随后复用已有 `validateRestrict("FollowUser", ...)`，只允许 `public`/`private`。未知值必须在 SDK 边界返回 `sdk.InvalidArgument`，不得进入 `/v1/user/follow/add`。
- Red 测试、命令及当前行为的预期失败：新增 `TestFollowUserRejectsUnknownRestrictBeforeNetwork` 后，focused test 实际失败，返回 `upstream_unavailable`，且 fake transport 被调用；这证明缺口是可观测的错误分类与越过本地边界，而非仅凭静态推断。
- Green 命令及验收断言：非法值测试通过并断言 transport 调用次数为 0；`TestFollowUserDefaultsEmptyRestrictToPublic` 通过并断言原 endpoint/form 仍发送 `restrict=public`。随后 `go test ./sdk/pixiv -count=1`、focused `go test`、focused `go test -race`、`go vet ./sdk/pixiv`、`go test ./... -count=1`、`go vet ./...`、`sh scripts/build.sh`、`go test` 文档检查与 `git diff --check` 均通过。公开 CLI reference 与 MCP 文档已说明 `public|private`，本修复没有新增文档契约或需要更新的 locale/Skill 文本。
- 公开兼容性影响：无新增 symbol、依赖、endpoint、默认值或 wire 变化；合法 `public`/`private` 与空值默认 public 保持原请求，只有原本会被发送的非法值改为本地 `InvalidArgument`。
- 回滚前提 / 依赖闭包：回滚需同时撤销 `FollowUser` 的校验调用、两条 external SDK 回归和本完成记录；不涉及账号、token、运行数据或配置。T12 对 R04 的依赖保持，不能只回滚生产行而保留已宣称的验证记录。
- 实际结果 / evidence / 风险：R04 已 verified，未发现新的 P0/P1；Goal 仍 incomplete，41 条 required capability 未提升。下一入口按 DAG 为 T12。

## T12 完成记录

- Owner package / 涉及文件：公开 SDK compatibility；`sdk/pixiv/ops_novel.go`、`sdk/pixiv/pixiv_test.go`、`internal/mcpserver/pixiv/pixiv_read_test.go`、`goal-3/api-migration-verification.md`、`goal-3/upstream-contract-matrix.md`、`goal-3/plan.md`、`docs/en/sdk.md`、`docs/zh-CN/sdk.md`。只处理 SDK symbol/cursor compatibility 与已 rejected novel content 的负向边界，没有进入 T39A 的 CLI/MCP map 实施。
- Depends on：T20、T01、T02、T03、T04、T05、T06、CHECK-03、R04 verified。
- 冻结 contract / fixture：完成逐 method 的旧 `Client` surface → request/result model → wrapper/deprecation/error map；保留旧 methods、requests、models、named fields、enum types、`AddBookmark`/`RemoveBookmark` wrapper 与 shared `sdk.Cursor` outer format。`CheckpointSearchArtworks`/`CursorContext` 记为 additive；仅 `SearchArtworks` binding version 为 2，旧 version-1 cursor 返回 `InvalidCursor`。`SearchNovels`/`SearchUsers` 明确冻结为 public-scoped cursor，并以跨 client synthetic fixture 锁定。`NovelContent` 保留 exported symbol/model/DTO，标记 deprecated，正数 ID 返回既有 `sdk.ContentUnavailable` 且不请求已 rejected endpoint；非法 ID 仍 `InvalidArgument`。
- Red 测试、命令及当前行为的预期失败：先新增 `TestNovelContentDeprecatedEntryPointDoesNotCallRejectedEndpoint`，运行 `go test ./sdk/pixiv -run '^TestNovelContentDeprecatedEntryPointDoesNotCallRejectedEndpoint$' -count=1` 实际失败：旧实现通过 fake transport 发起请求并返回 `upstream_unavailable`，而契约要求 `content_unavailable`/零网络。该 Red 证据确认问题是 deprecated wrapper 仍调用 rejected path，不是静态推断。既有 `TestNovelContentPublicParserPreservesUnknownBlock` 随契约改为 negative regression；MCP 对应旧成功 fixture 也改为结构化 unsupported error 断言。
- Green 命令及验收断言：`go test ./sdk/pixiv -run '^(TestNovelContentDeprecatedEntryPointDoesNotCallRejectedEndpoint|TestSearchNovelsWiresQueryAndCursor|TestSearchUsersWiresQueryAndCursor)$' -count=1`、`go test ./sdk/pixiv -run '^TestSearchNovelsAndUsersCursorsArePublicScoped$' -count=1`、`go test ./sdk/pixiv -run '^TestLegacySDKConsumerCompiles$' -count=1`、`go test ./sdk/pixiv -count=1`、`go test -race ./sdk/pixiv -count=1`、`go test ./internal/mcpserver/pixiv -run '^TestNovelContentReportsUnsupportedWithoutCallingRejectedEndpoint$' -count=1`、`go test ./...`、`go vet ./...`、`go test ./scripts/internal/publicapi -count=1`、`go test ./scripts/tests/documentation -count=1`、`sh scripts/build.sh`、`git diff --check` 均通过。public API inventory digest 仍通过，旧消费者 interface/literals 编译通过，MCP tool schema/name 未被本任务删除。
- 公开兼容性影响：未删除或重命名 public SDK symbol、request/model/named field、enum、error reason、constructor、resource method 或 cursor outer format；未新增依赖。唯一有意的运行时兼容边界是 `NovelContent` 从调用 rejected endpoint 改为本地 `ContentUnavailable`，并使现有 `novel_content` MCP 调用得到 structured error 而不是伪造正文；其 tool/schema compatibility 仍留给 T39A 逐项冻结。`SearchNovels`/`SearchUsers` 不新增 account binding，避免无批准的 cursor breaking change。
- 回滚前提 / 依赖闭包：回滚需同时撤销 `NovelContent` deprecated/no-network 实现、SDK/MCP negative tests、T12 symbol map 与双语 SDK 说明，以及 plan/matrix/tasks 状态；不能只恢复 production method 而保留“rejected endpoint 不可调用”的完成记录。未涉及账号、token、下载内容、运行配置、live API 或新依赖。
- 实际结果 / evidence / 风险：T12 已 verified。Goal 仍 incomplete，41 条 required capability 继续为 `scope_admitted`，没有被兼容审计提升为 `public_ready`。已知后续风险是 T39A 仍需冻结 CLI alias、MCP tool/input/output/default/error 与旧 JSON replay；T07–T11、T13–T45 仍按 DAG 实现 endpoint、SDK、shared、CLI/MCP、文档和最终发布门禁。下一入口按 goal-mode 规则为 T39A，不直接进入 T07。

## T39A 完成记录

- Owner package / 涉及文件：CLI/MCP migration contract；`goal-3/cli-migration-matrix.md`、`goal-3/mcp-compatibility-matrix.md`、`goal-3/api-migration-verification.md`、`goal-3/plan.md`、`docs/en/cli-reference.md`、`docs/zh-CN/cli-reference.md`、`docs/en/mcp-tools.md`、`docs/zh-CN/mcp-tools.md`、`skills/pixiv-cli/SKILL.md`、本文件。没有修改生产 endpoint、SDK、CLI/MCP handler 或 tool registration。
- Depends on：T12 verified。
- 冻结 contract / fixture：完成当前 CLI route → canonical operation → positional/flag default → compatibility decision map；完成注册表与 exact-set 测试对应的 40 个 Pixiv MCP tool 逐项 input required/default、structured output、`isError`/稳定 error、旧 JSON fixture 与 replay assertion map；明确保留 `add_bookmark.illust_id`、`bookmark_tags.bookmark_tags` 和 `novel_content` 的 wire/schema，其中正数 novel content 只返回 `content_unavailable`、不请求 rejected endpoint。现有代表性 replay 测试列入证据表，其余逐行离线回放留给 T37/T38/T43。
- Red 测试、命令及当前行为的预期失败：T39A 是 contract/documentation freeze，没有生产代码 Red 阶段。只读核验发现原 T39A 只有占位列，且 CLI/MCP locale 与 T12 已确定的 `novel_content` no-network/unsupported 行为仍有过时成功描述；本任务补齐矩阵、回放清单和双语说明，没有把未来 endpoint 实现伪装成已完成。
- Green 命令及验收断言：`go test ./scripts/tests/documentation -count=1`、`go test ./internal/mcpserver/pixiv -run '^(TestServerListsExpectedTools|TestMCPStdioKeepsJSONRPCOnStdout|TestSDKMutationToolsReturnStructuredSuccess|TestSDKMutationTypedErrorIsMCPError|TestNovelContentReportsUnsupportedWithoutCallingRejectedEndpoint|TestSDKUserListToolsSchemaRejectsRemovedLegacyFields|TestSDKUserListToolsUseCanonicalUserIDAndFilters)$' -count=1`、`go test ./internal/cli/commands/pixiv/... -count=1`、`git diff --check` 均通过；并按矩阵逐项运行当前 CLI 及相关 leaf `--help` 核对 route、selector 和 default。矩阵 40 个 tool name 与 `internal/mcpserver/pixiv/tools` 注册表逐项相同。
- 公开兼容性影响：不改变 SDK symbol、CLI/MCP wire、endpoint、依赖、账号、token、下载内容或 live API；只修正文档对已冻结行为的表述，并把 route/tool/default/error 与旧 JSON replay 约束显式化。后续 T24–T38 仍必须按本矩阵实现，T39B/T43 仍需逐项回放和全量回归；40 个 tool 的冻结不等于能力已 `public_ready`。
- 回滚前提 / 依赖闭包：文档与任务账本变更可整体回滚，但必须同时撤销两个 compatibility matrix、verification/plan 链接、双语 CLI/MCP/Skill 说明和 T39A 状态；不得只删除矩阵而保留后续 owner 对其字段/default/error 的引用。未涉及账号、token、运行配置、live API 或新依赖。
- 实际结果 / evidence / 风险：T39A 已 verified。代表性 stdio、mutation、user-list schema、novel-content no-network、exact registration replay 已通过；完整逐 tool 离线 replay、endpoint owner 实现、CLI/MCP 全量回归仍由 T37/T38/T39B/T43/T45 完成。41 条 required capability 仍为 `scope_admitted`，Goal 继续 incomplete；下一入口按 DAG 为 T07。

## T07 拆卡记录

- Owner package / 涉及文件：read endpoint owner umbrella；本文件。T07 同时覆盖 15 个 required capability，跨 artwork、novel、user/relationship 与 feed/relationship adjunct owner，按任务规则先拆为 T07A–T07D；本轮没有修改生产代码、protocol、SDK、CLI/MCP wire 或公开文档。
- Depends on：T12、T06 已 verified；四个子卡各自只依赖已冻结的 contract/compatibility，不依赖尚未完成的后续 SDK/CLI/MCP owner。
- 冻结 contract / fixture：T07A 负责 `artwork-search`、`artwork-series`、`ugoira-metadata` 等 artwork leaf；T07B 负责 `novel-search`、`novel-detail`、`novel-series`，其中 detail/series 必须迁移到已冻结的 v2 path；T07C 负责 `user-search`、`user-detail`、`user-artworks`、`user-novels`、`user-relationships`，bare-ID 命令 resolver 仍归 T21；T07D 负责 `trending`、`mypixiv`、`follow-mutation` 所需 endpoint leaf。每个子卡必须分别提交 method/path、request、DTO、null/empty、错误映射和离线 fixture；T07 umbrella 在四卡及其回归完成后审计汇总。
- Red 测试、命令及当前行为的预期失败：这是 task decomposition/ledger task，无生产代码 Red 阶段。静态审计确认 T07 capability-admission 中有 15 行，且现有 endpoint 目录同时包含 v1/v2 混合路径与 user/feed families；若不拆卡，无法把 novel v2 禁止回退、user identity、follow outcome 等不同风险绑定到独立 owner 和回归证据。
- Green 命令及验收断言：`awk` 核对 `capability-admission.md` 中 T07 adapter owner 共 15 个 capability；`rg --files internal/services/pixiv/endpoint` 核对 artwork/novel/user/feed leaf 目录；`go test ./scripts/tests/documentation -count=1` 与 `git diff --check` 在回写前后通过。任务表现已形成 T07A–T07D → T07 umbrella 的显式依赖，当前下一入口为 T07A。
- 公开兼容性影响：没有 public SDK symbol、endpoint 请求、CLI/MCP wire、默认值、依赖、账号或 live 数据变化；只增加任务分解和审计边界，15 个 capability 继续为 `scope_admitted`。
- 回滚前提 / 依赖闭包：回滚只需撤销四个子卡、T07 umbrella 依赖和本记录；后续若已有 owner 引用某子卡，必须同时撤销该依赖或先补兼容记录，不得只删除父卡。
- 实际结果 / evidence / 风险：T07 的拆卡已完成，但 T07 父任务仍为 pending，不能据此宣告任何 capability `contract_frozen`、`migration_ready` 或 `public_ready`。下一轮按 DAG 执行 T07A，不进入 T07B–T08。

## T07A 完成记录

- Owner package / 涉及文件：artwork read endpoint；`internal/services/pixiv/endpoint/artwork/detail/detail.go`、`internal/services/pixiv/endpoint/artwork/detail/detail_test.go`。search 与 series leaf 沿用已存在的 `/v1/search/illust`、`/v1/illust/series` adapter、DTO 和 required/null/empty/continuation 校验；本任务只补齐 ugoira frame 文件安全边界，没有进入 SDK、CLI 或 MCP。
- Depends on：T12 verified；T01 artwork contract 已冻结。
- 冻结 contract / fixture：按 `goal-3/upstream-contract-matrix.md` 的 T01 约束，ugoira metadata 必须有可用 archive 和非空 frames；每个 frame file 必须非空、相对、安全且不重复。既有 artwork search/series 离线 fake transport fixture 继续覆盖 method/path、query、DTO 映射、空列表和 malformed required envelope。
- Red 测试、命令及当前行为的预期失败：新增 `TestUgoiraRejectsUnsafeOrDuplicateFrameFiles` 后运行 `go test ./internal/services/pixiv/endpoint/artwork/detail -run '^TestUgoiraRejectsUnsafeOrDuplicateFrameFiles$' -count=1` 实际失败；旧 adapter 接受 `../000000.jpg`、绝对路径、`frames/../000000.jpg`、Windows 分隔符路径和重复文件名，并返回成功结果。
- Green 命令及验收断言：`UgoiraMetadata` 在 DTO 映射前拒绝 NUL、绝对路径、Windows volume、空/`.`/`..` path segment 与归一化后的重复 frame file，返回既有 `protocol.MalformedResponse()`；新增测试与 `TestUgoiraMapsRequiredMetadata` 通过。随后 `go test ./internal/services/pixiv/endpoint/artwork/detail ./internal/services/pixiv/endpoint/artwork/search ./internal/services/pixiv/endpoint/artwork/series -count=1`、对应三包 `go vet`、`git diff --check` 均通过。
- 公开兼容性影响：无 public SDK symbol、CLI/MCP wire、endpoint path、依赖或默认值变化；合法的普通 frame file 与既有 artwork search/series 请求保持不变。只把不满足已冻结安全契约的上游 payload 提前分类为 malformed，避免后续解包/落盘边界产生路径穿越、覆盖或重复歧义。
- 回滚前提 / 依赖闭包：回滚需同时撤销 `validFrameFiles` 校验、ugoira 负向 fixture 和本完成记录；不得只回滚生产校验而保留 T07A verified 状态。未涉及账号、token、下载内容、运行配置或 live API。
- 实际结果 / evidence / 风险：T07A 已 verified；artwork search、series、ugoira adapter leaf 的现有离线回归通过，ugoira 文件安全缺口已按 T01 contract 补齐。T07 umbrella 仍 pending，T07B/T07C/T07D 尚未完成，41 条 required capability 仍未提升到 `public_ready`，Goal 继续 incomplete；下一入口按 DAG 为 T07B。

## T07B 完成记录

- Owner package / 涉及文件：novel read endpoint；`internal/services/pixiv/protocol/protocol.go`、`internal/services/pixiv/endpoint/novel/{search,detail,series,novel}.go` 及对应离线测试；同步更新 `sdk/pixiv/pixiv_test.go` 与 `internal/mcpserver/pixiv/pixiv_sdk_wire_test.go` 的既有 wire fixture。没有修改 SDK/CLI/MCP 生产实现。
- Depends on：T12 verified；T02 novel contract 已冻结。
- 冻结 contract / fixture：novel search 继续使用 `/v1/search/novel` 与 required `novels` list；novel detail/series canonical path 改为 `/v2/novel/detail`、`/v2/novel/series`，不请求已 rejected 的 v1 path。detail 保留可选 `series_next`/`series_prev` 的正数 ID 与前后 title；series 要求 `novel_series_detail`、`novels` list、正数 series/user/novel ID，空 list 合法且 normalized items 非 nil，续页仍只提取 `last_order`。
- Red 测试、命令及当前行为的预期失败：先把 endpoint fixture 改为 v2 并加入 series required list/empty-list 断言，运行 `go test ./internal/services/pixiv/endpoint/novel/detail ./internal/services/pixiv/endpoint/novel/series -run '^(TestDetailMapsNovelAndSeriesReferences|TestSeriesMapsRouteQueryAndContinuation|TestSeriesRejectsMissingDetailOrInvalidNovel)$' -count=1` 实际失败：旧实现仍请求 v1、series 缺失/null `novels` 仍成功，且新前后 title 断言缺少 normalized 字段。
- Green 命令及验收断言：新增 v2 path 常量映射、detail 前后 series title、series required-list DTO/error mapping，并补 search/series 空列表 fixture。`go test ./internal/services/pixiv/endpoint/novel/search ./internal/services/pixiv/endpoint/novel/detail ./internal/services/pixiv/endpoint/novel/series ./sdk/pixiv ./internal/mcpserver/pixiv -count=1`、`go test ./... -count=1`、`go vet ./...`、`sh scripts/build.sh`、`git diff --check` 均通过；`rg` 负向核验确认 `internal/` 与 `sdk/` 不再请求 `/v1/novel/detail` 或 `/v1/novel/series`。
- 公开兼容性影响：未删除或重命名 SDK symbol、CLI/MCP tool/schema、request/response wire 或依赖；既有 `Novel`、`NovelSeries` 调用现在经同一 adapter 请求已冻结的 v2 path。v1 不做 fallback。SDK/MCP 测试只更新离线 path fixture；CLI/MCP 生产层未进入本任务。T14 后续仍负责 public SDK 的 series metadata、cursor 与源码兼容审计。
- 回滚前提 / 依赖闭包：回滚需同时撤销 v2 protocol 常量、detail/series adapter 与 required-list 校验、novel search/series fixture、SDK/MCP path fixture 和本完成记录；不得只恢复 v1 path 而保留 T07B verified 或后续 T14 对 v2 的依赖。未涉及账号、token、下载内容、运行配置或 live API。
- 实际结果 / evidence / 风险：T07B 已 verified；novel search、v2 detail、v2 series adapter 的离线 method/path/query/DTO/null/empty/error 回归通过，rejected v1 detail/series 未被请求。T07 umbrella 仍 pending，T07C/T07D 尚未完成，41 条 required capability 仍未达到 `public_ready`，Goal 继续 incomplete；下一入口按 DAG 为 T07C。

## T07C 完成记录

- Owner package / 涉及文件：user read endpoint；`internal/services/pixiv/endpoint/user/{search,detail,novels,related,followers,following,blocked}`、`internal/services/pixiv/endpoint/artwork/timeline/timeline.go` 的 `UserArtworks` 分支及对应离线测试。未修改 SDK、CLI/MCP production、resolver 或 continuation owner。
- Depends on：T12、T06 verified。
- 冻结 contract / fixture：覆盖 `/v1/search/user`、`/v1/user/detail`（`Current` 固定 `filter=for_android`）、`/v1/user/illusts`、`/v1/user/novels`（固定 `filter=for_android`）、`/v1/user/following`、`/v1/user/follower`、`/v1/user/related` 与 `/v2/user/list` 的 method/path/query/DTO 映射。required user/profile/workspace object、required user/novel/illust list、blocked 的 `users` 与兼容 `user_previews` envelope、正数实体 ID、空数组 non-nil、null/缺失/非法响应的 `protocol.MalformedResponse` 均有离线 fixture。UserArtworks 在 adapter leaf 规范化空值与既有 `illustration` 拼写为 upstream `illust`，保留 `illust/manga/ugoira`，非正 user ID 和未声明 subtype 在发起 transport 前拒绝；bare-ID resolver 继续归 T21，续页额外 key/path allowlist 继续归 T11。
- Red 测试、命令及当前行为的预期失败：新增 `TestUserArtworksNormalizesIllustrationAndRejectsInvalidRequest` 后运行 `go test ./internal/services/pixiv/endpoint/artwork/timeline -run '^TestUserArtworksNormalizesIllustrationAndRejectsInvalidRequest$' -count=1` 实际失败：旧实现把 `illustration` 原样发出，并接受 `user_id<=0` 与 `type=all`。严格校验初版随后由全量 `go test ./... -count=1` 暴露 MCP 省略 `type` 的既有默认路径，补充“空 subtype → illust”回归后再次 Red，修正后恢复兼容。
- Green 命令及验收断言：`go test ./internal/services/pixiv/endpoint/user/search ./internal/services/pixiv/endpoint/user/detail ./internal/services/pixiv/endpoint/user/novels ./internal/services/pixiv/endpoint/user/related ./internal/services/pixiv/endpoint/user/followers ./internal/services/pixiv/endpoint/user/following ./internal/services/pixiv/endpoint/user/blocked ./internal/services/pixiv/endpoint/artwork/timeline ./internal/mcpserver/pixiv -count=1`、`go test ./internal/services/pixiv/... ./sdk/pixiv -count=1`、`go test ./... -count=1`、`go vet ./...`、`go test ./scripts/tests/documentation -count=1`、`sh scripts/build.sh` 与 `git diff --check` 均通过；LSP diagnostics 对 9 个受影响 Go 文件无错误。测试覆盖 user search/detail/artworks/novels/relationships 的 DTO、required/null/empty/error 语义与 blocked envelope 兼容。
- 公开兼容性影响：未删除或重命名 SDK symbol、request/model、CLI/MCP tool/schema、resolver、依赖或账号行为；保留空 `user_artworks.type` 的既有默认成功路径，并将旧 SDK `illustration` 拼写安全映射为 App API `illust`。不符合 T06 的 user-artworks ID/subtype 请求不再触网。public SDK 对非法 subtype 的最终 `InvalidArgument` 分类与 cursor binding 仍由后续 T13/T14/T19 owner 完成；T11 仍负责 continuation allowlist，不在本卡提前实现。
- 回滚前提 / 依赖闭包：回滚需同时撤销 UserArtworks subtype/ID 规范化、9 个 endpoint fixture 增补和本完成记录；不得只恢复 wire 行为而保留 T07C verified。未涉及账号、token、下载内容、运行配置、live API 或新依赖；后续 T11/T13/T14/T21/T23/T34/T37 若已引用本 adapter 约束，回滚时需同步撤销相应依赖或先提供兼容修复。
- 实际结果 / evidence / 风险：T07C 已 verified；user search/detail/artworks/novels/relationships adapter leaf 与离线 DTO/null/empty/error 回归通过，T07 umbrella 仍等待 T07D 后再汇总，41 条 required capability 仍为 `scope_admitted`，Goal 继续 incomplete。剩余已知边界是 continuation 严格 allowlist（T11）、public SDK 错误/cursor 语义（T13/T14/T19）和 bare-ID resolver（T21）；下一可执行子卡按 DAG 为 T07D。

## T07D 完成记录

- Owner package / 涉及文件：feed/relationship adjunct endpoint；`internal/services/pixiv/endpoint/artwork/trending`、`internal/services/pixiv/endpoint/artwork/timeline` 的 MyPixiv 分支、`internal/services/pixiv/endpoint/novel/timeline` 的 MyPixiv 分支、`internal/services/pixiv/endpoint/user/mypixiv`、`internal/services/pixiv/endpoint/user/follow` 及对应离线测试。未修改 public SDK、CLI/MCP production、mutation read-back、continuation owner 或 live evidence。
- Depends on：T12、T06 verified；T01 artwork 与 T02 novel normalized contract 已冻结。
- 冻结 contract / fixture：trending 固定 `GET /v1/trending-tags/illust` 且无 query/continuation，required `trend_tags` 中的 tag、sample artwork 与正数 artwork ID 必须完整，空数组合法；MyPixiv users 固定 `GET /v1/user/mypixiv`、verified current user ID、`filter=for_android` 与正数 offset continuation，required `user_previews` 的 user ID 必须为正数；MyPixiv artworks/novels 固定 `/v2/illust/mypixiv` 与 `/v1/novel/mypixiv`，required list、正数 item/nested owner ID 与 non-nil empty list 保持；follow 固定 `POST /v1/user/follow/add` / `/v1/user/follow/delete`，Add 只接受正数 user ID 与 `public|private` restrict，Remove 只接受正数 user ID，2xx 仍是 accepted-only，不在本卡宣告关系读回成功。
- Red 测试、命令及当前行为的预期失败：先新增 `TestMyPixivRejectsNonPositiveNestedOwnerID`、`TestListRejectsNonPositiveUserIDBeforeTransport` 与 `TestFollowRejectsInvalidRequestsBeforeTransport`，分别运行 artwork timeline、MyPixiv users、follow 的 focused `go test -run ... -count=1`，实际失败：旧实现接受非正 nested owner/current-user ID 和非法 follow request，并已经调用 fake transport。
- Green 命令及验收断言：补充 MyPixiv artwork owner ID、MyPixiv users positive-ID preflight、follow ID/restrict preflight；新增 trending 的 required/null/empty/tag/sample ID fixture、MyPixiv users null/empty fixture、MyPixiv artwork/novel DTO fixture 和 follow transport-error propagation fixture。`go test ./internal/services/pixiv/endpoint/artwork/trending ./internal/services/pixiv/endpoint/artwork/timeline ./internal/services/pixiv/endpoint/novel/timeline ./internal/services/pixiv/endpoint/user/mypixiv ./internal/services/pixiv/endpoint/user/follow -count=1`、`go test ./internal/services/pixiv/... ./sdk/pixiv ./internal/mcpserver/pixiv -count=1`、`go test ./... -count=1`、`go vet ./...`、`go test ./scripts/tests/documentation -count=1`、`sh scripts/build.sh` 与 `git diff --check` 均通过；LSP diagnostics 对三个受影响 production endpoint 文件为空。
- 公开兼容性影响：不删除或重命名 SDK symbol、CLI/MCP tool/schema、request/response wire、依赖、账号行为或 follow 结果语义；合法请求的 path/query/form 保持不变。只把不满足 T06 的 endpoint 输入和 MyPixiv nested schema 分类为显式错误，并在发起 transport 前拒绝本地非法请求。SDK compatibility layer 继续负责空 restrict → `public` 默认；mutation read-back/outcome 仍留给 T35/T38/T07 后续边界，public `updated_at` 字段的跨 artwork/novel mapper 闭包留给 T13/T14。
- 回滚前提 / 依赖闭包：回滚需同时撤销 trending/MyPixiv/follow 负向与 DTO fixture、三个 endpoint 的校验、T07D 状态和本完成记录；不得只恢复 wire 行为而保留 T07D verified。未涉及账号、token、下载内容、运行配置、依赖或 live API；后续 T07 umbrella、T11、T13/T14、T35/T38 若已引用这些 adapter 约束，回滚时须同步撤销依赖或先补兼容修复。
- 实际结果 / evidence / 风险：T07D 已 verified；trending、MyPixiv users/artworks/novels 与 follow endpoint leaf 的 method/path/request/DTO/null/empty/error 离线证据已补齐，T07 umbrella 现在可在四个子卡完成后执行汇总审计。41 条 required capability 仍为 `scope_admitted`，未执行 live API，也未把 follow 2xx 提升为关系已改变或把 `updated_at` optional public mapper 提前宣告完成；Goal 继续 incomplete，下一入口为 T07 umbrella。

## T07 完成记录

- Owner package / 涉及文件：read endpoint umbrella audit；复核 T07A–T07D 的完成记录、`internal/services/pixiv/endpoint/{artwork,novel,user}` leaf adapter 与对应离线 fixture，以及 `goal-3/capability-admission.md`、本文件。父任务只做汇总与账本回写，没有新增生产代码、protocol、SDK、CLI/MCP wire 或 live API。
- Depends on：T12、T07A、T07B、T07C、T07D 均已 verified；T01/T02/T06 的 normalized contract 已作为四张子卡的共同前置。`bare-id-probe` 虽列在 T07 的 adapter 映射中，但其 resolver/probe 实现仍由 T21 负责，不在本父任务中提前完成。
- 冻结 contract / fixture：T07A 的 artwork search/series/ugoira adapter 与 frame 安全边界、T07B 的 novel search 及 `/v2/novel/detail`、`/v2/novel/series` required-list/empty/error 语义、T07C 的 user search/detail/artworks/novels/relationships ID/subtype/required-null-empty/error 语义、T07D 的 trending/MyPixiv/follow path、request/DTO/preflight/error 语义均有各自完成记录与 leaf fixture。四卡合计覆盖 14 个具体 endpoint capability；第 15 行 `bare-id-probe` 保留 T21 的边界，不把 resolver 误记为已实现。
- Red 测试、命令及当前行为的预期失败：这是依赖闭包与证据汇总的 audit-only task，没有生产代码 Red 阶段。审计开始时任务表准确显示 T12、T07A–T07D 已 verified 而 T07 仍 pending；没有用历史子卡完成记录自动替代父任务审计，也没有把 fixture 证据升级为 `public_ready`。
- Green 命令及验收断言：`awk` 核对 T07 adapter owner 共 15 行且全部为 `scope_admitted`；`rg --files internal/services/pixiv/endpoint` 核对目标 endpoint fixture 共 18 个；正向检查确认 `/v1/trending-tags/illust`、`/v2/illust/mypixiv`、`/v1/novel/mypixiv`、`/v1/user/mypixiv`、`/v1/user/follow/{add,delete}` 均存在；负向 `rg` 确认 `internal/` 与 `sdk/` 不含 rejected 的 `/v1/novel/detail` 或 `/v1/novel/series`。18 个目标 endpoint 包的聚焦 `go test ... -count=1`、`go test ./... -count=1`、`go vet ./...`、`go test ./scripts/tests/documentation -count=1`、`sh scripts/build.sh` 与 `git diff --check` 均通过。
- 公开兼容性影响：没有新增或删除 public SDK symbol、CLI/MCP tool/schema、endpoint wire、默认值、依赖、账号/token 行为或下载数据；合法请求路径与 DTO 由各子卡保持。41 条 required capability 仍全部为 `scope_admitted`，本审计不授予 `contract_frozen`、`migration_ready` 或 `public_ready`；不执行 live API。同步把任务账本末尾与上方完成记录矛盾的 R04 追加行改为 `verified`，不改变 R04 生产代码或其证据。
- 回滚前提 / 依赖闭包：本父任务提交只包含 T07 状态、T07 完成记录和 R04 账本状态的一致性修复，回滚时可整体撤销且不需要业务数据、账号、token、配置或生产代码回滚；若撤销任一 T07A–T07D 实现，必须连同对应 adapter、fixture、完成记录及父任务依赖重新审计，不得只把父行保留为 verified。
- 实际结果 / evidence / 风险：T07 umbrella 已 verified，四个子卡的 read adapter、DTO、错误映射与离线 leaf fixture 证据闭合；Goal 仍 incomplete，未完成的 SDK/public cursor（T13/T14/T19）、continuation allowlist（T11）、bare-ID resolver（T21）、CLI/MCP 与 mutation read-back（T35/T38）等边界保持原状，`updated_at` 的跨 artwork/novel public mapper 仍留给 T13/T14。按任务表下一入口为 T08。

## T08 拆卡记录

- Owner package / 涉及文件：bookmark endpoint owner umbrella；本文件、`goal-3/upstream-contract-matrix.md` 的 T03 contract、`internal/services/pixiv/endpoint/artwork/bookmark`、`internal/services/pixiv/endpoint/user/novelbookmarks` 及其测试。T08 跨两类内容和 read/mutation 边界，按任务规则拆为 T08A–T08D；本轮没有修改生产代码、protocol、SDK、CLI/MCP wire、公开文档或依赖。
- Depends on：T12、T03、T07 已 verified；T08A/T08B 只依赖已冻结的 bookmark contract 与 compatibility，T08C/T08D 分别依赖对应 read 子卡的 request/DTO 边界。`list/tags --type all` 的双流聚合、aggregate cursor、统一 Skip/Limit/OneBatch 与页原子性仍归 T19/T23/T27/T37，不在 endpoint 子卡中重复实现。
- 冻结 contract / fixture：T08A 负责现有 artwork bookmark `Artworks`、`Tags`、`Detail` read leaf，并补 required list、null/empty、tag name、continuation 与错误边界；未经验证的 artwork `type/content_type` 不新增 wire。T08B 负责现有 novel bookmark list，并为 `/v1/user/bookmark-tags/novel` 与 `/v2/novel/bookmark/detail` 建立严格 candidate snapshot 后再落 adapter/DTO。T08C/T08D 分别负责 artwork/novel add/remove endpoint leaf 的正数 ID、restrict/tags/form/path/error fixture；response-bearing transport、写后读回、结果不确定性和同账号恢复仍由 T09A/T15/T38/T44 等后续 owner 闭合。
- Red 测试、命令及当前行为的预期失败：这是 task decomposition/ledger task，无生产代码 Red 阶段。只读审计确认 artwork bookmark 当前已有 list/tags/detail，但 tags 使用普通 slice 会把 required `bookmark_tags` 的缺失/null 当成空结果；novel 当前只有 list，novel tags/detail/add/remove 的 wire/DTO/adapter/SDK 仍是 `not_tested`/candidate；两类 mutation 仍使用 error-only `PostForm`，不能用 2xx/nil 证明状态。上述缺口分别绑定到 T08A–T08D，不以一个宽泛父任务掩盖。
- Green 命令及验收断言：`goal-3/tasks.md:40` 与 T03 表核对 T08 的真实范围；`sed`/`rg` 核对 `internal/services/pixiv/endpoint/artwork/bookmark/bookmark.go` 的现有 method/path/DTO 与 `internal/services/pixiv/endpoint/user/novelbookmarks/novelbookmarks.go` 的 novel list leaf；`go test ./scripts/tests/documentation -count=1`、`git diff --check` 通过。拆卡后依赖闭包为 T08A/T08B → 对应 mutation 子卡 → T08 umbrella，当前下一入口为 T08A。
- 公开兼容性影响：只增加任务分解和审计边界，不新增 endpoint、public SDK symbol、CLI/MCP schema、默认值、依赖、账号/token 行为或 live 数据；四张子卡完成前不提升 bookmark capability 状态，41 条 required capability 继续为 `scope_admitted`。candidate novel path 与 mutation read-back 不因拆卡而成为已验证公开行为。
- 回滚前提 / 依赖闭包：回滚只需撤销 T08A–T08D 行、T08 parent 依赖与本记录，不涉及生产代码或业务数据。后续若任一子卡已被 T15/T19/T23/T27/T37/T38 引用，回滚时必须同步撤销依赖或先补兼容修复，不能只删除父卡。
- 实际结果 / evidence / 风险：T08 已完成拆卡但父任务仍为 pending；当前明确的首个实现任务为 T08A（artwork bookmark read）。novel candidate wire、mutation response/read-back、all 聚合和 strict live evidence 仍未完成，Goal 继续 incomplete。

## T08A 完成记录

- Owner package / 涉及文件：artwork bookmark read endpoint；`internal/services/pixiv/endpoint/artwork/bookmark/bookmark.go`、同 stem 的 `bookmark_test.go`，以及下游 SDK 离线回归 fixture `sdk/pixiv/pixiv_test.go`。没有修改 SDK 生产实现、CLI/MCP、protocol、公开 wire 或依赖。
- Depends on：T12、T03、T07 均已 verified；本卡只沿 T08 拆卡记录处理 artwork bookmark 的 `Artworks`、`Tags`、`Detail` read leaf，不进入 novel candidate 或 mutation 子卡。
- 冻结 contract / fixture：`Artworks` 将 `illusts` 作为 required list，缺失/JSON `null` 返回 `MalformedUpstreamResponse`，空数组生成 non-nil empty items；`next_url=null` 为终止，空值、缺失 `max_bookmark_id`、非正值和重复 key 为 malformed。`Tags` 将 `bookmark_tags` 作为 required list，缺失/JSON `null` 为 malformed，空数组合法且 non-nil，tag name 必须非空，并复用正 `offset` continuation。`Detail` 保留 404、`bookmark_detail:null` 和明确 `is_bookmarked:false` 的空状态归一化；明确未收藏却携带 restrict/tag 的矛盾 payload 返回 malformed，其他 transport/upstream error 原样传播。请求继续使用已冻结 path/query，未发送未经验证的 `type` 或 `content_type`。
- Red 测试、命令及当前行为的预期失败：先新增 `TestBookmarkTagsRequireBookmarkTagsList`，运行 `go test ./internal/services/pixiv/endpoint/artwork/bookmark -run '^TestBookmarkTagsRequireBookmarkTagsList$' -count=1` 实际得到缺失/null `bookmark_tags` 的 nil error；新增 `TestBookmarkDetailRejectsContradictoryUnbookmarkedFields` 后运行对应 focused test，旧实现同样错误地接受两种矛盾 payload。完成 detail 校验后首次 `go test ./... -count=1` 又暴露 `sdk/pixiv` 中旧的矛盾 absent fixture，随后将该下游样本改为合法的空状态，未弱化生产校验。
- Green 命令及验收断言：`TestBookmarkTagsRequireBookmarkTagsList`、`TestBookmarkArtworksRejectMalformedEnvelopeAndKeepEmptyPage`、`TestBookmarkReadPropagatesTransportErrors`、`TestBookmarkTagsRejectMalformedItemsAndContinuation`、`TestBookmarkDetailRejectsContradictoryUnbookmarkedFields` 及既有 list/detail/mutation fixture 均通过；`go test -race ./internal/services/pixiv/endpoint/artwork/bookmark -count=1`、相关 endpoint/SDK/MCP 回归、`go test ./... -count=1`、`go vet ./...`、`go test ./scripts/tests/documentation -count=1`、`sh scripts/build.sh`、`git diff --check` 均通过。LSP diagnostics 对三个受影响 Go 文件为空，code-review-expert 自审无 P0/P1/P2 finding。
- 公开兼容性影响：只收紧了已冻结 contract 明确要求的 malformed payload 分类，并补充离线负向回归；合法 artwork bookmark path/query、空页、未收藏空状态、404 和真实错误传播保持不变。没有新增 subtype wire、public SDK symbol、CLI/MCP surface、依赖、账号/token 行为或 live API；SDK 文件仅更新与新 contract 一致的测试 fixture。41 条 required capability 仍全部为 `scope_admitted`，本卡不授予 `public_ready`。
- 回滚前提 / 依赖闭包：回滚需同时撤销 endpoint required-list/矛盾字段校验、对应测试、SDK 下游 fixture 和本完成记录；不涉及业务数据、账号、token、运行配置或生成物。后续若 T11/T13/T15/T23/T27/T37 已引用这些边界，必须连同依赖闭包同步撤销或先补兼容修复，不能只恢复生产行。
- 实际结果 / evidence / 风险：T08A 已 verified。artwork bookmark read leaf 的 required/null/empty/error、continuation 和未验证 subtype wire 边界已由离线 fixture 闭合；未执行 live API。T08B（novel read）、T08C/T08D（两类 mutation）、T08 umbrella、all 聚合、严格 live/read-back 和后续 SDK/CLI/MCP 发布门禁仍为 pending；Goal-3 继续 incomplete，下一入口按拆卡顺序为 T08B。

## 实现任务准入卡

在当前任务下回写以下内容，或链接已有 contract/fixture；不要自动新增独立计划文件。跨 owner 的汇总任务必须先拆成单 owner 子卡；子卡使用父 ID 后缀，列出自己的依赖，父任务在全部子卡完成后完成。

```text
Owner package / 涉及文件：
Depends on：
冻结 contract / fixture：
Red 测试、命令及当前行为的预期失败：
Green 命令及验收断言：
公开兼容性影响：
回滚前提 / 依赖闭包：
实际结果 / evidence / 风险：
```

## 回滚与完成门禁

单独提交用于追踪，不代表任意提交都能独立撤销。回滚须检查依赖闭包：后续 SDK/CLI/MCP 已依赖时，同步撤销依赖任务，或先提供保持构建与公开契约的兼容修复；由执行者记录验证证据。不得误撤无关工作。

最终检查 T45、required_scope 全集、禁止 endpoint 的负向回归、源码与 wire compatibility、文档/Skill/completion 一致性。T44 未执行或任一 required 未 public_ready，Goal 保持 incomplete。

## CHECK-01 追加修复任务

- `R01`（pending）：发布前完成 cursor 完整性与回滚 gate。Owner 为 shared SDK/release compatibility；需检查 `sdk/cursor.go`、`sdk/pixiv/cursor.go`、T23A 分页报告、双语 SDK 文档及最终发布流程。验收必须明确 cursor 不是鉴权凭据，评估不可信输入是否需要完整性保护，并以跨版本回滚/迁移 fixture 证明 v2 cursor 与 shared collector、SDK、CLI/MCP 调用方的依赖闭包；没有证据不得发布。当前不为假设的威胁模型新增签名依赖或固定限制。

## CHECK-02 追加修复任务

- `R02`（verified，P1）：已修复 `internal/mcpserver/pixiv/internal/runtime/runtime.go` 中 MCP local filter `seen` 的 execution-attempt 生命周期。账号池从非零 opaque cursor replay 时会清空上一 attempt 的去重状态，再从同一初始 cursor 重新收集；真实 SDK + 离线 HTTP fixture 覆盖本地 filter、非零 cursor、safe replay、结果不遗漏不重复及 commit 边界。未修改 MCP schema，未加入静默重试，未扩大 T23A 范围；完整证据见上方 `R02 完成记录` 与分页报告。
- `R03`（verified，P2）：已修正 `tasks.md` 的 T03/T04 depends_on 与完成记录，并保留 `/goal-*/` 的通用忽略、仅为已批准的 `goal-3` 增加可追踪例外；未来 Goal 目录仍需明确决定后 force-add。变更仅限计划/仓库 tracking，未改变 required_scope 或公开行为；完整证据见上方 `R03 完成记录`。

## CHECK-03 追加修复任务

- `R04`（verified，P1）：`sdk/pixiv.Client.FollowUser` 已在发起 `/v1/user/follow/add` 前复用 `validateRestrict`，只接受 `public`/`private`；空值继续由兼容层默认 `public`。Red→Green、no-network 回归及完整证据见上方 `R04 完成记录`；不涉及 follow read-back、mutation outcome 或 MCP schema 的后续 T07/T35/T38 范围。
