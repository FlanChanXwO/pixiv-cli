# API 迁移 contract 与兼容验证台账

原观测日期：2026-09-05；修订日期：2026-09-07（Asia/Shanghai）。

## 当前门禁来源

本文件是 contract/compatibility 工作清单；当前状态及发布授权只见 [能力准入表](capability-admission.md)。历史 upstream 证据不意味着 migration_ready；该状态必须同时满足 contract freeze 与 T12 兼容决策。T39A 冻结后才能实施 CLI/MCP 迁移。未发布实现允许同步编写导出 SDK、注册、测试和文档，发布仍须 public_ready。

所有 required_scope 均须完成；阻塞能力停止发布但保持 Goal incomplete，不能自行移除范围。

## 已确认的迁移依据

| 能力 | 旧 contract | 目标 contract | Upstream evidence | 当前执行前提 | 结论 |
| --- | --- | --- | --- | --- | --- |
| Novel latest cursor | `offset` | `max_novel_id` | wire/response/两页已确认 | contract + T12 pending | 冻结后 TDD 修复；修复后再过 adapter/SDK gate |
| Novel detail | `/v1/novel/detail` | `/v2/novel/detail` | v1 rejected；v2 wire/response 已确认；无分页 | contract + T12 pending | 冻结后 TDD 迁移；必须保留 v2 series 字段 |
| Comment add/reply/delete wire | 无生产 operation | `/v1/*/comment/add\|delete` | 真实写入、读回和删除已有 evidence | contract + T12 pending | 冻结后写 adapter/SDK red tests；未通过前不公开 |
| Stamps read | 无生产 operation | `/v1/stamps` | wire/response 已确认；无分页 | contract + T12 pending | 冻结后写 adapter/SDK red tests |
| Novel ranking | 无生产 operation | `/v1/novel/ranking` | wire/response/两页已确认 | contract + T12 pending | 冻结后写 adapter/SDK red tests |

## 尚未达到迁移门禁

| 能力 | 候选 contract | 缺失证据 | 当前状态 | 实施规则 |
| --- | --- | --- | --- | --- |
| Novel series | `/v2/novel/series` + `last_order` | 真实第二页；candidate adapter；SDK | inconclusive | 不切生产 path |
| Novel comments v3 | `/v3/novel/comments` | 非空响应；第二页；adapter；SDK | inconclusive | 不从 v2 迁移 |
| Artwork comments v3 DTO | `/v3/illust/comments` | 非空 live DTO；`date`；numeric access control；第二页 | inconclusive / fixture mismatch | 不冻结 DTO |
| Artwork recommended continuation | `/v1/illust/recommended` | 完整 continuation 参数；成功第二页 | inconclusive | 不替换 cursor model |
| Novel search period | `/v1/search/novel` + `start_date/end_date` | 未进入 strict manifest；live 两页；adapter；SDK | not tested | 不删除 `Duration` |
| Novel bookmark tags | `/v1/user/bookmark-tags/novel` | wire/response；空列表；adapter；SDK | not tested | contract snapshot 前不新增 public operation |
| Novel bookmark detail | `/v2/novel/bookmark/detail` | wire/response；public/private 状态；adapter；SDK | not tested | contract snapshot 前不新增 public operation |
| Novel bookmark add/delete | `/v2/novel/bookmark/add` + `/v1/novel/bookmark/delete` | 隔离写入、detail、list、tags、恢复原状态；adapter；SDK | not tested | contract snapshot 前不新增 public mutation |
| Artwork bookmark tags | `/v1/user/bookmark-tags/illust` | wire/response；subtype 语义；adapter；SDK | not tested | 不扩展 subtype tags |
| Artwork bookmark subtype | `/v1/user/bookmarks/illust` + `type/content_type` | 参数是否被忽略；logical pagination | not tested | 默认只能 client-side 候选 |
| Recommended subtype | `/v1/illust/recommended` + `content_type` | illust/manga/ugoira 各两页 | not tested | 不公开 ugoira subtype |
| Latest subtype expansion | `/v1/illust/new` + compound `content_type` | manga/ugoira/组合值各两页 | partial | 只保留已确认的类型 |
| Novel comment total | v2/v3 comments | 非空 `total_comments`；是否需要 include 参数 | inconclusive | total 保持非强保证 |
| Artwork comment total | v3 comments | 非空 `total_comments`；是否需要 include 参数 | inconclusive | total 保持非强保证 |
| Bare ID probe | 多资源 detail endpoint | namespace 冲突；403/404 分类；请求成本 | not tested | 保持显式 `--type` |

## 已拒绝或排除

| 能力 | Contract | 结论 | 处理 |
| --- | --- | --- | --- |
| Novel detail v1 | `/v1/novel/detail` | rejected | 删除依赖，不做 fallback |
| Novel series v1 | `/v1/novel/series` | rejected | 删除依赖，不做 fallback |
| Novel content App API | `/v1/novel/content` | rejected | 不再承诺正文读取 |
| Novel WebView content | `/webview/v2/novel` | excluded | 不进入本 goal 默认实现 |
| Server-side rating | `x_restrict` request filter | rejected | 只允许 client-side filter |


## T12：SDK symbol map 必交付列

逐个记录旧 method、request/model、named field type、替代 method、wrapper/deprecation、预期错误和旧消费者编译测试。默认保持源码兼容：AddBookmark/RemoveBookmark 与新增 artwork 方法并存并委托同一实现。废弃 endpoint 不等于删除符号；novel content 可保留 deprecated 入口并明确返回不支持，具体错误冻结后测试，不能继续请求已拒绝 endpoint。

本轮搜索兼容决定：增加 CheckpointSearchArtworks 与可选 CursorContext，保留既有方法和 named types；仅 SearchArtworks binding version=2，旧搜索 cursor InvalidCursor，清除后重新开始；其他 operation 和 sdk.Cursor 外层格式不变。未校验身份的 New/NewWith client 只允许同实例续读；Open/OpenWith 使用 verified identity。

## T39A：MCP compatibility map 必交付列

旧 tool → 新 operation；逐项 input 字段名/必填/default、output 字段/shape、structured error/isError。add_bookmark 的 illust_id 等旧 wire 契约不能被通用 TARGET 替代。每项列出旧 JSON 请求 fixture 与回放断言；CLI alias 验证不能替代它。

## T09A：mutation transport 与结果

现有 PostForm 返回 error，2xx 不交付响应 body。先增加响应可解码的窄能力供 comment adapter 取得本轮 ID；保留 bookmark/follow 的旧 PostForm。request/response/读回共享同账号执行上下文。

明确未成功、已取得 ID 但读回失败、写入结果不确定三类结果必须可区分；后两类禁止自动重放。无法可靠取得 ID 时不能猜测最近评论并删除，清理只接受本轮 ID；保留真实失败与需要人工处理的状态。无匿名 fallback，无通用 mutation 自动重试。
