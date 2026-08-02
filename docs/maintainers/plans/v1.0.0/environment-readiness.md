# v1.0.0 环境就绪审计

审计日期：2026-08-03。该文件只记录非 secret 的本机能力与缺口；credential value、账号 identity、
浏览器路径内容和测试响应不得写入。

## 已就绪

- macOS 26.5.2 arm64，剩余磁盘约 402 GiB。
- Go 1.26.3、CGO、gopls 0.21.1、Git 2.50.1、SQLite 3.51.0。
- Rust 1.95.0、Cargo、jq、curl、pre-commit 4.6.0 与项目 `.pre-commit-config.yaml`。
- Docker CLI/Compose 与 Docker Desktop application 已安装；daemon 已启动并通过 `docker info` 验证，
  当前 server 为 Docker Desktop 29.4.2/aarch64。
- Chrome、Edge、Safari 已安装；macOS Keychain 可用。
- 本机 `pixiv` CLI 存在且账号 inventory 非空；legacy auth store 权限为 `0600`。
- FANBOX E2E Keychain item 存在；只保存计划允许的 session value。
- Go LSP 成功启动且当前 diagnostics 为空。
- `go test ./...`、`go test -race ./...`、`go vet ./...`、`sh scripts/build.sh`、
  `pre-commit run --all-files`、documentation 与 workflow policy tests 全部通过。

## 开工前必须完成

- 当前主工作树位于 `main`，不能直接执行大重写。必须按
  [实施顺序](implementation-plan.md#0-隔离工作区与冻结基线) 创建独立 worktree/branch，并让该
  worktree 包含完整计划目录。

## 不阻塞核心实施、但阻塞对应后期门禁

- FlareSolverr image 尚未拉取。只在最终 release-prep 核对来源并固定 image digest；不提前使用浮动
  `latest`，也不把 image 变成普通开发环境依赖。
- Firefox 当前未安装，但本机安装 Firefox 不是开工或发布前提。普通测试使用脱敏的真实 schema
  fixture；最终兼容性验证由各原生 runner 临时解包固定版本 Firefox、创建隔离 profile 并在结束后
  清理，不修改宿主机浏览器安装状态。
- 当前只确认凭据入口存在；按本次审计约定，不在开工前联网验证 Pixiv RFT rotation 或 FANBOX
  session。联网有效性、rotation 持久化与失效分类统一留到最终 release-prep；不能把入口存在性记录
  当作真实 E2E 成功。
- Windows/Linux browser provider、ACL/DPAPI/secret-service 只能由对应 native CI/host 验证，本机
  macOS 不能替代。

## 结论

除创建隔离 worktree 外，当前环境已可正式执行阶段 0 至核心实施；Docker daemon 不再是缺口，且
无需在宿主机安装 Firefox。FlareSolverr image 固定、临时 Firefox native evidence、真实 session/RFT
和跨平台 evidence 按 [最终验证操作手册](release-prep-runbook.md) 在发布前一次性完成。
