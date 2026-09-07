# Goal-3 CLI 与 MCP 迁移策略

修订日期：2026-09-07（Asia/Shanghai）。当前能力状态和发布授权只见 [能力准入表](capability-admission.md)。本表给出迁移策略，T39A 必须在实施前补齐逐符号/wire fixture 清单并冻结；T39B 做实施后审计。未发布实现可同步编写命令、schema、completion、测试和文档，不代表已公开。

## 身份与类型

Target kind 指 TARGET 标识的实体；Result kind 指操作内容；Subtype 是 endpoint 子类型。由 command-specific contract 决定是否需一致，不比较全局 URL kind 与 --type。ParseURL 保持纯本地，并保留 ReferenceKind 的 user/user bookmarks/artwork series/novel series 等细分信息。

### T20 类型语义冻结

以下是跨命令共享的词汇与边界；它冻结语义，不引入一个让所有命令接受任意值的 public `EntityTarget` union。具体 request 仍由 `sdk/pixiv` 的 endpoint-oriented method 承载。

| Layer | Canonical meaning | Allowed values / boundary | Explicitly not |
| --- | --- | --- | --- |
| Target kind | 输入身份或被操作的 namespace | `user`、`artwork`、`novel`、`series`、`comment`；`user_bookmarks`、`artwork_series`、`novel_series` 作为 `ReferenceKind` 的具体身份必须保留 | 不是返回结果类型，也不是搜索字段 `SearchTarget` |
| Result/entity kind | 本次操作读取、产生或返回的内容 | `artwork`、`novel`、`user`、`series`、`comment`、`tag`，由 command-specific contract 选择 | 不是 URL 的 `ReferenceKind`，也不因 subtype 改名 |
| Subtype | endpoint 支持的内容子类型 | artwork 目前为 `illust`、`manga`、`ugoira`；现有 SDK/wire 的 `illustration` 是 `illust` 的兼容拼写，不得在 T12 中静默删除既有名称 | `all`、`illust-and-ugoira` 是选择器/组合值，不是返回 subtype；text/reply/stamp 是 comment 操作类型 |

约束如下：

- `query` 是搜索词的输入形状，不是实体 Target kind。搜索的 `--type` 选择 Result kind；`SearchTarget` 只表达匹配字段，不能被解释成实体身份。
- `series` 是产品层 Target family，但 URL 解析必须保留 `ReferenceKindArtworkSeries` 与 `ReferenceKindNovelSeries`；series 内容的 Result kind 分别为 artwork 与 novel。`user_bookmarks` 只能表示 URL 已声明的收藏 namespace，不能覆盖 URL 中明确写出的内容类型。
- canonical identity record 的类型表示 namespace；DTO record 的兼容 `type` 字段可能携带历史 subtype。resolver 不得把 `Record.Type()` 当作全局类型：`illustration`/`manga`/`ugoira` 只可规范化为 artwork + subtype，`novel`/`user` 仍按对应 entity kind 处理。后续若增加结构化字段，entity kind 与 subtype 必须分列。
- comments listing/create 的父 Target 是 artwork 或 novel，Result kind 是 comment；reply/delete 的 Target 是 comment。`text`、`reply`、`stamp` 不参与 artwork/novel namespace 比较。

### T20 命令级冲突规则

| Command family | Target kind / input | Result kind or selector | Compatibility and conflict rule |
| --- | --- | --- | --- |
| `search` | query | artwork、novel、user | 只接受该命令已声明的 `--type`；artwork 的 subtype/filter 不得泄漏到 novel/user |
| `detail` | artwork、novel、user | 与 Target 相同 | URL 的 `ReferenceKind` 必须与 `--type` 精确对应；`--content` 只允许 novel；不接受 `all` |
| `series` | artwork series 或 novel series | artwork 或 novel | series URL 的具体 kind 决定 dispatch；显式 `--type` 不得与其冲突；不接受 `all` |
| `bookmark list` | user 或 user bookmarks | artwork、novel、all | `/users/{id}` 代表所属用户，因此与 `--type novel` 合法；明确声明 artwork 的 bookmarks URL 不得被解释为 novel；`all` 只在 list 生效 |
| `bookmark tags` | user 或 user bookmarks | tag，带 artwork/novel 内容标记 | `all` 只在 tags 生效；同名 tag 按内容 kind 保留，不合并 name/count；detail/add/remove 不继承此值 |
| `bookmark detail/add/remove` | artwork 或 novel | 对应作品或 mutation outcome | 作品 URL、structured record 和显式类型必须指向同一 namespace；不接受 `all`，不把 subtype 当 namespace |
| `comment` | read/create 为 artwork/novel；reply/delete 为 comment | comment 或 mutation outcome | 父作品/评论的 namespace 与命令契约不一致时返回 `InvalidArgument`；操作类型不是 subtype |
| feed/user operations | none 或 user | 由各命令冻结为 artwork、novel、user 或聚合结果 | 不从全局类型集合推断；只接受该 endpoint/command 的显式 selector |

Resolver 和错误行为冻结为：

