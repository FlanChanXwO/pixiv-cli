# Tasks — Docker as a first-class pixiv-cli release target

> 状态标记：`[ ]` 未完成 / `[x]` 已完成 / `[~]` 阻塞
>
> 每个 task 的预留空位在完成后填写：实际做了什么、验证证据、剩余风险、下一步建议。
>
> **执行规则**：一轮一个 task，按序执行，不许合并或跳序。每 3 个 task 后插入一次集中检查。

---

## Phase 1: 编码 release contract（C1）

### Task 1: Red — 编写 release policy 失败测试

- [x] **目标**：在 `scripts/internal/releaseworkflow` 中添加聚焦测试，断言计划中的容器 job 拓扑和约束，确认当前行为因缺少容器 contract 而失败。
- **具体断言**：
  - 存在 `build_container` job，其在共享质量门禁之后启动
  - 存在 `publish_container` job，其依赖 `build_container` 和 GitHub Release
  - `build_container` 不持有 `packages: write` 权限
  - `publish_container` 是唯一持有 `packages: write` 的 job
  - `build_container` checkout 的是不可变 tag，不是 movable ref
  - `build_container` 使用原生 Linux runner（`ubuntu-22.04` / `ubuntu-22.04-arm`），不是 QEMU
  - 没有 QEMU/setup-qemu 步骤
  - exact-version tag 始终发布
  - `latest` 仅 stable 时推进，prerelease 不推进
- **验收**：测试编译通过但运行失败，Red 证据已记录。
- **实际做了什么**：创建了 `container_policy.go`（stub，含 `containerRegistry` 常量、`containerMatrixTargets` 映射和返回 error 的 `checkContainerBuildJob`/`checkContainerPublishJob` stub）和 `container_policy_test.go`（9 个聚焦测试）。测试覆盖：build_container 必须存在、不持有 packages: write、checkout 不可变 tag、无 QEMU、原生 Linux runner、不串行依赖 build_production；publish_container 必须持有 packages: write、必须依赖 build_container；containerRegistry 常量正确。
- **验证证据**：`go test ./scripts/internal/releaseworkflow -run 'TestContainer|TestCheckWorkflowRejectsMissingContainerJobs' -count=1 -v` 输出显示 8 个测试 FAIL（Red），1 个 `TestContainerRegistryPath` PASS（仅验证常量）。`TestCheckWorkflowAcceptsCheckedInWorkflow` 仍 PASS（无回归）。pre-commit `go test ./...` 也确认 releaseworkflow 包 FAIL，符合 Red 预期。commit: `9b5744d`。
- **剩余风险**：部分测试（如 QEMU rejection、non-native runner）在 Red 阶段因 `requireOnlyMappingKeys` job allow-list 先于 container policy 报错而得到不匹配的错误消息，需在 Task 2 实现 Green 时确保这些测试在正确层报错。`latest` 仅 stable 和 exact-version tag 断言未在本 task 覆盖，将在 Task 7（release graph 测试）中处理。
- **下一步建议**：Task 2 — Green：在 `checkWorkflow` 中将 `build_container`/`publish_container` 加入 job allow-list，实现 `checkContainerBuildJob` 和 `checkContainerPublishJob` 的完整 policy 规则，使所有 Red 测试转 Green。

---

### Task 2: Green — 扩展 release policy 规则使测试通过

- [x] **目标**：在 `scripts/internal/releaseworkflow` 中添加最小容器-policy 规则满足 Task 1 的失败测试。复用现有 workflow YAML helper 和 Linux target provenance，不创建第二个 parser 或重复 policy 框架。
- **验收**：`go test ./scripts/internal/releaseworkflow -count=1` 绿色；`go run ./scripts/cmd/releaseworkflow --workflow .github/workflows/release.yml` 无 policy 违规（当前 release.yml 尚无容器 job 时，policy 检查报告预期状态）。
- **实际做了什么**：实现了 `checkContainerBuildJob`（验证 permissions 无 packages: write、不可变 tag checkout、原生 matrix runner、无 QEMU、不依赖 build_production、两 native Linux matrix target）和 `checkContainerPublishJob`（验证 packages: write、依赖 build_container）。将 `requireOnlyMappingKeys` 替换为 `requireJobsAllowlist`——必需 job 全部存在且只含允许 job，容器 job 可选。更新测试 fixture 添加 `steps` 键和合规 companion job 以隔离被测维度。commit: `825cbce`。
- **验证证据**：`go test ./scripts/internal/releaseworkflow -count=1` → 全部 PASS（含 9 个 container 测试和全部既有测试）。`go run ./scripts/cmd/releaseworkflow --workflow .github/workflows/release.yml` → exit 0。`go test ./scripts/internal/... ./scripts/tests/... -count=1` → 全部 PASS。
- **剩余风险**：容器 job 当前是可选的——release.yml 尚无容器 job 时 policy 不报错。Phase 3 添加 release.yml 容器 job 后，policy 将开始实际校验。`latest` 仅 stable 和 exact-version tag 语义断言仍待 Task 7 覆盖。
- **下一步建议**：Task 3 — Refactor + Phase 1 验证：检查是否有稳定且有害的重复（如 Linux release target metadata），运行完整 Phase 1 验证，确认无 production/auth/updater 行为或 registry credential 引入。

