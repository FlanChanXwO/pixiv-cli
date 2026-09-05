# API 升级与迁移验证台账

观测日期：2026-09-05（Asia/Shanghai）。

## 目的

本台账只回答一个问题：

> 计划提到的 API 升级或迁移，是否已经具备实施依据？

## 两层门禁

### `migration_ready`

允许开始生产实现。

必须满足：

- method、path、参数名和参数类型已由 live 请求确认。
- required / optional / empty / error 响应语义已确认。
- 普通分页已验证真实第二页。
- mutation 已完成真实写入、读回和隔离删除。

### `public_ready`

允许公开 SDK、CLI 和 MCP surface。

必须在 `migration_ready` 基础上继续满足：

- endpoint adapter 成功。
- public SDK 成功。
- adapter 与 SDK 结果一致。
- CLI / MCP contract tests 通过。

因此：

- 上游已确认但 adapter 未实现，可以进入 TDD 实现。
- 不能直接进入 public surface。
- 上游尚未确认，不能修改生产 endpoint。

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
| Novel bookmark tags | `/v1/user/bookmark-tags/novel` | wire/response；空列表；adapter；SDK | not tested | 不新增 public operation |
| Novel bookmark detail | `/v2/novel/bookmark/detail` | wire/response；public/private 状态；adapter；SDK | not tested | 不新增 public operation |
| Novel bookmark add/delete | `/v2/novel/bookmark/add` + `/v1/novel/bookmark/delete` | 隔离写入、detail、list、tags、恢复原状态；adapter；SDK | not tested | 不新增 public mutation |
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

阶段 A 完成时，所有计划中的 API 项必须属于以下二者之一：

1. `migration_ready`。
2. 从生产实施范围移除。

不能保留第三种状态。

因此：

- `inconclusive` 不能流入阶段 B。
- `not_tested` 不能流入阶段 B。
- `rejected` 只能进入删除或兼容处理。

## 当前结论

计划中提到的 API 升级**并非全部已验证**。

目前明确具备上游迁移依据的主要项目：

- Novel latest cursor。
- Novel detail v2。
- Comment mutation wire。
- Stamps read。
- Novel ranking。

其余候选必须在 T02-T06 完成验证。
否则从实施 surface 删除。
