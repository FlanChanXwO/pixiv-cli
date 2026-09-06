# Shaft-to-live 差异表

观测日期：2026-09-05（Asia/Shanghai）。

Shaft HEAD：`4a639490286e7e0b50b796645cdff675e2ff6e1b`。
代码时间：2024-04-05。

Shaft 只提供候选信息。
Live Pixiv 响应才是最终依据。

| 能力 | Shaft 做法 | Live 验证 | 当前 pixiv-cli | 结论 |
| --- | --- | --- | --- | --- |
| Novel detail | 使用 v2，并保留 series 导航 | v2 成功；v1 失败 | 仍使用 v1；series 被丢弃 | 迁移到 v2 |
| Novel series | 使用 `/v2/novel/series`；保存完整 `next_url` | v2 required 结构可用 | 仍使用 v1；v2 生产路径未接入 | 迁移并验证 |
| Novel new | 使用通用 `next_url` | 续页是 `max_novel_id` | 仍按 `offset` | 当前翻页受损 |
| Comments | 插画、小说均使用 v3；时间用 `date` | live comments 使用 `date` 和数字 access control | 使用旧 DTO；小说仍为 v2 | DTO 和路径需迁移 |
| Recommended | 直接保存并请求 `next_url` | 插画首页成功；第二页失败 | 只保存 offset | 保存受控完整 continuation |
| Comment add/reply | 共用 add path；reply 增加 parent ID | 真实写入 evidence 一致 | 未接入生产 adapter/SDK | 可作后续候选 |
| Comment delete | Shaft 未实现 | live evidence 成功 | 未接入生产 adapter/SDK | 不从 Shaft 推导 |
| Stamps | 只有孤立模型 | `/v1/stamps` 只读成功 | 未接入生产 adapter/SDK | 不从 Shaft 推导 |
| Novel ranking | AppApi 有 ranking path | live 首页成功；生产层缺失 | 未公开 | Goal-3 内独立 task |

## 可借鉴

- continuation 是服务端状态。
- series metadata 与列表应一起保留。
- 评论 parent chain 可作为内部模型参考。

## 不照搬

- 不直接请求任意 raw URL。
- 必须校验 host、path、endpoint identity 和 allowlisted query。
- 不静默丢弃未知 continuation 字段。
- 不用 Shaft 旧代码替代 live 证据。
