# ADR 0006：认证态以 App API 元数据提供作品和动图资源

## Status

已采纳。

> 原状态：Accepted；**v1 已删除匿名 Web 路径**（`internal/services/pixiv/webapi` 与匿名 Web/AJAX read 一并移除，App API 是唯一 Pixiv 内容路径）。本 ADR 中「匿名 Web 取得 original ZIP 才标记为 `original`」「匿名路径仍独立使用 Web detail/pages」等描述只适用于 v0；v1 契约见 `AGENTS.md`「没有匿名 Web fallback」与 `docs/maintainers/architecture.md` 的 webapi 删除说明。

## Decision

认证会话只从 App API 读取 `IllustDetail`、`IllustPages` 和 `UgoiraMetadata`。多页直接采用 App
`meta_pages`；单页从 App 的 single-page/image 字段派生一项公开 `meta_pages`。页面缺失或页数不符是明确的
上游 malformed error；该请求返回其 typed failure。

动图公开模型把“确实可下载的资源”与历史 URL 槽位分开：`download_url` 和 `download_quality` 必须成对、非空，
只指向 SDK 已验证的最佳 ZIP。App 只获得 medium 时，公开质量就是 `medium`，`zip_urls.original` 保持省略；
绝不把 medium 冒充 original。匿名 Web 取得 original ZIP 时才将其标记为 `original`。下载器只使用
`download_url`，资源 URL 仍须通过已验证的 `ResourceRef`、`OpenResource`/`Download` 流式处理。

这不是隐藏 fallback：App detail、页面或 ugoira metadata 的认证错误、网络错误和服务端错误原样成为 typed error。
匿名路径仍独立使用 Web detail/pages 或原图 ugoira metadata，且自身的多阶段读取维持原子结果契约。

## Consequences

- 认证 R18 作品不再依赖常会受登录墙影响的匿名 Web metadata，detail/pages/ugoira 的 App 失败可直接定位。
- 调用方始终能从非空下载资源对判断实际 ZIP 质量，不需要猜测 `original` 是否存在。
- Web 路由保持仅匿名白名单、范围明确且可审计。
- 资源代理保留 SSRF policy、header filtering 与流所有权边界。
