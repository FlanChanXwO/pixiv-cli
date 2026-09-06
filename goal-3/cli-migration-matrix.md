# Goal-3 CLI migration matrix

修订日期：2026-09-06（Asia/Shanghai）。

## 用途

本表是 Goal-3 Phase E 的 CLI surface 冻结表。CLI 可以收口为统一 TARGET 和 `--type`，但不得让 public SDK 为此变成动态 `EntityTarget` API。

本表的 `Accepted type` 区分两层：

- entity type：command 操作的资源实体，例如 `artwork`、`novel`、`user`。
- subtype：endpoint 支持的作品子类型，例如 `illust`、`manga`、`ugoira`。

`artwork` 表示 artwork endpoint 的实体聚合，不等同于 `illust`。未达到 `public_ready` 的 subtype 不得进入 help、completion、MCP schema 或正式文档。

| Old surface | Canonical surface | Accepted type | Alias / compatibility | Deprecated / removal | stdin | JSON/NDJSON | MCP | Decision |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `pixiv novel search WORD` | `pixiv search WORD --type novel` | entity: `novel` | 保留兼容 route | deprecated；完成迁移后再删除 | 继承 `search` | 保留现有格式 | 同一 semantic request | 保留至迁移完成 |
| `pixiv user search WORD` | `pixiv search WORD --type user` | entity: `user` | 保留兼容 route | deprecated；完成迁移后再删除 | 继承 `search` | 保留现有格式 | 同一 semantic request | 保留至迁移完成 |
| `pixiv follow add/remove` | `pixiv user follow/unfollow` | entity: `user` | 保留旧 route alias | deprecated | record/ID 按 user contract | 保留现有格式 | 同一 operation semantics | user owner 收口 |
| `pixiv user bookmarks` | `pixiv bookmark list TARGET` | entity: `artwork\|novel` | hidden alias 或 deprecated route，T39 冻结 | 按兼容扫描决定 | TARGET/record | 保留现有格式 | `user_bookmarks` 与 canonical request 对齐 | 不覆盖 URL type |
| `pixiv bookmark list/tags/detail/add/remove` | 同名 canonical bookmark surface | entity: `artwork\|novel`；tags/detail/mutation 按能力准入 | 无需泛化 SDK | 未准入 novel operation 不出 public surface | TARGET/record | JSON/NDJSON 语义不变 | read/mutation 分开注册 | explicit dispatch |
| `pixiv recommended all` | `pixiv recommended --type all` | entity: `artwork\|novel\|user\|all`；subtype 单独冻结 | positional `all` compatibility | deprecated positional form | 继承 record pipeline | 保留现有格式 | 聚合结果保持实体流独立 | 不把 subtype 当 entity |
| `pixiv timeline following/latest` | 同名 canonical timeline surface | entity: `artwork\|novel`；subtype 使用 `--content-type` | 旧 `--type` 仅保留 entity 语义 | 不把 subtype 塞入 entity type | 继承分页 contract | 保留现有格式 | canonical request 对齐 | latest continuation 单独绑定 |
| `pixiv ranking` | 同名 canonical ranking surface | 当前 entity: `artwork`；novel 仅在准入后增加 | 无 | 未准入 novel ranking 不公开 | 继承现有 record contract | 保留现有格式 | 同一 ranking semantics | 不另起 Goal |
| `pixiv detail TARGET` | 同名 canonical detail surface | entity: `artwork\|novel\|user` | 旧 route 保留 | novel content 单独排除 | TARGET/URL/record | 保留现有格式 | 同一 resolver | URL/type 冲突返回 `InvalidArgument` |
| `pixiv series TARGET` | 同名 canonical series surface | entity: `artwork\|novel` | 旧 route 保留 | 旧 v1 novel path 删除 | TARGET/URL/record | 保留现有格式 | 同一 resolver | v2 contract freeze 后公开 |
| `pixiv comment TARGET` | 同名 canonical comment surface | entity: `artwork\|novel` | 旧 read route 保留 | mutation 按 capability 独立准入 | TARGET/URL/record | 保留现有格式 | read/mutation schema 对齐 | SDK 保持 explicit methods |
| `pixiv mypixiv users/works` | 同名 canonical MyPixiv surface | users；works entity: `artwork\|novel`；额外 subtype 逐项准入 | 旧 `illust` 作为兼容 alias 时记录 | 未准入 subtype 删除 | record/ID | 保留现有格式 | 同一 typed filter | 不接受全局任意 type |
| `--content-type` | entity command 中的 subtype flag | command-specific subtype；不是 global entity type | 仅在语义等价时提供 `--type` alias | 不静默改写含义 | 继承所属 command | 保留现有格式 | schema 字段保持语义一致 | 先冻结 subtype matrix |
| `--download-path` | `--output` / `-o` | N/A | 现有 alias | 按现有兼容期保留 | N/A | 保留现有格式 | download contract 不变 | 不与 vNext entity migration 混合 |

## 冻结规则

1. 每条迁移必须明确 `retain`、`alias`、`hidden alias`、`deprecated` 或 `delete`；“迁移”单独出现不算决策。
2. entity type 与 subtype 不得混写；每个 command 只接受自己的合法值。
3. CLI、MCP 共享 canonical semantic request/validation；CLI 不调用 MCP，MCP 不调用 CLI。
4. JSON/NDJSON、stdin、batch/stream 错误语义不得因 alias 迁移静默改变。
5. public SDK 使用 endpoint-oriented explicit methods；CLI resolver 负责 TARGET dispatch。
6. T39 完成前，候选 surface 只能作为计划项，不能进入 help、completion、MCP schema 或正式 docs。
