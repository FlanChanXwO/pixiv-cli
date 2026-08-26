# Plan — Docker as a first-class pixiv-cli release target

## 1. 需求概述

为 `pixiv-cli` 添加官方 Docker 容器支持，使其成为一等公民分发目标。容器构建必须从与现有原生生产构建相同的不可变 release tag 出发，在共享质量门禁之后与原生生产构建路径并行运行，产出原生 `linux/amd64` 和 `linux/arm64` 镜像，并在 tagged release 工作流中发布多架构镜像到 GHCR。

## 2. 仓库现状与约束

### 2.1 既有 release 流程

- Release 源是不可变 `v*` tag，release policy 是 fail-closed（门禁不过则不放行）。
- 生产构建需要 `CGO_ENABLED=1` 和已提交的 Rust ugoira 静态库。
- Linux release 兼容性明确绑定 glibc 2.35 和原生 `ubuntu-22.04` / `ubuntu-22.04-arm` runner。
- 用户状态位于 `~/.pixiv-cli`；下载已支持显式输出路径。
- 现有 `scripts/internal/releaseworkflow` 包含 release policy 级别校验和测试。
- `.github/workflows/release.yml` 是现有 release 工作流。
- 仓库有 `scripts/test-package-release.sh` 用于包/release 回归验证。
- 仓库有 `scripts/tests/documentation` 文档测试。
- 仓库有 `scripts/cmd/releaseworkflow` 命令行工具用于校验工作流。

### 2.2 架构边界

- `cmd/pixiv` 只委托 `internal/cli`；`internal/cli` 是 thin controller，业务用例在 `internal/application`，生产组装只在 `internal/bootstrap`。
- CLI/MCP 的 Pixiv 能力只经 `internal/application.SDKService` 调用顶层 `pixiv` public SDK。
- MCP tool 注册和输入/输出适配在 `internal/mcpserver`；stdio runtime 由 `internal/bootstrap` 启动。
- `internal/utils/*` 保持协议无关；本地状态路径与权限常量位于 `internal/platform/localstate`。

### 2.3 文档规则

- 架构与包职责：`docs/maintainers/architecture.md`、`CONTEXT.md`、`docs/maintainers/adr/`。
- CLI 完整契约：`docs/en/cli-reference.md`、`docs/zh-CN/cli-reference.md`、`docs/ja/cli-reference.md`；README 只作为多语言入口。
- MCP tools：`docs/en/mcp-tools.md`、`docs/zh-CN/mcp-tools.md`。
- 开发流程、配置、测试：`docs/maintainers/development.md`。
- 文档路由规则见 `.agents/skills/pixiv-cli-docs/SKILL.md`。
- 修改 CLI/MCP tool、配置键、环境变量、输出语义、下载/认证/代理流程时，同步更新现有 locale 的 README、CLI reference 或对应 `docs/<locale>/`。

### 2.4 测试与验证规则

- 代码改动一律测试先行，走 TDD Red → Green → Refactor。
- 优先选择能证明目标行为的最小相关检查。
- 功能 PR 以 `.github/PULL_REQUEST_TEMPLATE.md` 的 release-note 声明记录用户可感知的变更。
- `changelog/unreleased/` 保留为 release-prep 入口；纯内部重构、测试和文档清理可选择 `None` 并说明理由。
- 代码改动必须补充或更新聚焦测试，并运行相关回归。

## 3. 执行方案

采用 TDD + 分阶段验证的方式，共 5 个阶段：

### Phase 1: 编码 release contract（C1）

- 扩展 `scripts/internal/releaseworkflow` 的 release policy 测试，先写 Red 测试断言容器 job 拓扑、不可变 tag 源、原生 Linux runner 映射、最小权限 registry 边界等。
- 然后扩展 policy 规则使测试转 Green。
- 重构仅在有稳定重复时做。
- 运行 `go test ./scripts/internal/releaseworkflow -count=1` 和 `go run ./scripts/cmd/releaseworkflow --workflow .github/workflows/release.yml` 验证。

### Phase 2: 构建容器运行时 contract（C2）

