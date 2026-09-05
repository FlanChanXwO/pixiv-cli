# goal-3 tasks：完整实施 goal

观测日期：2026-09-05（Asia/Shanghai）。

## 执行规则

- 本文件属于一个完整 goal。
- 一轮只执行一个未完成 task。
- 每三个 task 后执行集中检查。
- 先 Red，再 Green，再 Refactor。
- 未通过 contract gate 的能力不得进入 public surface。
- `blocked` 不得伪造为 `confirmed`。

## 已完成：验证与审计

- [x] 建立严格 manifest。
- [x] 建立 verdict gate。
- [x] 验证 Novel read matrix。
- [x] 验证 Artwork read matrix。
- [x] 验证真实第一页与第二页。
- [x] 通过当前 adapter 与 SDK probe。
- [x] 完成既有 mutation evidence。
- [x] 生成脱敏 JSON/Markdown evidence。
- [x] 保留并重新分级旧 85 条 evidence。
- [x] 生成 command-to-upstream matrix。
- [x] 生成 Shaft-to-live diff。
- [x] 生成 Wire/adapter/SDK diff。
- [x] 生成分页报告。
- [x] 生成 mutation 报告。
- [x] 生成风险审计。
- [x] 生成能力准入表。
- [x] 排除 WebView 默认路径。
- [x] 排除废弃 v1 novel detail、series、content。
- [x] 判定 server-side rating 参数当前不可确认。
- [x] 验证私有 artwork / novel bookmark 读接口。
- [x] 对照 Shaft continuation、comments、series、mutation 处理。
- [x] 使用临时 HOME 重跑 strict upstream read test。
- [x] 完成实施条件复查。
- [x] 将计划升级为单一完整实施 goal。

## 阶段 A：合约收口

- [ ] **T01**：建立实施前双门禁。同步 `api-migration-verification.md`。区分 `migration_ready` 与 `public_ready`。
- [ ] **T02**：补齐 novel detail v2、novel series v2、novel latest 的真实第二页验证。
- [ ] **T03**：补齐 artwork recommended、novel comments v3、artwork comments v3 的真实验证。
- [ ] **CHECK-01**：集中检查 T01-T03。确认 verdict、字段、cursor 和 adapter/SDK 一致。
- [ ] **T04**：验证 novel search period、novel/artwork bookmark tags、bookmark detail、artwork subtype、novel bookmark mutation round-trip。
- [ ] **T05**：验证 comment text、reply、stamp、delete 的 adapter/SDK round-trip。使用非主测试账号。
- [ ] **T06**：验证 stamps read、novel ranking、recommended subtype、latest subtype expansion、comment total 和 bare ID probe。失败能力不得公开。
- [ ] **CHECK-02**：冻结迁移范围。每个 API 必须是 `migration_ready` 或从阶段 B 移除。重新生成 matrix、admission 和 failure class。

## 阶段 B：Protocol 与 SDK

- [ ] **T07**：只为 `confirmed` / `pagination_exempt` contract 写 Protocol red tests。候选 API 只保留在 live validation tests。
- [ ] **T08**：只迁移已经达到 `confirmed` 的 API。novel detail/series、novel latest cursor、novel period 未达到 gate 时不得改生产 path。
- [ ] **T09**：只实现已经达到 `confirmed` 的 DTO 和 bookmark contract。comments v3 仍为候选时，只更新验证 fixture，不更新 public SDK。
- [ ] **CHECK-03**：集中检查 T07-T09。运行 SDK、adapter 和 live focused tests。
- [ ] **T10**：实现受控 full continuation state。绑定 endpoint、subtype 和 request digest。
- [ ] **T11**：实现 recommended、latest、search、bookmark 的 logical pagination。
- [ ] **T12**：实现已确认 mutation、stamps、ranking 的 SDK operation。未确认项保持验证代码可达、生产 surface 不可达。
- [ ] **CHECK-04**：集中检查 T10-T12。确认无 cursor 丢失、跨 subtype 复用或静默 fallback。

## 阶段 C：共享类型、Resolver 与 Filter

- [ ] **T13**：建立 artwork、illust、manga、ugoira、novel、user 的 canonical type model。
- [ ] **T14**：拆分 SearchArtworkType、RecommendedArtworkType、LatestArtworkType、BookmarkType、EntityType。
- [ ] **T15**：实现 URL、canonical record、显式 type 的统一 resolver。URL 识别保持纯本地。
- [ ] **CHECK-05**：集中检查 T13-T15。运行 resolver、type conflict、invalid combination tests。
- [ ] **T16**：实现 bare ID probe。仅在 T06 confirmed 时开放，否则要求显式 type。
- [ ] **T17**：实现 rating client-side filter 和 logical pagination。不得发送未经确认的伪 server filter。
- [ ] **T18**：实现 bookmark subtype、comics → manga alias 和 type-specific validation。
- [ ] **CHECK-06**：集中检查 T16-T18。检查类型泄露、过滤后续页和合法结果完整性。

## 阶段 D：CLI 与 MCP surface

- [ ] **T19**：统一 search、novel search、user search、trending 输入和输出。
- [ ] **T20**：统一 bookmark、recommended、timeline、ranking surface。
- [ ] **T21**：统一 detail、series、comment、user/follow/mypixiv surface。
- [ ] **CHECK-07**：集中检查 T19-T21。运行 CLI golden、pipe、NDJSON 和错误策略测试。
- [ ] **T22**：为已确认 SDK 能力补齐 MCP aggregation、input/output schema 和 structured errors。
- [ ] **T23**：同步 MCP tool registration、localized MCP docs、README 和 CLI reference。
- [ ] **T24**：实现 deprecated alias、hidden alias、无效 route 删除和 shell completion 迁移。
- [ ] **CHECK-08**：集中检查 T22-T24。确认 CLI、MCP、SDK、docs surface 一致。

## 阶段 E：回归、审查与交付

- [ ] **T25**：运行 Protocol、Cursor、Filter、Resolver、CLI、SDK、MCP tests。
- [ ] **T26**：运行真实 read regression。mutation 只在对应能力已获准入时运行。
- [ ] **T27**：运行 `go test ./...` 和 `sh scripts/build.sh`。
- [ ] **CHECK-09**：集中检查 T25-T27。修复本 goal 引入的失败。
- [ ] **T28**：更新 evidence、matrix、risk audit、feasibility report 和 admission。
- [ ] **T29**：运行脱敏扫描。确认没有 token、cookie、UID、用户名、正文、标题、原始 URL 和本机路径。
- [ ] **T30**：完成最终 review。确认 rejected/blocked 能力没有 public entry。
- [ ] **CHECK-10**：最终集中检查。满足完成标准后再结束 goal。

## 每个 task 的回写格式

```text
实际变更：
验证命令：
验证结果：
新增 evidence：
剩余风险：
下一步：
```

## 当前实施入口

当前可以启动本 goal。

但 T01-T06 必须先执行。

T07-T12 只允许使用已经 `confirmed` 的 contract。
它们是同一 goal 内的前置 gate。

不允许直接跳到 T07 或 CLI surface。
