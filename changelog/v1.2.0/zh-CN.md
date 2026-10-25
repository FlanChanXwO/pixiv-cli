# v1.2.0 — 2026-10-21

## 破坏性变更

- Pixiv 与 FANBOX 合并为同一个 MCP 连接器：`pixiv fanbox mcp` 改为 `pixiv mcp`，同类工具通过 `kind`、`relation`、`source` 等显式参数合并（stdio 由 65 个减至 49 个）。旧工具名已移除，不保留别名；请按[工具迁移表](https://github.com/FlanChanXwO/pixiv-cli/blob/v1.2.0/docs/zh-CN/mcp-tools.md#工具迁移)迁移。（[#146](https://github.com/FlanChanXwO/pixiv-cli/pull/146)）

## 新增

- `pixiv mcp --port` 通过受 OAuth 保护的 Streamable HTTP 提供同一套工具，支持授权确认/PKCE、`--host`/`--base-url` 以及图片/动图交付；stdio 仍是默认模式，并保留本地文件下载。（[#146](https://github.com/FlanChanXwO/pixiv-cli/pull/146)）
- HTTP 模式新增交互式 `pixiv_show_gallery` MCP App：缩略图网格、带作者头像的作品详情、可缩放的全屏原图查看器、收藏操作、点击标签发起精确标签搜索，以及点击复制作品 ID。（[#146](https://github.com/FlanChanXwO/pixiv-cli/pull/146), [#151](https://github.com/FlanChanXwO/pixiv-cli/pull/151)）
- `pixiv --licenses` 输出可执行文件内嵌的第三方许可声明。（[#147](https://github.com/FlanChanXwO/pixiv-cli/pull/147)）

## 变更

- 动图 GIF/APNG 编码改为流式纯 Go 实现；发布二进制以 `CGO_ENABLED=0` 构建，源码构建不再需要 Rust 或 C 链接器。帧顺序、时长、透明度、缩放和原子写出保持不变。（[#147](https://github.com/FlanChanXwO/pixiv-cli/pull/147)）
- README 不再包含 Telegram 群组徽章和链接。（[#134](https://github.com/FlanChanXwO/pixiv-cli/pull/134), [#150](https://github.com/FlanChanXwO/pixiv-cli/pull/150)）
- 配置路径、别名、环境变量覆盖与默认值改为由结构体字段标签生成；现有键名、环境变量、默认值和生成的 `config.toml` 内容保持兼容。（[#133](https://github.com/FlanChanXwO/pixiv-cli/pull/133)）

## 修复

- 作品搜索与详情输出在已知时保留上游的 `is_bookmarked`、`is_muted`、`visible`、`sanity_level` 和 `restriction_attributes`；深页搜索从请求的上游 offset 开始，不再重复抓取前面的批次。（[#131](https://github.com/FlanChanXwO/pixiv-cli/pull/131)）
- MCP `tools/list` 始终把工具的 `required` 编码为数组；严格校验的客户端不再因某个工具没有必填参数而拒绝该服务。（[#135](https://github.com/FlanChanXwO/pixiv-cli/pull/135)）
- Homebrew、GHCR 和 ClawHub 发布器可以在 Release 失败后恢复，无需重建资产。（[#122](https://github.com/FlanChanXwO/pixiv-cli/pull/122), [#123](https://github.com/FlanChanXwO/pixiv-cli/pull/123)）

## 安全

- Pull request 自动化只获得所需的权限，并且在 pull request 的 base 分支变化后仍会运行检查。（[#124](https://github.com/FlanChanXwO/pixiv-cli/pull/124), [#125](https://github.com/FlanChanXwO/pixiv-cli/pull/125), [#137](https://github.com/FlanChanXwO/pixiv-cli/pull/137), [#138](https://github.com/FlanChanXwO/pixiv-cli/pull/138), [#144](https://github.com/FlanChanXwO/pixiv-cli/pull/144), [#148](https://github.com/FlanChanXwO/pixiv-cli/pull/148)）

## 新贡献者

- @amalimm 完成首次贡献。（[#135](https://github.com/FlanChanXwO/pixiv-cli/pull/135)）

**完整变更**：[v1.1.1...v1.2.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.1.1...v1.2.0)

<!-- internal:
  [#149](https://github.com/FlanChanXwO/pixiv-cli/pull/149)
  [#152](https://github.com/FlanChanXwO/pixiv-cli/pull/152)
-->
