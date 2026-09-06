# 上游接口可行性与完整实施 goal 准入报告

观测日期：2026-09-05（Asia/Shanghai）。
修订日期：2026-09-06（Asia/Shanghai）。

## 结论

计划具备作为**单一完整实施 Goal**运行的结构，且按审查报告继续保持单一 Goal。

Goal-3 范围内 upstream 能力已作为可实施输入确认。现有 `inconclusive`、`not_tested` 等计数表示 evidence、contract snapshot 或生产覆盖尚未完成，不再表示接口不存在。候选 capability 仍必须先过 `contract_frozen` / `migration_ready`，公开能力还必须过 `public_ready` gate。

正确状态是：

```text
single_goal_ready = true
unrestricted_full_surface_ready = false
public_sdk_compatibility_decision = required_before_sdk
fail_closed_gate = required
```

也就是说：

- 可以启动一个完整 goal。
- 不需要再拆成多个后续 goal。
- Phase A 必须先完成 contract freeze。
- 未 `public_ready` 能力不能进入 public surface。
- 验证失败时，只停止对应 capability，不另起 Goal。

## 当前实时验证结果

使用本地凭证的临时 HOME 副本。
原始凭证数据库未修改。
本轮未执行 mutation。

strict upstream read test：通过。

```text
confirmed=9
rejected=3
inconclusive=13
blocked=0
not_tested=11
```

全量 `go test ./...`：通过。

## 能力准入

所有范围内能力属于同一个 Goal-3。表中历史状态仅表示 evidence/覆盖情况；实施动作由 contract freeze、adapter/SDK 和 public gate 决定。

| 能力 | 当前状态 | 完整 Goal 处理方式 |
| --- | --- | --- |
| artwork search | confirmed | 直接实施 SDK、CLI、MCP |
| artwork latest | confirmed | 直接实施 SDK、CLI、MCP |
| artwork ranking | confirmed | 直接实施 SDK、CLI、MCP |
| artwork ugoira metadata | confirmed | 直接实施 SDK、CLI、MCP |
| novel follow | confirmed | 直接实施 SDK、CLI、MCP |
| novel recommended | confirmed | 直接实施 SDK、CLI、MCP |
| user novels | confirmed / data-limited | 按 pagination exemption 实施 |
| user artworks | confirmed / data-limited | 按 pagination exemption 实施 |
| public/private bookmarks | confirmed / data-limited | 按 pagination exemption 实施 |
| novel detail v2 | scope-admitted / snapshot pending | T01-T02 冻结 contract 后实施 |
| novel series v2 | scope-admitted / snapshot pending | T02/T05 冻结 contract 后实施 |
| novel latest | scope-admitted / cursor snapshot pending | T02/T05 扩展现有 cursor 后实施 |
| artwork recommended | scope-admitted / continuation snapshot pending | T04/T05 冻结受控 continuation 后实施 |
| novel comments v3 | scope-admitted / DTO snapshot pending | T04 冻结 DTO 后实施 |
| artwork comments v3 | scope-admitted / fixture mismatch | T04 冻结 DTO；未冻结前不公开 |
| novel bookmark mutation | scope-admitted / production gap | T03/T06 freeze 后在 Goal-3 T08/T15 实施 |
| comment mutation | scope-admitted / production gap | T04/T06 freeze 后在 Goal-3 T09/T16-T17 实施 |
| stamps | scope-admitted / production gap | Goal-3 T09/T17 实施；public gate 后公开 |
| novel ranking | scope-admitted / production gap | Goal-3 T10/T18/T30 实施；public gate 后公开 |
| server-side rating | rejected | 不实施；使用 client-side filter |
| WebView content | not_tested / excluded | 不进入本 goal 默认路径 |

## 当前实施条件

### 已满足

- 证据格式已建立。
- verdict 分类已建立。
- adapter/SDK gate 已建立。
- 本地凭证可用于 read validation。
- 原始本地凭证可隔离。
- 目标目录资料已同步。
- API 迁移台账已建立。
- 全量 Go 测试通过。

### 必须在 Goal-3 内完成

- 所有范围内 capability 的 contract snapshot。
- 所有原始计划中尚未完整冻结的 API。
- 所有需要公开的 mutation adapter/SDK round-trip。
- 所有新 public surface 的协议测试。
- CLI、MCP、SDK、文档同步。
- 最终 live regression。

## 不可改变的事实

以下接口不能恢复：

- `/v1/novel/detail`。
- `/v1/novel/series`。
- `/v1/novel/content`。

以下行为不能伪装成已支持：

- server-side `x_restrict` rating filter。
- 只返回 HTTP 200 的候选接口。
- 未验证第二页的普通分页。
- 仅有 Shaft 证据、没有 live/evidence snapshot 的接口。
- 未完成 T12 的 public SDK compatibility decision。

## 可行性判断

### 作为一个完整 goal

可行。

但“完整 goal”包含合约收口和条件分流。
它不等于所有候选 API 都能直接迁移。

前提是：

1. T01-T06 先执行 contract freeze。
2. 每项 capability 单独记录 snapshot、实现和 public gate。
3. 失败能力不进入 public surface，也不另起 Goal。
4. 生产变更继续通过 TDD。
5. mutation 继续使用隔离账号和目标。

### 作为无条件全量公开

不可行。

原因：

- 多个 capability 尚未完成 contract snapshot、adapter 或 SDK owner。
- public SDK compatibility decision 尚未完成。
- ranking、stamps、mutation 必须在 Goal-3 内按 owner 拆 task，不能直接公开。
- server-side rating 已被 live 行为否定；只能使用 client-side filter。

## 最终判断

允许进入：

> **一个完整、带 contract freeze、compatibility 和 public gate 的实施 Goal。**

不允许进入：

> **把历史 evidence 状态误当成接口不存在，或绕过 gate 一次性公开全部 capability。**
