# Goal-3 capability 状态与 required_scope

> **SUPERSEDED STATE / FROZEN SCOPE EVIDENCE**：当前 layer 状态与 release readiness 只由 `goal-1/current-state.md` 维护和派生。本文件继续冻结原始 `required_scope=41`、历史 contract owner 与 acceptance 索引；下表 `State` 以及下文“唯一权威来源”等表述属于 Goal-3 历史状态，不得覆盖 Goal-1 当前事实。

修订日期：2026-09-07（Asia/Shanghai）。这是**当前 capability 状态的唯一权威来源**；其他文档只记录 contract、迁移策略或历史 evidence，不独立授予发布权限。

## 状态与发布规则

- `scope_admitted`：属于 required_scope，可建立任务，尚未证明目标 contract 完整冻结。
- `contract_frozen`：method/path、request、required/optional/null/empty DTO、continuation、错误、mutation 与 fixture 已冻结。
- `migration_ready`：contract_frozen 且 T20 类型与 T12 兼容决策冻结；允许在未发布实现中编写 exported SDK、CLI/MCP 注册、测试、completion 与文档。CLI/MCP 的迁移实施还依赖 T39A。
- `public_ready`：对应 adapter/SDK/CLI/MCP/兼容性/文档及要求的回归完成，可以进入正式发布 surface。
- `excluded`：用户事先明确排除的 endpoint 或行为；禁止调用与 fallback，但是否保留 deprecated Go symbol 由 T12 决定。

已有发布入口的存在不表示 vNext 目标 public_ready；基线行为只由历史 evidence 描述。表中尚无完整目标快照/兼容冻结证据的能力保持 scope_admitted，不再填写含义不明的 ready。T23A 仅在分页报告记录已验证的局部实现。

required_scope 在本次修订中固定为下表 required=yes 的全部条目。T00 负责核对冻结记录，不能自行缩减。任一 required 未 public_ready，Goal-3 必须保持 incomplete；阻塞须记原因。只有用户明确批准、记录日期/理由/受影响条目及验收变更的 scope change 才能调整 required_scope。失败能力可以停止发布，不能自动变成 excluded 或完成。

本次用户明确要求 bookmark list/tags --type all 均为 required；tags 同名标签按内容类型分别保留。其他范围沿用已承诺能力。

## 当前能力清单与责任链

任务 ID 见 [tasks](tasks.md)；同格多个任务均须完成。Acceptance 是能力特有断言，此外全部条目须通过公共 DTO/error/compatibility/docs 门禁。

