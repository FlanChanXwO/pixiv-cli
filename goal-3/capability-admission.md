# 完整实施 goal：能力准入表

观测日期：2026-09-05（Asia/Shanghai）。

## 准入规则

本文件区分两层：

- `migration_ready`：允许开始生产实现。
- `public_ready`：允许公开 SDK、CLI、MCP 和文档 surface。

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

| 能力 | Upstream verdict | Migration | Public | 完整 goal 动作 |
| --- | --- | --- | --- | --- |
| novel follow | confirmed | ready | ready | 直接实施 surface 整理 |
| novel recommended | confirmed | ready | ready | 直接实施 surface 整理 |
| artwork search all/illust/manga/ugoira | confirmed | ready | ready | 直接实施 surface 整理 |
| artwork latest illust | confirmed | ready | ready | 直接实施 surface 整理 |
| artwork ranking | confirmed | ready | ready | 直接实施 surface 整理 |
| ugoira metadata | confirmed | ready | ready | 直接实施 surface 整理 |
| user novels | pagination_exempt | ready | ready | 直接实施 |
| user artworks | pagination_exempt | ready | ready | 直接实施 |
| public/private novel bookmark list | pagination_exempt | ready | ready | 直接实施 |
| public/private artwork bookmark list | pagination_exempt | ready | ready | 直接实施 |
| novel detail v2 | wire/response confirmed | ready | not ready | 可以开始 TDD 迁移；完成 adapter/SDK 后公开 |
| novel latest `max_novel_id` | wire/response/two pages confirmed | ready | not ready | 可以开始 TDD 修复；完成 adapter/SDK 后公开 |
| comment mutation wire | live read-back confirmed | ready | not ready | 可以开始 adapter/SDK TDD；未完成前不公开 |
| stamps read | wire/response confirmed | ready | not ready | 可以开始 adapter/SDK TDD |
| novel ranking | wire/response/two pages confirmed | ready | not ready | 可以开始 adapter/SDK TDD |
| novel series v2 | pagination inconclusive | not ready | not ready | 仅验证；不切生产 path |
| artwork recommended full cursor | second page inconclusive | not ready | not ready | 仅验证；不冻结 cursor |
| novel comments v3 | non-empty/pagination inconclusive | not ready | not ready | 仅验证；不从 v2 迁移 |
| artwork comments v3 | live empty inconclusive；fixture mismatch | not ready | not ready | 仅验证；不冻结 DTO |
| novel search period | not tested | not ready | not ready | 加入 strict manifest 后验证 |
| bookmark tags/detail/mutation | not tested | not ready | not ready | 加入 strict manifest 后验证 |
| bookmark subtype | not tested | not ready | not ready | 加入 strict manifest 后验证 |
| recommended/latest subtype expansion | partial/not tested | not ready | not ready | 完成 subtype 两页验证 |
| comment total semantics | inconclusive | not ready | not ready | 非空响应验证后决定 |
| bare ID probe | not tested | not ready | not ready | 保持显式 `--type` |
| server-side rating | rejected | forbidden | forbidden | 禁止实施；使用 client-side filter |
| `/v1/novel/detail` | rejected | forbidden | forbidden | 删除依赖 |
| `/v1/novel/series` | rejected | forbidden | forbidden | 删除依赖 |
| `/v1/novel/content` | rejected | forbidden | forbidden | 不承诺正文读取 |
| WebView fallback | excluded | forbidden | forbidden | 不实施 |

## 进入 public surface 的条件

能力只有在对应 task 完成后才能进入：

- SDK 导出方法。
- CLI help / command tree。
- MCP tool registration。
- shell completion。
- README / CLI reference。
- Skill 文档。

任何未确认能力都只能保留在：

- live test。
- evidence。
- internal candidate code。
- plan / risk audit。

## 失败处理

- `confirmed`：允许实施。
- `inconclusive`：继续验证。
- `rejected`：不得实现。
- `blocked`：写明阻塞条件。
- `not_tested`：不得公开。

## API 迁移规则

以下词语只允许用于 `confirmed` contract：

- 升级。
- 转移。
- 替换生产 path。
- 新增 public SDK。
- 新增 CLI/MCP surface。

`inconclusive`、`rejected`、`not_tested` 只能进入验证 task。

因此，当前 v2 novel detail、v2 novel series、novel latest cursor、comments v3 均不能直接视为已验证迁移。
