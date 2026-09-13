# 未发布

> 此处用于准备发布说明。先审计目标 tag 范围，再直接编写下一版双语 Markdown；审计中的每个 PR 或 direct commit 都要出现在两种语言中。

## 新增

- 为 `pixiv search SOURCE` 与 Pixiv MCP `reverse_search` tool 新增反向搜图。CLI 会自动把显式 HTTP(S) URL 和现有常规文件识别为图片模式；SauceNAO、ascii2d color/BOVW 与 `all` provider 返回稳定 JSON envelope、通用 artwork/user record 以及 canonical record 的 NDJSON，并明确报告 provider partial 结果。([`69caa31`](https://github.com/FlanChanXwO/pixiv-cli/commit/69caa31)、[`6599dec`](https://github.com/FlanChanXwO/pixiv-cli/commit/6599dec)、[`ef0dcfe`](https://github.com/FlanChanXwO/pixiv-cli/commit/ef0dcfe)、[`e67e21f`](https://github.com/FlanChanXwO/pixiv-cli/commit/e67e21f)、[`959414e`](https://github.com/FlanChanXwO/pixiv-cli/commit/959414e)、[`ce03802`](https://github.com/FlanChanXwO/pixiv-cli/commit/ce03802)、[`298e0f3`](https://github.com/FlanChanXwO/pixiv-cli/commit/298e0f3))

## 安全

- 反向搜图每个 source 只加载一次到私有快照，并在 provider 工作结束后清理；发布输出和诊断不会包含 source 字符串、凭据、临时路径、cookie、CSRF/redirect 值或上游 body。MCP 契约有意允许私有文件以及私网/loopback/link-local URL，但只适用于可信本机 client，同时说明第三方上传、保存和 URL 缓存影响。([`69caa31`](https://github.com/FlanChanXwO/pixiv-cli/commit/69caa31)、[`3e2cb47`](https://github.com/FlanChanXwO/pixiv-cli/commit/3e2cb47)、[`80d5729`](https://github.com/FlanChanXwO/pixiv-cli/commit/80d5729)、[`8169787`](https://github.com/FlanChanXwO/pixiv-cli/commit/8169787)、[`4632334`](https://github.com/FlanChanXwO/pixiv-cli/commit/4632334)、[`4cfc4d4`](https://github.com/FlanChanXwO/pixiv-cli/commit/4cfc4d4))
- 在 public CLI/MCP 边界脱敏 reverse-search provider failure，只暴露经过审查的稳定 `code`/`message`；wrapped cause 与上游诊断保持私有。([`7505ae8`](https://github.com/FlanChanXwO/pixiv-cli/commit/7505ae8))

## 文档

- 在双语用户文档、维护者文档和产品 skill 中说明反向搜图 source 识别、provider/配置、stdin-only SauceNAO credential、第三方隐私影响、MCP 可信 client 边界、partial 结果、通用 artwork record 以及 opt-in 上游兼容性流程。([`d103eb4`](https://github.com/FlanChanXwO/pixiv-cli/commit/d103eb4)、[`9cf51d7`](https://github.com/FlanChanXwO/pixiv-cli/commit/9cf51d7))

## 配置与诊断

- 恢复统一的 `[logging].level`/`[logging].format` 配置及 `PIXIV_LOG_LEVEL`、`PIXIV_LOG_FORMAT` 覆盖；`debug`
  诊断只写 stderr，MCP stdout 继续保持 JSON-RPC。
- `pixiv config` 依据同一份 schema 管理 logging、下载目录、请求节奏、代理和账号池配置；首次生成的
  `config.toml` 由 schema 元数据自动生成，且不会覆盖已有文件。
- 新增 `reverse_search_provider` 与 `reverse_search_pixiv_only` 配置、仅限 stdin 且始终脱敏的
  `saucenao_api_key`，以及不显示 key 内容的 `SAUCENAO_API_KEY` 环境覆盖；public SDK 的构造与 API 保持不变。([`d4a1254`](https://github.com/FlanChanXwO/pixiv-cli/commit/d4a1254)、[`ce03802`](https://github.com/FlanChanXwO/pixiv-cli/commit/ce03802)、[`9cf51d7`](https://github.com/FlanChanXwO/pixiv-cli/commit/9cf51d7))
- reverse-search runtime 配置现明确区分 standard/source-SauceNAO 与 ascii2d proxy，说明 Chrome-146
  User-Agent/client-hint 配对，以及可选、仅用于 challenge 的 FlareSolverr JSON control 与独立 browser upstream
  proxy。native ascii2d image upload 仍是 multipart，provider 自身限制为 10 MB；不引入全局 1 MiB 压缩上传规则。
  ([`c01402a`](https://github.com/FlanChanXwO/pixiv-cli/commit/c01402a)、[`50d13a3`](https://github.com/FlanChanXwO/pixiv-cli/commit/50d13a3)、[`f9c1525`](https://github.com/FlanChanXwO/pixiv-cli/commit/f9c1525)、[`341cfbb`](https://github.com/FlanChanXwO/pixiv-cli/commit/341cfbb)、[`471af9b`](https://github.com/FlanChanXwO/pixiv-cli/commit/471af9b))

## 维护

- 修复 Pixiv 当前用户查询，改用有效的 `/v1/user/detail`；最新作品流支持 `max_illust_id` 分页；当 CDN 的 `.png` URL 实际返回 JPEG 时，修正缩略图文件扩展名与 MCP MIME 元数据。
- 加固 FANBOX identity-scoped cursor 绑定（Home、Supporting、Creators）到已验证的 FANBOX 账号 ID，使在一个账号下生成的 cursor 不能被重放到另一个账号的流上；CreatorPosts 与 TaggedPosts 仍是公开作用域。([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))
- 将嵌入 URL 的 FANBOX resource ref 替换为只含稳定 identity（kind、所属 creator/post、attachment id）的信封；`OpenResource`/`SaveResource` 在 session 内无 locator 缓存时从可信 metadata 重新解析新鲜且经 allowlist 校验的 locator，session cookie 只发送给需要凭据的 `downloads.fanbox.cc` host。([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))
- 修复逻辑分页 `has_more`：当某批次被 limit 截断时，即使上游 cursor 已空也保持 true，覆盖共享 traversal 引擎与 FANBOX MCP runtime。([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))
- 限制下载页码范围的展开幅度，并使目录模板段拒绝绝对路径、空段或穿越段；直接 resource 源的文件名改用完整 ref 的摘要，而非会因不同 ref 共享前缀而互相覆盖的截断前缀。([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))
- 把非 original 下载质量映射到对应 artwork variant resource，使 `SaveResource` 重新解析正确 locator；文件名生成在遇到未知占位符、不匹配花括号或缺失 `{date}` 值时提前失败，而不是写出空文件名。([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))
- 下载现在报告部分成功：每个作品原子写入其文件，单个作品的独立失败以失败集合返回而非中止整批，只有 context 取消才会立即停止。([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))
- 账号移除现在默认在 TTY 上确认，移除默认账号后自动重新选中第一个剩余账号。([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))
- 新增默认关闭的 `PIXIV_REVERSE_SEARCH_E2E=1` 维护脚本，用于在获授权环境观察真实 provider 兼容性；source 和 key 只从私有环境提供，绝不作为命令参数传入，且不属于普通 release 门禁。([`d103eb4`](https://github.com/FlanChanXwO/pixiv-cli/commit/d103eb4))
