# ADR 0003: Agent Instruction Precedent Strategy

## Status

Accepted.

## Context

The repository needs AI-agent instructions and local skills, but no single well-known repository matches this project exactly. Ant Design has a useful skills layout, GitHub MCP Server matches the MCP server shape, GitHub CLI matches Go CLI structure, gh-aw and Kubernetes show concise root instructions, and Codex shows strong architecture guardrails.

## Decision

Use external repositories as selective precedents rather than a single template. `AGENTS.md` stays the canonical, concise entrypoint; `CLAUDE.md` only references it; Copilot gets a short repository hint file; detailed collaboration rules live under `docs/maintainers/agents/`; repo-local skills are introduced only for repeated Review, Docs, and Commit workflows.

## Consequences

Future instruction changes should explain which local boundary or workflow they protect, not merely copy another repository. Ant Design is a precedent for skill layout only, while GitHub MCP Server, GitHub CLI, gh-aw/Kubernetes, and Codex inform MCP, CLI, root-guide, and architecture guardrail choices respectively.
