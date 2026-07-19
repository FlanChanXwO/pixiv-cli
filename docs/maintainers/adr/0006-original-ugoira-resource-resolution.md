# ADR 0006：通过 Web metadata 解析 original ugoira 资源

## Status

已采纳。

## Decision

请求 `quality=original` 时，认证会话可以查询 Pixiv Web metadata 以解析 original ugoira zip，因为 App API metadata 只提供 medium variant。这是明确的资源版本补全，不是 App API 失败后的自动 fallback。

同一界限适用于顶层 `pixiv` detail/pages enrichment：App API 仍是主数据路径；App 失败即返回失败。资源 URL 只能作为已验证的 `ResourceRef` 进入代理/下载流程，并经 `OpenResource`/`Download` 流式处理。

补全采用原子结果契约：`IllustDetail` 或 `UgoiraMetadata` 只有在其主数据与必需的 Web 补全都成功时才返回结果。Web 补全失败时返回 `nil` 与对应的 typed error，不返回 App 已取得的数据。匿名 `IllustDetail` 的 Web detail/pages 两阶段也遵循同一契约；任一阶段失败均不返回 partial result。

这一行为是已采纳的公开语义，而不是隐藏 fallback：

- App detail 的 wire model 与 mapper 可以表达并保留 `MetaPages`，但这不构成所有作品、所有时间都完整的上游保证；已有 App pages 也不会跳过明确的 Web pages 补全。
- App ugoira metadata 只提供 medium zip，无法单独满足 original 质量。
- 当前公开结果没有 enrichment completeness 或 provenance 状态。Web 登录墙等失败发生时若直接返回 App 数据，调用方无法区分完整结果与降级结果，会形成静默降级，并令 detail 与 ugoira 的完成语义不一致。

未来若要引入 partial result，必须先设计显式、稳定的 enrichment 状态或 provenance，说明 detail 与 ugoira 的一致规则，提供兼容迁移方案，并以登录墙、App 数据不完整和成功补全 fixture 覆盖新旧调用方；不得把 partial result 作为无标记的错误兜底。

## Consequences

- 调用方可请求 original 资源，不需要暗中降级质量。
- Web 路由保持范围明确、可审计。
- 登录墙、权限或网络导致的 Web 补全失败会成为用户可观察的 typed error；不会伪装成成功。
- 资源代理保留 SSRF policy、header filtering 与流所有权边界。