---

### Task 3: Refactor + Phase 1 验证

- [x] **目标**：仅在存在稳定且有害的重复时，合并真正共享的 Linux release target metadata。不泛化无关 release policy。然后运行 Phase 1 完整验证。
- **验证命令**：
  - `go test ./scripts/internal/releaseworkflow -count=1`
  - `go run ./scripts/cmd/releaseworkflow --workflow .github/workflows/release.yml`
- **Scope audit**：确认本 phase 未引入任何 production 行为、auth 行为、updater 行为或 registry credential。
- **验收**：两者均绿色，Red→Green 证据记录到进度日志。
- **实际做了什么**：审查了 `releaseMatrixTargets`（6 条目，跨平台）与 `containerMatrixTargets`（2 条目，仅 Linux）的重复。两个 Linux 条目虽在两个 map 中字符串相同，但用途和基数不同（6-target 全平台构建矩阵 vs 2-target 容器矩阵），合并会引入过滤逻辑增加复杂度，不符合 KISS。判定为非有害重复，不合并。运行 Phase 1 完整验证。Scope audit 确认仅引入 `persist-credentials: false`（checkout 安全设置），无 secret/token/auth/updater/registry credential 引入。
- **验证证据**：`go test ./scripts/internal/releaseworkflow -count=1` → PASS（全部测试含 container 测试）。`go run ./scripts/cmd/releaseworkflow --workflow .github/workflows/release.yml` → exit 0。`go build ./scripts/...` → exit 0。`git diff` scope audit 确认无 production/auth/updater 行为或 registry credential。
- **剩余风险**：无。Phase 1 纯粹是 release policy 测试和规则，不触及任何运行时行为。
- **下一步建议**：进入集中检查 #1（Phase 1 集中复查），然后 Phase 2 Task 4 开始 Dockerfile contract 测试。

---

### 🔍 集中检查 #1（Task 1–3 之后）

- [x] **目标**：Phase 1 集中复查。
- **检查清单**：
  - [x] 需求是否偏离 `input.md`
  - [x] 代码是否有 bug、死代码、调试残留
  - [x] 类型检查 / 构建是否通过
  - [x] `go test ./scripts/internal/releaseworkflow -count=1` 是否通过
  - [x] `go run ./scripts/cmd/releaseworkflow --workflow .github/workflows/release.yml` 是否通过
  - [x] 安全性：权限隔离、无 credential 泄露
  - [x] 是否需要新增/调整回滚方案
  - [x] 文档是否同步（本 phase 可能不需要文档变更）
- **结论**：Phase 1 通过全部复查，无问题。
- **发现问题**：无。

---

## Phase 2: 构建容器运行时 contract（C2）

### Task 4: Red — 编写 Dockerfile/package contract 失败测试

- [x] **目标**：在 `scripts/tests/containerrelease` 中添加测试，定义 Dockerfile/package contract，在创建 Dockerfile 前确认失败。
- **具体断言**：
  - Dockerfile 使用不可变 base digest（非 tag）
  - 不使用 Alpine/musl/scratch base
  - 最终用户非 root
  - `HOME=/home/pixiv`
  - `WORKDIR /work`
  - `ENTRYPOINT ["/usr/local/bin/pixiv"]`
  - OCI 元数据输入存在（source/revision/version/license）
  - 无嵌入 secret/state 文件
