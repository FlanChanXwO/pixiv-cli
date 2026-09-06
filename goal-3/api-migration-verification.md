# API 升级与迁移验证台账

观测日期：2026-09-05（Asia/Shanghai）。
修订日期：2026-09-06（Asia/Shanghai）。

## 目的

本台账回答两个问题：

1. 已纳入 Goal-3 的 capability contract 是否已冻结到足以实施？
2. adapter、SDK、CLI 和 MCP 是否已达到公开条件？

用户确认的 upstream 可用性属于 Goal-3 范围输入。台账中的 `confirmed`、`partial`、`inconclusive`、`not_tested` 保留为 evidence/覆盖状态，不再作为拆分 Goal 或否定能力存在的结论。

## 分层门禁

### `contract_frozen`

允许把范围内 capability 交给实现 task。必须写明：

- method、path、参数名和参数类型。
- required / optional / empty / error 响应语义。
- continuation 参数、endpoint binding 和第二页 fixture；无分页则明确记录。
- mutation 写入、读回、隔离删除和恢复原状态。
- regression fixture 与 evidence redaction 规则。

### `migration_ready`

允许开始生产实现。必须满足 `contract_frozen`，并且 public SDK compatibility decision 已冻结。

### `public_ready`

允许公开 SDK、CLI 和 MCP surface。

必须在 `migration_ready` 基础上继续满足：

- endpoint adapter 成功。
- public SDK 成功。
- adapter 与 SDK 结果一致。
- CLI / MCP contract tests 通过。

因此：

- scope-admitted 但 adapter 未实现的能力，可以进入独立 TDD 实现 task。
- 未 `migration_ready` 不能切换生产 path。
- 未 `public_ready` 不能进入 public surface。
- 不因历史 `inconclusive` 另起 Goal；应完成 snapshot、实现或明确排除。

## 已确认的迁移依据

| 能力 | 旧 contract | 目标 contract | Upstream | Migration | Public | 结论 |
| --- | --- | --- | --- | --- | --- | --- |
| Novel latest cursor | `offset` | `max_novel_id` | wire/response/两页已确认 | ready | not ready | 可以 TDD 修复；修复后再过 adapter/SDK gate |
| Novel detail | `/v1/novel/detail` | `/v2/novel/detail` | v1 rejected；v2 wire/response 已确认；无分页 | ready | not ready | 可以 TDD 迁移；必须保留 v2 series 字段 |
| Comment add/reply/delete wire | 无生产 operation | `/v1/*/comment/add|delete` | 真实写入、读回和删除已有 evidence | ready | not ready | 可以写 adapter/SDK red tests；未通过前不公开 |
| Stamps read | 无生产 operation | `/v1/stamps` | wire/response 已确认；无分页 | ready | not ready | 可以写 adapter/SDK red tests |
| Novel ranking | 无生产 operation | `/v1/novel/ranking` | wire/response/两页已确认 | ready | not ready | 可以写 adapter/SDK red tests |

## 尚未达到迁移门禁

