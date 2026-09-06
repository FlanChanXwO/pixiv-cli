# 完整实施 goal：能力准入表

观测日期：2026-09-05（Asia/Shanghai）。
修订日期：2026-09-06（Asia/Shanghai）。

## 准入规则

本文件区分四层：

- `scope_admitted`：能力属于 Goal-3，允许建立独立 task。
- `contract_frozen`：implementation contract 已固化。
- `migration_ready`：允许开始生产实现。
- `public_ready`：允许公开 SDK、CLI、MCP 和文档 surface。

用户确认的 upstream 可用性是 scope 输入；历史 evidence verdict 只描述证据和覆盖状态，不再触发另起 Goal。

完整逐项台账见 `api-migration-verification.md`。

普通 read 必须通过：

```text
Wire + Response + Pagination + Adapter + SDK
```

用户数据受限 case 可以使用：

```text
Wire + Response + Adapter + SDK
```

并标记：

```text
pagination_exempt
```

mutation 还必须通过：

```text
non-primary account + real target + write/read-back + isolated delete
```

## 当前准入

除表中标记为 `out` 的排除项外，所有范围内能力的 `Scope` 均为 `scope_admitted`。`Migration` 表示 contract freeze 后的生产实现状态；`Public` 表示是否已完成 public surface，不把历史 upstream verdict 当作接口存在性判断。

| 能力 | Scope | Evidence | Migration | Public | 完整 Goal 动作 |
| --- | --- | --- | --- | --- | --- |
| novel follow | scope_admitted | confirmed | ready | ready | 直接实施 surface 整理 |
| novel recommended | scope_admitted | confirmed | ready | ready | 直接实施 surface 整理 |
| artwork search all/illust/manga/ugoira | scope_admitted | confirmed | ready | ready | 直接实施 surface 整理 |
| artwork latest illust | scope_admitted | confirmed | ready | ready | 直接实施 surface 整理 |
| artwork ranking | scope_admitted | confirmed | ready | ready | 直接实施 surface 整理 |
| ugoira metadata | scope_admitted | confirmed | ready | ready | 直接实施 surface 整理 |
| user novels | scope_admitted | confirmed / pagination_exempt | ready | ready | 直接实施 |
| user artworks | scope_admitted | confirmed / pagination_exempt | ready | ready | 直接实施 |
| public/private novel bookmark list | scope_admitted | confirmed / pagination_exempt | ready | ready | 直接实施 |
| public/private artwork bookmark list | scope_admitted | confirmed / pagination_exempt | ready | ready | 直接实施 |
| novel detail v2 | scope_admitted | wire/response confirmed | ready | not ready | T01-T02 snapshot 后开始 TDD；adapter/SDK 后公开 |
| novel latest `max_novel_id` | scope_admitted | wire/response/two pages confirmed | ready | not ready | T02/T05 snapshot 后开始 TDD；adapter/SDK 后公开 |
| comment mutation wire | scope_admitted | live read-back confirmed | ready | not ready | Goal-3 T04/T09/T16 独立实现；未完成前不公开 |
| stamps read | scope_admitted | wire/response confirmed | ready | not ready | Goal-3 T04/T09/T17 独立实现 |
| novel ranking | scope_admitted | wire/response/two pages confirmed | ready | not ready | Goal-3 T02/T10/T18/T30 独立实现 |
| novel series v2 | scope_admitted | pagination inconclusive | snapshot pending | not ready | T02/T05 冻结；不另起 Goal |
| artwork recommended full cursor | scope_admitted | second page inconclusive | snapshot pending | not ready | T04/T05 冻结；复用现有 cursor |
| novel comments v3 | scope_admitted | non-empty/pagination inconclusive | snapshot pending | not ready | T04 冻结；不越过 SDK/public gate |
| artwork comments v3 | scope_admitted | live empty inconclusive；fixture mismatch | snapshot pending | not ready | T04 冻结；不冻结未确认 DTO |
| novel search period | scope_admitted | not tested | snapshot pending | not ready | T02 冻结后实现；不删除 `Duration` |
| bookmark tags/detail/mutation | scope_admitted | not tested | snapshot pending | not ready | T03/T06 冻结后在 T08/T15 实施 |
| bookmark subtype | scope_admitted | not tested | snapshot pending | not ready | T03/T22/T27 实施；按 logical pagination 验证 |
| recommended/latest subtype expansion | scope_admitted | partial/not tested | snapshot pending | not ready | T04/T10/T28-T29 实施；逐项准入 |
| comment total semantics | scope_admitted | inconclusive | snapshot pending | not ready | T04/T33 实施；无证据时保持非强保证 |
| bare ID probe | scope_admitted | not tested | snapshot pending | not ready | T21 受控 probe；未冻结前要求显式 `--type` |
| server-side rating | out | rejected | forbidden | forbidden | 不实施；使用 client-side filter |
| `/v1/novel/detail` | out | rejected | forbidden | forbidden | 删除依赖 |
| `/v1/novel/series` | out | rejected | forbidden | forbidden | 删除依赖 |
| `/v1/novel/content` | out | rejected | forbidden | forbidden | 不承诺正文读取 |
| WebView fallback | out | excluded | forbidden | forbidden | 不实施 |

## 进入 public surface 的条件

能力只有在对应 task 完成后才能进入：

- SDK 导出方法。
- CLI help / command tree。
- MCP tool registration。
- shell completion。
- README / CLI reference。
- Skill 文档。

任何未 `public_ready` 能力都只能保留在：

- live test。
- evidence。
- internal candidate code。
- plan / risk audit。

## 失败处理

- `confirmed`：可作为 evidence 输入；生产实现仍须通过 `contract_frozen` / `migration_ready`。
- `inconclusive` / `not_tested`：补齐 snapshot 或实现证据；不得公开。
- `rejected`：不得实现；只做删除或兼容处理。
- `blocked`：写明阻塞条件；不另起 Goal。

## API 迁移规则

以下词语只允许用于 `contract_frozen` / `migration_ready` contract：

- 升级。
- 转移。
- 替换生产 path。
- 新增 public SDK。
- 新增 CLI/MCP surface。

`inconclusive` / `not_tested` 只能进入 contract snapshot、验证或 internal candidate task；不得进入 public surface。

因此，当前 v2 novel detail、v2 novel series、novel latest cursor、comments v3 仍须完成 contract snapshot；历史 evidence 状态不等于接口不存在，也不等于 public_ready。
