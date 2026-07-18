---
name: pixiv-cli-mcp-tool
description: Add or change a pixiv-cli MCP tool with full sync of registration, tests, localized MCP docs, README, and CHANGELOG.
---

# pixiv-cli MCP Tool

新增或修改 MCP tool 时使用，防止漏同步测试和文档。

## 边界

- tool 注册、参数 schema、structured output 和 tool 文本在 `internal/mcpserver`；stdio runtime 由 `internal/bootstrap` 启动。
- Pixiv 能力只经 `internal/application.SDKService` 调用顶层 `pixiv` public SDK，不直连 `internal/pixiv/appapi`、`webapi`、`oauth` 或 `resource`。
- MCP stdout 保留给 JSON-RPC；日志和诊断写 stderr。
- web fallback 遵守 `AGENTS.md` 注意事项中的唯一规则；fallback 不支持的能力要报真实错误，不伪造数据。

## 变更 checklist

1. 在 `internal/mcpserver` 注册/修改 tool；认证类错误要暴露真实原因。
2. 补充或更新 `internal/mcpserver` 聚焦测试，运行 `go test ./internal/mcpserver/...`，涉及共享行为时运行 `go test ./...`。
3. 同步 `docs/en/mcp-tools.md` 与 `docs/zh-CN/mcp-tools.md`：名称、参数、返回语义和 fallback 行为。
4. 用户可见变化同步 `README.md`，并写入 `CHANGELOG.md` 的 `[Unreleased]`。
5. 新增任何 timeout、截断、条数或重试限制必须有依据，并落实代码注释 + 文档 + 测试。
