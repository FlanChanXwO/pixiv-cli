# Goal-3 CLI 与 MCP 迁移策略

修订日期：2026-09-07（Asia/Shanghai）。当前能力状态和发布授权只见 [能力准入表](capability-admission.md)。本表给出迁移策略，T39A 必须在实施前补齐逐符号/wire fixture 清单并冻结；T39B 做实施后审计。未发布实现可同步编写命令、schema、completion、测试和文档，不代表已公开。

## 身份与类型

Target kind 指 TARGET 标识的实体；Result kind 指操作内容；Subtype 是 endpoint 子类型。由 command-specific contract 决定是否需一致，不比较全局 URL kind 与 --type。ParseURL 保持纯本地，并保留 ReferenceKind 的 user/user bookmarks/artwork series/novel series 等细分信息。

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
