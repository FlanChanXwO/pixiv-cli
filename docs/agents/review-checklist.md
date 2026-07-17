# Review Checklist

代码审查时先看行为风险，再看风格。输出 finding-first，按严重程度排序，并给出文件/行号。

## 边界

- `internal/cli` 是否只做 Cobra、TTY、OAuth loopback 和 presenter；业务用例是否留在 `internal/application`。
- `internal/bootstrap` 是否仍是生产 wiring 的唯一入口；MCP runtime 是否仍由 bootstrap 启动。
- CLI/MCP 是否经 `internal/application.SDKService` 调用顶层 `pixiv` public SDK；不得绕到 `internal/pixiv/appapi`、`webapi`、`oauth` 或 `resource` 协议适配包。
- `internal/config` 是否只维护 `config.toml` schema、defaults、effective runtime config 和 sparse writes。
- `internal/utils/*` 是否保持协议无关；Pixiv/MCP/config 协议值不要搬进 generic utils/common。

## 行为风险

- refresh token 只允许显式 `pixiv auth token [UID]` 原样写 stdout；该命令必须离线、无额外输出，且 token 不得进入 stderr、JSON、MCP result、日志、错误或测试 fixture。其他命令不得打印 token。
- 认证、网络、Pixiv API、文件系统、`ffmpeg` 错误要暴露真实原因，不要返回空成功。
- 不新增无依据的 timeout、截断、分页上限、重试上限、静默 fallback 或隐藏降级。
- Pixiv web fallback 只在无 refresh token 且 `web_fallback_enabled=true` 时使用；App API 错误不要自动 fallback。
- MCP stdout 保留给 JSON-RPC；日志和人类可读诊断写 stderr。

## MCP 与 CLI

- MCP tool 名称、参数、structured output、delivery mode 或文本语义变化时，更新 `docs/mcp-tools.md` 和聚焦测试。
- CLI 命令、flag、输出 JSON、token 优先级、账号/config 行为变化时，更新 README 或 docs。
- 用户可见变化、兼容性影响、废弃/移除或安全影响要写入 `CHANGELOG.md` 的 `[Unreleased]`。

## 测试

- 代码改动需要新增或更新聚焦测试，并运行相关包测试。
- 共享行为、CLI/MCP 公开接口、下载、认证或 config 变更，优先运行 `go test ./...`。
- 文档/agent-only 改动至少运行 `git diff --check` 并检查链接。
- 真实 Pixiv e2e 是 opt-in；不能运行时说明原因和剩余风险。
