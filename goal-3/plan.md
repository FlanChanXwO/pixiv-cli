# goal-3：pixiv-cli vNext 完整实施计划

观测日期：2026-09-05（Asia/Shanghai）。

## 计划状态

本文件已从“验证后分批修复”升级为：

> **单一 goal 的完整实施阶段。**

它覆盖：

```text
合约收口 → SDK / Protocol → 共享类型 → Resolver → Filter → CLI → MCP → 文档 → 回归
```

这不是一次无门禁的大改动。

同一 goal 内仍按 task 顺序执行。
每个 task 独立测试。
每三个 task 做一次集中检查。

未确认能力必须在同一 goal 内先完成验证。
验证失败则该能力停止。
不得伪造为 `confirmed`。

## 目标

完成 pixiv-cli vNext 的：

1. Pixiv App API contract 校正。
2. 已有 novel 能力修复。
3. novel bookmark 能力补齐。
4. comment read/write/delete 能力收口。
5. stamps 与 novel ranking 的条件式接入。
6. artwork type、rating、pagination 统一。
7. resource resolver 统一。
8. CLI public surface 迁移。
9. SDK、Protocol、CLI、MCP、Skill 和文档同步。
10. 兼容 alias、废弃入口和错误语义收口。
11. 维护 `api-migration-verification.md`。确保每个迁移 API 已验证或已移出范围。

## 明确边界

### 已验证，可进入直接实施

当前只有以下 contract 可以直接进入 SDK、CLI 或 MCP 实施：

- novel follow。
- novel recommended。
- artwork search：all、illust、manga、ugoira。
- artwork latest。
- artwork ranking。
- ugoira metadata。
- user novels。
- user artworks。
- public/private novel bookmarks 的已验证读路径。
- public/private artwork bookmarks 的已验证读路径。

其中 user、artwork、bookmark 数据受限 case 使用 `pagination_exempt`。

### 同一 goal 内先验证，验证后才可迁移

以下都是候选 contract。
它们**不是已验证的 API 迁移**：

- `v1 novel detail` → `v2 novel detail`。
- `v1 novel series` → `v2 novel series`。
- novel latest `offset` → `max_novel_id`。
- artwork recommended 的完整 continuation。
- novel comments v2 → v3。
- artwork comments → v3 DTO。
- novel bookmark tags/detail/add/remove。
- comment text/reply/stamp/delete。
- stamps read。
- novel ranking。
- novel search period。
- artwork bookmark tags 与 subtype。
- recommended/latest subtype、comment total 和 bare ID probe。

它们必须先完成 live + adapter + SDK 验证。
只有 matrix 变为 `confirmed`，才允许迁移或公开。

### 其他实施范围

下列工作可以在 contract gate 之后实施：

- artwork、manga、ugoira、novel、user 类型归一化。
- URL、record、显式 type 的统一 resolver。
- rating client-side filter。
- 已确认能力对应的 CLI surface。
- 已确认能力对应的 MCP parity。
- 旧入口迁移、alias、deprecation 和文档同步。

任何 CLI surface 都不能先于对应的 SDK contract。

### 条件纳入

以下能力只有完成 live + adapter + SDK 验证后才可公开：

- novel ranking。
- stamps。
- artwork comment text/reply/delete。
- novel comment text/stamp/delete。
- `bookmark --type all`。
- recommended 的 illust/manga/ugoira subtype。
- timeline latest 的 manga/ugoira/组合 subtype。
- bare ID 自动探测。
- server-side rating filter。

条件能力失败时：

- 保留证据。
- 标记 `rejected`、`inconclusive` 或 `blocked`。
- 不添加 public SDK、CLI 或 MCP surface。

### 排除本 goal

- `/v1/novel/detail`。
- `/v1/novel/series`。
- `/v1/novel/content`。
- WebView fallback。
- `pixiv novel content` 的成功承诺。旧 App API 已 rejected；没有确认替代接口。
- 把 `x_restrict` 当作已确认的 server-side rating 参数。
- 任意只返回 HTTP 200、未通过 adapter/SDK 的候选接口。
- 任意隐式匿名 Web fallback。

## 实施门禁

### 普通 read 能力

必须同时满足：

