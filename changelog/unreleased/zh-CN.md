# 未发布

> Release-prep 暂存区：审计目标 tag 范围后直接写入下一版双语说明。审计范围内的每个 PR 或 direct commit 都需出现在两种语言中；仅内部改动列在末尾的 `internal` 注释里，渲染 Release 正文时不会显示。

## 破坏性变更

- SDK：`Artwork.Kind`/`RawKind` 与 `ArtworkKind` 合并为 `ContentType`（`illust`、`manga`、`ugoira`），未知值原样保留；空标签译名和缺失的头像/封面字段省略。
- CLI JSON：内容列表改为 `{records, pagination}`，ID 为字符串；`recommended --type all` 按 `artworks`、`novels`、`users` 分区。账号摘要删除 `has_token`、`premium_status_checked_at`、`warning`、`eligible`，以及列表/bundle 导入结果中重复的 `default_user_id`。
- NDJSON：作品行使用 `type:"artwork"`，子类型筛选改用 `content_type`；未知上游作品类型保留并可继续处理。管道诊断删除固定的 `kind:"record_error"`。
- MCP 参数：实体 `kind` 改为 `type`；`pixiv_recommended` 使用 `type=all|artwork|novel|user`，作品可另设 `content_type=all|illust|manga`，结果按实体分区。`fanbox_creators.kind` 与 CLI `fanbox creators --kind` 改为 `relation` / `--relation`。