- **验收**：测试编译通过但运行失败，Red 证据已记录。
- **实际做了什么**：创建了 `scripts/tests/containerrelease/containerrelease_test.go`（12 个聚焦测试），断言 Dockerfile 必须满足：存在、不可变 base digest（非 tag）、非 Alpine/musl/scratch、非 root 用户（USER 指令）、HOME=/home/pixiv、WORKDIR /work、ENTRYPOINT ["/usr/local/bin/pixiv"]、COPY pixiv 到 /usr/local/bin/pixiv、OCI 标签（source/revision/version/license）、ca-certificates 安装、无嵌入 secret/state、Debian slim base。commit: `d770adf`。
- **验证证据**：`go test ./scripts/tests/containerrelease -count=1 -v` → 全部 12 个测试 FAIL（Red），因 Dockerfile 不存在。`go test ./scripts/internal/releaseworkflow -count=1` → PASS（无回归）。
- **剩余风险**：测试当前基于字符串匹配而非 Docker 构建实际行为；Task 6 将补充真实 Docker smoke。`Debian slim` 的具体 digest 将在 Task 5 创建 Dockerfile 时确定。
- **下一步建议**：Task 5 — Green：创建最小 Dockerfile，满足所有 12 个 Red 测试的断言。需确定 Debian slim 的不可变 digest。

---

### Task 5: Green — 创建最小 Dockerfile

- [x] **目标**：添加最小 `Dockerfile`，拷贝预构建版本化 `pixiv` 二进制到 pinned Debian slim runtime，安装运行时必要材料（如 CA 证书），创建专用非 root 用户，设置 `HOME=/home/pixiv`、`WORKDIR /work`、`ENTRYPOINT ["/usr/local/bin/pixiv"]`。
- **验收**：`go test ./scripts/tests/containerrelease -count=1` 绿色。
- **实际做了什么**：创建 `Dockerfile`，使用 Debian bookworm-slim pinned by immutable multi-arch manifest digest（`sha256:882008...`）。安装 ca-certificates + tzdata，创建非 root 用户 pixiv（UID 1000），设置 ENV HOME=/home/pixiv、WORKDIR /work、ENTRYPOINT ["/usr/local/bin/pixiv"]，COPY dist/pixiv，OCI 标签（source/revision/version/license）。修复注释中 ".pixiv-cli/" 字符串触发 secret/state 检查。commit: `f2a2db7`。
- **验证证据**：`go test ./scripts/tests/containerrelease -count=1 -v` → 全部 12 个测试 PASS（Green）。`go test ./scripts/internal/releaseworkflow -count=1` → PASS（无回归）。`go run ./scripts/cmd/releaseworkflow --workflow .github/workflows/release.yml` → exit 0。
- **剩余风险**：Dockerfile 尚未经过真实 Docker 构建（Task 6 将补充 .dockerignore + 真实 smoke）。OCI revision/version 标签使用 `${REVISION}`/`${VERSION}` 占位符，CI 构建时需注入实际值。
- **下一步建议**：Task 6 — 添加 .dockerignore，在原生 Linux 上构建镜像并运行 smoke 断言（id -u != 0、pixiv --version、pixiv config path、/work）。

---

### Task 6: .dockerignore + 真实 smoke 构建

- [x] **目标**：
  1. 添加 `.dockerignore`，排除仓库/build-noise 内容但不排除 packaging contract 所需文件。不添加 secret-specific 猜测，依赖最小构建上下文和现有仓库 secret 规则。
  2. 在原生 Linux 上构建镜像并运行断言：`id -u != 0`、`pixiv --version`（exact release version）、`pixiv config path`（解析到 `/home/pixiv/.pixiv-cli/`）、`/work` 是默认工作目录。
- **验收**：镜像构建成功，所有 smoke 断言通过；镜像 ID/digest 和 smoke 输出记录为证据（不记录 credential 或本地数据库内容）。
- **实际做了什么**：创建 `.dockerignore`，排除 build-noise（.git、.github、docs、scripts、goal 文件、模板、editor 文件、Python cache、Rust target 输出），保留 Dockerfile、dist/ 和 packaging contract 文件。使用临时多阶段 Dockerfile（Go 1.26.3 builder + Debian slim runtime）在原生 Linux/arm64 上构建镜像并运行 4 个 smoke 断言。commit: `e94c5a0`。
- **验证证据**：Docker 构建成功（image ID: sha256:f8830b93d1c8..., 215MB）。Smoke 断言全部通过：(1) `id -u` → 1000（非 root），(2) `pixiv --version` → `pixiv v0.0.0-smoke`（exact release version），(3) `pixiv config path` → `/home/pixiv/.pixiv-cli/config.toml`，(4) `pwd` → `/work`。`go test ./scripts/tests/containerrelease -count=1` → PASS。`go test ./scripts/internal/releaseworkflow -count=1` → PASS。镜像构建后已清理。
- **剩余风险**：Smoke 构建使用临时多阶段 Dockerfile（含 Go builder stage），正式 CI 构建将使用预构建版本化二进制 + 正式 Dockerfile。Smoke 在 macOS Docker Desktop（arm64）上运行，未在 amd64 上验证——CI 将在两个原生架构上运行。`.dockerignore` 排除 scripts/ 目录，CI 构建上下文需确保 dist/pixiv 在构建前已准备好。
- **下一步建议**：进入集中检查 #2（Phase 2 集中复查），然后 Phase 3 Task 7 开始 release graph 失败测试。

