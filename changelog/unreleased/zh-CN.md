# 未发布

> 此处用于准备发布说明。先审计目标 tag 范围，再直接编写下一版双语 Markdown；审计中的每个 PR 或 direct commit 都要出现在两种语言中。

## 配置与诊断

- 恢复统一的 `[logging].level`/`[logging].format` 配置及 `PIXIV_LOG_LEVEL`、`PIXIV_LOG_FORMAT` 覆盖；`debug`
  诊断只写 stderr，MCP stdout 继续保持 JSON-RPC。
- `pixiv config` 依据同一份 schema 管理 logging、下载目录、请求节奏、代理和账号池配置；首次生成的
  `config.toml` 由 schema 元数据自动生成，且不会覆盖已有文件。

## 维护

- 修复 Pixiv 当前用户查询，改用有效的 `/v1/user/detail`；最新作品流支持 `max_illust_id` 分页；当 CDN 的 `.png` URL 实际返回 JPEG 时，修正缩略图文件扩展名与 MCP MIME 元数据。
- 加固 FANBOX identity-scoped cursor 绑定（Home、Supporting、Creators）到已验证的 FANBOX 账号 ID，使在一个账号下生成的 cursor 不能被重放到另一个账号的流上；CreatorPosts 与 TaggedPosts 仍是公开作用域。([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))
- 将嵌入 URL 的 FANBOX resource ref 替换为只含稳定 identity（kind、所属 creator/post、attachment id）的信封；`OpenResource`/`SaveResource` 在 session 内无 locator 缓存时从可信 metadata 重新解析新鲜且经 allowlist 校验的 locator，session cookie 只发送给需要凭据的 `downloads.fanbox.cc` host。([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))
- 修复逻辑分页 `has_more`：当某批次被 limit 截断时，即使上游 cursor 已空也保持 true，覆盖共享 traversal 引擎与 FANBOX MCP runtime。([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))
- 限制下载页码范围的展开幅度，并使目录模板段拒绝绝对路径、空段或穿越段；直接 resource 源的文件名改用完整 ref 的摘要，而非会因不同 ref 共享前缀而互相覆盖的截断前缀。([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))
- 把非 original 下载质量映射到对应 artwork variant resource，使 `SaveResource` 重新解析正确 locator；文件名生成在遇到未知占位符、不匹配花括号或缺失 `{date}` 值时提前失败，而不是写出空文件名。([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))
- 下载现在报告部分成功：每个作品原子写入其文件，单个作品的独立失败以失败集合返回而非中止整批，只有 context 取消才会立即停止。([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))
- 账号移除现在默认在 TTY 上确认，移除默认账号后自动重新选中第一个剩余账号。([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))
