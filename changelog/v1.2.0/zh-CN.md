# v1.2.0 — 2026-10-21

## 破坏性变更

- Pixiv 与 FANBOX 合并为同一个 MCP 连接器：`pixiv fanbox mcp` 由 `pixiv mcp` 取代；同类 MCP 工具通过 `kind`、`relation`、`source` 等显式选择参数合并（stdio 由 65 个减至 49 个）。旧工具名直接注销，不保留别名；请按[工具迁移表](https://github.com/FlanChanXwO/pixiv-cli/blob/v1.2.0/docs/zh-CN/mcp-tools.md#工具迁移)迁移。([#146](https://github.com/FlanChanXwO/pixiv-cli/pull/146))

## 新增

- `pixiv mcp --port` 以 OAuth 保护的 Streamable HTTP 提供同一套工具，支持 owner 授权确认/PKCE、`--host`/`--base-url`、辅助登录以及按能力授权的图片/动图分发；默认仍为 stdio，并保留本地文件下载。([#146](https://github.com/FlanChanXwO/pixiv-cli/pull/146))
- 为 HTTP 模式新增交互式 `pixiv_show_gallery` MCP App：缩略图网格与作品详情页，包含作者头像、支持缩放与渐进加载的全屏原图查看器、收藏操作、点击标签发起精确标签搜索，以及点击复制作品 ID。([#146](https://github.com/FlanChanXwO/pixiv-cli/pull/146)、[#151](https://github.com/FlanChanXwO/pixiv-cli/pull/151))
- 新增离线 `pixiv --licenses` 选项，输出可执行文件内嵌的第三方许可声明。([#147](https://github.com/FlanChanXwO/pixiv-cli/pull/147))

## 变更

- 动图 GIF/APNG 编码改为流式纯 Go 实现；发布二进制以 `CGO_ENABLED=0` 构建，源码构建不再需要 Rust 或 C 链接器。帧顺序、时长、透明度、缩放与原子写出保持不变。([#147](https://github.com/FlanChanXwO/pixiv-cli/pull/147))

## 修复

- 作品搜索与详情输出保留上游的 `is_bookmarked`、`is_muted`、`visible`、`sanity_level`、`restriction_attributes`，仅在未知时省略；无本地筛选的深页搜索直接从上游原始 offset 开始，不再重复抓取前面的批次。([#131](https://github.com/FlanChanXwO/pixiv-cli/pull/131))
- MCP `tools/list` 中输入 schema 的 `required` 始终编码为数组，严格校验的客户端不再因无必填参数的工具而拒绝该服务。([#135](https://github.com/FlanChanXwO/pixiv-cli/pull/135))

## 文档

- README 先新增、后移除 Telegram 群组徽章与链接。([#134](https://github.com/FlanChanXwO/pixiv-cli/pull/134)、[#150](https://github.com/FlanChanXwO/pixiv-cli/pull/150))

## 维护

- 以结构体字段标签作为配置路径、别名、环境变量覆盖与默认值的唯一来源，现有键名、环境变量、默认值与初始配置文件内容保持兼容。([#133](https://github.com/FlanChanXwO/pixiv-cli/pull/133))
- 修复下游发布器恢复、在 Linux 上验证 Homebrew，并加固可信变更分类与单调的 Homebrew 恢复。([#122](https://github.com/FlanChanXwO/pixiv-cli/pull/122)、[#123](https://github.com/FlanChanXwO/pixiv-cli/pull/123)、[#124](https://github.com/FlanChanXwO/pixiv-cli/pull/124))
- 恢复 PR 分拣自动化，收窄 PR metadata 权限，跨 check suite 查找 smoke worker，在修改 base 后运行 Quality，并让可信 smoke worker 支持纯 Go head。([#125](https://github.com/FlanChanXwO/pixiv-cli/pull/125)、[#137](https://github.com/FlanChanXwO/pixiv-cli/pull/137)、[#138](https://github.com/FlanChanXwO/pixiv-cli/pull/138)、[#144](https://github.com/FlanChanXwO/pixiv-cli/pull/144)、[#148](https://github.com/FlanChanXwO/pixiv-cli/pull/148))
- 限制新增永久测试、保持 PR 范围聚焦，并准备对应的 v1.2.0 发布元数据。([#149](https://github.com/FlanChanXwO/pixiv-cli/pull/149))

## 新贡献者

- @amalimm 在 [#135](https://github.com/FlanChanXwO/pixiv-cli/pull/135) 完成首次贡献

**完整变更**：[v1.1.1...v1.2.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.1.1...v1.2.0)
