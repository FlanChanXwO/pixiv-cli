# Goal-3：Pixiv vNext 完整实施计划

修订日期：2026-09-07（Asia/Shanghai）。本轮依据固定基线 `2167445280f1f6b6ce3ab8f6dbb3d082746b713d` 的 REQUEST_CHANGES 审查（5 P1、4 P2）及 `goal3-pagination-repro`，保留 Goal-3 为单一完整 Goal。

## 目标、范围与完成定义

沿用 upstream contract → endpoint adapter → public SDK → shared semantics → CLI/MCP → compatibility/docs/regression 分层，不重写架构，不重复做 API 存在性探索。

[能力准入表](capability-admission.md) 是 required_scope、当前实施状态与发布授权的唯一来源。[任务依赖](tasks.md) 是执行顺序的权威表。历史 evidence verdict 只描述各自观测轮次，不能提升实施或发布状态。

完整交付必须使 required_scope 的**每个 capability**达到 public_ready，并通过禁止 endpoint/行为负向检查、旧消费者编译、旧 MCP JSON 回放、双语文档/Skill/completion 和全量测试/构建。任何必交付未完成，Goal 保持 incomplete；只允许用户明确批准的 scope change 调整清单。停止公开失败能力不能算完成 Goal。

本轮执行范围仅为生产搜索分页基础修复 T23A 与计划修订；其他 vNext 能力不实现、不据历史 evidence 勾选完成。T23A 的 contract、兼容决定与测试来自本次用户批准的修复计划及 [分页报告](pagination-validation-report.md)，不把这一局部批准扩张到未来 endpoint 迁移。

## 允许实现与允许发布

