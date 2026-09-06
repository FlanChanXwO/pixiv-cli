# goal-3：pixiv-cli vNext 完整实施计划

观测日期：2026-09-06（Asia/Shanghai）。
修订依据：`Goal-3 vNext 计划修改建议报告`。

## 计划结论

Goal-3 保持为**单一完整 Goal**，不拆分为 Goal-3a、Goal-3b 或后续 Goal。

目标是一次完成 Pixiv vNext public surface 收口，使以下层保持一致：

```text
upstream contract
    → protocol / adapter
    → public SDK
    → shared types / cursor / pagination / resolver / filter
    → CLI / MCP
    → compatibility / documentation / regression
```

Goal 层面要求完整交付；Task 层面要求单一 owner、可独立测试、提交和回滚。Task 数量增加不代表 Goal 被拆分。

本计划接受用户提供的审查结论：Goal-3 涉及的 upstream 能力属于本 Goal 的可实施范围。历史 evidence 中的 `inconclusive`、`not_tested` 等状态保留为证据完整性和实现覆盖记录，不再解释为“接口不存在”。它们仍可阻止 contract freeze 或 public surface，但不再触发另起 Goal，也不阻止为已确认范围建立独立实现 task。

## 目标

1. 冻结 Pixiv App API implementation contract。
2. 修正已存在的 novel、artwork、bookmark、comment、ranking、recommended、latest 能力。
3. 条件接入 novel bookmark、comment mutation、stamps、novel ranking 与 subtype 能力。
4. 统一 canonical types、已有 cursor、shared pagination/traversal、resolver 和 filter 语义。
5. 收口 CLI 与 MCP public surface，并完成 alias、deprecation、completion 和文档迁移。
6. 保持 public Go SDK 的兼容策略明确，不由执行 Agent 自行决定 breaking change。
7. 以 adapter、SDK、CLI、MCP、live regression 和脱敏审计证明完整交付。

## 范围与排除

### Goal-3 范围

以下能力属于本 Goal，按 Phase A 的 contract freeze 资料实施：

- artwork search、latest、ranking、ugoira metadata。
- novel search、detail、series、latest、recommended、ranking、follow。
- user artworks、user novels、user relationships。
- public/private artwork bookmark 与 novel bookmark。
- artwork / novel comments read、create、reply、stamp、delete。
- recommended、latest、bookmark subtype 与 logical pagination。
- rating client-side filter。
- CLI、MCP、SDK、Skill、README、CLI reference 和 MCP docs 的一致迁移。

`scope_admitted` 表示能力属于 Goal-3；不等于已经完成 adapter、SDK 或 public surface。

### 条件 public 能力

下列能力仍属于 Goal-3，但只有完成 contract snapshot、adapter、SDK 和对应回归后才能公开：

- novel detail / series v2。
- novel latest `max_novel_id` continuation。
- artwork recommended 完整 continuation。
- comments v3 DTO 与 total semantics。
- novel bookmark tags/detail/add/remove。
- artwork bookmark tags 与 subtype。
- recommended/latest subtype expansion。
- stamps。
- novel ranking。
- comment text/reply/stamp/delete。
- bare ID probe。
- `bookmark --type all`。
- server-side filter 以外的 rating 语义扩展。

失败能力仍记录真实 failure class，并从 public SDK、CLI、MCP、completion 和正式文档 surface 排除；不延期到另一个 Goal。

### 明确排除

- `/v1/novel/detail`。
- `/v1/novel/series`。
- `/v1/novel/content`。
- WebView fallback 或任何隐式匿名 Web fallback。
- 将 `x_restrict` 当作已确认的 server-side rating 参数。
- 将仅有 HTTP 200、缺少 required fields 或未通过 adapter/SDK 的候选接口公开。

## 状态与实施门禁

### 状态定义

| 状态 | 含义 | 允许动作 |
| --- | --- | --- |
| `scope_admitted` | 能力属于 Goal-3 范围；用户确认其 upstream 方向可实施 | 建立独立 task |
| `contract_frozen` | method、path、request、response、continuation、mutation、error 和 fixture 已写入 snapshot | 开始生产实现准备 |
| `migration_ready` | contract 已冻结，兼容策略已冻结，生产实现可开始 | 编写 adapter、SDK 和对应测试 |
| `public_ready` | adapter、SDK、CLI/MCP contract tests 和文档通过 | 公开 public surface |
| `rejected` / `excluded` | 本 Goal 不支持或明确排除 | 删除依赖，不作 fallback |

