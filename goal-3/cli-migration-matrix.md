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
| ranking | 同名 | none | artwork, novel | 按 ranking contract | retain；默认 artwork，novel 通过 `--type novel` 显式选择 |
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

## T39A：CLI route compatibility map

CLI 的当前 route/flag canonical 说明在双语
[`cli-reference.md`](../docs/en/cli-reference.md) 与
[`cli-reference.md`](../docs/zh-CN/cli-reference.md)；本节只冻结迁移关系和
兼容边界，不复制整份 flag reference。当前树通过 `pixiv --help` 及各 leaf
`--help` 核对，未使用 Cobra `Aliases` 隐式改变 route；表中的“兼容路径”是
仍注册的明确 command path。

### 共享 CLI 输入/输出边界

| Surface | Frozen contract |
| --- | --- |
| positional/stdin | command-specific positional value 优先；允许 stdin 的命令只消费一个完整值。URL/record 解析由该 command 的 resolver 负责，不把裸 ID 当作全局类型。 |
| entity selector | `--type/-t` 选择 Result/entity kind；`--content-type` 只选择 endpoint subtype。`all` 只在明确列出的聚合命令有效。 |
| list pagination | `--limit/-l` 省略为一个 upstream batch，正数填充 logical page，`0` 读到 cursor 结束；`--page/-p` 从 1 开始且要求正数 `--limit`。CLI 不解析或持久化 MCP/SDK opaque cursor。 |
| output | `--json` 输出 structured JSON；列表在非终端按现有规则可输出 canonical NDJSON；`--ndjson` 只用于列表。stdout 不输出 token、Cookie、签名 URL 或请求头。 |
| transport | `--proxy` 与 `--no-proxy` 二选一，作用域仅当前命令；数据命令只从本地已选账号/账号池取认证，不接受 `--uid`/`--refresh-token` 覆盖。 |
| errors | 本地输入冲突、未知 enum、非法 ID、不可达/已排除 endpoint 均显式失败；不得把 upstream error、取消或“不支持”伪装为空结果或静默 fallback。 |

### 当前 route → canonical operation → compatibility decision

