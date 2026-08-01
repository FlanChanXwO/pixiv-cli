# 未发布

> 此处是 release-prep 暂存区。功能 PR 在模板中提供分类和摘要；经审核的 release-prep plan 会将这些来源归并到下一个版本说明。

## 破坏性变更

- 移除独立的 `pixiv filter` 命令；视觉列表和 `pixiv download` 直接使用 `--filter EXPR`。
- CLI 的 `--ugoira-format` 改为 `--ugoira-mode gif|apng|zip|frames`。

## 新增

- 新增安全的插画表达式筛选、管道视觉列表自动 NDJSON、下载归档、元数据 sidecar、目录模板、重试控制、请求间隔、Ugoira ZIP/frames 输出、开区间页选择、公开书签/插画系列下载来源，以及 SOCKS 代理 URI。
