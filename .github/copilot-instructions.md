# pixiv-cli Copilot Instructions

Read [AGENTS.md](../AGENTS.md) for repository boundaries and task-specific routes. Its linked `.agents/skills/pixiv-cli-*/SKILL.md` files are ordinary readable instructions; no global skill installation is required.

Use `go.mod`, checked-in workflows, and the relevant local test skill for commands. Keep CLI/MCP calls on the public SDK boundary, machine stdout protocol-clean, and credentials out of suggested code and examples. Read the development skill before generating source changes and the MCP/native skill when those boundaries are involved.
