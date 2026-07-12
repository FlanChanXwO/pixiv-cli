# Changelog

本文件记录项目中值得用户和集成方关注的变化。

格式遵循 [Keep a Changelog 1.1.0]。项目开始切正式版本后，再按
[Semantic Versioning] 维护版本段与比较链接。

## [Unreleased]

当前还没有切出正式版本；未发布改动先汇总到这里。

### Added

- 新增项目级 changelog，集中记录用户可见变化、兼容性说明和发布准备事项。
- 新增 POSIX sh 构建脚本 `sh scripts/build.sh`，默认将二进制输出到 `build/`。
- 新增 `pixiv version [--json]` 与根 `pixiv --version`；JSON 输出包含 `version`、`commit`、`build_date`。
- 新增 `pixiv update [--check] [--prerelease] [--proxy URL]`，以及可关闭的 `update_check_enabled` 自动 stable 更新提示；自动检查不会污染 JSON/MCP stdout 或改变业务命令退出码。
- 新增内置 Rust ugoira GIF/APNG encoder；生产下载路径不再依赖 `ffmpeg`。
- 新增 Release asset、checksum/Ed25519 签名、Homebrew formula renderer 与六 native runner workflow 的本地发布准备。

### Changed

- Breaking: 本地 auth 账号从自定义账号名改为 Pixiv UID；`auth add/login` 不再接收账号名，`auth use/remove/check` 使用 UID，`--uid` 取代 `--profile` 作为主选择参数。
- Breaking: `auth.json` schema 改为 `default_user_id` 与 `accounts[].user_id/username`；旧 `default_account/accounts[].name` 文件需要重新 `pixiv auth add` 或 `pixiv auth login`。
- `pixiv auth login` 默认打开浏览器时也保留终端粘贴兜底，可直接粘贴 callback URL、`pixiv://...` URL 或原始 authorization code。
- `pixiv auth login` 接受 Pixiv 官方 callback URL 与 `pixiv://account/login` 缺省 OAuth state 的授权码回填，同时继续要求本地 loopback callback 携带正确 state。
- `pixiv auth login` 在 macOS 默认会优先注册本地 `pixiv://` callback helper 并打开默认浏览器，以复用已有 Pixiv 登录态；helper 只把最终 callback URL 转交给本轮 CLI loopback，不安装扩展、不点击页面、不读取 cookie/token。若 helper 不可用，CLI 会退回专用 Chromium/Edge DevTools 捕获；macOS 上仍保留 Edge/Chrome/Chromium/Safari 标签页与 Chromium session/history 只读观察，并在 Pixiv 卡在 `post-redirect` 授权接力页时校验本轮 OAuth 后等待 `pixiv://` handoff，不再自动重开白页；状态不可读或 Pixiv 未生成 callback 时继续保留手动回填路径。

### Security

- 已创建公开 source/tap 仓库、受保护 `release` Environment、隔离的 Ed25519 签名与 tap deploy
  credentials；私钥仅位于该 Environment 与 macOS Keychain 恢复副本。此准备不创建 tag、Release 或
  tap formula，正式发布仍受 native evidence 与安装验收门禁阻断。
- native-evidence 的 policy command 现与 cgo encoder 解耦，可在目标 staticlib 生成前 fail-closed
  地检查 workflow；避免缺库被误报为 policy 或 runner 配置通过。
- 新增独立、无 secret/发布副作用的 six-target native evidence workflow 与本地 AST policy；每个
  runner 会记录实际 staticlib、版本化 binary、完整许可证 archive 与 source/hash evidence。未收集到
  经审计 main SHA 的六份真实 artifact 时，release 继续被阻断。
- Release workflow 现由可解析的 YAML policy 检查所有 action 的 full-SHA pin、最小权限、精确
  trigger/runner matrix；默认分支 trust gate 在独立、无 Environment/secret 的 job 完成后，publish
  才能访问精确的 signing secret。policy 同时拒绝被软失败或条件跳过的质量门禁，并把 SemVer channel
  的 stable/prerelease 分支绑定到实际 `gh release create` 参数，避免排版变化或 action tag/门禁移除
  悄然削弱发布供应链。
