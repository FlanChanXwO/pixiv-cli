# Mutation 验证报告

观测日期：2026-09-05（Asia/Shanghai）。

## 既有 live evidence

已有非主账号的真实写入 evidence：

- 插画文本评论。
- 插画回复评论。
- 小说 stamp 评论。
- 小说文本评论。
- 插画评论删除。
- 小说评论删除。

写入均遵守：

- 真实可评论目标。
- 写入后读回。
- 保存本轮 comment ID。
- 删除仅使用本轮 comment ID。

## 历史轮次状态（2026-09-05）

本轮没有重复执行 mutation。
未接入公共 SDK、CLI 或 MCP。

## Shaft 对照

Shaft 只确认 add/reply 的历史调用形态：

- 共用 add endpoint。
- reply 增加 `parent_comment_id`。

Shaft 没有 delete 实现。
Shaft 没有打通 stamps API。

## 准入结论

所有 mutation 仍为 `not_tested`（针对当前生产层）。
上游 wire/read-back evidence 不等于公开 SDK contract。
在 Goal-3 内建立按 capability/owner 拆分的独立 TDD task。


## Novel bookmark mutation 缺口

原始计划还要求：

- `/v2/novel/bookmark/detail`。
- `/v2/novel/bookmark/add`。
- `/v1/novel/bookmark/delete`。
- list 与 tags 读回。
- 删除后恢复原状态。

这些 case 尚未进入当前 strict mutation manifest。
正式发布前须完成验证；未发布实现的准入见能力准入表。

## 2026-09-07 响应与失败契约补充

本轮未执行真实 mutation。当前门禁只见 [能力准入表](capability-admission.md)。T09A 先于 comment adapter/SDK，提供可解码 form response；本轮 ID 必须来自创建响应的冻结字段。已取得 ID 但读回失败与结果不确定必须单独暴露，不属于可自动重放失败。写入/读回/清理使用同账号 execution context；只清理本轮 ID，无法证明 ID 时保留需处理状态，不猜测删除。
