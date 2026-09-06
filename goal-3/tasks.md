# goal-3 tasks：完整实施 Goal

观测日期：2026-09-06（Asia/Shanghai）。

## 执行规则

- 所有 task 属于同一个 Goal-3，不创建 Goal-3a、Goal-3b 或后续 Goal。
- 一个 task 只负责一个 capability/owner，完成后独立提交、测试和回滚。
- Contract Freeze、审计和矩阵 task 以 evidence snapshot 与一致性检查为 Red/Green 等价门禁；生产代码 task 必须 Red → Green → Refactor。
- `scope_admitted` 不等于 `public_ready`；public surface 必须通过 adapter、SDK、CLI/MCP 和文档门禁。
- 不重建 cursor、pagination、traversal 或 URL parser。
- `blocked`、`rejected`、`excluded` 不得伪造为可公开能力；失败留在当前 Goal 内处理或从当前 public scope 移除。

## 已完成：验证与审计基线

- [x] 建立 strict manifest、verdict gate 和 evidence 格式。
- [x] 完成 Novel / Artwork read matrix、真实 read evidence 和分页报告。
- [x] 完成既有 mutation evidence、脱敏 JSON/Markdown evidence 和 mutation 报告。
- [x] 生成 command-to-upstream、Shaft-to-live、Wire/adapter/SDK 差异表。
- [x] 排除 WebView 默认路径、废弃 v1 novel detail/series/content 和 server-side rating filter。
- [x] 将 Goal-3 确定为单一完整 Goal。
- [x] 接受并记录本次 vNext 计划修改建议：cursor/pagination 复用、SDK 兼容决策、CLI migration matrix、owner-sized task。

## Phase A：Contract Freeze

这些 task 不再探索“接口是否存在”；它们把已确认范围固化为 implementation contract，并记录字段、continuation、mutation、error 和 regression fixture。

- [ ] **T01**：冻结 artwork search/latest/ranking 与 ugoira metadata contract。
- [ ] **T02**：冻结 novel search/detail/series/latest/recommended/ranking/follow contract。
- [ ] **T03**：冻结 artwork/novel bookmark list、tags、detail 和 subtype contract。
- [ ] **T04**：冻结 artwork/novel comments read、create、reply、stamp、delete 与 total semantics contract。
- [ ] **T05**：冻结所有 endpoint continuation payload、allowlist 参数、endpoint/subtype binding、request digest 和第二页 fixture；只扩展现有 Pixiv cursor 体系。
- [ ] **T06**：冻结 error classification、mutation isolation/read-back、evidence redaction 和 scope/admission matrix。
- [ ] **CHECK-01**：检查 T01-T03 的 endpoint、DTO、type scope 和 fixture 一致性。
- [ ] **CHECK-02**：检查 T04-T06；冻结 `contract_frozen` / `migration_ready` / `public_ready` 分层。

## Phase B：Protocol / Adapter

- [ ] **T07**：实现 artwork/novel v2 read adapters；只处理 Phase A 已冻结 contract。
- [ ] **T08**：实现 artwork/novel bookmark list、tags、detail 和 mutation adapters。
- [ ] **T09**：实现 artwork/novel comment、reply、stamp、delete adapters。
- [ ] **CHECK-03**：运行 T07-T09 的 adapter red/green、DTO decoding 和 error mapping tests。
- [ ] **T10**：实现 ranking、recommended、latest 和 subtype adapters。
- [ ] **T11**：实现 continuation extraction/sanitization；`next_url` 只能解析为 allowlist state，不能原样进入 cursor。
- [ ] **CHECK-04**：检查所有 adapter 的 endpoint binding、空列表、错误语义和 no-secret 约束。

## Phase C：SDK / Cursor

- [ ] **T12**：完成 `Public SDK Compatibility Decision`。默认保持源码兼容；若接受 breaking change，先记录范围、迁移、版本和 module 策略。
- [ ] **T13**：实现 artwork public SDK methods/models，保留现有 exported named type compatibility。
- [ ] **T14**：实现 novel public SDK methods/models，保留 series 与 continuation 语义。
- [ ] **T15**：实现 explicit bookmark SDK：`AddArtworkBookmark`、`RemoveArtworkBookmark`、novel 对应方法、list/tags/detail。
- [ ] **CHECK-05**：运行 T12-T15 的 public signature、adapter/SDK 对照和 compatibility tests。
- [ ] **T16**：实现 explicit artwork/novel comment SDK read/create/reply/delete contract。
- [ ] **T17**：实现 stamps SDK operation；stamp 与 text/reply/delete 语义分别建模。
- [ ] **T18**：实现 ranking/recommended/latest SDK operations，未通过 gate 的 subtype 不公开。
- [ ] **T19**：扩展现有 `sdk/pixiv` cursor binding；需要变化时递增 binding version，不创建第二套 cursor format。
- [ ] **CHECK-06**：运行 SDK、cursor、continuation mismatch、cross-subtype reuse 和 secret-redaction tests。