| Current route | Canonical operation / target | Required input and defaults | Compatibility decision and replay gate |
| --- | --- | --- | --- |
| `pixiv search [WORD\|IMAGE_PATH_OR_URL]` | `SearchArtworks`、`SearchNovels`、`SearchUsers`；图片源走 reverse-search facade；`--trending-tags` 走 trending | `WORD` 在实体搜索时必需；`--type/-t=artwork` 默认；artwork subtype 用 `--content-type` | 保留 canonical route；route/type/flag 组合必须由 command-specific validation 拒绝冲突，旧搜索 JSON/NDJSON 与分页回归由 T24/T43 承接。 |
| `pixiv novel search WORD` | `SearchNovels(SearchNovelsRequest)` | `WORD` 必需；`--search-by=tag-partial`、`--sort=date_desc` 默认；只暴露基础 period/search fields | 保留兼容路径；等价目标是 `pixiv search WORD --type novel`，不删除旧 route，也不引入 rating/text-length/original-only 字段。 |
| `pixiv user search WORD` | `SearchUsers(SearchUsersRequest)` | `WORD` 必需；列表分页按共享规则 | 保留兼容路径；等价目标是 `pixiv search WORD --type user`。 |
| `pixiv detail ID_OR_URL` | `Artwork`、`Novel`、`User` | `--type/-t=artwork` 默认；artwork 可用受控 URL，novel/user 要求正数 ID | 保留 route；URL 与显式 type 冲突直接 `InvalidArgument`。`--content` 只保留输入兼容，正数 novel 现在返回 `content_unavailable` 且不调用 rejected content endpoint。 |
| `pixiv series SERIES_ID` | `ArtworkSeries` 或 `NovelSeries` | `--type/-t` 必填；`SERIES_ID` 正数；list pagination 可选 | 保留 route；类型先于 ID dispatch，novel v1 rejected path 不作为 fallback。 |
| `pixiv comment ID` | `ArtworkComments` 或 `NovelComments` | `--type/-t` 必填；`ID` 正数；list pagination 可选 | 保留 read route；create/reply/stamp/delete 是后续 additive operation，不偷改当前输出。 |
| `pixiv ranking` | `ArtworkRanking` 或 `NovelRanking` | `--type/-t=artwork` 默认；`--mode=day` 默认；`--date` 只适用于 artwork；list pagination 可选 | 保留 route；novel ranking 不是旧 route 的隐式分支，使用显式 `--type novel` 发布。 |
| `pixiv recommended [KIND]` | kind-specific recommendation operations | `--type/-t` 与位置 `KIND` 任选其一；支持 artwork/novel/user/all；list pagination 可选 | 保留位置参数作为兼容写法；若两种 selector 冲突返回 `InvalidArgument`；`all` 的流顺序/原子页契约由 T28/T43 验证。 |
| `pixiv timeline following [--type KIND]` | `FollowingArtworks` 或 `FollowingNovels` | `--type` 必填；artwork `--content-type=all` 默认，`--restrict=public` 默认 | 保留 route；`--type` 是 entity，`--content-type` 是 subtype，不互相替代。 |
| `pixiv timeline latest [--type KIND]` | `LatestArtworks` 或 `LatestNovels` | `--type` 必填；artwork `--content-type=illust` 默认且只接受 `illust/manga` | 保留 route；不把 search 的 `all` 当作 latest subtype；novel continuation 的 `max_novel_id` 修复由后续 owner 完成。 |
| `pixiv mypixiv users` | `MyPixivUsers` | list pagination 可选 | 保留 route；始终使用已认证 runtime identity。 |
| `pixiv mypixiv works [USER_ID]` | `MyPixivArtworks` 或 `MyPixivNovels` | `--type` 必填；提供 USER_ID 时保留 artwork subtype 兼容规则 | 保留 route；公开 entity `artwork` 与旧 `illust` 拼写均须按既有兼容决策处理，不改成动态 union。 |
| `pixiv user detail USER_ID` | `User` | 正数 USER_ID | 保留 route；不以 user URL 或裸 ID 猜测其他 namespace。 |
| `pixiv user artworks [USER_ID]` | `UserArtworks` | USER_ID 可省略并解析认证用户；`--type=illustration` 旧默认，允许 `illust/manga/ugoira` | 保留 route；ID/default/type 字段保持，后续 resolver 只扩展已冻结的受控输入。 |
| `pixiv user novels [USER_ID]` | `UserNovels` | USER_ID 可省略并解析认证用户；list pagination 可选 | 保留 route；失败不切 artwork 或 Web fallback。 |
| `pixiv user bookmarks [USER_ID]` | `UserArtworkBookmarks` | USER_ID 可省略；`--restrict=public`、`--tag` 可选 | 保留旧 artwork-only route；新 typed `bookmark list` 不得删除它或改写其默认值。 |
| `pixiv user following/followers [USER_ID]` | `UserFollowing` / `UserFollowers` | USER_ID 可省略；`--restrict=public` 默认；list pagination 可选 | 保留 route；关系 scope 不接受全局动态 type。 |
| `pixiv user related USER_ID` | `RelatedUsers` | 正数 USER_ID；list pagination 可选 | 保留 route；不把 `USER_ID` 重命名为通用 TARGET。 |
| `pixiv user blocked [USER_ID]` | `UserBlockedUsers` | USER_ID 可省略并解析认证用户；list pagination 可选 | 保留 route；App API failure final，不做匿名/Web fallback。 |
| `pixiv user follow add/remove USER_ID` | `FollowUser` / `UnfollowUser` | 正数 USER_ID；add 的 `--restrict=public` 默认；`--on-error=skip` 默认 | 保留 route；mutation access control/read-back/outcome 由 T35/T38 owner 补齐，不自动重放不确定写入。 |
| `pixiv bookmark list [USER_ID]` | artwork/novel bookmark list adapters | `--type=artwork` 默认，后续 required `all`；`--restrict=public`、`--tag` 可选；USER_ID/URL 由 resolver 处理 | 保留并作为 canonical typed list；`user URL + --type novel` 合法；`all` 聚合不改变单类型旧输出。 |
| `pixiv bookmark tags [USER_ID]` | artwork/novel bookmark tag adapters | `--type=artwork` 默认，后续 required `all`；`--restrict=public` 默认；USER_ID/URL 可选 | 保留 route；同名 tag 按内容类型分别输出 count，不跨类型合并。 |
| `pixiv bookmark detail ARTWORK_ID` | `ArtworkBookmark` | 正数 ARTWORK_ID；不接受 `all` | 保留 artwork-only route；未收藏必须保持明确的 false/empty 状态。 |
| `pixiv bookmark add/remove ILLUST_ID` | `AddBookmark` / `RemoveBookmark` | 正数 ILLUST_ID；add 的 `--restrict=public`、`--on-error=skip` 默认，`--tag` 可重复 | 保留 `ILLUST_ID` 和旧输出；novel mutation 是未来显式 additive route，不替换旧 artwork wrapper。 |
| `pixiv follow add/remove USER_ID` | `FollowUser` / `UnfollowUser` | 与 `user follow` 相同 | 保留 root compatibility route；不强制用户迁移后才执行。 |
| `pixiv download`、`pixiv mcp`、`pixiv auth/config/update` | 现有 download/runtime/lifecycle ports | 不属于本轮 vNext data-route migration | route 与安全 stdout/credential contract 冻结不变；T39A 不把它们改写成 Pixiv entity target。 |

### CLI 兼容回放清单

T39A 的 CLI 证据只冻结 route/flag 解析，不替代 MCP JSON replay。最小回放集合为：

1. `pixiv search cat --type artwork --limit 1 --json`、
   `pixiv novel search cat --limit 1 --json`、`pixiv user search cat --limit 1 --json`：
   三条搜索 route 的 selector、默认 sort 和 JSON shape。
2. `pixiv detail 101 --type artwork --json`、
   `pixiv detail 201 --type novel --content --json`：后者必须是
   `content_unavailable`，且不能发 `/v1/novel/content`。
3. `pixiv bookmark list 401 --type novel --limit 1 --json` 与
   `pixiv bookmark tags 401 --type all --limit 1 --json`：typed target、all
   聚合和输出原子性，由 T27/T43 在实现后补齐。
4. `pixiv recommended all --json` 与
   `pixiv recommended --type all --json`：位置参数兼容与 flag 形式冲突规则。
5. `pixiv follow add 401 --restrict private` 与
   `pixiv user follow add 401 --restrict private`：两条 route 的旧 alias
   行为和 mutation error boundary。

以上 route 不能作为 T39A 之外 endpoint 已实现的证据；它们只是冻结的输入/输出
回放向量。后续实现失败时必须保留 route 并显式报告错误，不能删 route 逃避兼容门禁。