1. Wire 已确认 method、path、参数名和类型。
2. Response 已确认 required、optional、空列表和错误语义。
3. 有 continuation 时已完成第二页验证。
4. 当前 endpoint adapter 成功。
5. 当前公开 SDK 成功。
6. adapter 与 SDK 结果一致。

### 用户数据受限能力

以下能力采用用户确认的例外：

- user novels。
- user artworks。
- public/private bookmarks。

必须满足：

- 真实接口成功。
- 参数语义正确。
- adapter 成功。
- SDK 成功。

这类 case 不强制第二页。
证据必须写明 `pagination_exempt`。

### mutation 能力

必须满足：

- 使用非主账号。
- 使用真实可评论或可收藏目标。
- 写入前读取 access control。
- 写入后立即读回。
- 保存本轮产生的 ID。
- 删除只使用本轮 ID。
- 完成 adapter 与 SDK round-trip。
- 不保存正文、标题、UID、账号名或 token。

### API 迁移双门禁

完整台账见 `api-migration-verification.md`。

`migration_ready` 允许开始生产实现：

- live wire 已确认。
- response contract 已确认。
- 普通分页已完成第二页。
- mutation 已完成写入、读回和隔离删除。

`public_ready` 允许公开 surface：

- 已满足 `migration_ready`。
- adapter 成功。
- SDK 成功。
- adapter 与 SDK 一致。
- CLI/MCP contract tests 通过。

“升级迁移”至少必须满足 `migration_ready`。

因此：

- `inconclusive` 不能改生产 path。
- `rejected` 不能作为 fallback。
- `not_tested` 不能创建 public SDK 方法。
- candidate adapter 只能存在于验证代码。
- 生产实现只能在 `migration_ready` 后开始。
- public contract 只能在 adapter、SDK 和对应回归全部通过后冻结。
- `v1 novel content` 不得自动转为 WebView。

当前状态：

- v2 novel detail：`migration_ready`，`public_ready` 未完成。
- novel latest `max_novel_id`：`migration_ready`，`public_ready` 未完成。
- v2 novel series：仍是候选验证项。
- comments v3：仍是候选验证项。

只能把前两项描述为“上游迁移依据已确认”。
不能描述为“完整 public contract 已确认”。

### continuation

Shaft 的设计只作为参考。

允许借鉴：

- 保存服务端 continuation。
- 首页和续页分开处理。
- series metadata 与 items 一起保存。
- reply 使用 parent comment。

不直接复制：

- 不直接请求任意 raw URL。
- 必须校验 host、path 和 endpoint 绑定。
- 必须绑定影响结果集的请求参数。
- 不得把单个 offset 当作完整 continuation。

## 单一 goal 执行阶段

### 阶段 A：合约收口

先执行所有尚未达到 `migration_ready` 的必要验证：

- novel series v2 两页。
- artwork recommended 第二页。
- novel comments v3 非空响应。
- artwork comments v3 非空响应。
- novel bookmark tags/detail/add/remove round-trip。
- comments mutation round-trip。
- stamps read 与 novel stamp comment 分别验证。
- novel ranking 的 adapter/SDK 链路。
- novel search period。
- artwork bookmark tags 与 subtype。
- recommended/latest subtype、comment total 和 bare ID probe。

WebView 不进入默认验证路径。

验证结果驱动后续 task。

### 阶段 B：Protocol 与 SDK

修正：

- novel series v2。
- novel latest `max_novel_id`。
- comments v3 DTO。
- full continuation state。
- novel period date range。
- recommended subtype 与 cursor digest。
- bookmark novel request / response / mutation。
- 已确认的 comment、stamp、ranking operation。

所有 public SDK 方法先于 CLI 接入。

### 阶段 C：共享模型与过滤

建立窄而明确的类型：

```text
SearchArtworkType
RecommendedArtworkType
LatestArtworkType
BookmarkType
EntityType
```

建立统一 cursor：

- 保存服务端 continuation。
- 保存 endpoint identity。
- 保存 request parameter digest。
- 拒绝跨 subtype 复用。
- 支持 logical pagination。

rating 规则：

- server-side 参数未确认时，不向上游发送伪参数。
- 使用返回对象的 `x_restrict` 做 client-side filter。
- client-side filter 必须继续消耗 continuation。
- 不静默截断合法结果。

