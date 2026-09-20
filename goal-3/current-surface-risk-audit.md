# 当前公开能力风险审计

观测日期：2026-09-05（Asia/Shanghai）。
修订日期：2026-09-06（Asia/Shanghai）。

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
| P2 | ranking | live path 可用；生产层不存在 | 新增能力缺少 owner | Goal-3 内独立 ranking task；完成 adapter/SDK gate 后公开 |
| P2 | stamps | live path 可用；生产层不存在 | 新增能力缺少 owner | Goal-3 内独立 stamps task；完成 adapter/SDK gate 后公开 |
| P2 | comment mutation | live wire/read-back 有证据；生产层不存在 | 直接接入会越过架构边界 | Goal-3 内按 artwork/novel owner 拆分 TDD task |

## 未发现全面损坏

2026-09-05 历史记录：全量 Go 测试通过（不计作本轮验证）。
未发现搜索、follow、推荐小说、插画最新、插画排行、ugoira metadata 的新增回归证据。

但单元测试不能替代上述 live contract 回归。

## 完整 goal 实施策略

本风险表不再要求另立后续 goal。

当前 goal 先执行 T00/T20、T01-T06、T12/T39A 范围、类型、合约和兼容冻结。

之后才进入 Protocol、SDK、CLI 和 MCP 修改。

未确认能力保持不可达。

因此：

- 单一 goal 可启动。
- 全量 public surface 不可盲目放行。
- 每个风险项都有独立停止条件。
- 失败不会降级为空结果或静默 fallback。


## API 迁移审计结论

本审计中的历史 verdict 描述 evidence 和当前生产覆盖，不再表示 upstream 能力不存在。Goal-3 已将相关能力纳入同一个实施范围；Phase A 负责把已确认范围固化为 contract snapshot。

未完成 contract freeze 前：

- 可以建立独立验证、adapter 和 SDK task。
- 不切换未经冻结的生产 endpoint。
- 不新增 public SDK、CLI/MCP 入口。
- 不提供静默 fallback。

ranking、stamps、comment mutation 属于 Goal-3，不得延期到另一个 Goal。


## Contract snapshot 缺口：原始计划尚未完整冻结

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

这些缺口在 Phase A 处理。它们是 contract freeze 缺口，不是另起 Goal 的理由；未冻结时不能进入 public surface。

## 2026-09-07 固定基线审查处置

当前 capability 状态只见 [能力准入表](capability-admission.md)，不以本历史风险等级授予发布权限。本轮外部审查未确认 P0；上表保留原轮次分级，不能混算成此次审查结论。

| Finding | 本轮处置 | 后续验收归属 |
| --- | --- | --- |
| P1-1 批内续读遗漏/末批空 cursor | 真实生产测试 Red；T23A 修复共享 checkpoint、SDK 与 CLI/MCP | T23 接入其余 endpoint；分页报告 |
| P1-2 Target 与 Result 类型混淆 | T20 与迁移矩阵区分三层；user URL + novel bookmarks 合法 | T21/T27 |
| P1-3 前置任务倒置 | T12/T20/T39A 前移，显式 depends_on | 依赖 DAG 检查 |
| P1-4 发布状态循环 | 唯一 capability authority；未发布实现允许编写 SDK/注册/docs | T41/T45 |
| P1-5 子集验收冒充完整交付 | required_scope 固定；未完成保持 incomplete；scope change 需明确批准 | T00/T45 |
| P2-6 owner/all 缺口 | recommended 归 T01/T05；list/tags all 必交付且 typed counts | 准入 owner 链、T03/T27 |
| P2-7 mutation 响应/不确定结果 | T09A 显式前置；同账号 ID/读回/清理，禁止不确定重放 | T09/T16/T38 |
| P2-8 SDK/MCP 兼容缺口 | symbol map/wrapper/旧消费者；MCP wire map/旧 JSON 回放；cursor 版本策略 | T12/T39A/T39B |
| P2-9 粒度/回滚不可执行 | 实现前任务卡、跨 owner 子卡、依赖闭包回滚 | tasks 准入规则 |

其余生产行为本轮未修改；不将 stamps、novel mutation 或 all 计划修订记作实现完成。