- 先写 `scripts/tests/containerrelease` 测试定义 Dockerfile/package contract（不可变 base digest、非 Alpine/musl/scratch、非 root 用户、预期 HOME、/work、ENTRYPOINT、OCI 元数据、无嵌入 secret/state）。
- 添加最小 `Dockerfile`，拷贝预构建版本化 `pixiv` 二进制到 pinned Debian slim runtime，安装运行时必要材料（如 CA 证书），创建专用非 root 用户，设置 `HOME=/home/pixiv`、`WORKDIR /work`、`ENTRYPOINT ["/usr/local/bin/pixiv"]`。
- 添加 `.dockerignore`。
- 原生 Linux 上构建镜像并运行 smoke 断言（`id -u != 0`、`pixiv --version`、`pixiv config path`、`/work`）。

### Phase 3: 集成原生多架构构建和 GHCR 发布（C3）

- 先写 release graph 测试（`build_container` 在共享质量门禁后与 `build_production` 并行；`publish_container` 是唯一有 `packages: write` 的 job；exact-version tag 始终发布；`latest` 仅 stable）。
- 添加两目标 `build_container` matrix（原生 `ubuntu-22.04` 和 `ubuntu-22.04-arm`），不登录 registry、不持有 `packages: write`。
- 确保 DAG 让 `build_container` 和 `build_production` 在共享质量门禁后成为兄弟节点。
- 添加 credential-free 容器 smoke 工作流。
- 添加 `publish_container`（仅 `packages: write`），用 workflow token 认证 GHCR，加载/推送两架构镜像，创建多架构 manifest，应用 OCI 标签。
- 发布 tag：每 release 发布 `vX.Y.Z`；仅 stable 时推进 `latest`。
- 发布可重跑/幂等；不隐藏 registry 错误；文档说明恢复边界。

### Phase 4: 文档化支持的 Docker UX（C4）

- 先写可锁定的文档 fixture/测试（如有稳定链接/命令 contract）。
- 更新 `README.md`（英文入口），再同步 `README.zh-CN.md`。
- 覆盖 exact-version 和 `latest` pull 语义、持久 `~/.pixiv-cli` volume、`/work` 下载 bind mount、镜像架构支持。
- Auth 文档：推荐 stdin-based `pixiv auth import` + 持久状态 volume。
- MCP 文档：`docker run --rm -i ... mcp` stdio 模式。
- 升级文档：容器通过 pull/redeploy 升级。
- 维护者文档：更新 `docs/en/maintainers/development.md` 和 `docs/zh-CN/maintainers/development.md`。
- 运行 `go test ./scripts/tests/documentation -count=1` 和 `git diff --check`。

### Phase 5: 集成验证和审查（C5）

- 运行所有聚焦验证（release policy、container release、documentation 测试）。
- 运行 `sh scripts/test-package-release.sh` 和 `go test ./...`。
- 确认 credential-free 容器 smoke 工作流在两个原生 Linux 架构上绿色。
- 运行 `git diff --check` 确认只有本 goal 理由充分的文件变更。
- 使用仓库 review checklist 检查不可变 tag 绑定、权限、action SHA pinning、GHCR auth、stable/prerelease tag 语义、glibc/原生 runner 映射、错误行为。
- 重读每个 C1–C5 验收标准对照当前仓库状态和证据。

## 4. 风险分析

### 4.1 高风险

| 风险 | 影响 | 缓解 |
|------|------|------|
| 容器构建 job 意外获得 registry 写入权限 | 安全风险——构建 job 可直接推送未验证镜像 | policy 测试强制 build job 无 `packages: write`；publish job 唯一持权 |
| 不可变 tag 绑定被绕过 | 构建源可变，provenance 断裂 | policy 测试断言 `build_container` checkout 不可变 tag；不允许 movable ref |
| GitHub Release 与 GHCR 非原子发布 | release 成功但 push 失败导致不一致 | 文档化恢复边界；push 失败留 workflow failed；可重跑 publish 不重建 |
| `latest` tag 被预发布推进 | 用户拉到不稳定的 latest | 测试断言 `latest` 仅 stable；prerelease 只发布 exact-version |

### 4.2 中风险

| 风险 | 影响 | 缓解 |
|------|------|------|
| Dockerfile base image digest 可变 | 构建不可复现 | pinned by immutable digest；测试断言非 tag-based |
| 非 root 用户配置缺失 | 容器以 root 运行 | 测试断言 `id -u != 0`；Dockerfile 明确创建用户 |
| glibc 版本不匹配 | 运行时崩溃 | 使用 Debian slim glibc 兼容 ubuntu-22.04 构建的二进制；测试断言 glibc runtime |
| `~/.pixiv-cli` 状态路径在容器内不可持久化 | 用户数据丢失 | 文档明确 volume 挂载；测试断言 `pixiv config path` 解析到 `/home/pixiv/.pixiv-cli/` |