| Capability | Required | State | Contract owner | Adapter | SDK | Shared semantics | CLI | MCP | Acceptance | Evidence / contract index |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| artwork-search | yes | scope_admitted | T01 | T07 | T13 | T19,T22,T23 | T24 | T37 | 四种内容类型、AI、query/账号绑定与分页 | [upstream-contract-matrix.md](upstream-contract-matrix.md) |
| artwork-latest | yes | scope_admitted | T01 | T10 | T18 | T19,T22,T23 | T29 | T37 | max_illust_id、已承诺 subtype 与两页 | [upstream-contract-matrix.md](upstream-contract-matrix.md) |
| artwork-ranking | yes | scope_admitted | T01 | T10 | T18 | T19,T23 | T30 | T37 | mode/date 与两页 | [upstream-contract-matrix.md](upstream-contract-matrix.md) |
| artwork-recommended | yes | scope_admitted | T01,T05 | T10 | T18 | T19,T22,T23 | T28 | T37 | 基础 DTO、完整 continuation、subtype 两页 | [upstream-contract-matrix.md](upstream-contract-matrix.md) |
| ugoira-metadata | yes | scope_admitted | T01 | T07 | T13 | T21 | T31 | T37 | required metadata；无分页 | [upstream-contract-matrix.md](upstream-contract-matrix.md) |
| novel-search | yes | scope_admitted | T02 | T07 | T14 | T19,T23 | T25 | T37 | period/date 与两页 | [upstream-contract-matrix.md](upstream-contract-matrix.md) |
| novel-detail | yes | scope_admitted | T02 | T07 | T14 | T19,T23 | T31 | T37 | v2 detail/series metadata | [upstream-contract-matrix.md](upstream-contract-matrix.md) |
| novel-series | yes | scope_admitted | T02 | T07 | T14 | T19,T23 | T32 | T37 | v2 last_order 与两页 | [upstream-contract-matrix.md](upstream-contract-matrix.md) |
| novel-latest | yes | scope_admitted | T02 | T10 | T18 | T19,T23 | T29 | T37 | max_novel_id 与两页 | [upstream-contract-matrix.md](upstream-contract-matrix.md) |
| novel-recommended | yes | scope_admitted | T02 | T10 | T18 | T19,T23 | T28 | T37 | 两页 continuation | [upstream-contract-matrix.md](upstream-contract-matrix.md) |
| novel-ranking | yes | scope_admitted | T02 | T10 | T18 | T19,T23 | T30 | T37 | ranking mode 与两页 | [upstream-contract-matrix.md](upstream-contract-matrix.md) |
| novel-follow | yes | scope_admitted | T02 | T10 | T18 | T19,T23 | T29 | T37 | following feed 两页 | [upstream-contract-matrix.md](upstream-contract-matrix.md) |
| artwork-bookmark-list | yes | scope_admitted | T03 | T08 | T15 | T19,T21,T23 | T27 | T37 | public/private；空列表/字段/类型/续读 | [api-migration-verification.md](api-migration-verification.md) |
| artwork-bookmark-tags | yes | scope_admitted | T03 | T08 | T15 | T19,T21,T23 | T27 | T37 | public/private；空列表/字段/类型/续读 | [api-migration-verification.md](api-migration-verification.md) |
| artwork-bookmark-detail | yes | scope_admitted | T03 | T08 | T15 | T19,T21,T23 | T27 | T37 | public/private；空列表/字段/类型/续读 | [api-migration-verification.md](api-migration-verification.md) |
| artwork-bookmark-mutation | yes | scope_admitted | T03 | T08 | T15 | T19,T21,T23 | T27 | T38 | public/private；写入/detail/list/tags 读回与恢复 | [mutation-validation-report.md](mutation-validation-report.md) |
| novel-bookmark-list | yes | scope_admitted | T03 | T08 | T15 | T19,T21,T23 | T27 | T37 | public/private；空列表/字段/类型/续读 | [api-migration-verification.md](api-migration-verification.md) |
| novel-bookmark-tags | yes | scope_admitted | T03 | T08 | T15 | T19,T21,T23 | T27 | T37 | public/private；空列表/字段/类型/续读 | [api-migration-verification.md](api-migration-verification.md) |
| novel-bookmark-detail | yes | scope_admitted | T03 | T08 | T15 | T19,T21,T23 | T27 | T37 | public/private；空列表/字段/类型/续读 | [api-migration-verification.md](api-migration-verification.md) |
| novel-bookmark-mutation | yes | scope_admitted | T03 | T08 | T15 | T19,T21,T23 | T27 | T38 | public/private；写入/detail/list/tags 读回与恢复 | [mutation-validation-report.md](mutation-validation-report.md) |
| bookmark-subtype | yes | scope_admitted | T03 | T08 | T15 | T22,T23 | T27 | T37 | 筛选后 logical limit、不遗漏不重复 | [upstream-contract-matrix.md](upstream-contract-matrix.md) |
| bookmark-list-all | yes | scope_admitted | T03,T20,T39A | T08 | T15 | T19,T23 | T27 | T37 | artwork 后 novel；统一预算、双流 checkpoint、页原子失败 | [cli-migration-matrix.md](cli-migration-matrix.md) |
| bookmark-tags-all | yes | scope_admitted | T03,T20,T39A | T08 | T15 | T19,T23 | T27 | T37 | artwork 后 novel；统一预算、双流 checkpoint、页原子失败；typed 标签原计数 | [cli-migration-matrix.md](cli-migration-matrix.md) |
| artwork-comments-read | yes | scope_admitted | T04 | T09 | T16 | T19,T23 | T33 | T37 | date、numeric access control、null/empty、total 非强保证、两页 | [upstream-contract-matrix.md](upstream-contract-matrix.md) |
| artwork-comments-mutation | yes | scope_admitted | T04,T06 | T09A,T09 | T16,T17 | T21 | T33 | T38 | create/reply/stamp/delete；本轮 ID、同账号读回/清理、不确定结果 | [mutation-validation-report.md](mutation-validation-report.md) |
| novel-comments-read | yes | scope_admitted | T04 | T09 | T16 | T19,T23 | T33 | T37 | date、numeric access control、null/empty、total 非强保证、两页 | [upstream-contract-matrix.md](upstream-contract-matrix.md) |
| novel-comments-mutation | yes | scope_admitted | T04,T06 | T09A,T09 | T16,T17 | T21 | T33 | T38 | create/reply/stamp/delete；本轮 ID、同账号读回/清理、不确定结果 | [mutation-validation-report.md](mutation-validation-report.md) |
| stamps | yes | scope_admitted | T04 | T09 | T17 | T21 | T33 | T37 | stamps 列表与 stamp 引用独立验证 | [upstream-contract-matrix.md](upstream-contract-matrix.md) |
| user-artworks | yes | scope_admitted | T06 | T07 | T13,T14 | T21,T23 | T34 | T37 | 对应 request/DTO/错误、分页或 mutation 契约 | [upstream-contract-matrix.md](upstream-contract-matrix.md) |
| user-novels | yes | scope_admitted | T06 | T07 | T13,T14 | T21,T23 | T34 | T37 | 对应 request/DTO/错误、分页或 mutation 契约 | [upstream-contract-matrix.md](upstream-contract-matrix.md) |
| user-relationships | yes | scope_admitted | T06 | T07 | T13,T14 | T21,T23 | T34 | T37 | 对应 request/DTO/错误、分页或 mutation 契约 | [upstream-contract-matrix.md](upstream-contract-matrix.md) |
| user-detail | yes | scope_admitted | T06 | T07 | T13,T14 | T21,T23 | T31 | T37 | 对应 request/DTO/错误、分页或 mutation 契约 | [upstream-contract-matrix.md](upstream-contract-matrix.md) |
| user-search | yes | scope_admitted | T06 | T07 | T13,T14 | T21,T23 | T26 | T37 | 对应 request/DTO/错误、分页或 mutation 契约 | [upstream-contract-matrix.md](upstream-contract-matrix.md) |
| trending | yes | scope_admitted | T06 | T07 | T13,T14 | T21,T23 | T26 | T37 | 对应 request/DTO/错误、分页或 mutation 契约 | [upstream-contract-matrix.md](upstream-contract-matrix.md) |
| follow-mutation | yes | scope_admitted | T06 | T07 | T13,T14 | T21,T23 | T35 | T38 | 对应 request/DTO/错误、分页或 mutation 契约 | [upstream-contract-matrix.md](upstream-contract-matrix.md) |
| mypixiv | yes | scope_admitted | T06 | T07 | T13,T14 | T21,T23 | T36 | T37 | 对应 request/DTO/错误、分页或 mutation 契约 | [upstream-contract-matrix.md](upstream-contract-matrix.md) |
| bare-id-probe | yes | scope_admitted | T06,T20 | T07 | T13,T14 | T21 | T31 | T37 | namespace、403/404、请求成本；无法判定时明确报错 | [upstream-contract-matrix.md](upstream-contract-matrix.md) |
| rating-filter | yes | scope_admitted | T20 | T10 | T18 | T22,T23 | T24,T27,T28,T29 | T37 | 不发送 server x_restrict；过滤后预算与绑定 | [upstream-contract-matrix.md](upstream-contract-matrix.md) |
| logical-pagination | yes | scope_admitted | T05 | T11 | T19 | T23A,T23 | T24,T27 | T37 | 稳定源连续续读等价完整遍历、末批 checkpoint | [pagination-validation-report.md](pagination-validation-report.md) |