- `pixiv mcp` 不再接受 `mcp_gallery_cache_ttl` 与 `mcp_gallery_concurrency` 设置，请从 `config.toml` 中删除。（[#168](https://github.com/FlanChanXwO/pixiv-cli/pull/168)）

## 新增

- 新增 `pixiv whoami`，无需联网即可显示当前本地 Pixiv 账号和缓存的会员状态；`--json` 输出与 `auth list --json` 的账号项一致。
- MCP 新增 `pixiv_show_artwork` 工具，Gallery 布局会适配手机屏幕和嵌入式面板。（[#167](https://github.com/FlanChanXwO/pixiv-cli/pull/167)）

## 变更

- CLI 文本输出统一使用账号身份、状态标签、无标题的单行列表与详情卡片；空结果在 stderr 输出 `No results.`，退出码为 0。对于支持 `--json` 或 `--ndjson` 的命令，解析文本的脚本请改用这些选项，机器输出的迁移见上方破坏性变更。
- `pixiv fanbox download` 成功时 stdout 为空，不再打印保存路径；该命令没有 `--json` 或 `--ndjson` 选项。

- JSON 日志恢复 ERROR 命令失败记录；debug 日志增加 SDK 错误创建位置和安全的 typed 网络细节，覆盖 OAuth 换 token 失败。

- CLI 写到 stderr 的日志改为 `[HH:MM:SS.mmm] [operation] [LEVEL] [上级目录/文件:行号]: 消息 key=value`，按终端着色，转义控制字符并缩进多行值；JSON 日志带 `source`。默认文本日志只保留一行错误。
- 收藏读取部分失败时，MCP 搜索筛选条件会保留，并报告部分结果，不再丢弃筛选条件或已成功读取的条目。（[#165](https://github.com/FlanChanXwO/pixiv-cli/pull/165)）


## 修复

- CLI 文本输出将上游标题、用户名、评论与标签转义为单行；FANBOX 创作者和帖子命令会报告输出写入失败。
- CLI 错误以人读文本说明登录、参数、上游 HTTP 状态、超时与取消，并给出下一步；本地枚举参数先于打开账号校验。
- 未保存账号时，`pixiv auth list --json` 输出 `"accounts": []`。

**完整变更**：打下一版 tag 时补充。

<!-- internal:
  [#230](https://github.com/FlanChanXwO/pixiv-cli/pull/230)
  [#231](https://github.com/FlanChanXwO/pixiv-cli/pull/231)
  [#228](https://github.com/FlanChanXwO/pixiv-cli/pull/228)
  [#229](https://github.com/FlanChanXwO/pixiv-cli/pull/229)
  [#227](https://github.com/FlanChanXwO/pixiv-cli/pull/227)
  [#218](https://github.com/FlanChanXwO/pixiv-cli/pull/218)
  [#220](https://github.com/FlanChanXwO/pixiv-cli/pull/220)
  [#222](https://github.com/FlanChanXwO/pixiv-cli/pull/222)
  [#223](https://github.com/FlanChanXwO/pixiv-cli/pull/223)
  [#224](https://github.com/FlanChanXwO/pixiv-cli/pull/224)
  [#225](https://github.com/FlanChanXwO/pixiv-cli/pull/225)
  [#226](https://github.com/FlanChanXwO/pixiv-cli/pull/226)
  [#177](https://github.com/FlanChanXwO/pixiv-cli/pull/177)
  [#183](https://github.com/FlanChanXwO/pixiv-cli/pull/183)
  [#185](https://github.com/FlanChanXwO/pixiv-cli/pull/185)
  [#159](https://github.com/FlanChanXwO/pixiv-cli/pull/159)
  [#160](https://github.com/FlanChanXwO/pixiv-cli/pull/160)
  [#161](https://github.com/FlanChanXwO/pixiv-cli/pull/161)
  [#162](https://github.com/FlanChanXwO/pixiv-cli/pull/162)
  [#163](https://github.com/FlanChanXwO/pixiv-cli/pull/163)
  [#164](https://github.com/FlanChanXwO/pixiv-cli/pull/164)
  [#166](https://github.com/FlanChanXwO/pixiv-cli/pull/166)
  [#170](https://github.com/FlanChanXwO/pixiv-cli/pull/170)
  [#171](https://github.com/FlanChanXwO/pixiv-cli/pull/171)
  [#172](https://github.com/FlanChanXwO/pixiv-cli/pull/172)
  [#173](https://github.com/FlanChanXwO/pixiv-cli/pull/173)
  [#174](https://github.com/FlanChanXwO/pixiv-cli/pull/174)
  [#175](https://github.com/FlanChanXwO/pixiv-cli/pull/175)
  [#176](https://github.com/FlanChanXwO/pixiv-cli/pull/176)
  [#178](https://github.com/FlanChanXwO/pixiv-cli/pull/178)
  [#179](https://github.com/FlanChanXwO/pixiv-cli/pull/179)
  [#180](https://github.com/FlanChanXwO/pixiv-cli/pull/180)
  [#181](https://github.com/FlanChanXwO/pixiv-cli/pull/181)
  [#182](https://github.com/FlanChanXwO/pixiv-cli/pull/182)
  [#184](https://github.com/FlanChanXwO/pixiv-cli/pull/184)
  [#186](https://github.com/FlanChanXwO/pixiv-cli/pull/186)
  [#187](https://github.com/FlanChanXwO/pixiv-cli/pull/187)
  [#188](https://github.com/FlanChanXwO/pixiv-cli/pull/188)
  [#189](https://github.com/FlanChanXwO/pixiv-cli/pull/189)
  [#190](https://github.com/FlanChanXwO/pixiv-cli/pull/190)
  [#191](https://github.com/FlanChanXwO/pixiv-cli/pull/191)
  [#192](https://github.com/FlanChanXwO/pixiv-cli/pull/192)
  [#193](https://github.com/FlanChanXwO/pixiv-cli/pull/193)
  [#194](https://github.com/FlanChanXwO/pixiv-cli/pull/194)
  [#195](https://github.com/FlanChanXwO/pixiv-cli/pull/195)
  [#196](https://github.com/FlanChanXwO/pixiv-cli/pull/196)
  [#197](https://github.com/FlanChanXwO/pixiv-cli/pull/197)
  [#198](https://github.com/FlanChanXwO/pixiv-cli/pull/198)
  [#199](https://github.com/FlanChanXwO/pixiv-cli/pull/199)
  [#200](https://github.com/FlanChanXwO/pixiv-cli/pull/200)
  [#202](https://github.com/FlanChanXwO/pixiv-cli/pull/202)
  [#203](https://github.com/FlanChanXwO/pixiv-cli/pull/203)
  [#204](https://github.com/FlanChanXwO/pixiv-cli/pull/204)
  [#205](https://github.com/FlanChanXwO/pixiv-cli/pull/205)
  [#206](https://github.com/FlanChanXwO/pixiv-cli/pull/206)
  [#207](https://github.com/FlanChanXwO/pixiv-cli/pull/207)
  [#208](https://github.com/FlanChanXwO/pixiv-cli/pull/208)
  [#209](https://github.com/FlanChanXwO/pixiv-cli/pull/209)
  [#210](https://github.com/FlanChanXwO/pixiv-cli/pull/210)
  [#211](https://github.com/FlanChanXwO/pixiv-cli/pull/211)
  [#212](https://github.com/FlanChanXwO/pixiv-cli/pull/212)
  [#213](https://github.com/FlanChanXwO/pixiv-cli/pull/213)
  [#214](https://github.com/FlanChanXwO/pixiv-cli/pull/214)
  [#215](https://github.com/FlanChanXwO/pixiv-cli/pull/215)
  [#216](https://github.com/FlanChanXwO/pixiv-cli/pull/216)
  [#217](https://github.com/FlanChanXwO/pixiv-cli/pull/217)
  [#219](https://github.com/FlanChanXwO/pixiv-cli/pull/219)
  [#221](https://github.com/FlanChanXwO/pixiv-cli/pull/221)
-->
