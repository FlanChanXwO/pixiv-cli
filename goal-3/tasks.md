# Goal-3 tasks：完整实施 Goal

修订日期：2026-09-07（Asia/Shanghai）。审查基线：`2167445280f1f6b6ce3ab8f6dbb3d082746b713d`。

## 执行规则与当前入口

Goal-3 保持单一完整 Goal。当前状态与 required_scope 只来自 [能力准入表](capability-admission.md)。表格的 depends_on 是显式依赖；按拓扑顺序执行，不能按旧编号顺序跳过前置任务。

T23A 是本轮用户单独批准的既有生产缺陷修复，不等待未来 vNext endpoint 合约冻结；它只完成搜索续读基础，不完成 T19/T23 的其他 endpoint 或整个 Goal。其实现状态不授予其他能力发布权限。其余生产能力仍须先 T00/T20、对应 contract、T12，再 T39A 与各 owner 实现。

Contract/计划任务以证据与一致性检查为验收；代码任务逐个 Red → Green → Refactor。不得改写历史 inconclusive/not_tested。任何 required capability 失败都使 Goal incomplete，不能通过排除它结束 Goal。

## 任务依赖表

Status 的 verified 表示本轮实现与相关验证完成，证据见分页报告（修复提交 5162685）；pending 不能被历史 evidence 自动提升。

| Task | Owner / responsibility | depends_on | Deliverable / acceptance | Status |
| --- | --- | --- | --- | --- |
| T00 | scope/计划 owner | none | 冻结 required_scope；状态唯一来源、批准记录、完整性验收 | pending |
| T20 | shared semantics | T00 | 冻结 Target kind / Result kind / Subtype；命令级冲突规则 | pending |
| T01 | artwork contract | T00,T20 | 冻结 artwork search/series/latest/ranking/recommended/ugoira 基础 request、DTO、subtype | pending |
| T02 | novel contract | T00,T20 | 冻结 novel search/detail/series/latest/recommended/ranking/follow | pending |
| T03 | bookmark contract | T00,T20 | 冻结两类 list/tags/detail/mutation/subtype 及 list/tags all 聚合契约 | pending |
| T04 | comment contract | T00,T20 | 冻结 artwork/novel comments read/create/reply/stamp/delete、stamps、total | pending |
| T05 | continuation contract | T01,T02,T03,T04 | 冻结 allowlist、query/account/subtype binding、第二页 fixture；复用现有 cursor | pending |
| T06 | error/其他 read contract | T00,T01,T02,T03,T04 | 冻结 user search/detail/relationships、trending、follow、mypixiv、error、mutation outcome 与脱敏 | pending |
| T12 | SDK compatibility | T20,T01,T02,T03,T04,T05,T06 | 冻结 symbol map、旧 wrapper、named types、旧消费者编译、cursor 版本恢复；默认源码兼容 | pending |
| T39A | CLI/MCP migration | T12 | 冻结 CLI 路由和 MCP tool/input/output compatibility map；旧 JSON 回放清单 | pending |
| T07 | read endpoint owners | T12 | 按 artwork/novel endpoint leaf 拆卡，实现 read adapter（含 v2、user/trending/relationships、follow/mypixiv 所需 endpoint）、DTO 与错误映射 | pending |
| T08 | bookmark endpoint owners | T12 | 按两类 list/tags/detail/mutation leaf 拆卡实现 | pending |
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
