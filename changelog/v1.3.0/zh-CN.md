# v1.3.0 — 2026-10-31

## 破坏性变更

- **Go SDK：** 删除 `XxxDTO` 类型及 `ToXxxDTO` 转换函数，改用可直接序列化的公开实体模型。`AddBookmark` / `RemoveBookmark` 改用 `AddArtworkBookmark` / `RemoveArtworkBookmark`；移除 `SetAIArtworkVisibility`。`Artwork.Kind` / `RawKind` 与 `ArtworkKind` 改用 `Artwork.ContentType` 和 `ContentType`（`illust`、`manga`、`ugoira`），未知上游值原样保留。JSON 省略空标签译名及缺失的头像、封面字段。（[#170](https://github.com/FlanChanXwO/pixiv-cli/pull/170)、[#271](https://github.com/FlanChanXwO/pixiv-cli/pull/271)）
- **CLI JSON 与 NDJSON：** 内容列表统一为 `{records, pagination}`，ID 使用字符串。作品记录统一使用 `type:"artwork"`，子类型通过 `content_type` 筛选。`recommended --type all` 按 `artworks`、`novels`、`users` 分区返回。管道诊断删除 `kind:"record_error"`。读取这些输出的脚本需要同步调整。（[#271](https://github.com/FlanChanXwO/pixiv-cli/pull/271)）
- **MCP 与 FANBOX 选择参数：** MCP 实体选择参数由 `kind` 改为 `type`。`pixiv_recommended` 接受 `type=all|artwork|novel|user`，作品子类型通过 `content_type=all|illust|manga` 选择，结果按实体分区。`fanbox_creators.kind` 改为 `relation`，CLI `fanbox creators --kind` 改为 `--relation`。（[#271](https://github.com/FlanChanXwO/pixiv-cli/pull/271)）
- **账号 JSON：** 认证摘要删除 `has_token`、`premium_status_checked_at`、`warning`、`eligible`；账号列表与 bundle 导入结果删除重复的 `default_user_id`。空账号列表的 `auth list --json` 输出为 `"accounts":[]`。依赖上述字段的调用方需要同步调整。（[#271](https://github.com/FlanChanXwO/pixiv-cli/pull/271)）
- **Gallery 配置：** 移除 `mcp_gallery_cache_ttl` 与 `mcp_gallery_concurrency`，缓存有效期固定为 15 分钟，元数据与图片的读取并发上限各固定为 4。请从 `config.toml` 删除旧设置；加载配置时会忽略残留的未声明键。（[#168](https://github.com/FlanChanXwO/pixiv-cli/pull/168)、[#185](https://github.com/FlanChanXwO/pixiv-cli/pull/185)）
- 移除无意义的 `pixiv --licenses` 命令及随包分发的第三方许可证汇总；请删除脚本中对该参数的调用。（[#234](https://github.com/FlanChanXwO/pixiv-cli/pull/234)）

## 新增

- 新增 `pixiv whoami [--json]`，直接显示当前本地 Pixiv 账号与缓存的会员状态，无需联网或轮换 token；JSON 输出与 `auth list --json` 的账号项一致。（[#260](https://github.com/FlanChanXwO/pixiv-cli/pull/260)）
- HTTP MCP 新增 `pixiv_show_artwork` 工具，用于展示单个作品；Gallery 适配手机与嵌入式面板，并增加立方体翻页效果。（[#167](https://github.com/FlanChanXwO/pixiv-cli/pull/167)）

## 变更

- 终端输出统一使用易读的账号身份、状态标签、单行列表与详情卡片。空结果向 stderr 输出 `No results.` 并成功退出；`fanbox download` 成功时 stdout 保持为空。解析文本的脚本请优先使用命令支持的 `--json` 或 `--ndjson`，并按上方说明迁移机器输出。（[#269](https://github.com/FlanChanXwO/pixiv-cli/pull/269)、[#270](https://github.com/FlanChanXwO/pixiv-cli/pull/270)）
- 精简 stderr 日志，显示时间、操作、级别与源码位置。JSON 日志包含命令失败记录；debug 日志补充 SDK 错误创建位置与安全的网络细节，覆盖 OAuth 换 token 失败。默认文本日志只保留一行错误。（[#258](https://github.com/FlanChanXwO/pixiv-cli/pull/258)、[#261](https://github.com/FlanChanXwO/pixiv-cli/pull/261)）

## 修复

- MCP 搜索显式指定收藏策略、未指定收藏数区间时，仍保留作品筛选条件；收藏数区间与作品筛选在分页前共同生效。CLI 与 MCP 的 `best_effort` 搜索即使候选为空或已耗尽，也会报告 `partial`。（[#165](https://github.com/FlanChanXwO/pixiv-cli/pull/165)）
- 系列、收藏、评论、反向搜图等自动选择 NDJSON 的命令，在下游提前关闭管道时正常退出。（[#246](https://github.com/FlanChanXwO/pixiv-cli/pull/246)）
- 转义终端输出中的上游标题、用户名、评论与标签；登录、参数、HTTP、超时与取消错误提供可读说明及下一步，本地枚举参数先于打开账号校验。FANBOX 创作者与帖子命令会报告输出写入失败。（[#268](https://github.com/FlanChanXwO/pixiv-cli/pull/268)）

**完整变更：** [v1.2.1...v1.3.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.2.1...v1.3.0)

<!-- internal:
  [#159](https://github.com/FlanChanXwO/pixiv-cli/pull/159)
  [#160](https://github.com/FlanChanXwO/pixiv-cli/pull/160)
  [#161](https://github.com/FlanChanXwO/pixiv-cli/pull/161)
  [#162](https://github.com/FlanChanXwO/pixiv-cli/pull/162)
  [#163](https://github.com/FlanChanXwO/pixiv-cli/pull/163)
  [#164](https://github.com/FlanChanXwO/pixiv-cli/pull/164)
  [#166](https://github.com/FlanChanXwO/pixiv-cli/pull/166)
  [#171](https://github.com/FlanChanXwO/pixiv-cli/pull/171)
  [#172](https://github.com/FlanChanXwO/pixiv-cli/pull/172)
  [#173](https://github.com/FlanChanXwO/pixiv-cli/pull/173)
  [#174](https://github.com/FlanChanXwO/pixiv-cli/pull/174)
  [#175](https://github.com/FlanChanXwO/pixiv-cli/pull/175)
  [#176](https://github.com/FlanChanXwO/pixiv-cli/pull/176)
  [#177](https://github.com/FlanChanXwO/pixiv-cli/pull/177)
  [#178](https://github.com/FlanChanXwO/pixiv-cli/pull/178)
  [#179](https://github.com/FlanChanXwO/pixiv-cli/pull/179)
  [#180](https://github.com/FlanChanXwO/pixiv-cli/pull/180)
  [#181](https://github.com/FlanChanXwO/pixiv-cli/pull/181)
  [#182](https://github.com/FlanChanXwO/pixiv-cli/pull/182)
  [#183](https://github.com/FlanChanXwO/pixiv-cli/pull/183)
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
  [#201](https://github.com/FlanChanXwO/pixiv-cli/pull/201)
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
  [#218](https://github.com/FlanChanXwO/pixiv-cli/pull/218)
  [#219](https://github.com/FlanChanXwO/pixiv-cli/pull/219)
  [#220](https://github.com/FlanChanXwO/pixiv-cli/pull/220)
  [#221](https://github.com/FlanChanXwO/pixiv-cli/pull/221)
  [#222](https://github.com/FlanChanXwO/pixiv-cli/pull/222)
  [#223](https://github.com/FlanChanXwO/pixiv-cli/pull/223)
  [#224](https://github.com/FlanChanXwO/pixiv-cli/pull/224)
  [#225](https://github.com/FlanChanXwO/pixiv-cli/pull/225)
  [#226](https://github.com/FlanChanXwO/pixiv-cli/pull/226)
  [#227](https://github.com/FlanChanXwO/pixiv-cli/pull/227)
  [#228](https://github.com/FlanChanXwO/pixiv-cli/pull/228)
  [#229](https://github.com/FlanChanXwO/pixiv-cli/pull/229)
  [#230](https://github.com/FlanChanXwO/pixiv-cli/pull/230)
  [#231](https://github.com/FlanChanXwO/pixiv-cli/pull/231)
  [#232](https://github.com/FlanChanXwO/pixiv-cli/pull/232)
  [#233](https://github.com/FlanChanXwO/pixiv-cli/pull/233)
  [#235](https://github.com/FlanChanXwO/pixiv-cli/pull/235)
  [#236](https://github.com/FlanChanXwO/pixiv-cli/pull/236)
  [#237](https://github.com/FlanChanXwO/pixiv-cli/pull/237)
  [#238](https://github.com/FlanChanXwO/pixiv-cli/pull/238)
  [#239](https://github.com/FlanChanXwO/pixiv-cli/pull/239)
  [#240](https://github.com/FlanChanXwO/pixiv-cli/pull/240)
  [#241](https://github.com/FlanChanXwO/pixiv-cli/pull/241)
  [#242](https://github.com/FlanChanXwO/pixiv-cli/pull/242)
  [#243](https://github.com/FlanChanXwO/pixiv-cli/pull/243)
  [#244](https://github.com/FlanChanXwO/pixiv-cli/pull/244)
  [#245](https://github.com/FlanChanXwO/pixiv-cli/pull/245)
  [#247](https://github.com/FlanChanXwO/pixiv-cli/pull/247)
  [#248](https://github.com/FlanChanXwO/pixiv-cli/pull/248)
  [#249](https://github.com/FlanChanXwO/pixiv-cli/pull/249)
  [#250](https://github.com/FlanChanXwO/pixiv-cli/pull/250)
  [#251](https://github.com/FlanChanXwO/pixiv-cli/pull/251)
  [#252](https://github.com/FlanChanXwO/pixiv-cli/pull/252)
  [#253](https://github.com/FlanChanXwO/pixiv-cli/pull/253)
  [#254](https://github.com/FlanChanXwO/pixiv-cli/pull/254)
  [#255](https://github.com/FlanChanXwO/pixiv-cli/pull/255)
  [#256](https://github.com/FlanChanXwO/pixiv-cli/pull/256)
  [#257](https://github.com/FlanChanXwO/pixiv-cli/pull/257)
  [#259](https://github.com/FlanChanXwO/pixiv-cli/pull/259)
  [#262](https://github.com/FlanChanXwO/pixiv-cli/pull/262)
  [#263](https://github.com/FlanChanXwO/pixiv-cli/pull/263)
  [#264](https://github.com/FlanChanXwO/pixiv-cli/pull/264)
  [#265](https://github.com/FlanChanXwO/pixiv-cli/pull/265)
  [#266](https://github.com/FlanChanXwO/pixiv-cli/pull/266)
  [#267](https://github.com/FlanChanXwO/pixiv-cli/pull/267)
-->
