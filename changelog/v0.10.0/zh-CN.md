# v0.10.0 — 2026-08-01

## 破坏性变更

- 将作品筛选迁移到视觉列表和下载命令的 <code>--filter EXPR</code>，替代 <code>pixiv filter</code>；将 CLI <code>--ugoira-format</code> 替换为 <code>--ugoira-mode</code>。 ([#46](https://github.com/FlanChanXwO/pixiv-cli/pull/46))

## 新增

- 新增由 SDK、CLI 和 MCP 共用的类型化本地作品筛选器，支持标签与绘图工具集合谓词，并在请求 Pixiv 前完成校验。 ([#46](https://github.com/FlanChanXwO/pixiv-cli/pull/46))
- 新增可靠下载能力：SQLite 作品归档、原子元数据 sidecar、扩展的文件名与目录模板、可配置重试与请求间隔、开放页码范围，以及 GIF/APNG/ZIP/逐帧 ugoira 输出。 ([#46](https://github.com/FlanChanXwO/pixiv-cli/pull/46))
- 支持将公开书签页和插画系列 URL 作为下载来源，并按规范作品 ID 去重；运行期和更新请求一致支持 HTTP(S)、SOCKS5 与 SOCKS5H 代理 URI。 ([#46](https://github.com/FlanChanXwO/pixiv-cli/pull/46))

## 变更

- 视觉列表命令在 stdout 非交互时输出规范 Record NDJSON，可直接管道到 <code>pixiv download</code>，无需外部 JSON 处理器。 ([#46](https://github.com/FlanChanXwO/pixiv-cli/pull/46))

**完整变更**：[v0.9.1...v0.10.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v0.9.1...v0.10.0)