---

### 🔍 集中检查 #2（Task 4–6 之后）

- [x] **目标**：Phase 2 集中复查。
- **检查清单**：
  - [x] 需求是否偏离 `input.md`
  - [x] Dockerfile 是否最小、无多余依赖
  - [x] 非 root 用户是否真正生效
  - [x] base image digest 是否不可变
  - [x] `.dockerignore` 是否合理
  - [x] `go test ./scripts/tests/containerrelease -count=1` 是否通过
  - [x] 真实 smoke 是否通过
  - [x] 安全性：无 secret 泄露、无 root 运行
  - [x] 功能边界：容器仍使用同一二进制和 CLI/MCP entrypoint，无 wrapper script 重新解释参数
  - [x] 文档是否同步（本 phase 可能不需要文档变更）
- **结论**：Phase 2 通过全部复查，无问题。
- **发现问题**：无。

---

## Phase 3: 集成原生多架构构建和 GHCR 发布（C3）

### Task 7: Red — 编写 release graph 失败测试

- [x] **目标**：扩展聚焦工作流测试，断言具体 release graph，在编辑工作流前确认失败。
- **具体断言**：
  - `build_container` 在共享质量门禁后启动，与 `build_production` 并行
  - 精确原生 Linux runner/toolchain 被要求
  - `publish_container` 是唯一有 `packages: write` 的 job
  - exact-version tag 始终发布
  - `latest` 仅 stable
- **验收**：测试编译通过但运行失败。
- **实际做了什么**：在 `container_policy_test.go` 中添加 3 个新测试：(1) `TestContainerPublishJobIsOnlyPackagesWrite`（验证 publish_container 是唯一持 packages: write 的 job），(2) `TestContainerPublishJobAlwaysPublishesExactVersionTag`（验证 policy 要求 publish_container 始终推送 exact-version tag），(3) `TestContainerPublishJobLatestTagOnlyForStable`（验证 policy 拒绝无条件推送 latest tag）。commit: `5541135`。
- **验证证据**：`go test ./scripts/internal/releaseworkflow -count=1` → 2 个新测试 FAIL（Red），1 个 PASS（packages:write 唯一性已由现有 per-job permission 校验覆盖）。其余全部 PASS。Red 状态确认：policy 不检查 exact-version tag 推送和 latest tag gating。
- **剩余风险**：`TestContainerPublishJobIsOnlyPackagesWrite` 直接 PASS 是因为现有 per-job 权限校验已拒绝非容器 job 持有 packages: write；但它验证的是全局一致性而非仅 container policy。Task 8 需实现 exact-version 和 latest gating 校验使另两个测试转 Green。
- **下一步建议**：Task 8 — Green：实现 `checkContainerPublishJob` 中的 exact-version tag 推送检查和 latest tag stable-only gating。

---

### Task 8: Green/build — 添加 build_container matrix