| 能力 | 候选 contract | 缺失证据 | 当前状态 | 实施规则 |
| --- | --- | --- | --- | --- |
| Novel series | `/v2/novel/series` + `last_order` | 真实第二页；candidate adapter；SDK | inconclusive | 不切生产 path |
| Novel comments v3 | `/v3/novel/comments` | 非空响应；第二页；adapter；SDK | inconclusive | 不从 v2 迁移 |
| Artwork comments v3 DTO | `/v3/illust/comments` | 非空 live DTO；`date`；numeric access control；第二页 | inconclusive / fixture mismatch | 不冻结 DTO |
| Artwork recommended continuation | `/v1/illust/recommended` | 完整 continuation 参数；成功第二页 | inconclusive | 不替换 cursor model |
| Novel search period | `/v1/search/novel` + `start_date/end_date` | 未进入 strict manifest；live 两页；adapter；SDK | not tested | 不删除 `Duration` |
| Novel bookmark tags | `/v1/user/bookmark-tags/novel` | wire/response；空列表；adapter；SDK | not tested | contract snapshot 前不新增 public operation |
| Novel bookmark detail | `/v2/novel/bookmark/detail` | wire/response；public/private 状态；adapter；SDK | not tested | contract snapshot 前不新增 public operation |
| Novel bookmark add/delete | `/v2/novel/bookmark/add` + `/v1/novel/bookmark/delete` | 隔离写入、detail、list、tags、恢复原状态；adapter；SDK | not tested | contract snapshot 前不新增 public mutation |
| Artwork bookmark tags | `/v1/user/bookmark-tags/illust` | wire/response；subtype 语义；adapter；SDK | not tested | 不扩展 subtype tags |
| Artwork bookmark subtype | `/v1/user/bookmarks/illust` + `type/content_type` | 参数是否被忽略；logical pagination | not tested | 默认只能 client-side 候选 |
| Recommended subtype | `/v1/illust/recommended` + `content_type` | illust/manga/ugoira 各两页 | not tested | 不公开 ugoira subtype |
| Latest subtype expansion | `/v1/illust/new` + compound `content_type` | manga/ugoira/组合值各两页 | partial | 只保留已确认的类型 |
| Novel comment total | v2/v3 comments | 非空 `total_comments`；是否需要 include 参数 | inconclusive | total 保持非强保证 |
| Artwork comment total | v3 comments | 非空 `total_comments`；是否需要 include 参数 | inconclusive | total 保持非强保证 |
| Bare ID probe | 多资源 detail endpoint | namespace 冲突；403/404 分类；请求成本 | not tested | 保持显式 `--type` |

## 已拒绝或排除

| 能力 | Contract | 结论 | 处理 |
| --- | --- | --- | --- |
| Novel detail v1 | `/v1/novel/detail` | rejected | 删除依赖，不做 fallback |
| Novel series v1 | `/v1/novel/series` | rejected | 删除依赖，不做 fallback |
| Novel content App API | `/v1/novel/content` | rejected | 不再承诺正文读取 |
| Novel WebView content | `/webview/v2/novel` | excluded | 不进入本 goal 默认实现 |
| Server-side rating | `x_restrict` request filter | rejected | 只允许 client-side filter |

## 启动生产迁移前的冻结条件

Phase A 完成时，每个范围内 capability 必须具备以下路径之一：

1. `contract_frozen`，随后进入 `migration_ready` 和实现 task。
2. 明确从当前 public implementation scope 移除。
3. `rejected` / `excluded`，进入删除或兼容处理。

`inconclusive`、`not_tested` 是待冻结或待补证据的状态，不是另一个 Goal；它们在 contract 未冻结前不能进入 public surface。

## Public SDK Compatibility Decision

T12 必须先于任何 exported SDK contract 修改完成。Goal-3 默认保持源码兼容：保留已有 exported named types，内部使用窄 validator/type 限制合法值。CLI 可以采用更窄的 type abstraction，不强迫 public SDK breaking。

若必须接受 breaking change，必须先记录 breaking 范围、migration guide、deprecated symbol strategy、semantic version strategy 和是否需要 `/v2` module；执行 Agent 不得自行决定。

## 当前结论

Goal-3 采用单一完整 Goal。当前台账中的历史 verdict 不代表能力不存在；它们用于定位 contract snapshot、adapter、SDK 或 public coverage 缺口。

具备上游迁移依据、但仍需生产实现的项目包括：

- Novel latest cursor。
- Novel detail v2。
- Comment mutation wire。
- Stamps read。
- Novel ranking。

其余范围内 capability 在 Phase A 完成 contract snapshot 后，按 `migration_ready`、`public_ready` 逐层推进。失败能力在本 Goal 内停止、修复或移出 public implementation scope，不延期到另一个 Goal。
