# 分页验证报告

观测日期：2026-09-05（Asia/Shanghai）。

## 已确认两页

- novel-follow：offset，跨页无重复。
- novel-recommended：offset，跨页无重复。
- artwork search：四种 content type，offset，跨页无重复。
- artwork latest：max_illust_id，跨页无重复。
- artwork ranking：offset，跨页无重复。
- ugoira metadata：无分页。

## 已确认 continuation，但当前实现不兼容

### novel-new

- Live continuation：`max_novel_id`。
- 当前 adapter/SDK：`offset`。
- 结论：当前续页 rejected。

## continuation 风险

### illust-recommended

- 首页返回 continuation。
- 当前 adapter 只保留 offset。
- 其他服务端参数会丢失。
- 历史第二页失败。
- 本轮未能取得第二页失败的唯一根因。
- 结论：inconclusive。

### novel-series-v2

- v2 path、`series_id`、`last_order` 已由 Shaft 和 live evidence 支持。
- 当前生产 path 仍是 v1。
- 生产 adapter/SDK v2 两页尚未冻结。
- 结论：inconclusive。

## 数据受限例外

以下不强制第二页：

- user novels。
- user artworks。
- public/private bookmarks。

这些 case 只需接口、参数、adapter、SDK 成功。

## 终止规则

禁止把“有 next_url”当作第二页成功。
第二页必须有 required fields。
若继续请求失败，保留真实 failure class。


## 原始计划待补分页

- novel search period：需要第一页和第二页日期范围一致。
- artwork recommended subtype：每种 subtype 都要验证第二页。
- artwork latest subtype expansion：每种 subtype 都要验证 `max_illust_id`。
- bookmark subtype client filter：必须验证 logical pagination。
- comments total：需要非空目标和真实 continuation。