- Release workflow policy 现 fail-closed 地拒绝 YAML alias、merge key 与重复 mapping key，并禁止
  required job/ancestry gate 被条件跳过；每项质量检查固定为独立的直接命令 step，validate/build checkout
  显式不持久化凭据，避免工作目录、shell 控制流或 credential 语义悄然改变发布结果。
- Release workflow 的四个 checkout 现固定为 canonical full-SHA source；trust job 只能在完整、无凭据
  checkout 后立即验证 tag ancestry，签名 job 也拒绝 `ref`、`repository`、`path` 等会令源码与发布 asset
  脱钩的 checkout 重定向。
- Rust ugoira crate 的 crates.io 依赖现以完整 locked `vendor/` 闭包及 Cargo checksum 随源码审计；
  release 验证会在空 Cargo cache 下完成 metadata/build/test 与六 target 许可证检查，缺失 vendor、
  checksum 不匹配或 registry fallback 都会明确失败。
- 受支持 binary 现在内置可轮换的 production Ed25519 key ID→public key trust root，并以已知真实签名和
  SPKI fingerprint 回归验证；私钥仍不进入源码。正式发布仍被完整 six-target staticlib/manifest、native
  runner 证据、受保护 `release` Environment 与实际 Release/tap 验证阻断。
- 更新检查写入前会把既有 `pixiv-cli` cache 目录在 Unix-like 平台收紧为 `0700`；Windows 保持其 ACL 语义。
- Release installer 会在下载前校验 Releases API 或 ETag cache 中每个选中 asset 的精确 GitHub HTTPS 来源；跨 host、仓库、tag、asset 或含歧义 URL 的记录会明确失败，绝不请求该 URL。
- Release installer 拒绝 archive 原始路径中含有任何 `..` segment 的条目，防止归一化后的路径绕过解包安全校验。
- Release installer 在 archive 解包、版本预检、staging 及最终替换前都会保留调用方取消；取消更新会清理临时文件且不会替换当前 executable。
- Release installer 与 Windows pending-update backup cleanup 在当前 executable 为软链接时保留链接入口，只操作解析后的真实目标；断链会明确失败而不会改写链接或删除备份。

## [0.1.0] - Release candidate (not published)

> 此段是未来 `v0.1.0` 的 release notes 输入，**不是**已发布公告或发布日期。公开发布前必须完成
> 六个真实 staticlib 与 manifest、workflow/native artifact 证据、production
> Ed25519 key/受保护 `release` Environment、公开仓库/Release/tap 及真实安装验收。

### Added

- `pixiv version [--json]`、根 `pixiv --version` 与构建 metadata。
- `pixiv update` 的 Homebrew stable/beta、精确 tag `go install` 与签名 Release binary 策略；开发构建拒绝更新。
- 普通 CLI 成功后的 stable 更新提示、24 小时节流和最多 3 秒的自动检查时间边界。
- 以 Rust staticlib 驱动的 ugoira GIF/APNG encoder，以及固定 six-target Release asset/checksum/signature 格式。
- Homebrew stable `pixiv-cli` 与 future beta `pixiv-cli-beta` formula 模板；两者冲突并同装 `pixiv`，不依赖 `ffmpeg`。

### Security

- Release installer 设计为在替换前验证 Ed25519 签名 checksum、archive SHA-256 与下载 binary version；
  受支持 binary 只信任随源码提交的 production public key，绝不因私钥或 Release 尚未部署而改用其他来源。
- v0.1.0 不含 Apple notarization 或 Windows Authenticode；发布后直接下载仍可能显示 Gatekeeper/SmartScreen 提示。

[Keep a Changelog 1.1.0]: https://keepachangelog.com/zh-CN/1.1.0/
[Semantic Versioning]: https://semver.org/spec/v2.0.0.html
