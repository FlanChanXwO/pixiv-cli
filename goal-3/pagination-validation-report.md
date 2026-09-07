# 分页验证报告

观测日期：2026-09-05（Asia/Shanghai）。

## 已确认两页

- novel-follow：offset，跨页无重复。
- novel-recommended：offset，跨页无重复。
- artwork search：四种 content type，offset，跨页无重复。
- artwork latest：max_illust_id，跨页无重复。
- artwork ranking：offset，跨页无重复。
- ugoira metadata：无分页。

## 已确认 continuation，但当前实现不兼容

### novel-new

- Live continuation：`max_novel_id`。
- 当前 adapter/SDK：`offset`。
- 结论：当前续页 rejected。

## continuation 风险

### illust-recommended

- 首页返回 continuation。
- 当前 adapter 只保留 offset。
- 其他服务端参数会丢失。
- 历史第二页失败。
- 本轮未能取得第二页失败的唯一根因。
- 结论：inconclusive。

### novel-series-v2

- v2 path、`series_id`、`last_order` 已由 Shaft 和 live evidence 支持。
- 当前生产 path 仍是 v1。
- 生产 adapter/SDK v2 两页尚未冻结。
- 结论：inconclusive。

## 数据受限例外

以下不强制第二页：

- user novels。
- user artworks。
- public/private bookmarks。

这些 case 只需接口、参数、adapter、SDK 成功。

## 终止规则

禁止把“有 next_url”当作第二页成功。
第二页必须有 required fields。
若继续请求失败，保留真实 failure class。


## 原始计划待补分页

- novel search period：需要第一页和第二页日期范围一致。
- artwork recommended subtype：每种 subtype 都要验证第二页。
- artwork latest subtype expansion：每种 subtype 都要验证 `max_illust_id`。
- bookmark subtype client filter：必须验证 logical pagination。
- comments total：需要非空目标和真实 continuation。


## Goal-3 实施约束

本报告记录 continuation evidence，不要求另起 Goal。实现时扩展现有 `sdk.Cursor` / `sdk/pixiv` binding，并复用 `internal/shared/pagination`、`internal/shared/traversal` 与 `CollectFilteredPagesFrom`；不要在 CLI、MCP 或 SDK 层重建分页 engine。`next_url` 只能由 adapter 解析为 allowlist continuation state。


## 2026-09-07：逻辑分页续读缺陷与修复（T23A）

审查固定 HEAD：`2167445280f1f6b6ce3ab8f6dbb3d082746b713d`。
原函数 blob：`8789589dcebe059eeb3821dc7846c0e32a714115`，位于 `internal/shared/pagination/pagination.go`。
用户提供的 goal3-pagination-repro 是独立 Go module，仅提取原函数与最小类型，不属于真实 API/全仓测试。

### 原始失败与生产 Red

离线材料在其 pagination-repro 子目录运行 `GOTOOLCHAIN=local GOPROXY=off GOWORK=off go test -v -count=1 ./...`：

```text
TestResumeDoesNotLoseBatchRemainder: FAIL
first=[1 2] next="second" has_more=true second=[4]
resume lost items: got [1 2 4], want [1 2 3 4]
TestLastBatchRemainderHasUsableContinuation: FAIL
items=[1 2] next="" has_more=true
TestControlFilterBeforeLimitAcrossPages: PASS
```

首次命令曾误在 module 外层目录运行，得到 setup failed；随后在正确目录重跑，上述结果才是行为证据。

生产包 `TestFilteredContinuationPreservesUnconsumedItems` 在原函数上实际出现相同两项失败。SDK 首个测试修正缺少 create_date 的 fixture 后，因缺少 checkpoint capability 失败；binding 测试随后分别因 local filter/client 未绑定及旧版本仍被接受失败。CLI/MCP 的 `TestSearchContinuationDoesNotLoseRemainder` 用真实 SDK、离线 HTTP fixture，在 HEAD 收集器与尚未接线的 owner 上，过滤/未过滤四项均因末批 cursor 为空失败；测试后恢复本轮收集器实现。