1. 先消费结构化 canonical identity record；校验正数 ID、command allowlist、entity kind 与独立 subtype。旧 subtype record 仅按上面的兼容映射归入 artwork，不猜测 novel/user。
2. 对 URL 只调用纯本地 `ParseURL`。保留 user、user bookmarks、artwork series、novel series 的 `ReferenceKind`；URL 已声明的 namespace 与 `--type` 不一致时直接返回 `InvalidArgument`，不得换一种类型重试。
3. 对显式类型 ID，只按该 command 的单一声明或用户明确选择解释；不能把裸数字当作全局可判定的 artwork/novel/user/series。
4. bare-ID probe 只有在对应 command contract 明确允许、候选 namespace 已冻结且每个结果能区分成功/不存在/禁止/网络失败时才可使用。403、网络错误或不确定响应不能被当作“类型不匹配”并触发隐式 fallback；多个候选成功或无法判定时返回 `InvalidArgument`。
5. `all` 不是 subtype。只有本矩阵明确列出的聚合命令可接受它；detail、series、bookmark detail/add/remove 及其他未声明命令必须显式拒绝。

CLI 与 MCP 可以共享上述 semantic validation，但互不调用；MCP tool 名固定的 namespace 仍视为 command contract。SDK 继续保留 endpoint-oriented methods，T20 不改变既有 public symbol、wire 字段或默认值。

| Old surface | Canonical surface | Target kind | Result kind / --type | Subtype | Compatibility decision |
| --- | --- | --- | --- | --- | --- |
| novel search WORD | search WORD --type novel | query | novel | none | retain deprecated route；本 Goal 不删除 |
| user search WORD | search WORD --type user | query | user | none | retain deprecated route |
| follow add/remove | user follow/unfollow | user | user | none | retain deprecated alias |
| user bookmarks USER_ID | bookmark list TARGET | user / user bookmarks | artwork, novel, all | artwork filter | retain hidden alias，旧 defaults 不变 |
| bookmark list TARGET | 同名 | user / user bookmarks | artwork, novel, all | artwork filter | retain；all 新增聚合契约见下 |
| bookmark tags TARGET | 同名 | user / user bookmarks | artwork, novel, all | 只接受已冻结的 tags subtype | retain；all 标签保留内容类型 |
| bookmark detail/add/remove TARGET | 同名 | artwork 或 novel | 对应作品类型；不接受 all | 不把子类型当 namespace | retain；explicit SDK dispatch |
| recommended all | recommended --type all | none | artwork, novel, user, all | 按 artwork endpoint 冻结 | retain deprecated positional all |
| timeline following/latest | 同名 | none | artwork, novel | --content-type | retain；latest continuation 单独绑定 |
| ranking | 同名 | none | artwork, novel | 按 ranking contract | retain；novel 待回归后发布 |
| detail TARGET | 同名 | artwork, novel, user | 与目标身份一致 | none | retain；novel content 禁止旧 endpoint，符号兼容见 T12 |
| series TARGET | 同名 | artwork series, novel series | artwork, novel | none | retain；URL series identity 决定 dispatch |
| comment TARGET | 同名 | read/create 为 artwork/novel；reply/delete 按 comment contract | artwork, novel namespace | text/reply/stamp 是操作类型 | retain read；新增 mutation 分别冻结 |
| mypixiv users/works | 同名 | 按具体命令 | user 或 artwork/novel | command-specific | retain 既有合法 alias，不新增全局任意类型 |
| --content-type | subtype flag | 所属 command | 不覆盖 --type entity 语义 | command-specific | retain；不建立语义不同的 --type alias |
| --download-path | --output / -o | N/A | N/A | N/A | retain 现有 alias，本轮不混入下载迁移 |

`pixiv bookmark list https://www.pixiv.net/users/123 --type novel` 合法；user URL 指所属用户，novel 指收藏内容。作品 URL 与不同作品 namespace 的显式类型冲突则 InvalidArgument；structured record 遵循相同规则。未冻结 bare-ID probe 前要求可判定的显式类型，probe 冻结后仍不得把 403/网络错误当作“换一个类型试试”的依据。

## bookmark list/tags --type all（必交付）

- 只适用于 list/tags；detail/add/remove 明确拒绝 all。
- 按 artwork → novel 顺序连接两个实体流，各流保持 upstream 顺序；Skip/Limit 对连接后的逻辑序列只应用一次，不给每类分别发放完整预算，也不预留固定配额。
- cursor 在现有 Pixiv payload 中记录当前流、两流 continuation/checkpoint 与完成状态，绑定 operation、用户、restrict/tag/subtype/local filter 和账号。all 与单类型 cursor 不互用。
- OneBatch 表示当前实体流第一个有匹配内容的批次，允许跳过空流；不为凑两类结果额外抓一批。到达边界时保存下一流起点。
- tags 的每条结果显式标记 artwork/novel，保留各自 name/count；同名标签不跨类型合并、不相加、不去重。list 同样保留规范化实体类型。
- 一次逻辑页所需的任一流失败，整次失败，不返回成功子集。JSON/NDJSON 在该逻辑页收集成功后才输出；不回收之前调用已交付页面。
- 验收含跨流 limit、第一流耗尽/为空、第二流错误、末批截断、重复名称不同 count、恢复无遗漏重复和 changed binding 拒绝。

## SDK/MCP 与输出兼容

public SDK 保持 endpoint-oriented methods，不泛化为动态 EntityTarget API。CLI/MCP 共享产品 semantic validation，互不调用。T12 负责旧 SDK symbol map/编译，T39A 负责旧 MCP tool/input/output/default/error 逐项 map/回放，不能以“同一 semantic request”替代 wire 决策。

默认保留旧 tool 名称、必填字段及返回结构，例如 add_bookmark.illust_id；新类型能力采用明确新增 operation/schema，不静默改变旧调用方默认值。确需 breaking 先经明确批准并记录迁移。既有 stdin、JSON/NDJSON、skip/fail-fast 和 MCP structured error/stdout 边界保持；上述 all 是新增的页原子契约，不改变旧命令的输出策略。