| artwork-series | yes | scope_admitted | T01 | T07 | T13 | T19,T21,T23 | T32 | T37 | series identity、continuation 两页 | [matrix](upstream-contract-matrix.md) |
| recommended-all | yes | scope_admitted | T01,T02,T06 | T10 | T18 | T19,T23 | T28 | T37 | 保留 artwork/novel/user 实体流及既有聚合兼容 | [migration](cli-migration-matrix.md) |

## 证据与例外

历史 confirmed/partial/inconclusive/not_tested/rejected 仅表示原轮次 evidence，不是上述状态。不得改写历史失败以补齐门禁。普通分页要求真实第二页与 adapter/SDK 对照 fixture；user artworks/novels、public/private bookmarks 的数据受限例外可标 pagination_exempt，但须说明目标受限原因并保留合成两页逻辑回归，不能豁免逻辑续读正确性。

Mutation 必须隔离账号、真实目标、读回、本轮 ID 与同账号清理；无法确定是否写入时保留不确定状态，禁止普通失败重试。完整契约见 [迁移台账](api-migration-verification.md)。

## 事先明确排除

`/v1/novel/detail`、`/v1/novel/series`、`/v1/novel/content`、WebView/匿名 fallback、未经确认的 server-side `x_restrict`。在替代 adapter/SDK 验证和兼容策略完成后停止旧 path；不能顺手删除公开 Go symbol、CLI alias 或 MCP schema。
