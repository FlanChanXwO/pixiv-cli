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

## 本轮状态

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
在验证前不能新增 public SDK/CLI/MCP mutation。
