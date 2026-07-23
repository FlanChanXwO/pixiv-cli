# v0.1.0 — 2026-07-13

## 新增

- 新增项目级 changelog，集中记录用户可见变化、兼容性说明和发布准备事项。
- 新增 POSIX sh 构建脚本 `sh scripts/build.sh`，默认将二进制输出到 `build/`。
- 新增 `pixiv version [--json]` 与根 `pixiv --version`；JSON 输出包含 `version`、`commit`、`build_date`。
- 新增 `pixiv update [--check] [--prerelease] [--proxy URL]`，支持 Homebrew stable/beta、精确 tag `go install` 与签名 Release binary 策略；开发构建拒绝更新。
- 新增可关闭的 `update_check_enabled` 自动 stable 更新提示；普通 CLI 成功后最多每 24 小时检查一次，自动检查最多等待 3 秒，且不会污染 JSON/MCP stdout 或改变业务命令退出码。
- 新增内置 Rust ugoira GIF/APNG encoder；生产下载路径不再依赖 `ffmpeg`。
- 新增经六平台 native runner build/smoke 与统一 source digest 验证的 committed Rust staticlibs 和 `manifest.json`，供受支持的 source build 与精确 tag `go install` 使用。
- 新增固定 six-target Release asset 格式、checksum/Ed25519 签名、Homebrew formula renderer 与六 native runner workflow。
- 新增 Homebrew stable `pixiv-cli` 与 beta `pixiv-cli-beta` formula 模板；两者冲突并同装 `pixiv`，不依赖 `ffmpeg`。
- Release workflow 现把同一份已发布 checksum 渲染为 stable/beta Homebrew formula，在 macOS/Linux 双架构真实安装并核对版本后，才以独立 deploy key 向 public tap 推送唯一对应 formula。

## 变更

- Breaking: 本地 auth 账号从自定义账号名改为 Pixiv UID；`auth add/login` 不再接收账号名，`auth use/remove/check` 使用 UID，`--uid` 取代 `--profile` 作为主选择参数。
- Breaking: `auth.json` schema 改为 `default_user_id` 与 `accounts[].user_id/username`；旧 `default_account/accounts[].name` 文件需要重新 `pixiv auth add` 或 `pixiv auth login`。
- `pixiv auth login` 默认打开浏览器时也保留终端粘贴兜底，可直接粘贴 callback URL、`pixiv://...` URL 或原始 authorization code；接受 Pixiv 官方 callback URL 与 `pixiv://account/login` 缺省 OAuth state 的授权码回填，同时继续要求本地 loopback callback 携带正确 state。
- `pixiv auth login` 在 macOS 默认会优先注册本地 `pixiv://` callback helper 并打开默认浏览器，以复用已有 Pixiv 登录态；helper 只把最终 callback URL 转交给本轮 CLI loopback，不安装扩展、不点击页面、不读取 cookie/token。helper 不可用、状态不可读或 Pixiv 未生成 callback 时保留手动回填路径。

## 修复

- 修复 Linux 原生 Rust staticlib 的 `libm` 链接，以及 Windows checkout 对 first-party crate、Cargo vendor、本地 locked dependency 和生成 license bundle 的文本转换，使六平台保持同一 Rust source identity；Windows cgo 使用 Rust import libraries 和 LLD-backed Clang 链接 `*-pc-windows-msvc` staticlib，release `.zip` 使用预装 7-Zip，避免 Git Bash 缺少 `zip`。

## 安全

- 发布基础设施使用公开 source/tap 仓库、受保护 `release` Environment、隔离的 Ed25519 签名与 tap deploy credentials；私钥仅位于该 Environment 与 macOS Keychain 恢复副本。
- native-evidence 与 release workflow policy 均 fail-closed，检查 action full-SHA pin、最小权限、精确 trigger/runner matrix、required job/ancestry gate、canonical immutable source checkout 与 stable/prerelease 发布通道映射。
- Homebrew tap secret 仅可进入最终 protected deploy job 的最后 push step；Rust ugoira crates.io 依赖以完整 locked `vendor/` 闭包及 Cargo checksum 审计，并在空 Cargo cache 中验证。
- 受支持 binary 内置可轮换的 Ed25519 public-key trust root；安装器在替换前验证签名 checksum、archive SHA-256 与下载 binary version，并严格验证 GitHub HTTPS 来源、archive path、取消、cache 权限与软链接目标。
- v0.1.0 不含 Apple notarization 或 Windows Authenticode；直接下载仍可能显示 Gatekeeper/SmartScreen 提示。
