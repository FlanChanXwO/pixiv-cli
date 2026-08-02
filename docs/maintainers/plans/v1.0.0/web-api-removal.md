# v1.0.0 Pixiv App API only 与 Web API 删除

## 状态

已采纳，目标版本 v1.0.0。

本计划决策取代 [ADR 0009](../../adr/0009-public-pixiv-sdk-and-caller-adapter.md) 中匿名 Web read、Web enrichment
和 Web fallback 的决策；不改变其“公开 SDK 与调用方 adapter 分离”的其余结论。

## 背景

当前实现同时包含 App API 和 `internal/services/pixiv/webapi`。无 refresh token 且配置
`web_fallback_enabled=true` 时可以进入匿名 Web/AJAX 路径。这条路径让公开 SDK 的结果取决于
Pixiv 页面协议、反爬状态、地域网络和未显式声明的 backend，错误也容易被 fallback 掩盖。

Web API 并非技术上完全不可用。代表性项目显示了两种不同选择：

- [PixivPy](https://github.com/upbit/pixivpy/blob/4f2e9ea7fff6247d9f5bfe5a862e92c5dfe3b6dd/pixivpy3/aapi.py)
  使用 App API 作为 SDK 产品面；
- [gallery-dl](https://github.com/mikf/gallery-dl/blob/master/gallery_dl/extractor/pixiv.py) 的 Pixiv
  核心路径需要登录 App API，只对特定缺失信息使用有边界的 Web/AJAX 补充；
- [Pixiv-Shaft AppApi](https://github.com/CeuiLiSA/Pixiv-Shaft/blob/classic/app/src/main/java/ceui/lisa/http/AppApi.java)
  使用 App API，而其独立
  [WebApi](https://github.com/CeuiLiSA/Pixiv-Shaft/blob/classic/app/src/main/java/ceui/lisa/http/WebApi.java)
  以同步 Cookie 支持局部能力；
- [PixivBatchDownloader](https://github.com/xuejianxianzun/PixivBatchDownloader/blob/master/src/ts/API.ts)
  作为浏览器扩展使用同源 Cookie、CSRF 与 Web AJAX。

这些是有代表性的架构样本，不是对所有 Pixiv 应用的数量统计。它们支持的结论是：服务端 SDK/下载器
常把 App API 作为核心；运行在 Pixiv 浏览器上下文中的应用仍能合理使用 Web API。反爬趋严会增加
服务端 Cookie、challenge 与页面协议的维护成本，但不能推出“网页 API 已经没法用”。

2026-08-03 的无凭据实测中，代表性的 App detail、ranking、search、user detail 与 trending-tags
endpoint 均返回 OAuth `invalid_request`，历史 no-login endpoint 返回不存在。因此 App-only 同时意味着
auth-only；SDK 不承诺匿名读取。

## 决策

- v1 删除 `internal/services/pixiv/webapi` 的生产实现，不保留 dormant adapter 或未来占位。
- 删除 Web route、fallback 选择、公开/内部 base URL option、backend enum、相关环境变量、文档和专项
  E2E。App API 出错时直接返回规范化错误，不自动切换协议。
- `web_fallback_enabled` 只保留迁移墓碑：旧配置显式包含它时返回 `removed_setting`；
  `pixiv config unset web_fallback_enabled` 允许用户清理。墓碑不得重新驱动运行时分支。
- 保留 `internal/services/pixiv/resource`，因为它负责第一方媒体的安全流式读取，不是 Web 内容 API。
- 保留公开 `pixiv.ParseURL`，因为它是纯本地 URL 解析器，不发起 Web 请求。
- App API 为小说内容返回的官方 webview 路径可以留在 `appapi` 内部，但公开结果必须是结构化
  `NovelContent`；这不构成通用 Web backend。
- 不宣传“Web API 不可用”或“所有应用都选择 App API”。对外准确表述为：v1 的核心 Pixiv SDK
  选择 App API，以获得单一认证模型、可测试路由和明确失败语义。

## 未来重新引入的门槛

未来 minor version 只有同时满足以下条件，才可提案增加独立 Web 能力：

- 有明确、持续的产品用例，且 App API 无法提供等价能力；
- 调用方显式选择 Web package/client，并显式提供所需浏览器会话；
- 不读取浏览器或本地文件，不与 Pixiv refresh token 转换或互用；
- 不作为 App API 的自动 fallback，也不让同一 operation 随运行环境静默换 backend；
- challenge 返回 `challenge_required`，不接入自动绕过；
- 有独立模型、错误、契约测试、敏感信息测试和维护责任说明。

满足门槛表示可以新增能力，不意味着恢复已删除的 `webapi` 实现。浏览器同源能力更适合由浏览器扩展
或专用 adapter 所有，而不是塞回通用 Go SDK。

## 后果

- v1 SDK 的认证与路由只有一套，离线契约测试和错误语义更稳定。
- 匿名用户和只持有 Pixiv Web Cookie 的调用方不再能通过核心 SDK 读取内容，迁移时会得到明确错误。
- 少数 App API 缺失但 Web 可取的数据不会被静默补齐；需要等独立能力提案或由调用方自行实现 adapter。
- 删除 Web backend 减少反爬、Cookie、CSRF、页面 schema 和 challenge 变化对核心 SDK 的维护压力。
- 若未来 Web API 仍有价值，可以 additive 方式新增专用产品面，不需要破坏已冻结的 App-only Client。
