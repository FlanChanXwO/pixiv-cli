# 0009: 跨机器浏览器登录使用一次性 OAuth handoff relay

## 状态

已采纳。

## 背景

Pixiv App OAuth 使用由 Pixiv 控制的固定回调，当前共享 `pixiv-android` client 不能把 redirect URI 改成
用户服务器的任意 HTTP(S) 地址。用户仍可能希望服务器保存 token，而浏览器在另一台桌面机器上完成授权。

## 决策

- 保留 Pixiv 的固定 OAuth callback 与既有 PKCE/state/token exchange；relay 只接收最终
  `pixiv://account/login` callback，不充当 OAuth redirect URI。
- 同时配置 `login_relay_public_url` 与 `login_relay_listen_addr` 的 server 在 `auth login` 时输出一次性 handoff URL。
  浏览器打开 URL 后直接转交已安装 desktop CLI 的 handler；终端不显示 OAuth URL，也不渲染项目确认页或手动回填表单。
- desktop 打开精确的 `pixiv://account/remote-login` 后，以本次会话 capability 领取 OAuth URL，并只在本地保存本次有效的
  handoff state。收到 `pixiv://account/login` 后，把 callback 回传给创建会话的 server；新的 handoff 覆盖旧状态。
- 只有活跃 handoff 才接管精确的 `pixiv://account/login`；其他 callback 仍优先交给活跃 loopback bridge，或定向给旧 handler。
  `pixiv://account/remote-login` 也采取精确 allowlist。
- 删除 `pixiv auth devices`、Ed25519 key 和设备记录。已有 `remote-devices.json` 保留在磁盘但完全忽略；
  `login_relay_secret` 与 `login_relay_target_url` 也静默忽略，不再触发转发。
- macOS 与 Windows 在正常 CLI 启动时按需准备 persistent handler；desktop Linux 继续在交互式登录时使用 XDG handler，
  headless Linux 仅运行 server relay。relay 可使用 HTTP 或 HTTPS，并支持 direct TLS PEM 或同机反向代理终止 HTTPS。

## 后果

浏览器 desktop 需要安装 pixiv-cli，才能使用 remote handoff；没有 desktop handler 的 client 不能完成该流程。macOS、Windows 与桌面 Linux 可作为 client；headless Linux 可作为 server。已有旧设备记录不需迁移，更新后会被忽略；旧 private relay settings 也无需清理才可继续使用配置文件。binary 已删除时，用户可用系统关联 UI 选择所需的 handler。
