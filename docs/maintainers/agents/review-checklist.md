# Review Checklist

代码审查时先看行为风险，再看风格。输出 finding-first，按严重程度排序，并给出文件/行号。

## 边界

- `internal/cli` 是否只做 Cobra、TTY、OAuth loopback 和 presenter；业务用例是否留在 `internal/application`。
- `internal/bootstrap` 是否仍是生产 wiring 的唯一入口；MCP runtime 是否仍由 bootstrap 启动。
- CLI/MCP 是否经 `internal/application.SDKService` 调用顶层 `pixiv` public SDK；不得绕到 `internal/services/pixiv/appapi`、`webapi`、`oauth` 或 `resource` 协议适配包。
- `internal/application/config` 是否只维护 `config.toml` schema、defaults、effective runtime config 和 sparse writes。
- `internal/utils/*` 是否保持协议无关；Pixiv/MCP/config 协议值不要搬进 generic utils/common。

## 行为风险

- refresh token 只允许显式、不带 `--output` 的 `pixiv auth export [UID]` 以 raw token 加换行输出，或 `pixiv auth export --all` 以 versioned secret bundle 输出；两者必须 local-only、无额外输出，且不得读取环境 token、联网、刷新、修改状态、运行 startup cleanup/automatic update。带 `--output` 的 export stdout 只能是无 secret 摘要。token 不得进入 stderr、JSON、MCP result 或错误；测试 fixture 禁止真实或可用凭据，但允许明显无效、不可认证的 synthetic canary 用于证明不会泄漏；其他命令不得打印 token。
- `auth import [REFRESH_TOKEN]` 的位置参数有 argv/shell history 泄露风险；无参 TTY 必须隐藏输入，非 TTY 才读取 raw stdin。`auth import --file PATH|-` 是离线、原子 bundle restore，必须拒绝 token 与 proxy flag 组合；不得新增 `auth add`、`auth token`、`--token` alias，也不得新增持久认证 MCP tool。
- auth bundle 是未加密、含 secret 的 point-in-time backup，不是 live sync；rotation 后旧 bundle 与其他机器副本可能 stale。restore 写失败必须准确保留 `LocalWriteCommitOutcome`：提交前为 `not_committed`，replacement 后 durability/cleanup 失败为 `committed`，无法确认恢复结果为 `unknown`，不得伪造 rollback。
- 认证、网络、Pixiv API、文件系统、`ffmpeg` 错误要暴露真实原因，不要返回空成功。
- 不新增无依据的 timeout、截断、分页上限、重试上限、静默 fallback 或隐藏降级。
- Pixiv web fallback 只在无 refresh token 且 `web_fallback_enabled=true` 时使用；App API 错误不要自动 fallback。
- MCP stdout 保留给 JSON-RPC；运行期失败必须保留 structured result 并设置 `isError=true`，不得写项目级日志。
- bootstrap 安装脚本必须固定官方 Release 来源，先验证 checksum 和暂存 binary 再替换；不得静默安装前置工具、提权、读取凭据或把初始 SHA-256 完整性检查误写成 Ed25519 来源认证。

## MCP 与 CLI

- MCP tool 名称、参数、structured output、delivery mode 或文本语义变化时，更新 `docs/en/mcp-tools.md`、`docs/zh-CN/mcp-tools.md` 和聚焦测试。
- CLI 命令、flag、输出 JSON、token 优先级、账号/config 行为变化时，更新 README 或 docs。
- 用户可见变化、兼容性影响、废弃/移除或安全影响要同步写入 `changelog/unreleased/en.md` 与 `changelog/unreleased/zh-CN.md`。

## 测试

- 代码改动需要新增或更新聚焦测试，并运行相关包测试。
- 共享行为、CLI/MCP 公开接口、下载、认证或 config 变更，优先运行 `go test ./...`。
- 文档/agent-only 改动至少运行 `git diff --check` 并检查链接。
- 真实 Pixiv e2e 是 opt-in；不能运行时说明原因和剩余风险。