历史 matrix 的 `confirmed`、`partial`、`inconclusive`、`not_tested` 继续保留。它们描述现有证据和覆盖状态；执行时必须将其转化为 contract snapshot、实现 task 或明确排除，不得把状态含义扩张为“另一个 Goal”。

### 普通 read

必须具备：

1. method、path、参数名和类型。
2. required、optional、空列表和错误语义。
3. 有 continuation 时的真实 continuation 参数和第二页回归 fixture。
4. endpoint adapter。
5. public SDK。
6. adapter 与 SDK 结果一致。

### 用户数据受限 case

user novels、user artworks、public/private bookmarks 可使用 `pagination_exempt`。仍必须验证真实接口、参数、adapter 和 SDK；证据必须明确说明未执行第二页的原因和影响。

### mutation

必须使用非主账号和真实目标；写入前读取 access control，写入后立即读回，保存本轮 ID，删除仅使用本轮 ID，并完成 adapter/SDK round-trip。不得保存 token、cookie、UID、账号名、标题、正文或原始 URL。

### public surface

只有同时满足 `migration_ready`、adapter、SDK、CLI/MCP contract tests 和文档同步，能力才可进入：

- exported SDK method 或 model。
- CLI help、completion 和 command tree。
- MCP tool registration、schema 和 structured errors。
- README、CLI reference、SDK/MCP docs 和 `skills/pixiv-cli/`。

## 不可违反的架构约束

### Cursor：扩展现有实现

项目已有两层 cursor：

```text
sdk.Cursor
    → sdk/pixiv continuation envelope
```

Goal-3 只扩展现有 Pixiv continuation payload，不创建第二套 cursor abstraction。必须复用：

- `sdk.Cursor` 的 opaque envelope。
- `sdk/pixiv` 的 product、operation、binding version、query digest、account/client binding 和 validation。
- endpoint-specific sanitized continuation state。

禁止：

- 在 CLI 或 MCP 层实现 endpoint pagination token。
- 绕过 `sdk.Cursor`。
- 将 `next_url`、signed URL、token、cookie、credential 或非 continuation query 直接写入持久化 cursor。

upstream 返回 `next_url` 时，由 adapter 解析并只保留 allowlist continuation 参数，例如 `offset`、`max_illust_id`、`max_novel_id`、`last_order`。若编码语义变化，只递增现有 `cursorBindingVersion`；不得创建新的 cursor format。

### Pagination / traversal：复用现有 shared 实现

Goal-3 不重建 pagination/traversal engine。统一使用：

```text
SDK endpoint
    → opaque cursor
    → internal/shared/traversal
    → internal/shared/pagination
    → product-level filter
```

已有 `internal/shared/pagination.CollectFilteredPagesFrom` 已处理 filter-before-logical-limit、Skip、Limit、OneBatch、cursor continuation、重复 cursor 检测和 pooled client replay。新增代码只负责 endpoint cursor adapter、参数绑定和 Pixiv-specific filter，不形成 CLI、MCP、SDK 三套分页实现。

### Resolver：复用 `pixiv.ParseURL`

resolver 顺序：

```text
structured canonical record
    → existing pixiv.ParseURL
    → bare ID + explicit --type
    → bare ID without --type 的受控 probe
```

URL 解析保持纯本地，不重写已有 `pixiv.ParseURL`。URL 已包含 entity identity 时，冲突的显式 `--type` 返回 `InvalidArgument`，不得覆盖 URL 类型。bare ID probe 只有 namespace、错误分类、请求成本和失败行为都冻结后才开放；否则要求显式 `--type`。

### Public Go SDK 兼容策略

SDK task 前必须完成 `Public SDK Compatibility Decision`（T12）。Goal-3 默认选择保持源码兼容：

- 保留已有 exported named types，尤其是 public request field 的类型。
- 通过内部 narrow validator/type 限制合法值。
- CLI 可采用更窄的 vNext type abstraction，不强迫 public SDK 同步 breaking。

若必须接受 breaking change，必须在 T12 明确记录 breaking 范围、migration guide、deprecated symbol strategy、semantic version strategy 和是否需要 `/v2` module；实现 Agent 不得自行决定。

### Bookmark / Comment SDK contract

CLI 可以统一 `TARGET` 与 `--type`，public SDK 不因此泛化为 `EntityTarget` 动态 API。优先保持 endpoint-oriented explicit methods：

