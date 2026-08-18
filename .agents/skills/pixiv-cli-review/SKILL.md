---
name: pixiv-cli-review
description: Review pixiv-cli worktree, commit range, or GitHub pull request for architectural boundaries, public CLI/MCP/SDK contracts, security, documentation obligations, tests, and workflow policy. Use for code review, PR review, pre-release review, or review of agent/workflow changes.
---

# pixiv-cli Review

以 finding-first 方式审查本仓库 worktree、commit range 或 GitHub PR。先读取
`AGENTS.md` 与本 SKILL 的 [核对清单](#核对清单)，再按受影响领域补读
`docs/zh-CN/maintainers/architecture.md`、`docs/zh-CN/maintainers/development.md`、
PR 模板和对应 locale contract。

## 审查边界

- 默认只读：不要擅自修复代码、改 PR body、提交 review、resolve thread、merge 或 push；用户明确要求后才执行对应写操作。
- 先确定范围，不把工作区中与目标无关的既有改动当作本次问题。审查 PR 时优先使用 `gh pr view`、`gh pr diff`；审查本地改动时使用 `git status --short`、`git diff --stat`、`git diff --name-status` 和目标 base 的 diff。
- 代码、构建定义和结构化配置优先使用可用 LSP 查定义、调用方、引用和诊断；删除公开符号、跨文件逻辑或测试前先确认影响范围。
- 不只看风格：优先检查架构边界、错误语义、secret/token 泄露、无依据 timeout/重试/截断/fallback、CLI/MCP/SDK 契约、发布/工作流安全和测试覆盖。
- 用户可见行为、兼容性、废弃/移除或安全变化必须同步所需文档；普通 PR 不预写最终双语版本说明。

## 流程

1. 收集并声明审查范围、base/ref、已有用户改动和相关文件。
2. 读取维护清单，按"边界 → 行为/安全风险 → CLI/MCP/SDK 契约 → 文档 → 测试/构建"顺序核对；对 workflow 变化同时运行对应 policy command。
3. 对每个可复现问题定位到具体文件和紧凑行号，说明触发条件、影响和最小修复方向；不能由仓库证据确认的内容放入 Open Questions，不猜测。
4. 运行与范围匹配的聚焦测试、`git diff --check` 和必要的 lint/vet/build；记录未运行的真实 API、跨平台或凭据环境测试及剩余风险。
5. 若没有 finding，明确写"未发现阻塞问题"，并说明仍未覆盖的风险；不要用"LGTM"掩盖未完成检查。

## 输出

按严重程度降序输出，不先写泛泛总结：

```text
Findings
- [P1] path:line 问题。触发条件。影响。建议的最小修复。

Open Questions
- 需要用户或外部证据确认的事项；没有则写 None。

Summary
审查范围；已运行/未运行的测试、policy 和构建；剩余风险。
```

优先级含义：P0 为立即阻断发布或高危安全/数据损害，P1 为应在合并前修复的高影响问题，P2 为应修复但不阻断的正确性/契约问题，P3 为低风险可维护性问题。发现问题时不要把修复直接混入审查结果；若用户要求修复，再切换到实现流程并重新验证。

## 核对清单

代码审查时先看行为风险，再看风格。输出 finding-first，按严重程度排序，并给出文件/行号。

> [!IMPORTANT]
> 边界规则以 `AGENTS.md` 与 `docs/zh-CN/maintainers/architecture.md` 为准；以下为审查时的快速核对项，发生冲突时以仓库源码与架构文档为权威。

### 边界

- `internal/cli` 的 command owner 是否只处理 Cobra、TTY、OAuth loopback、presenter 与 owner-local 窄端口；`internal/cli/root.go` 是否统一全局 flag、启动生命周期与退出码。
- 是否重新引入 composition root、`Runtime`、service locator 或 CLI/MCP constructor；MCP stdio 是否由 CLI MCP 命令启动，而不是恢复独立 `internal/mcpserver/stdio` 包。
- CLI/MCP 是否经账号服务与窄端口调用 public SDK；不得直连 `internal/services/{pixiv,fanbox}` 协议适配包。
- `internal/config/settings` 是否只维护 `config.toml` schema、defaults、effective runtime config、immutable snapshots 和 sparse writes。
- `internal/utils/*` 是否保持协议无关；Pixiv/MCP/config 协议值不要搬进 generic utils/common。

### 行为风险

> [!WARNING]
> refresh token 只允许显式、不带 `--output` 的 `pixiv auth export [UID]` 以 raw token 加换行输出，或 `pixiv auth export --all` 以 versioned secret bundle 输出；两者必须 local-only、无额外输出，且不得读取环境 token、联网、刷新、修改状态、运行 startup cleanup/automatic update。带 `--output` 的 export stdout 只能是无 secret 摘要。token 不得进入 stderr、JSON、MCP result 或错误；测试 fixture 禁止真实或可用凭据，但允许明显无效、不可认证的 synthetic canary 用于证明不会泄漏；其他命令不得打印 token。

- `auth import [REFRESH_TOKEN]` 的位置参数有 argv/shell history 泄露风险；无参 TTY 必须隐藏输入，非 TTY 按首个非空白字节区分 raw token 与严格 bundle。bundle 只经 stdin 管道或重定向离线、原子恢复，必须拒绝 token 与 proxy flag 组合；不得恢复 `--file`，也不得新增 `auth add`、`auth token`、`--token` alias 或持久认证 MCP tool。
- auth bundle 是未加密、含 secret 的 point-in-time backup，不是 live sync；rotation 后旧 bundle 与其他机器副本可能 stale。restore 写失败必须准确保留 `LocalWriteCommitOutcome`：提交前为 `not_committed`，replacement 后 durability/cleanup 失败为 `committed`，无法确认恢复结果为 `unknown`，不得伪造 rollback。
- 认证、网络、Pixiv API、文件系统、`ffmpeg` 错误要暴露真实原因，不要返回空成功。
- 不新增无依据的 timeout、截断、分页上限、重试上限、静默 fallback 或隐藏降级。
- 没有匿名 Web fallback：App API 是唯一 Pixiv 内容路径，失败直接返回规范化错误，不自动切换协议；已删除的 `web_fallback_enabled` 若仍显式存在应返回 `removed_setting`。
- MCP stdout 保留给 JSON-RPC；运行期失败必须保留 structured result 并设置 `isError=true`，不得写项目级日志。
- bootstrap 安装脚本必须固定官方 Release 来源，先验证 checksum 和暂存 binary 再替换；不得静默安装前置工具、提权、读取凭据或把初始 SHA-256 完整性检查误写成 Ed25519 来源认证。

### MCP 与 CLI

- MCP tool 名称、参数、structured output、delivery mode 或文本语义变化时，更新 `docs/en/mcp-tools.md`、`docs/zh-CN/mcp-tools.md` 和聚焦测试。
- CLI 命令、flag、输出 JSON、token 优先级、账号/config 行为变化时，更新 README 或 docs。
- 用户可见变化、兼容性影响、废弃/移除或安全影响要同步所需的 public contract。普通 PR 不预写版本说明；release-prep 要用审计结果确认双语 changelog 覆盖范围内的全部来源。

### 测试

- 代码改动需要新增或更新聚焦测试，并运行相关包测试。
- 共享行为、CLI/MCP 公开接口、下载、认证或 config 变更，优先运行 `go test ./...`。
- 文档/agent-only 改动至少运行 `git diff --check` 并检查链接。
- 真实 Pixiv e2e 是 opt-in；不能运行时说明原因和剩余风险。

> [!NOTE]
> **测试资产完成条件**（人工核对，不恢复目录/AST/allowlist scanner）：
> - 测试归属 owner：断言真实运行对象的测试放在该 owner 的目录或产品根包内；root 只保留 registry/inventory/stdio/lifecycle 聚合断言。
> - 命名：无 `taskNN_` / 无理由 `legacy_` 测试文件名与测试名；测试名描述行为而非迁移历史。
> - helper 归属：helper-only 测试文件按 owner 就近放置；只有跨 2+ 文件复用的 helper 留在共享位置；不预设 `internal/testkit`（真实跨包复用例外需证据，见 `docs/zh-CN/maintainers/development.md#测试文件布局`）。
> - same-package 例外披露：测试留在生产包内时，目录必须在 `docs/zh-CN/maintainers/development.md#测试文件布局` 登记（permanent 理由或 temporary 删除条件），禁止「迁移期」类无期限表述。
> - capability 边界：新增 CLI/MCP/SDK 入口前对照 `docs/zh-CN/maintainers/development.md#能力边界` 的 unsupported/evidence-gated 清单；无入口能力不得以 schema 占位或 mock 空结果「预留」。
