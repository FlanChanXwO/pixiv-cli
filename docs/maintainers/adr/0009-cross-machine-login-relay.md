# 0009: 跨机器浏览器登录使用按需协议 handler 与显式 relay

## 状态

已采纳。

## 背景

Pixiv App OAuth 使用由 Pixiv 控制的固定回调，当前共享 `pixiv-android` client 不能把 redirect URI 改成
用户服务器的任意 HTTP(S) 地址。用户仍可能希望服务器保存 token，而浏览器在另一台桌面机器上完成授权。

## 决策

- 保留 Pixiv 的固定 OAuth callback 与既有 PKCE/state/token exchange；server relay 只接收最终
  `pixiv://account/login` callback，不充当 OAuth redirect URI。
- 系统 `pixiv://` handler 是持久、按需启动的，不创建常驻 daemon。活跃本地 loopback endpoint 优先；其不存在
  才使用 `login_relay_target_url` 和私有 shared secret 向 server relay 预检并 POST。
- relay server 只在 `auth login` 运行期间存在，一次会话只接收一个 callback，并同时验证 bearer secret、精确
  allowlist、PKCE/state 和会话状态。
- handler manifest 不存 secret，只存恢复旧系统关联所需元数据。非 allowlist URL 定向交给旧 handler；解除
  `login_relay_target_url` 时仅在 pixiv-cli 仍为默认 handler 才恢复旧关联。
- 支持 direct TLS PEM，或公开 HTTPS URL 加同机反向代理/loopback listener。HTTP 可用但必须明显警告；没有
  Web fallback、浏览器自动化、Cookie/历史读取、本地订阅库或静默成功。

## 后果

用户需要通过安全渠道在 server/client 配置相同 secret，并自行在浏览器机器打开 server 显示的 Pixiv URL。
macOS、Windows 与桌面 Linux 可作为 client；headless Linux 只能作为 server。卸载前应运行
从私有 `config.toml` 删除 `login_relay_target_url`；binary 已删除时，用户按 private manifest 用系统关联 UI 人工恢复。
