---
name: pixiv-cli-mcp-tool
description: Add, change, or review a pixiv-cli Pixiv or FANBOX MCP tool, including schema, SDK routing, structured output, errors, and stdio behavior. Use for MCP tool implementation or compatibility work, not routine installed-CLI operation.
---

# Maintain pixiv-cli MCP Tools

Read root `AGENTS.md`, [pixiv-cli-develop](../pixiv-cli-develop/SKILL.md), and the relevant locale MCP reference. Find a current neighboring tool and its registration/tests; do not copy an obsolete bootstrap or application-service pattern.

## Identify ownership and contract

Pixiv tools live in `internal/mcpserver/pixiv/tools/<tool>`; FANBOX tools in the parallel FANBOX owner. Their product roots aggregate registrations. CLI MCP commands start separate stdio servers. Use existing owner-local runtime, record, output, and filter helpers, and the public SDK for product operations.

For reverse search, tool code may depend on the top-level reverse-search service contract, never provider/assembly internals. Keep provider transport and credential wiring in production assembly. Do not introduce a general service locator, shared mutable global client, or anonymous Web fallback.

Define the tool name, input validation, output schema, optional fields, cursor/record semantics, side effects, and error result before implementation. Distinguish unsupported upstream behavior from locally implementable filtering. Exposing a parameter without working semantics is not a capability.

## Implement through tests

1. Add a focused test that exercises registration/schema or the real handler boundary with synthetic SDK/HTTP fixtures; run it and observe the missing behavior.
2. Validate schema and local inputs before opening an SDK snapshot or issuing requests. Reuse typed enums and shared record/pagination contracts where they are the owner.
3. Keep `context.Context`, cancellation, account selection, resource lifetime, and operation-specific permissions intact. FANBOX sessions and Pixiv pool selection remain separate.
4. Emit the declared structured result; failures set `isError=true` without corrupting stdout or exposing credentials. Legitimate empty results remain successful; partial outcomes must not be mislabeled complete.
5. Run handler/schema tests, registration tests, and relevant SDK/stdio integration tests. A built MCP server is long-lived: use an existing fixture client for smoke verification instead of starting it and waiting for output indefinitely.

Update both locale MCP references and any affected CLI/SDK/product-skill documentation. Run the existing documentation tests and review the actual diff. Record live/network behavior as unverified unless an authorized real test ran; do not create a new release note or publish a tool as part of ordinary implementation.
