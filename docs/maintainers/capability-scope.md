# Capability Scope: Unsupported and Evidence-Gated Capabilities

> 本清单是 Goal 3 `capability-matrix.md` 的 maintainer 侧镜像，规定 v1 中**不允许有入口**的能力的唯一 owner、证据门槛与关闭条件。它不是「待办功能」清单；每一项都是负向契约——如果某天有人给这些能力加了 CLI/MCP/SDK 入口，必须同时在本文件登记并更新矩阵。
>
> 依据：Goal 3 findings；Goal 4 `tasks.md` T18；`goal-3/inventory/capability-matrix.md`。

## 规则

- **unsupported**：v1 明确不支持，新增入口属于缺陷。
- **evidence-gated**：当前无入口；若未来要支持，必须先满足下方「关闭条件」并同步本文件与矩阵，禁止先用 schema 占位或 mock 空结果。
- 核验命令均为 negative grep / 目录存在性检查，写入每项；review 时人工重跑，不恢复 AST scanner。

## Unsupported（2 项）

| ID | 唯一 owner | 证据门槛（现状） | 关闭条件 |
| --- | --- | --- | --- |
| `ART-SEARCH-RATING` | `internal/cli/pixiv/search`（flag 诊断路径）+ `sdk/pixiv`（search request 无 rating 字段） | CLI `--rating` 显式报 "rating filter is not supported by the v1 App API search contract"；MCP `search_illust` schema 无 rating 参数 | 仅当 v1 App API search 契约新增 rating 语义时；届时同步 SDK request 字段、CLI flag、MCP schema、三语文档与本文件 |
| `NOVEL-SEARCH-ADVANCED` | 无 owner（**不得新增**） | SDK/MCP schema 无 advanced 字段 | 同上：上游契约出现后可评估，禁止 schema-only 占位 |

## Evidence-Gated（10 项）

| ID | 唯一 owner | 证据门槛（现状） | 关闭条件 |
| --- | --- | --- | --- |
| `NOVEL-RANKING` | 无 owner（禁止用 artwork ranking 冒充） | SDK 无 `NovelRanking` 导出；MCP 无 `novel_ranking` tool | 上游 App API 提供 novel ranking 后按正常 capability 流程立项 |
| `NOVEL-BOOKMARK-MUTATION` | 无 owner | SDK 无 `AddNovelBookmark` 类导出；`user_novel_bookmarks` 是只读 | 同上 |
| `COMMENT-WRITE` | 无 owner | MCP `comment_post`/`comment_add` 目录 = 0；SDK `PostComment`/`DeleteComment` 导出 = 0；仅存在 comments **读取** owner（`internal/services/pixiv/artwork/comments`、`novel/comments`） | 上游提供可验证的写契约后再立项 |
| `NOTIFICATION` | 无 owner | MCP `notification` 目录 = 0；SDK `Notification*` 导出 = 0 | 同上 |
| `AUTOCOMPLETE` | 无 owner | MCP `autocomplete` 目录 = 0；SDK `Autocomplete*` 导出 = 0；未合并进 `search` query | 同上 |
| `WEB-RESTRICTED-READ` | 无 owner（v1 语义：**不存在匿名 Web fallback**） | 无 `webapi` 包；`web_fallback_enabled` 为墓碑键（`config get/set` → `removed_setting`） | 关闭条件 = 不重开匿名 Web 路径；任何恢复 Web/AJAX 的提案必须先改 AGENTS 冻结契约并过 ADR |
| `USER-BLOCK-MUTE-REPORT` | 无 owner | MCP `mute`/`report` 目录 = 0；SDK `BlockUser`/`MuteUser`/`ReportUser` 导出 = 0；与只读 `USER-BLOCKED` 严格区分 | 上游提供可验证的 mutation 契约后再立项 |
| `WATCHLIST-MARKER` | 无 owner | MCP `watchlist` 目录 = 0；SDK `Watchlist*` 导出 = 0；未以本地 archive 替代服务端状态 | 上游提供后按正常流程立项 |
| `BOOKMARK-USERS` | 无 owner | 无对应 tool/SDK 导出；`bookmark_detail` 仅当前用户 detail | 上游提供后可评估 |
| `SPOTLIGHT-PIXIVISION` | 无 owner（scope 外） | MCP `spotlight`/`pixivision` 目录 = 0；SDK `Spotlight*`/`Pixivision*` 导出 = 0 | 明确 scope 外；仅当产品范围变更时重新评估 |

## 变更纪律

- 新增/恢复任何上表能力的入口 = 功能变更，必须：更新本文件对应行的状态与关闭条件 → 更新 `goal-3/inventory/capability-matrix.md` → 过 review checklist 的 capability 检查 → 按 PR release-note 规则声明。
- 禁止只加 schema 字段、只建空 tool 目录或 mock 空结果来「预留」能力。
- 本文件与矩阵不一致时，以本文件（maintainer 侧权威）为准并修正矩阵。