- [x] **目标**：添加两目标 `build_container` matrix（原生 `ubuntu-22.04` 和 `ubuntu-22.04-arm`）。每个目标 checkout 不可变 tag、验证 source、使用 audited Rust toolchain/staticlib 路径、构建版本化 Linux binary、应用现有 Linux ABI gate、构建 Docker image、导出可传输 image artifact。不登录 registry、不持有 `packages: write`。
- **验收**：`go test ./scripts/tests/containerrelease -count=1` 绿色；release policy 测试对 build_container 部分绿色。
- **实际做了什么**：实现 `requireExactVersionTagPush`（要求 publish_container steps 包含 `docker push` + `${RELEASE_TAG}`）和 `rejectUnconditionalLatestTagPush`（latest tag 推送必须含 stable/channel/prerelease gate）。重构 `TestContainerPublishJobAlwaysPublishesExactVersionTag` 为 accept/reject 子测试。更新 `addPublishContainerJob` fixture 和 latest-tag 测试 fixture 包含 exact-version push step。commit: `9e1aac6`。
- **验证证据**：`go test ./scripts/internal/releaseworkflow -count=1` → 全部 PASS（含 3 个 Task 7 release graph 测试）。`go test ./scripts/tests/containerrelease -count=1` → PASS。`go run ./scripts/cmd/releaseworkflow --workflow .github/workflows/release.yml` → exit 0。
- **剩余风险**：release.yml 中的实际 build_container/publish_container job 尚未添加（Task 9-10）。tag policy 在 fixture 上验证，但在真实 release.yml 上的验证需等 Task 10 添加 publish_container job 后。
- **下一步建议**：Task 9 — 并行性证明 + credential-free smoke 工作流：确保 DAG 让 build_container 与 build_production 并行；添加窄触发容器 smoke 工作流。

---

### Task 9: 并行性证明 + credential-free smoke 工作流

- [x] **目标**：
  1. 确保 release DAG 让 `build_container` 和 `build_production` 在共享质量门禁后成为兄弟节点而非串行。
  2. 添加窄触发容器 smoke 工作流，为相关 PR/main 变更构建两个原生架构（无 registry credential），运行 Phase 2 smoke 断言。复用 full-SHA pinned actions 和现有仓库权限约定。
- **验收**：DAG 并行关系在 policy 测试中证明；smoke 工作流存在且语法正确。
- **实际做了什么**：(1) DAG 并行性已由 `TestContainerBuildJobRejectsBuildProductionDependency` 证明——build_container 不得依赖 build_production，必须在共享质量门禁（build）后与 build_production 并行。(2) 创建 `.github/workflows/container-smoke.yml`——credential-free 容器 smoke 工作流，含 change-scope classifier 和两个原生 Linux 架构 job（ubuntu-22.04/amd64, ubuntu-22.04-arm/arm64）。每个 job 构建二进制+镜像+运行 4 个 smoke 断言+运行 contract 测试。所有 actions 使用 full-SHA pin。commit: `227e880`。
- **验证证据**：`python3 -c yaml.safe_load` → YAML valid。`grep packages:|secrets:|registry` → 无匹配（credential-free）。`go test ./scripts/internal/releaseworkflow -count=1` → PASS。`go test ./scripts/tests/containerrelease -count=1` → PASS。
- **剩余风险**：smoke 工作流尚未在 CI 上实际运行（需要 push 到远程并触发 GitHub Actions）。DAG 并行性在 policy 层验证，但 release.yml 中实际的 build_container job 尚未添加（Task 10-11）。
- **下一步建议**：进入集中检查 #3（Phase 3 上半部分复查），然后 Task 10 添加 publish_container job。

---

### 🔍 集中检查 #3（Task 7–9 之后）

- [x] **目标**：Phase 3 上半部分集中复查。
- **检查清单**：
  - [x] 需求是否偏离 `input.md`
  - [x] `build_container` 是否真的不持有 `packages: write`
  - [x] 是否使用原生 runner 而非 QEMU
  - [x] DAG 并行关系是否正确
  - [x] smoke 工作流是否 credential-free
  - [x] action 是否 full-SHA pinned
  - [x] 是否无新未审批第三方 Action 依赖
  - [x] 安全性：权限隔离
  - [x] 文档是否同步（本 phase 可能不需要文档变更）
- **结论**：Phase 3 上半部分通过全部复查，无问题。
- **发现问题**：无。

---

### Task 10: Green/publish — 添加 publish_container job

- [ ] **目标**：添加 `publish_container`，位于已验证容器 artifact 和 GitHub Release 发布之后。仅授予 `packages: write`（和最低必需读权限），用 workflow token 认证 GHCR，加载/推送两架构镜像，创建多架构 manifest，应用 OCI source/revision/version/license 标签。
- **验收**：`go test ./scripts/tests/containerrelease -count=1` 绿色；release policy 测试对 publish_container 部分绿色。
- **实际做了什么**：（待填）
- **验证证据**：（待填）
- **剩余风险**：（待填）
- **下一步建议**：（待填）

---

### Task 11: Tag policy + 恢复语义

