# v0.10.0 — 2026-08-01

## 破坏性变更

- 作品筛选从 `pixiv filter` 移到视觉列表与下载命令的 `--filter EXPR`；CLI 的 `--ugoira-format` 改为 `--ugoira-mode`。（[#46](https://github.com/FlanChanXwO/pixiv-cli/pull/46)）

## 新增

- SDK、CLI 和 MCP 共用类型化的本地插画筛选器，支持标签与绘图工具条件，并在请求 Pixiv 前完成校验。（[#46](https://github.com/FlanChanXwO/pixiv-cli/pull/46)）
- 下载可写入 SQLite 归档、生成原子元数据 sidecar、使用扩展的文件名与目录模板、配置重试与请求间隔、下载开放页码范围，并把动图输出为 GIF、APNG、ZIP 或逐帧。公开书签页和插画系列 URL 可作为下载来源，HTTP(S)/SOCKS5/SOCKS5H 代理同时适用于下载和更新。（[#46](https://github.com/FlanChanXwO/pixiv-cli/pull/46)）

## 变更

- 输出被管道接收时，视觉列表命令会输出规范 Record NDJSON，可直接传给 `pixiv download`。（[#46](https://github.com/FlanChanXwO/pixiv-cli/pull/46)）

**完整变更**：[v0.9.1...v0.10.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v0.9.1...v0.10.0)

<!-- internal:
  [#47](https://github.com/FlanChanXwO/pixiv-cli/pull/47)
  [#48](https://github.com/FlanChanXwO/pixiv-cli/pull/48)
-->
