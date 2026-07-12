# Changelog

本文件记录项目中值得用户和集成方关注的变化。

格式遵循 [Keep a Changelog 1.1.0]。项目开始切正式版本后，再按
[Semantic Versioning] 维护版本段与比较链接。

## [Unreleased]

当前还没有切出正式版本；未发布改动先汇总到这里。

### Added

- 新增项目级 changelog，集中记录用户可见变化、兼容性说明和发布准备事项。
- 新增 POSIX sh 构建脚本 `sh scripts/build.sh`，默认将二进制输出到 `build/`。

### Changed

- Breaking: 本地 auth 账号从自定义账号名改为 Pixiv UID；`auth add/login` 不再接收账号名，`auth use/remove/check` 使用 UID，`--uid` 取代 `--profile` 作为主选择参数。
- Breaking: `auth.json` schema 改为 `default_user_id` 与 `accounts[].user_id/username`；旧 `default_account/accounts[].name` 文件需要重新 `pixiv auth add` 或 `pixiv auth login`。
- Breaking: 移除公开 CLI 参数 `--no-proxy`；需要临时代理覆盖时继续使用 `--proxy URL`，代理优先级为 `--proxy` > `https_proxy`/`HTTPS_PROXY` > `config.toml`。
- `pixiv auth login` 默认打开浏览器时也保留终端粘贴兜底，可直接粘贴 callback URL、`pixiv://...` URL 或原始 authorization code。
- `pixiv auth login` 接受 Pixiv 官方 callback URL 与 `pixiv://account/login` 缺省 OAuth state 的授权码回填，同时继续要求本地 loopback callback 携带正确 state。
- `pixiv auth login` 在 macOS 默认会优先注册本地 `pixiv://` callback helper 并打开默认浏览器，以复用已有 Pixiv 登录态；helper 只把最终 callback URL 转交给本轮 CLI loopback，不安装扩展、不点击页面、不读取 cookie/token。若 helper 不可用，CLI 会退回专用 Chromium/Edge DevTools 捕获；macOS 上仍保留 Edge/Chrome/Chromium/Safari 标签页与 Chromium session/history 只读观察，并在 Pixiv 卡在 `post-redirect` 授权接力页时校验本轮 OAuth 后等待 `pixiv://` handoff，不再自动重开白页；状态不可读或 Pixiv 未生成 callback 时继续保留手动回填路径。

[Keep a Changelog 1.1.0]: https://keepachangelog.com/zh-CN/1.1.0/
[Semantic Versioning]: https://semver.org/spec/v2.0.0.html