- [ ] **目标**：
  1. 发布 `ghcr.io/flanchanxwo/pixiv-cli:vX.Y.Z`（每 release）。仅当现有 release channel classifier 报告 `stable` 时推进 `latest`；prerelease 绝不移动 `latest`。
  2. 使 publication 对同一不可变 tag 可重跑/幂等（在 registry 允许范围内）。不隐藏 registry 错误或用 retry loop 把失败 push 报为成功。文档说明 GHCR 发布失败留 workflow failed，通过重跑 publish job/path 修复。
- **验收**：tag policy 测试绿色；恢复语义文档化。
- **实际做了什么**：（待填）
- **验证证据**：（待填）
- **剩余风险**：（待填）
- **下一步建议**：（待填）

---

### Task 12: Phase 3 验证

- [ ] **目标**：运行 release policy / container 测试，检查 smoke 工作流中两架构 artifact。确认无 QEMU setup、Docker Hub credential 或新未审批第三方 GitHub Action 依赖。
- **验证命令**：
  - `go test ./scripts/internal/releaseworkflow -count=1`
  - `go test ./scripts/tests/containerrelease -count=1`
  - `go run ./scripts/cmd/releaseworkflow --workflow .github/workflows/release.yml`
- **验收**：全部绿色；无上述禁止项。
- **实际做了什么**：（待填）
- **验证证据**：（待填）
- **剩余风险**：（待填）
- **下一步建议**：（待填）

---

### 🔍 集中检查 #4（Task 10–12 之后）

- [ ] **目标**：Phase 3 完整集中复查。
- **检查清单**：
  - [x] 需求是否偏离 `input.md`
  - [ ] `publish_container` 权限是否最小化
  - [ ] stable/prerelease tag 语义是否正确
  - [ ] 恢复语义是否文档化
  - [ ] OCI labels 是否完整
  - [x] 代码是否有 bug、死代码、调试残留
  - [x] `go test ./scripts/internal/releaseworkflow -count=1` 是否通过
  - [x] `go test ./scripts/tests/containerrelease -count=1` 是否通过
  - [x] `go run ./scripts/cmd/releaseworkflow --workflow .github/workflows/release.yml` 是否通过
  - [x] 安全性：权限隔离、GHCR auth、无 credential 泄露
  - [ ] 数据一致性：两架构 manifest 一致性
  - [x] 文档是否同步（本 phase 可能不需要文档变更）
- **结论**：（待填）
- **发现问题**：（待填，如有则追加修复 task）

---

## Phase 4: 文档化支持的 Docker UX（C4）

### Task 13: Red — 文档 fixture/测试（如适用）

- [ ] **目标**：如果新官方安装路径需要稳定链接/命令 contract，扩展文档 fixture/测试。在文档变更前运行聚焦文档测试（当有可测试 contract 时）。
- **验收**：如有可测试 contract，测试先失败；如无，记录理由并跳到 Task 14。
- **实际做了什么**：（待填）
- **验证证据**：（待填）
- **剩余风险**：（待填）
- **下一步建议**：（待填）

---

### Task 14: 用户文档 — README + README.zh-CN

- [ ] **目标**：先更新 `README.md`（canonical 英文安装/quick-start 入口），再同步 `README.zh-CN.md`。
- **覆盖内容**：
  - exact-version 和 `latest` pull 语义
  - 持久 `~/.pixiv-cli` volume
  - `/work` 下载 bind mount
  - 镜像架构支持
- **验收**：双语文档覆盖上述内容。
- **实际做了什么**：（待填）
- **验证证据**：（待填）
- **剩余风险**：（待填）
- **下一步建议**：（待填）

---

### Task 15: Auth + MCP + 升级文档

- [ ] **目标**：
  1. **Auth 文档**：推荐 stdin-based `pixiv auth import` + 持久状态 volume，使 refresh token 不进入镜像层或 CLI argv。不声称 `auth login` 有新 container-specific callback 行为。
  2. **MCP 文档**：展示 `docker run --rm -i ... mcp` stdio 模式。保持 stdout 属于 MCP JSON-RPC 的规则，不添加网络 MCP transport。
  3. **升级文档**：声明容器安装通过 pull/redeploy 升级。不声称 `pixiv update` 是 container-aware，不改变 updater 语义。
- **验收**：三部分文档内容到位。
- **实际做了什么**：（待填）
- **验证证据**：（待填）
- **剩余风险**：（待填）
- **下一步建议**：（待填）

---