## Phase D：Shared Semantics

- [ ] **T20**：冻结 artwork、illust、manga、ugoira、novel、user canonical types；区分 entity type 与 subtype type。
- [ ] **T21**：实现统一 resolver，复用 `pixiv.ParseURL`；补齐 structured record、URL、explicit type、bare-ID probe 和 conflict errors。
- [ ] **T22**：实现 normalized rating/content-type filter；server-side rating 未确认时只做 client-side filter。
- [ ] **T23**：接入 `internal/shared/pagination`、`internal/shared/traversal` 和 `CollectFilteredPagesFrom`；不新增 CLI/MCP/SDK 分页 engine。
- [ ] **CHECK-07**：运行 canonical type、resolver、filter、logical limit、continuation 和 repeated cursor tests。

## Phase E：CLI（按 command owner）

- [ ] **T24**：search surface：`--type` entity 与 artwork subtype 语义。
- [ ] **T25**：novel search surface：兼容 route、period contract 和 migration matrix。
- [ ] **T26**：user search 与 trending surface。
- [ ] **T27**：bookmark surface：list/tags/detail/add/remove，按 artwork/novel explicit dispatch。
- [ ] **T28**：recommended surface：entity type、subtype 和 `all` 兼容语义。
- [ ] **T29**：timeline surface：following/latest 的 entity type 与 `--content-type` subtype。
- [ ] **T30**：ranking surface：artwork 与已准入 novel ranking 分开验证。
- [ ] **T31**：detail surface：artwork/novel/user resolver 与 novel content exclusion。
- [ ] **T32**：series surface：artwork/novel explicit type 和 series continuation。
- [ ] **T33**：comment surface：read/create/reply/stamp/delete 的 entity-specific dispatch。
- [ ] **T34**：user surface：detail/artworks/novels/relationships。
- [ ] **T35**：follow surface：`user follow/unfollow` canonical route 与旧 alias。
- [ ] **T36**：mypixiv surface：users/works 的 type-specific validation。
- [ ] **CHECK-08**：运行 T24-T26 的 CLI golden、stdin、JSON/NDJSON 和 error strategy tests。
- [ ] **CHECK-09**：运行 T27-T30 的 CLI golden、logical pagination、mutation 和 public gate tests。
- [ ] **CHECK-10**：运行 T31-T36 的 resolver、pipe、completion candidate 和 compatibility tests。

## Phase F：MCP / Compatibility / Docs

- [ ] **T37**：补齐已准入 read capabilities 的 MCP aggregation、input/output schema 和 structured errors。
- [ ] **T38**：补齐已准入 mutation capabilities 的 MCP operations、access-control errors 和 read-back result。
- [ ] **T39**：完成 `cli-migration-matrix.md`；逐项决定 alias、hidden alias、deprecated、delete、stdin、JSON/NDJSON 和 MCP。
- [ ] **T40**：迁移 shell completion、help、invalid route 删除和 deprecated flag 行为。
- [ ] **T41**：同步 README、CLI reference、SDK docs、MCP docs 和 `skills/pixiv-cli/`；只记录已 `public_ready` 能力。
- [ ] **CHECK-11**：检查 CLI/MCP/SDK/matrix/docs surface 一致性和未准入能力不可达性。

## Phase G：Regression / Delivery

- [ ] **T42**：运行 Protocol、adapter、SDK、cursor 和 shared semantic regression。
- [ ] **T43**：运行 CLI、MCP、completion、alias/deprecation regression。
- [ ] **T44**：运行 live read/mutation regression；mutation 只使用隔离账号、真实目标和本轮 ID。
- [ ] **T45**：运行 `go test ./...`、`sh scripts/build.sh`、public surface audit、evidence redaction 和 migration audit。
- [ ] **CHECK-12**：最终检查完成标准；确认 rejected/excluded 能力无 public entry，再结束 Goal-3。

## 每个 task 的回写格式

```text
实际变更：
负责 owner：
验证命令：
验证结果：
新增 evidence：
剩余风险：
下一步：
```

## 当前实施入口

当前可启动单一 Goal-3。

执行顺序必须从 T01 开始；T07 之前完成 Phase A contract freeze；T12 必须先完成 public SDK compatibility decision；T24 之前冻结 `goal-3/cli-migration-matrix.md`。

任何未 `public_ready` 的能力只能存在于 contract/evidence、internal candidate code 或验证 task，不得进入 CLI help、completion、MCP schema、Skill、README 或正式 docs。