### 4.3 低风险

| 风险 | 影响 | 缓解 |
|------|------|------|
| 文档中英语义不一致 | 用户困惑 | 文档测试断言双语命令/路径/registry 名一致 |
| 引入不必要的第三方 GitHub Action | 供应链风险 | 全 SHA pinning；测试确认无新未审批依赖 |
| `.dockerignore` 排除过多/过少 | 构建上下文问题或泄露 | 仅排除 build-noise，不排除 packaging contract 所需文件 |

## 5. 验证方式

| 验证项 | 命令 | 覆盖标准 |
|--------|------|----------|
| Release policy | `go test ./scripts/internal/releaseworkflow -count=1` | C1 |
| Release policy 命令 | `go run ./scripts/cmd/releaseworkflow --workflow .github/workflows/release.yml` | C1 |
| Container release | `go test ./scripts/tests/containerrelease -count=1` | C2, C3 |
| 文档 | `go test ./scripts/tests/documentation -count=1` | C4 |
| 全量 Go 测试 | `go test ./...` | C5 |
| 包/release 回归 | `sh scripts/test-package-release.sh` | C5 |
| Diff 检查 | `git diff --check` | C5 |
| 容器 smoke | 原生 Docker 构建 + `pixiv --version` / `pixiv config path` / `id -u != 0` | C2, C3 |
| CI 容器 smoke 工作流 | credential-free 两架构 smoke workflow | C3 |

## 6. 回滚方案

- 所有变更在单独分支 `goal/docker-container-release` 上进行，不影响 `main`。
- 如果某个 phase 验证失败，可通过 `git revert` 或 `git reset` 回退到该 phase 开始前的 commit。
- 如果整体需要回滚，删除该分支或在 main 上不合并即可。
- Dockerfile、`.dockerignore`、新增 workflow 文件、新增测试文件都是新增文件，删除即可回滚。
- 对 `scripts/internal/releaseworkflow` 和 `release.yml` 的修改可通过 `git checkout main -- <file>` 恢复。
- 回滚后不影响现有原生 release 流程，因为容器支持是增量添加的。

## 7. 默认假设

以下假设基于现有仓库状态和需求文档，后续会话不能再回头问用户：

1. **Registry 路径**：`ghcr.io/flanchanxwo/pixiv-cli`（小写），基于 GitHub 仓库 owner `FlanChanXwO`。
2. **Runtime base**：Debian slim（glibc-based），pinned by immutable digest，不使用 tag。
3. **非 root 用户名**：`pixiv`，HOME=/home/pixiv。
4. **工作目录**：`/work`，用于下载/文件操作 bind mount。
5. **ENTRYPOINT**：`["/usr/local/bin/pixiv"]`，不使用 wrapper script。
6. **构建方式**：先构建版本化 Linux 二进制（复用现有 build_production 路径），再 COPY 进 Dockerfile（多阶段构建或预构建后拷贝）。
7. **MCP 模式**：`docker run --rm -i ghcr.io/flanchanxwo/pixiv-cli mcp`，stdout 属于 JSON-RPC。
8. **Auth 方式**：推荐 `echo '<token>' | docker run --rm -i ... auth import`，利用持久 volume。
9. **镜像标签**：`vX.Y.Z`（每 release）、`latest`（仅 stable）。
10. **OCI labels**：source、revision、version、license 等 provenance 标签。
11. **此 PR 不写 changelog**：release notes 是 release-preparation 工作，按仓库策略此实现 PR 选 `None` 并说明理由。
12. **不修改 `pixiv update`**：容器通过 pull 升级，此 goal 只文档说明。
13. **Phase 1 先行**：release policy 测试先于任何工作流/Dockerfile 实现。
14. **原生 runner**：`ubuntu-22.04`（amd64）和 `ubuntu-22.04-arm`（arm64），不使用 QEMU。
15. **action pinning**：使用 full-SHA pinning，遵循仓库现有约定。
16. **旧格式文件处理**：保留 `goals/docker-container-release/` 目录作为历史记录，新 goal-mode 文件放在 `goal-1/` 目录，两者不冲突。