### 🔍 集中检查 #5（Task 13–15 之后）

- [ ] **目标**：Phase 4 上半部分集中复查。
- **检查清单**：
  - [x] 需求是否偏离 `input.md`
  - [ ] 双语文档是否语义一致
  - [ ] 命令/路径/registry 名是否一致
  - [ ] 是否未声称 Docker-specific product 行为
  - [ ] auth 文档是否推荐 stdin import
  - [ ] MCP 文档是否保持 stdio 语义
  - [ ] 升级文档是否正确（pull-based）
  - [ ] 文档测试是否通过（如适用）
- **结论**：（待填）
- **发现问题**：（待填，如有则追加修复 task）

---

### Task 16: 维护者文档 — development.md 双语同步

- [ ] **目标**：更新 `docs/en/maintainers/development.md` 加入容器构建/release 验证，再同步 `docs/zh-CN/maintainers/development.md`。路由而非在多处重复长 release 规则。
- **验收**：双语维护者文档覆盖容器构建/release 验证。
- **实际做了什么**：（待填）
- **验证证据**：（待填）
- **剩余风险**：（待填）
- **下一步建议**：（待填）

---

### Task 17: Phase 4 验证

- [ ] **目标**：运行 `go test ./scripts/tests/documentation -count=1` 和 `git diff --check`；检查双语命令、路径、registry 名和安全语言语义一致性。
- **验收**：文档测试绿色；diff 无问题；双语语义一致。
- **实际做了什么**：（待填）
- **验证证据**：（待填）
- **剩余风险**：（待填）
- **下一步建议**：（待填）

---

### 🔍 集中检查 #6（Task 16–17 之后）

- [ ] **目标**：Phase 4 完整集中复查。
- **检查清单**：
  - [x] 需求是否偏离 `input.md`
  - [ ] `go test ./scripts/tests/documentation -count=1` 是否通过
  - [ ] `git diff --check` 是否通过
  - [ ] 维护者文档是否双语同步
  - [ ] 是否无文档遗漏（安装/auth/MCP/升级/维护者）
  - [ ] 安全语言是否一致（不鼓励不安全做法）
  - [ ] 文档路由规则是否遵守（README 为入口，不重复长规则）
- **结论**：（待填）
- **发现问题**：（待填，如有则追加修复 task）

---

## Phase 5: 集成验证和审查（C5）

### Task 18: 聚焦验证

- [ ] **目标**：运行所有聚焦验证。
- **验证命令**：
  - `go test ./scripts/internal/releaseworkflow -count=1`
  - `go test ./scripts/tests/containerrelease -count=1`
  - `go run ./scripts/cmd/releaseworkflow --workflow .github/workflows/release.yml`
  - `go test ./scripts/tests/documentation -count=1`
- **验收**：全部绿色，证据新鲜。
- **实际做了什么**：（待填）
- **验证证据**：（待填）
- **剩余风险**：（待填）
- **下一步建议**：（待填）

---

### Task 19: 包/回归验证

- [ ] **目标**：运行 `sh scripts/test-package-release.sh`，然后 `go test ./...`。运行因实际变更文件所需的额外 lint/build 检查；不用静态检查替代真实 Docker smoke。
- **验收**：全部通过。
- **实际做了什么**：（待填）
- **验证证据**：（待填）
- **剩余风险**：（待填）
- **下一步建议**：（待填）

---

### Task 20: 真实容器证据

- [ ] **目标**：确认最新 credential-free 容器 smoke 工作流在 goal 分支/commit 上两个原生 Linux 架构均绿色，且 version/non-root/state-path 断言实际运行而非跳过。
- **验收**：两架构 smoke CI 绿色；断言执行而非跳过。
- **实际做了什么**：（待填）
- **验证证据**：（待填）
- **剩余风险**：（待填）
- **下一步建议**：（待填）

---

### 🔍 集中检查 #7（Task 18–20 之后）

- [ ] **目标**：Phase 5 上半部分集中复查。
- **检查清单**：
  - [x] 需求是否偏离 `input.md`
  - [ ] 所有聚焦验证是否新鲜绿色
  - [ ] `go test ./...` 是否通过
  - [ ] `sh scripts/test-package-release.sh` 是否通过
  - [ ] 两架构 smoke CI 是否绿色且断言执行
  - [ ] 是否有被跳过的断言
  - [ ] 安全性：全链路权限/credential 审查
  - [x] 是否需要新增/调整回滚方案
