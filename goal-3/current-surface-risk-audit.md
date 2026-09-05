# 当前公开能力风险审计

观测日期：2026-09-05（Asia/Shanghai）。

| 级别 | 能力/命令 | 当前事实 | 风险 | 处置 |
| --- | --- | --- | --- | --- |
| P0 | `pixiv detail -t novel` | 当前仍走 v1；v1 live 不可用 | 小说 detail 可能失败 | 改 v2 后再回归 |
| P0 | `pixiv series -t novel` | 当前仍走 v1；v1 缺 required detail | 结果可能不完整 | 改 v2 后再回归 |
| P0 | `pixiv detail -t novel --content` | App v1 不可用 | content 命令可能失败 | WebView 仅作独立候选；不做 fallback |
| P1 | `pixiv timeline latest -t novel` | live continuation 是 `max_novel_id`；SDK 仍用 offset | 第二页失败或错页 | TDD 修正 cursor |
| P1 | `pixiv recommended -t artwork` | 首页成功；第二页失败 | 翻页不可靠 | 保存受控完整 continuation |
| P1 | `pixiv comment -t artwork` | live `date` / numeric access control；DTO 仍旧 | 解析失败或字段丢失 | 校正 adapter DTO |
| P1 | `pixiv comment -t novel` | v3 候选可行；生产仍 v2 | 版本和字段不一致 | v3 候选单独回归 |
| P1 | Novel detail series | live 有 `novel.series`；模型无字段 | 系列信息丢失 | 明确加入模型或记录为非目标 |
| P2 | rating flag | live 忽略 `x_restrict` | 形成伪过滤能力 | 仅做客户端过滤或不公开 |
| P2 | ranking | live path 可用；生产层不存在 | 新增能力缺少 owner | 单独 goal |
| P2 | stamps | live path 可用；生产层不存在 | 新增能力缺少 owner | 单独 goal |
| P2 | comment mutation | live wire/read-back 有证据；生产层不存在 | 直接接入会越过架构边界 | 单独 TDD goal |

## 未发现全面损坏

全量 Go 测试通过。
未发现搜索、follow、推荐小说、插画最新、插画排行、ugoira metadata 的新增回归证据。

但单元测试不能替代上述 live contract 回归。

## 完整 goal 实施策略

本风险表不再要求另立后续 goal。

当前 goal 先执行 T01-T06 合约收口。

之后才进入 Protocol、SDK、CLI 和 MCP 修改。

未确认能力保持不可达。

因此：

- 单一 goal 可启动。
- 全量 public surface 不可盲目放行。
- 每个风险项都有独立停止条件。
- 失败不会降级为空结果或静默 fallback。


## API 迁移审计结论

当前计划中的 API 升级项不能全部视为已验证。

已确认项可以直接进入实施。
候选项必须先完成 live、adapter、SDK 和分页 gate。

未满足 gate 前：

- 不切换生产 endpoint。
- 不新增 public SDK。
- 不新增 CLI/MCP 入口。
- 不提供静默 fallback。


## 新增缺口：原始计划未完整进入 strict manifest

以下 API 或参数仍缺强制验证 case：

- `/v1/search/novel` 的 `start_date/end_date`。
- `/v1/user/bookmark-tags/novel`。
- `/v2/novel/bookmark/detail`。
- `/v2/novel/bookmark/add`。
- `/v1/novel/bookmark/delete`。
- `/v1/user/bookmark-tags/illust`。
- artwork bookmark subtype 参数。
- artwork recommended subtype。
- artwork latest 扩展 subtype。
- comments `total_comments` 的非空语义。

这些缺口必须在阶段 A 处理。
未验证时不能进入阶段 B。
