# Homebrew tap 发布凭据接入清单

本文件只描述凭据边界和人工接入条件；不含私钥、密钥生成命令或可提交的
credential material。实际配置由 **Task20** 在公开发布前完成。

1. 先在 source repository 的 release job 本地验证 draft Release 的六个 archive、
   `checksums.txt`、Ed25519 manifest，并用
   `sh scripts/test-homebrew-formula.sh` 从该 checksums fixture 渲染并检查 formula。
   任一资产、checksum、签名或 formula 校验失败时，禁止连接或 push tap。
2. 由 Task20 的受权维护者按组织认可的密钥管理流程创建**专用于**
   `FlanChanXwO/homebrew-tap` 的可写 SSH deploy key；公钥仅登记在 tap repository，
   不得复用 source repository、个人账户或 release 签名用的密钥。
3. 将该私钥仅保存为 source repository 的受保护 `release` Environment secret
   `HOMEBREW_TAP_DEPLOY_KEY`。不得放入 repository variables、workflow YAML、tap
   repository、日志、artifact 或本目录；fork/PR workflow 不得读取它。
4. Task20 配置 source repository 的 `release` Environment 保护规则与 tag 创建权限，
   使只有已通过 release 资产/签名门禁的受信 tag job 能读取此 secret；部署时固定
   tap remote 的 SSH host key，并只使用一次性的受限 checkout/commit/push 步骤。
5. 在真实推送前，确认 renderer 给 stable tag 生成 `Formula/pixiv-cli.rb`、给 prerelease
   tag 生成 `Formula/pixiv-cli-beta.rb`，二者均来自同一已验证 Release 的 URL 与 SHA-256。
   发布后在 macOS 与 Linux 验证安装和 `pixiv version`；发现泄露或权限漂移时，立即撤销
   tap deploy key 并重新执行此流程。
