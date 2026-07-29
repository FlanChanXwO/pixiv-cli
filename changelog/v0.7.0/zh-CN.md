# v0.7.0 — 2026-07-25

## 新增

- Release binary 更新与版本化安装器资产现在携带静态、已测试的免费 GitHub Release-source 列表；它们在本机探测可用路由，支持路径式和查询参数式代理模板，且绝不远程拉取镜像列表。
- 现有 `detail` 与 `download` 输入现在接受严格的官方 Pixiv 作品 URL；`download` 也接受已认证作者主页和作品页 URL，以遍历全部插画、漫画和 ugoira，但不下载小说。
- MCP `illust_detail` 现在恰好接受 ID 或 URL 之一，`download` 接受 URL；插画查询 tool 会在紧凑文本摘要之外返回 typed structured result。
- 插画 `search` 与 MCP `search_illust` 现支持官方 App 的 `keyword` 搜索（标签、标题、说明文字）、包含边界的显式日期、公开收藏数边界，以及 `half-year` / `year` 快捷日期范围。收藏数边界同时需要 App OAuth 与有效的 Pixiv 高级会员。缺少 App OAuth 时，App 专属筛选会返回认证要求。
- 每个已公开 GitHub Release 现在都会向 SkillHub 提交同一 tag 对应的 `pixiv-cli` 产品 skill。提交前会先在本地校验，工作流必须取得 SkillHub 返回的 `skillId` 与审核状态才会成功。

## 变更

- 已配置的 `--proxy`、`https_proxy` 或 `HTTPS_PROXY` 继续优先于公共 Release 源。更新器保持规范 GitHub Release 身份和 Ed25519/SHA-256 验证；安装器在接受经代理下载的 archive 前仍直连 GitHub 取得 checksum。
- 下载 JSON 现在以 `{items, failures}` 报告规范作品 URL、ID、类型、页码和本地路径。部分失败会报告全部已完成结果并以非零退出；不会创建下载历史、缓存或跨次去重。