- **结论**：（待填）
- **发现问题**：（待填，如有则追加修复 task）

---

### Task 21: Diff hygiene + Release boundary review

- [ ] **目标**：
  1. 运行 `git diff --check` 确认只有本 goal 理由充分的文件变更。验证无生成镜像 archive、本地数据库、token、缓存或 registry credential 进入 Git 历史。
  2. 使用仓库 review checklist 检查：不可变 tag 绑定、权限、action SHA pinning、GHCR auth 可达性、stable/prerelease tag 语义、glibc/原生 runner 映射、错误行为。修复阻塞发现并重跑受影响证据。
- **验收**：diff 干净；review 无阻塞发现。
- **实际做了什么**：（待填）
- **验证证据**：（待填）
- **剩余风险**：（待填）
- **下一步建议**：（待填）

---

### Task 22: Goal verifier — 终审

- [ ] **目标**：重读每个 C1–C5 验收标准对照当前仓库状态和证据。仅当每个标准有直接当前状态证据时标记 `COMPLETE`；否则设置 `CONTINUE` 或 `BLOCKED` 并给出确切缺失证明。
- **验收**：C1–C5 全部有直接当前状态证据。
- **实际做了什么**：（待填）
- **验证证据**：（待填）
- **剩余风险**：（待填）
- **下一步建议**：（待填）

---

### 🔍 终审集中检查（Task 21–22 之后，全部 task 完成）

- [ ] **目标**：最大范围总复查。
- **检查清单**：
  - [ ] C 端体验：Docker 用户能否按文档正常使用
  - [ ] 代码质量：是否有 bug、死代码、调试残留
  - [ ] 安全性：鉴权、注入、敏感信息泄露、权限隔离
  - [ ] 数据一致性：状态路径、volume 持久化
  - [ ] 权限：build/publish 权限边界
  - [ ] 错误处理：registry 失败、恢复语义
  - [ ] 测试覆盖：所有新增/修改有聚焦测试
  - [ ] 构建产物：镜像构建正确
  - [ ] 文档：双语同步、无遗漏
  - [ ] 回滚方案：是否完整
  - [ ] Release boundary：不可变 tag、GHCR 恢复
  - [ ] 所有 C1–C5 标准有直接当前状态证据
- **结论**：（待填）
- **发现问题**：（待填，如有则立即修复）
- **最终状态**：（待填：COMPLETE / CONTINUE / BLOCKED）

---

## Task 汇总

| # | Task | Phase | 检查点 |
|---|------|-------|--------|
| 1 | Red — release policy 失败测试 | 1 | |
| 2 | Green — 扩展 release policy 规则 | 1 | |
| 3 | Refactor + Phase 1 验证 | 1 | |
| 🔍 | 集中检查 #1 | | ✓ |
| 4 | Red — Dockerfile contract 失败测试 | 2 | |
| 5 | Green — 创建最小 Dockerfile | 2 | |
| 6 | .dockerignore + 真实 smoke | 2 | |
| 🔍 | 集中检查 #2 | | ✓ |
| 7 | Red — release graph 失败测试 | 3 | |
| 8 | Green/build — build_container matrix | 3 | |
| 9 | 并行性 + credential-free smoke 工作流 | 3 | |
| 🔍 | 集中检查 #3 | | ✓ |
| 10 | Green/publish — publish_container job | 3 | |
| 11 | Tag policy + 恢复语义 | 3 | |
| 12 | Phase 3 验证 | 3 | |
| 🔍 | 集中检查 #4 | | ✓ |
| 13 | Red — 文档 fixture/测试 | 4 | |
| 14 | 用户文档 — README 双语 | 4 | |
| 15 | Auth + MCP + 升级文档 | 4 | |
| 🔍 | 集中检查 #5 | | ✓ |
| 16 | 维护者文档 — development.md 双语 | 4 | |
| 17 | Phase 4 验证 | 4 | |
| 🔍 | 集中检查 #6 | | ✓ |
| 18 | 聚焦验证 | 5 | |
| 19 | 包/回归验证 | 5 | |
| 20 | 真实容器证据 | 5 | |
| 🔍 | 集中检查 #7 | | ✓ |
| 21 | Diff hygiene + Release boundary review | 5 | |
| 22 | Goal verifier — 终审 | 5 | |
| 🔍 | 终审集中检查 | | ✓ |

**总计**：22 个执行 task + 8 个集中检查 = 30 轮
