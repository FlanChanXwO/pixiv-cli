# Changelog

本文件记录项目中值得用户和集成方关注的变化。

格式遵循 [Keep a Changelog 1.1.0]。项目开始切正式版本后，再按
[Semantic Versioning] 维护版本段与比较链接。

## [Unreleased]

当前还没有切出正式版本；未发布改动先汇总到这里。

### Added

- 新增项目级 changelog，集中记录用户可见变化、兼容性说明和发布准备事项。
- 新增 POSIX sh 构建脚本 `sh scripts/build.sh`，默认将二进制输出到 `build/`。
- 新增公开 Go 包 `github.com/FlanChanXwO/pixiv-cli/pkg/pixiv`；提供具体 `*pixiv.Client`、稳定模型、错误分类、opaque cursor 与受策略限制的资源流访问。
- 新增 `pixiv user artworks/bookmarks/following [USER_ID]`，省略 `USER_ID` 时使用当前认证用户；新增 `bookmark add/remove` 与 `follow add/remove`。
- MCP 新增 `user_artworks`、收藏/关注分页与收藏、关注写操作，并为这些结果提供 structured output。
- 新增可注入的 `slog` 诊断日志与 `log_level`/`log_format` 配置，支持 `PIXIV_LOG_LEVEL`/`PIXIV_LOG_FORMAT` 覆盖。

### Changed

- Breaking: 本地 auth 账号从自定义账号名改为 Pixiv UID；`auth add/login` 不再接收账号名，`auth use/remove/check` 使用 UID，`--uid` 取代 `--profile` 作为主选择参数。
- Breaking: `auth.json` schema 改为 `default_user_id` 与 `accounts[].user_id/username`；旧 `default_account/accounts[].name` 文件需要重新 `pixiv auth add` 或 `pixiv auth login`。
- Breaking: 移除公开 CLI 参数 `--no-proxy`；需要临时代理覆盖时继续使用 `--proxy URL`，代理优先级为 `--proxy` > `https_proxy`/`HTTPS_PROXY` > `config.toml`。
- `pixiv auth login` 默认打开浏览器时也保留终端粘贴兜底，可直接粘贴 callback URL、`pixiv://...` URL 或原始 authorization code。
- `pixiv auth login` 接受 Pixiv 官方 callback URL 与 `pixiv://account/login` 缺省 OAuth state 的授权码回填，同时继续要求本地 loopback callback 携带正确 state。
- `pixiv auth login` 在 macOS 默认会优先注册本地 `pixiv://` callback helper 并打开默认浏览器，以复用已有 Pixiv 登录态；helper 只把最终 callback URL 转交给本轮 CLI loopback，不安装扩展、不点击页面、不读取 cookie/token。若 helper 不可用，CLI 会退回专用 Chromium/Edge DevTools 捕获；macOS 上仍保留 Edge/Chrome/Chromium/Safari 标签页与 Chromium session/history 只读观察，并在 Pixiv 卡在 `post-redirect` 授权接力页时校验本轮 OAuth 后等待 `pixiv://` handoff，不再自动重开白页；状态不可读或 Pixiv 未生成 callback 时继续保留手动回填路径。
- 列表 CLI 改用 `--limit` 和逻辑 `--page`；`--offset` 已废弃。CLI/MCP 不暴露 SDK cursor，MCP 返回逻辑分页 metadata。
- 有 refresh token 时 App API 失败不再自动回落 Web；Web 仅用于无 token 的匿名白名单读操作和明确的资源 enrichment。

[Keep a Changelog 1.1.0]: https://keepachangelog.com/zh-CN/1.1.0/
[Semantic Versioning]: https://semver.org/spec/v2.0.0.html
