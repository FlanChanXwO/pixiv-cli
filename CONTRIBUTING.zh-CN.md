# 为 pixiv-cli 贡献

[English](CONTRIBUTING.md) | 简体中文

感谢你帮助改进 `pixiv-cli`。我们欢迎聚焦的 bug report、文档修复、测试、兼容性工作和边界清晰的功能。

## 开始之前

- 先检索已有 issue 和 pull request，避免重复提交。
- 大功能、public API 变更、新依赖、认证变更或兼容性破坏应在实施前讨论。
- 不要在 issue、fixture、commit 或 CI 日志中包含 Pixiv token、Cookie、下载作品、本地数据库、缓存、机器配置或私有 API 响应。
- 保持改动聚焦；无关清理更适合单独提交 pull request。

## 开发环境

受支持的源码构建使用：

- Go `1.26.3`；
- `CGO_ENABLED=1` 和目标平台可用的 C linker；
- 仓库中经 manifest 校验、与目标平台匹配的 Rust ugoira static library。

只有修改 ugoira encoder 或重新构建 static library 时才需要 Rust。不要在无关贡献中安装或重新生成 native artifact。Windows 构建通过 Git Bash、MSYS2 或 WSL 运行。

在仓库根目录构建和测试：

```bash
go test ./...
sh scripts/build.sh
./build/pixiv --help
```

Native library 校验、opt-in 真实 API 测试、发布门禁和平台细节见[开发流程](docs/development.md)。

## 架构边界

- `cmd/pixiv` 委托 `internal/cli`；CLI controller 把业务 use case 留在 `internal/application`。
- Production wiring 只放在 `internal/bootstrap`。
- CLI 与 MCP 的 Pixiv 能力通过 `internal/application.SDKService` 调用顶层 public `pixiv` SDK，不得直连 App/Web/OAuth/resource 协议 adapter。
- MCP 注册和 transport 适配放在 `internal/mcpserver`；stdout 只用于 JSON-RPC。
- `internal/utils/*` 保持协议无关；文件应聚焦于一个职责或少数紧密相关职责。

修改这些边界前，请阅读 [docs/architecture.md](docs/architecture.md) 与仓库 [AGENTS.md](AGENTS.md)。

## 使用测试驱动开发

代码变更采用 red-green-refactor：

1. 添加一个会因目标行为尚未实现而失败的聚焦测试。
2. 实现让它通过的最小完整变更。
3. 在不改变已验证公开行为的前提下重构。
4. 先运行聚焦测试，再运行相关回归。

可行时通过 public boundary 测试公开行为。不得把真实的认证、网络、Pixiv API、文件系统或编码失败隐藏为空成功或静默 fallback；不得增加无依据的 timeout、截断、分页上限、重试限制或隐藏降级。

真实 Pixiv Web 与已认证 App canary 均为 opt-in。未经用户明确授权，不得使用其本地账号运行；也不要把真实 token 放入可能写入 shell history 的命令行。

## 文档与兼容性

修改命令、flag、SDK API、MCP tool、配置键、环境变量、输出契约、认证流程、代理行为、下载行为或已知限制时，在同一 pull request 同步文档。

- 保持 `README.md` 与 `README.zh-CN.md` 对应。
- 保持 `docs/cli-reference.md` 与 `docs/cli-reference.zh-CN.md` 对应。
- 按文件职责更新 `docs/sdk.md`、`docs/mcp-tools.md`、`docs/development.md` 或 `docs/architecture.md`。
- 用户可感知的新增、修复、变更、废弃、移除或安全影响写入 `CHANGELOG.md` 的 `[Unreleased]`。
- CLI 命令、flag 或安全语义变化时检查 `skills/pixiv/`。

稳定规则只在一个权威文档中定义，其他位置应链接过去，避免复制大段内容。

## Pull request checklist

请求 review 前确认：

- [ ] 改动保持聚焦，并说明了用户可感知行为。
- [ ] 新增或修改代码有聚焦测试，并且测试曾先证明失败。
- [ ] `go test ./... -count=1` 通过。
- [ ] 涉及共享、认证、下载、CLI、MCP 或 SDK 行为时，`go test -race ./... -count=1` 通过。
- [ ] `go vet ./...` 通过。
- [ ] `sh scripts/build.sh` 通过。
- [ ] pre-commit 可用时，`python -m pre_commit run --all-files` 通过。
- [ ] `git diff --check` 通过。
- [ ] 需要同步的英文与简体中文文档已对应。
- [ ] 未包含凭据、下载内容、本地状态或机器相关产物。

Commit message 推荐使用 Conventional Commits，例如 `feat(cli): add account selection` 或 `docs: clarify anonymous fallback`。除非未来规范明确要求，项目不要求 CLA、DCO sign-off 或 signed commit。

## 许可证

提交贡献即表示你同意该贡献可按仓库的 [MIT License](LICENSE) 分发。