状态规则只定义于 [能力准入表](capability-admission.md#状态与发布规则)。contract 与兼容策略冻结后，可以在未发布实现中编写 exported SDK、CLI/MCP 注册、配套测试和文档，CLI/MCP 迁移还须 T39A 冻结。文档与实现一起验证，消除“先 public_ready 才能编写 public_ready 所需材料”的循环。

正式发布只包含达到 public_ready 的变更；required capability 失败时停止发布并保留不完整状态。已发布基线入口是否需要兼容 wrapper、明确不支持错误或其他迁移措施由 T12 决定，不能因新目标尚未 public_ready 就删除全部旧入口。

普通 read 要求 method/path/request、required/optional/null/empty/error、adapter/SDK 对照与分页 fixture。pagination_exempt 只用于已约定的用户数据受限 live case，不能豁免合成逻辑分页回归。mutation 要求独立账号、真实目标、写前 access control、本轮 ID、写后读回及同账号隔离清理。

## Cursor、分页与筛选

继续使用 `sdk.Cursor` 外层 envelope、`sdk/pixiv` product/operation/version/query/account/client binding、endpoint sanitized continuation 和 shared pagination/traversal。禁止 CLI/MCP 自行解析 endpoint token，禁止持久化 next_url、signed URL、token、cookie、原始查询或用户内容。

原计划声称 CollectFilteredPagesFrom 已完整支持 continuation 不成立：逻辑截断后直接返回上游 next 会遗漏批内余项，末批会产生 HasMore=true/空 cursor。T23A 已按先失败测试修正搜索基础；T23 仍须把契约接入其他 endpoint，而非只增加筛选参数。

共享算法在批内截断时交给产品 checkpoint 回调“本批输入 cursor + 消费位置”。位置按 fetch 返回序列计数，包含本地过滤和 Skip 消费的项目。只有完整消费匹配结果时才能转向上游 next。首次/末次批内截断同样要有可恢复 checkpoint。

Pixiv 搜索在既有 payload 中保存原批次 offset 与累计消费位置。恢复顺序：请求原批次 → SDK 规范化及 AI filter → 消费前缀恢复 → CLI/MCP 本地筛选 → logical Skip/Limit/OneBatch。初始 cursor 不含 endpoint token 时也可以创建 checkpoint。

本轮新增 `Client.CheckpointSearchArtworks(request, consumed)` 与 `SearchArtworksRequest.CursorContext`。后者只进入查询摘要，不发送上游；两端统一绑定实际收藏筛选上下界及策略。SearchArtworks binding version 单独升到 2，其他 operation 保持现有版本；旧搜索 cursor 返回 InvalidCursor，调用者清除它后从头开始，不静默重启。搜索 cursor 绑定 verified account；无 verified identity 时只能同一 client 恢复。

验收：相同查询、筛选和稳定源序列下，连续使用返回 cursor 等价于完整遍历的对应部分，不遗漏、不重复；包含批内、末批、过滤空批、Skip/Limit/OneBatch、重复 cursor、取消、账号池重放与错误传播。恢复重新请求原批次，不提供实时数据快照；上游重排/删除仍可能改变结果，位置越界明确返回 InvalidCursor。限制与恢复语义同步 SDK 文档。

rating 使用 client-side filter，除已冻结的真实服务端参数外不发送 x_restrict。future subtype、local filter 必须进入相应 cursor binding。

## 类型、解析和聚合契约

T20 已在 [CLI/MCP 迁移矩阵的类型语义冻结](cli-migration-matrix.md#t20-类型语义冻结) 中明确三层：Target kind 是输入身份（user/artwork/novel/series/comment）；Result/entity kind 是操作内容；Subtype 是 illust/manga/ugoira 等子类型。不建立所有命令都接受所有值的全局类型。

resolver 依次消费 structured canonical record、现有纯本地 ParseURL、显式类型 ID 和冻结后的受控 bare-ID probe；冲突、`all` 与错误规则以该节为准。保留 ReferenceKind 的 user/user bookmarks/artwork series/novel series 区别。URL 与 --type 是否冲突由命令契约判定：bookmark list/tags 的 user URL 加 --type novel 合法；作品 mutation 输入身份与选择的作品类型必须一致。

[artwork 基础 contract](upstream-contract-matrix.md#t01-artwork-基础-contract-冻结)、[CLI 迁移矩阵](cli-migration-matrix.md) 冻结路由、flag、stdin/JSON/NDJSON 与 MCP 映射。bookmark list/tags --type all 均为 required：先 artwork 后 novel、各流顺序不变、统一 Skip/Limit、cursor 记录当前流及各流 checkpoint；tags 按内容类型保留同名标签和各自 count。需要的流之一失败时整次逻辑页失败，收集成功后才输出 JSON/NDJSON。detail/add/remove 不接受 all；SDK 保持 endpoint-oriented methods，由共享产品语义组织聚合。

## SDK/MCP 兼容与 mutation

T12 默认源码兼容，交付逐项 symbol map：旧 method/request/model/named field type → 新入口 → wrapper/deprecation → 旧消费者编译测试。AddBookmark/RemoveBookmark 保留委托 wrapper；新增明确 artwork/novel 方法不自动删除旧方法。无效 novel content endpoint 与旧 exported symbol 分别处理，不能让 deprecated wrapper 继续调用已排除 path。任何必要 breaking change 先经明确批准，记录迁移、版本/module 策略。

T39A 冻结 MCP compatibility map：旧 tool 名称、必填字段、默认值、input/output 结构、错误语义与新 request 逐项对应；例如 add_bookmark 的 illust_id 不因 CLI TARGET 统一而被删除。用旧 JSON 请求回放证明兼容，CLI alias 测试不能替代它。

T09A 在 T09/T16 前提供响应可解码的窄 form transport；既有不读取响应的 PostForm bookmark/follow 路径保留。必须区分：确定未成功、已取得本轮创建 ID 但读回失败、请求结果不确定且没有可靠 ID。后两者不能作为安全自动重放依据，不能通过猜测最新评论推断本轮 ID。读回、清理和状态恢复绑定同一账号 execution context；只删除本轮 ID，不自动删除既有数据，不新建通用 mutation 重试。

## 实施依赖与回滚

执行链：required_scope/类型 → Contract Freeze → SDK compatibility/CLI-MCP migration freeze → transport/adapter → SDK/cursor → shared resolver/filter/pagination → CLI/MCP/compatibility/docs → required_scope 全集回归和发布门禁。

T12 前移至所有生产 adapter；T20 前于 T12；T39 拆为 T39A 冻结（前于 T24–T38）与 T39B 实施后审计。artwork recommended 基础 contract 归 T01，continuation 归 T05。每个 capability 的完整 owner 链在准入表，显式 depends_on 在 tasks。

任务开始前必须有 owner/files、依赖、冻结 contract/fixture、实际可运行 Red 命令及失败原因、Green 断言、兼容影响、回滚依赖闭包；跨 owner 任务先拆子卡。提交用于审计，回滚按依赖闭包或先行兼容修复处理；没有构建证据不能承诺任意单提交可独立撤销。

最终运行 adapter/SDK/shared、CLI/MCP/旧消费者回归、live read/mutation、go test ./...、sh scripts/build.sh、脱敏和 required_scope 审计。本轮不执行 live API，也不宣告整个 Goal 完成。