### 阶段 D：Resolver

resolver 顺序：

```text
canonical record → Pixiv URL → 显式 --type → bare ID
```

URL 识别必须纯本地。

bare ID 只有在 namespace、错误分类和成本都确认后才启用。
否则要求显式 `--type`。

覆盖：

- artwork。
- novel。
- user。
- artwork series。
- novel series。
- bookmark target。
- comment target。

### 阶段 E：CLI 与 MCP

统一：

- `--type`。
- `detail`、`series`、`comment` owner。
- `bookmark` owner。
- `user`、`timeline`、`recommended` owner。
- URL 自动识别。
- pipe 默认 NDJSON。
- 显式格式选择。
- batch / stream 错误策略。

候选 public surface：

```text
pixiv search WORD
pixiv novel search WORD
pixiv user search WORD
pixiv user detail USER
pixiv user artworks USER
pixiv user novels USER
pixiv user following USER
pixiv user followers USER
pixiv user follow USER
pixiv user unfollow USER
pixiv bookmark list TARGET --type artwork|novel
pixiv bookmark tags --type artwork|novel
pixiv bookmark detail TARGET --type artwork|novel
pixiv bookmark add TARGET --type artwork|novel
pixiv bookmark remove TARGET --type artwork|novel
pixiv recommended --type artwork|manga|novel
pixiv timeline following --type artwork|novel
pixiv timeline latest --type illust|manga|novel
pixiv detail TARGET --type artwork|novel|user
pixiv series TARGET --type artwork|novel
pixiv comment TARGET --type artwork|novel
pixiv ranking --mode MODE
pixiv reverse-search IMAGE
pixiv download SRC... -o DIR
```

未通过条件门禁的 subtype 不出现在 help、completion、MCP schema 或正式文档中。

### 阶段 F：兼容迁移与文档

删除或迁移前扫描：

- README。
- locale CLI reference。
- MCP docs。
- Skill。
- tests。
- shell completion。
- 脚本。
- Issues / PR。

旧入口分类：

- 高频入口：deprecated alias。
- 重复入口：hidden alias + deprecation。
- 完全无效入口：删除。

候选迁移：

- `follow add/remove` → `user follow/unfollow`。
- `user bookmarks` → `bookmark list`。
- `--content-type` → `--type`。
- `--restrict` → `--visibility`，仅在兼容性评估通过时实施。
- `--on-error` → `--fail-fast`，仅在语义等价时实施。
- `--download-path` → `-o`。

### 阶段 G：集中验证与交付

必须完成：

- Protocol tests。
- Cursor tests。
- Filter tests。
- Resolver tests。
- CLI tests。
- SDK tests。
- MCP tests。
- live regression。
- `go test ./...`。
- `sh scripts/build.sh`。
- 脱敏 evidence 检查。
- 公开 surface 风险审计。

## 停止条件

出现以下任一情况，停止对应能力：

- live wire 与实现不一致。
- required 字段不稳定。
- 第二页无法确认。
- adapter 或 SDK 失败。
- 参数被服务端静默忽略。
- continuation 无法安全绑定。
- mutation 无法隔离和读回。
- 需要匿名 fallback 才能成功。

停止单项能力，不得把失败改写为空结果或默认成功。

## 回滚

- 每个 task 单独提交。
- 不混入无关格式化。
- 每个 public surface 保留迁移前行为说明。
- 单项失败时回滚该 task，不回滚已确认的其他修复。
- 公共 SDK 变更必须能通过旧入口 alias 兼容期。

## 完成标准

只有同时满足以下条件，goal 才能完成：

1. 所有进入 public surface 的能力都有 contract evidence。
2. 所有普通分页都有第二页证据。
3. 所有例外 case 明确标记 `pagination_exempt`。
4. 所有 public SDK 方法都有 adapter 对照测试。
5. 所有 CLI 命令都有 CLI regression。
6. MCP schema、SDK、CLI 和文档一致。
7. rejected 能力没有公开入口。
8. blocked 能力写明阻塞条件。
9. 不保存 secret 或用户内容。
10. 全量 Go 测试和构建通过。
