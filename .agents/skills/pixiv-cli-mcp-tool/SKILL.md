---
name: pixiv-cli-mcp-tool
description: Add, change, or review a pixiv-cli Pixiv or FANBOX MCP tool, including schema, SDK routing, structured output, errors, stdio delivery and HTTP authorization boundaries. Use for MCP tool implementation or compatibility work, not routine installed-CLI operation.
---

# Maintain pixiv-cli MCP Tools

Read root `AGENTS.md`, [pixiv-cli-develop](../pixiv-cli-develop/SKILL.md), and the relevant locale MCP reference. Find a current neighboring tool and its registration/tests; do not copy an obsolete bootstrap or application-service pattern. `register.go` is the only registration entry; domain files (`accounts.go`, `media.go`, `bookmarks.go`, …) hold the tool definitions and `<tool>Handler` methods on the product tool set.

## Identify ownership and contract

Pixiv tools live in the single package `internal/mcpserver/pixiv/tools`; FANBOX tools in the parallel `internal/mcpserver/fanbox/tools`. Their product roots register into one server constructed by `internal/mcpserver.New`; product runtimes remain independent. Pixiv tool names use `pixiv_`, FANBOX uses `fanbox_`, and old unprefixed aliases are not registered. `pixiv mcp` defaults to stdio, with local filesystem downloads injected only for that transport; explicit `--port` enables HTTP with OAuth/bearer checks. Register Gallery and owner operations only when their media/account/login ports exist. Use existing owner-local runtime, record, output, and filter helpers, and the public SDK for product operations.

For reverse search, tool code may depend on the top-level reverse-search service contract, never provider/assembly internals. Keep provider transport and credential wiring in production assembly. Do not introduce a general service locator, shared mutable global client, or anonymous Web fallback.

Define the tool name, input validation, output schema, optional fields, cursor/record semantics, side effects, standard tool annotations, and error result before implementation. Distinguish unsupported upstream behavior from locally implementable filtering. Exposing a parameter without working semantics is not a capability.

## Implement through tests

1. Apply the [coding handbook's test submission rules](../../../docs/en/maintainers/coding-standards.md#test-submission-rules) at a registration/schema or real-handler boundary, using synthetic SDK/HTTP fixtures. Add coverage for a missing contract; use [the test workflow](../pixiv-cli-test/SKILL.md) for execution and regression evidence.
2. Validate schema and local inputs before opening an SDK snapshot or issuing requests. Reuse typed enums and shared record/pagination contracts where they are the owner.
3. Keep `context.Context`, cancellation, account selection, resource lifetime, and operation-specific permissions intact. FANBOX sessions and Pixiv pool selection remain separate.
4. Emit the declared structured result; failures set `isError=true` without exposing credentials. HTTP authentication failures remain HTTP errors, not tool results. Legitimate empty results remain successful; partial outcomes must not be mislabeled complete.
5. Run handler/schema tests, registration tests, and relevant SDK/stdio/HTTP integration tests. A built MCP server is long-lived: use an existing fixture client for smoke verification instead of starting it and waiting for output indefinitely.

Update both locale MCP references and any affected CLI/SDK/product-skill documentation. Run the existing documentation tests and review the actual diff. Record live/network behavior as unverified unless an authorized real test ran; do not create a new release note or publish a tool as part of ordinary implementation.
