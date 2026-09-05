# 上游接口可行性与完整实施 goal 准入报告

观测日期：2026-09-05（Asia/Shanghai）。

## 结论

计划现在具备作为**单一完整实施 goal**运行的结构。

但计划中提到的升级迁移 API 并非全部已确认。
候选迁移必须先过 `migration_ready` gate。
公开能力还必须再过 `public_ready` gate。

正确状态是：

```text
single_goal_ready = true
unrestricted_full_surface_ready = false
fail_closed_gate = required
```

也就是说：

- 可以启动一个完整 goal。
- 不需要再拆成多个后续 goal。
- goal 必须先完成合约收口。
- 未确认能力不能越过 gate。
- 验证失败时，只停止对应能力。

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

| 能力 | 当前状态 | 完整 goal 处理方式 |
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
| novel detail v2 | inconclusive | T02 先收口，再实施 |
| novel series v2 | inconclusive | T02 先完成两页，再实施 |
| novel latest | inconclusive | T02 先修 `max_novel_id` contract |
| artwork recommended | inconclusive | T03 先完成第二页根因验证 |
| novel comments v3 | inconclusive | T03 先完成非空 response 验证 |
| artwork comments v3 | rejected / DTO mismatch | T03 修 DTO 后重新确认 |
| novel bookmark mutation | not_tested / production gap | T04 live round-trip 后实施 |
| comment mutation | not_tested / production gap | T05 live round-trip 后实施 |
| stamps | not_tested / production gap | T06 live + adapter + SDK 后决定 |
| novel ranking | not_tested / production gap | T06 仅验证；confirmed 后才实施 |
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

### 必须在 goal 内完成

- 所有剩余 `inconclusive` 的必要 read contract。
- 所有原始计划中尚未进入 strict manifest 的 API。
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
- 仅有 Shaft 证据、没有 live 证据的接口。

## 可行性判断

### 作为一个完整 goal

可行。

但“完整 goal”包含合约收口和条件分流。
它不等于所有候选 API 都能直接迁移。

前提是：

1. T01-T06 先执行。
2. 每项能力单独判断 verdict。
3. 失败能力不进入 public surface。
4. 生产变更继续通过 TDD。
5. mutation 继续使用隔离账号和目标。

### 作为无条件全量接入

不可行。

原因：

- novel detail/series/latest 尚有实现漂移。
- artwork recommended 第二页根因未冻结。
- comments v3 当前 DTO 不匹配。
- ranking、stamps、mutation 尚无生产 adapter/SDK owner。
- server-side rating 已被 live 行为否定。

## 最终判断

允许进入：

> **一个完整、带前置收口 gate 的实施 goal。**

不允许进入：

> **绕过验证、一次性强行公开全部候选能力。**