这些 Red 不使用编译错误冒充行为失败。取消传播也先验证 canceled context 被错误当作成功，再补传播检查。

### 实现与兼容决定

共享收集器在批内截断时通过 checkpoint 回调保存 fetch 序列的已消费位置，包含本地过滤/Skip。Pixiv SDK 通过既有 payload 保存原批次 offset 与累计位置；恢复在 SDK AI filter 之后应用位置。CLI/MCP 两个 owner 统一使用共享算法与收藏筛选 context 摘要，未过滤路径同样不再把上游 next 当作逻辑 next。

新增 CheckpointSearchArtworks 与 CursorContext，保留原方法及 named types。SearchArtworks 单独使用 binding version 2，旧搜索 cursor 明确 InvalidCursor，清除后重查；其他 operation 版本及 sdk.Cursor 外层格式不变。Open/OpenWith 绑定已验证账号；New/NewWith 未验证身份时仅同实例可续读。没有新增 CLI flag/MCP schema、生产 endpoint、网络重试、缓存、固定超时或条数上限。

必要校验：消费位置非正数或溢出时 InvalidArgument；恢复位置越过当前批次时 InvalidCursor；checkpoint 回调必须非空并返回非零 cursor，防止已复现的余项不可达。它们只约束无效续读状态，不截断合法数据。

稳定源序列可保证续读无遗漏无重复。重新抓取实时批次不是快照，上游重排/删除/新增可能改变结果。本轮没有真实 API 数据稳定性验证，也没有实现其他 endpoint 的逻辑 checkpoint。

### 本轮验证入口

```bash
go test ./internal/shared/pagination ./internal/shared/traversal ./sdk/pixiv ./internal/cli/commands/pixiv/search ./internal/mcpserver/pixiv/tools/search_illust -count=1
go test ./scripts/tests/documentation -count=1
go test ./...
go vet ./...
sh scripts/build.sh
```

上述命令已通过。SDK fixture 覆盖 JSON/Text 往返、AI 后位置、累计位置、后续批次、越界、query/filter/subtype/账号/client/旧版本拒绝与 context 不发送上游。生产分页覆盖过滤空批、Skip/Limit/OneBatch、错误和取消；CLI/MCP 保留原 wire，并由全仓既有 schema/structured error/stdout 测试回归。8 个受影响生产文件的 LSP diagnostics 无错误。

完整 Goal 状态只见 [能力准入表](capability-admission.md)，T23A 不替代 T19/T23 其余 endpoint、未来 all 聚合或 T44 live read/mutation。

### 最终补充验证与自审

`go test -race ./... -count=1` 通过；补充两端 `TestSearchContinuationReplayDiscardsFailedAttempt` 与 filtered repeated-cursor/predicate-error 回归后，三个受影响包的 `go test -race ... -count=1` 再次通过。第一次尝试中已收集但未交付的条目不会混入第二次尝试。

按 code-review-expert 检查消费下标、首批/末批、累计位置、错误传播、账号绑定、脱敏、边界归属及兼容影响；未发现未解决的阻塞问题。稳定源限制与旧 cursor 失效均已记录；未新增无依据阈值。检查了产品 Skill 的分页/complete-source 文案，既有 CLI --page/--limit 使用方式不变，不向产品 Skill 提前发布未来 all 功能。

T23A 任务卡：owner 为 shared pagination、sdk/pixiv、CLI search 和 MCP search_illust，属于本轮明确批准的跨层缺陷修复；落地按共享算法、SDK、两调用方的独立测试切片顺序推进。无未来 vNext 任务依赖。Red/Green 命令及结果见上；公开影响为新增 SDK checkpoint 接口及搜索 cursor v2。回滚须将收集器签名、SDK 方法及两调用方作为一个依赖闭包回退，不能单独撤回 callback 或 SDK method；已发行 v2 cursor 的回滚兼容需单独处理。T19/T23/T44 仍未完成。

生产修复提交：`5162685`。该提交的 pre-commit gofmt 与 go test ./... 均通过；计划一致性测试随后续计划提交交付。