```text
AddArtworkBookmark / RemoveArtworkBookmark
AddNovelBookmark / RemoveNovelBookmark
ArtworkComments / AddArtworkComment / DeleteArtworkComment
NovelComments / AddNovelComment / DeleteNovelComment
```

reply、stamp、detail、tags 等独立语义必须在 request/model 中明确表达，并由 CLI resolver 负责 dispatch。

## CLI migration matrix

`goal-3/cli-migration-matrix.md` 是 CLI 迁移权威表，必须在 CLI task 开始前冻结。每行明确：

- old surface 与 canonical surface。
- accepted entity type 与 subtype type。
- alias、hidden alias、deprecated 或 delete。
- stdin、JSON/NDJSON 和 MCP 行为。

关键语义：

- `artwork` 表示 artwork endpoint 的实体聚合，不等同于 `illust`；subtype 单独记录。
- `recommended` 的 entity type 与 recommended subtype 不混写。
- `timeline latest` 的 entity type 与 `--content-type` subtype 不混写。
- 不建立一个让所有 command 接受所有值的全球 `EntityType`。
- `--content-type` 只有在与 `--type` 语义不冲突时才迁移；否则作为 subtype flag 保留。
- 未达到 `public_ready` 的 subtype 不得出现在 help、completion、MCP schema 或正式文档。

## 实施阶段

### Phase A：Contract Freeze（T01-T06）

将已确认范围固化为 implementation contract，不再重复探索“接口是否存在”。冻结 endpoint、request、response DTO、continuation、mutation、error semantics 和 regression fixtures。旧 evidence 状态只作为缺口索引。

### Phase B：Protocol / Adapter（T07-T11）

按 owner 实现 HTTP transport 适配、DTO decoding、mutation adapter 和 continuation extraction。adapter 不把内部 DTO 泄漏给 public SDK；next URL 只通过 allowlist sanitization 进入 continuation state。

### Phase C：SDK / Cursor（T12-T19）

先完成 public SDK compatibility decision，再实现 explicit endpoint-oriented SDK methods，扩展现有 Pixiv cursor binding，补齐 adapter/SDK 对照测试。

### Phase D：Shared Semantics（T20-T23）

冻结 canonical types，实现基于 `ParseURL` 的 resolver、type conflict validation、normalized filter，并接入已有 pagination/traversal。rating 默认走 client-side filter：fetch、normalize、filter、继续消耗 continuation，不能发送未经 contract 允许的 server query。

### Phase E：CLI（T24-T36）

按 command owner 分 task：search、novel search、user search/trending、bookmark、recommended、timeline、ranking、detail、series、comment、user、follow、mypixiv。每个 task 单独 Red → Green → Refactor → commit。

### Phase F：MCP / Compatibility / Docs（T37-T41）

按 read/mutation 补齐 MCP aggregation、schema、registration 和 structured errors；执行 alias/deprecation、completion、README、CLI reference、SDK/MCP docs 与 Skill 同步。CLI、MCP 共享 canonical semantic request/validation，不互相调用，也不共享错误的 transport ownership。

### Phase G：Regression（T42-T45）

完成 SDK/protocol、CLI/MCP、live API、全量构建、public surface、evidence redaction 和 migration audit。Goal 只有在所有 public surface 一致且 rejected/excluded 能力无公开入口时完成。

## 停止与回滚

发现 response required 字段不稳定、continuation 无法安全绑定、参数语义被忽略、mutation 无法隔离读回、adapter/SDK 不一致或需要匿名 fallback 时，停止对应能力，保留真实错误和 evidence，不改写为空结果或默认成功。

每个 task 单独提交，不混入无关格式化。单项失败回滚对应 task，不回滚其他已确认修复。public SDK 变更必须遵循 T12 兼容决策和迁移说明。

## 完成标准

1. 所有 public 能力都有 contract snapshot 和 evidence。
2. 普通分页有第二页 fixture；例外 case 明确标记 `pagination_exempt`。
3. 每个 public SDK method 都有 adapter 对照测试。
4. 每个 CLI command 都有 regression；CLI migration matrix 已冻结。
5. MCP schema、SDK、CLI、Skill 和文档一致。
6. Cursor、pagination、traversal、resolver 和 filter 未出现重复实现。
7. rejected/excluded 能力无 public entry。
8. 没有 token、cookie、credential、用户内容或原始 URL 泄漏。
9. `go test ./...` 与 `sh scripts/build.sh` 通过。
