# Homebrew tap 发布凭据接入清单

本文件描述自动 tap 部署的凭据边界；不含私钥、密钥生成命令或可提交的
credential material。远端 Environment 与 deploy key 已由 **Task20** 接入。

1. Release workflow 先验证六个 archive、`checksums.txt` 与 Ed25519 manifest，核对 draft
   asset 集合后公开 Release，再把**同一份** `release/checksums.txt` 作为短期 workflow
   artifact 交给 formula renderer。stable 只生成 `pixiv-cli.rb`，prerelease 只生成
   `pixiv-cli-beta.rb`；下游不得重新下载或重算 checksum。
2. 由 Task20 的受权维护者按组织认可的密钥管理流程创建**专用于**
   `FlanChanXwO/homebrew-tap` 的可写 SSH deploy key；公钥仅登记在 tap repository，
   不得复用 source repository、个人账户或 release 签名用的密钥。
3. 将该私钥仅保存为 source repository 的受保护 `release` Environment secret
   `HOMEBREW_TAP_DEPLOY_KEY`。不得放入 repository variables、workflow YAML、tap
   repository、日志、artifact 或本目录；fork/PR workflow 不得读取它。
4. renderer artifact 必须先在 macOS Intel、macOS arm64、Linux amd64 与 Linux arm64
   runner 上分别复制到隔离 local staging tap 的 `Formula/`，以精确
   `brew trust --tap pixiv-cli-release/staging` 写入 runner 的临时 trust store，再以 tap-qualified
   name 执行真实 `brew install`，并解析 `pixiv version --json` 断言 embedded version 等于 tag。它不使用
   workspace formula path、developer/环境变量 bypass 或公开 tap；任一 matrix job 失败时，最终 tap job 不会启动。
5. Homebrew 下游 jobs 中只有最后的 `deploy_homebrew_tap` 声明受保护 `release` Environment；
   publish job 仍在同一 Environment 内隔离 Release signing secrets。deploy job 先以 public HTTPS
   clone tap、只 stage 对应的一个 `Formula/*.rb` 并核对 staged diff。最后一个 step 才读取
   `HOMEBREW_TAP_DEPLOY_KEY`，用 `templates/homebrew/github.com-known-hosts`
   固定的 GitHub ED25519 host key、`StrictHostKeyChecking=yes` 与一次性 `0600` key 将
   精确 commit push 为 `HEAD:main`。
6. draft Release 的匿名 asset URL 不可供 Homebrew 安装，因此 Release 必须在四架构安装前
   公开。若后续安装失败，已公开 Release 不会自动回滚，但 tap 保持不变；维护者应修复原因并
   按发布策略处置 Release，不能手工绕过 matrix 后单独 push formula。发现 secret 泄露或权限
   漂移时，立即撤销 tap deploy key。

`sh scripts/test-release-workflow.sh` 与 `sh scripts/test-homebrew-formula.sh` 只验证本地策略和
fixture；它们不能替代正式 tag workflow 的四个平台外部安装证据。
