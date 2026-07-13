# ADR 0006：通过 Web metadata 解析 original ugoira 资源

## Status

已采纳。

## Decision

请求 `quality=original` 时，认证会话可以查询 Pixiv Web metadata 以解析 original ugoira zip，因为 App API metadata 只提供 medium variant。这是明确的资源版本补全，不是 App API 失败后的自动 fallback。

同一界限适用于 `pkg/pixiv` detail/pages enrichment：App API 仍是主数据路径；App 失败即返回失败。资源 URL 只能作为已验证的 `ResourceRef` 进入代理/下载流程，并经 `OpenResource`/`Download` 流式处理。

## Consequences

- 调用方可请求 original 资源，不需要暗中降级质量。
- Web 路由保持范围明确、可审计。
- 资源代理保留 SSRF policy、header filtering 与流所有权边界。
