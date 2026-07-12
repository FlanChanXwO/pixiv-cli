# Pixiv 对接可替换化 PRD

## 问题陈述

当前项目的图库采集、作品入库、图片代理、采集源探测、过滤规则和管理后台配置都围绕 Pixiv 上游协议展开。虽然项目已经通过 Pixiv 协议层集中 HTTP 请求，但业务层仍然直接使用 Pixiv 语义，例如 PID、UID、illust type、xRestrict、ugoira、i.pximg.net 图片路径、Ajax envelope、Cookie 登录态和 Pixiv 特定采集模式。

用户希望把当前项目需要用到的 Pixiv 接口数据和所需接口完整导出成产品需求文档，让 Pixiv 对接部分可以被一个新实现完全替换掉。替换后的实现可以继续访问 Pixiv，也可以接入兼容 Pixiv 语义的代理、镜像、聚合服务或自建内容源，但必须向项目内部提供稳定、可测试、可观测的同等能力。

## 解决方案

建设一个可替换的“作品来源 Provider”能力层，把外部上游协议与项目内部图库领域模型分离。Provider 负责完成认证、上游访问、列表发现、作品详情、分页图片、ugoira 元数据、图片资源代理和轻量探测；项目内部只依赖稳定的作品数据契约、采集源契约和资源代理契约。

替换目标不是删除项目里的 Pixiv 领域数据表或公开 API 语义，而是把“如何从 Pixiv 或兼容源取得这些数据”变成可替换实现。现有 `pixiv_app_api`、`pixiv_crawler`、`rss_feed` 可以先作为默认 Provider 的兼容实现，后续允许新增另一套 Provider 并在配置中切换。

## 用户故事

1. 作为自托管图库管理员，我希望 Pixiv 上游对接可以被替换，这样即使 Pixiv Web Ajax 行为变化，图库也能继续运行。
2. 作为自托管图库管理员，我希望替换 Provider 能提供同等作品元数据，这样图库、随机图、审核和管理后台不需要产品级重写。
3. 作为自托管图库管理员，我希望采集发现模式被明确记录，这样切换前可以确认替换 Provider 支持当前配置的采集源。
4. 作为自托管图库管理员，我希望继续支持用户收藏发现，这样我维护的 Pixiv 收藏仍可进入图库。
5. 作为自托管图库管理员，我希望继续支持用户作品发现，这样指定作者 UID 的作品仍可入库。
6. 作为自托管图库管理员，我希望继续支持关键词搜索发现，这样基于 tag 或关键词的采集仍可使用。
7. 作为自托管图库管理员，我希望继续支持排行榜发现，这样每日、每周或每月趋势内容仍可导入。
8. 作为自托管图库管理员，我希望继续支持关注用户新作发现，这样 Provider 拥有登录态时仍可发现关注创作者的新作品。
9. 作为自托管图库管理员，我希望继续支持 crawler 或 feed 形式的 PID 发现，这样备用来源仍可通过链接找到作品。
10. 作为自托管图库管理员，我希望采集源探测使用同一套 Provider 契约，这样启用采集源前能证明替换实现真的可用。
11. 作为自托管图库管理员，我希望探测错误暴露真实的上游或配置原因，这样失败不会被伪装成空结果。
12. 作为自托管图库管理员，我希望替换配置支持必要的 user agent、代理和凭据设置，这样部署约束仍可管理。
13. 作为自托管图库管理员，我希望凭据与 source params、前端展示隔离，这样替换工作不会泄露 cookie、token 或其他机密。
14. 作为自托管图库管理员，我希望既有 source budget 继续生效，这样替换 Provider 不会意外过量抓取。
15. 作为自托管图库管理员，我希望 source cursor 保持为 Provider 自有 JSON 状态，这样每种发现模式都能安全续抓。
16. 作为图库用户，我希望作品标题、作者、标签和页数保持准确，这样替换后搜索和展示行为不变。
17. 作为图库用户，我希望 R18 过滤继续生效，这样公开接口仍遵守既有安全设置。
18. 作为图库用户，我希望 AI 生成作品过滤继续生效，这样已配置的收藏质量规则被保留。
19. 作为图库用户，我希望缩略图和原图继续通过本服务加载，这样客户端不需要直连上游 CDN。
20. 作为图库用户，我希望多页作品保持完整顺序，这样漫画和组图不会丢页或乱序。
21. 作为图库用户，我希望 ugoira 作品保留 zip URL 和帧元数据，这样动图后续仍可渲染或处理。
22. 作为管理后台用户，我希望采集源页面继续显示相同的 source type、params、filters、budget 和最近运行状态，这样替换 Provider 对日常运维尽量透明。
23. 作为管理后台用户，我希望探测预览包含样例 PID 或样例作品，这样保存采集源前可以快速核对结果。
24. 作为审核 worker，我希望入库页面保留宽高、扩展名和原图 URL，这样 NSFW 审核仍可抓取并评估每一页。
25. 作为审核 worker，我希望审核队列仍随入库页面创建，这样替换 Provider 不会绕过审核。
26. 作为采集编排器，我希望 adapter 只产出 PID hit 和轻量元数据，这样发现和详情入库仍保持分离。
27. 作为采集编排器，我希望 Provider 错误能区分作品不可用和系统失败，这样删除、私密或受限作品会被跳过而不是反复重试。
28. 作为采集编排器，我希望 stage 1 filter 在可用轻量元数据上执行，这样可以减少不必要的详情请求。
29. 作为入库管线，我希望详情数据包含 stage 2 filter 需要的全部字段，这样低质量或不允许的作品不会写库。
30. 作为入库管线，我希望单页和多页处理规则明确，这样缺失分页数据不会污染图片记录。
31. 作为入库管线，我希望上游可用时提供标签翻译，这样 tag alias 与搜索能力可继续受益。
32. 作为运维维护者，我希望所有外部作品 Provider HTTP 调用集中管理，这样可观测性、代理、限流和策略调整更容易。
33. 作为运维维护者，我希望 Provider 调用支持请求取消，这样服务关闭或人工取消时不会卡住入库。
34. 作为运维维护者，我希望图片代理校验继续严格，这样替换工作不会引入 SSRF 或任意抓取风险。
35. 作为运维维护者，我希望替换相关测试使用 mock Provider，这样 CI 不需要真实 Pixiv 凭据。
36. 作为后续开发者，我希望 Provider 接口是“小接口、深模块”，这样替换实现不需要改图库、审核、采集编排或控制器。
37. 作为后续开发者，我希望当前 Pixiv Ajax 响应字段与内部 Provider 数据字段有清晰映射，这样兼容实现不用重新阅读整套代码。
38. 作为后续开发者，我希望 Provider 不支持的能力被显式报告，这样部分替换不会静默丢掉 `following_new` 或 `ugoira` 等模式。
39. 作为后续开发者，我希望替换边界保留现有数据库语义，这样仅切换上游 Provider 不需要迁移数据表。
40. 作为后续开发者，我希望文档明确不做哪些事，这样替换工程不会膨胀成图库产品重设计。

## 实现决策

- 新建或抽取 Provider client 模块，由它持有全部外部作品源 HTTP 行为：请求构造、headers、凭据、代理、限流、状态映射、JSON/XML 解析和图片流式转发。
- 保持既有 gallery、audit、public API 和 admin API 基于当前内部 artwork model 工作。替换目标是上游访问层，不是已存图库领域。
- 保留两段式入库模型：source adapter 发现候选 PID 与轻量元数据；ingest pipeline 在写库前获取规范化详情、分页数据和 ugoira 元数据。
- 定义稳定的作品详情数据契约，必填字段为：作品 ID、标题、作者 ID、作者名、R18 标记或原始限制等级、AI 类型、作品类型、页数、收藏数、浏览数、上传时间、首图宽、首图高、首图原图 URL、标签和可选标签翻译。
- 定义稳定的作品分页数据契约，必填字段为：页序号、原图 URL、扩展名、宽、高。
- 定义稳定的 ugoira 元数据契约，必填字段为：作品 ID、可选预览源 URL、原始 zip URL、MIME 类型、原始帧列表；帧列表需包含每帧文件名和 delay。
- 定义稳定的发现作品数据契约，用于列表、搜索、排行榜、关注新作等端点，必填字段为：作品 ID、标题、作者 ID、作者名、限制等级、AI 类型、作品类型、页数、上传时间、宽、高、收藏数、标签，以及可选模式附加元数据，例如排名。
- 保留 Provider 的“作品不可用”语义：403、404、上游 `error=true`、删除作品、私密作品和地区受限作品必须映射为 artwork unavailable，而不是通用系统错误。
- 保留 Provider 的失败语义：网络失败、响应结构错误、异常状态码、凭据无效和代理配置错误必须作为可排查的上游或配置错误暴露。
- 当前必需上游能力：作品详情，等价于 Pixiv `GET /ajax/illust/{pid}`。
- 当前必需上游能力：作品分页，等价于 Pixiv `GET /ajax/illust/{pid}/pages`。
- 当前必需上游能力：ugoira 元数据，等价于 Pixiv `GET /ajax/illust/{pid}/ugoira_meta`。
- 当前必需上游能力：用户收藏，等价于 Pixiv `GET /ajax/user/{uid}/illusts/bookmarks`，需要支持 tag、offset、limit、rest、language 语义并能翻页获取可见收藏作品。
- 当前必需上游能力：用户作品，等价于 Pixiv `GET /ajax/user/{uid}/illusts`，需要 offset、limit 和 total 语义。
- 当前必需上游能力：作品搜索，等价于 Pixiv `GET /ajax/search/artworks/{word}`，需要 page、search target、sort、可选 duration、all-mode 和 language 语义。
- 当前必需上游能力：排行榜，等价于 Pixiv `GET /ajax/illust/ranking`，需要 ranking mode、date 和 page 语义。
- 当前必需上游能力：关注最新作品，等价于 Pixiv `GET /ajax/follow_latest/illust`，需要 page 和可见性 mode 语义；若 Provider 无法执行登录态发现，必须显式返回 unsupported。
- 当前必需资源能力：流式提供原图、master 缩略图、裁剪缩略图和 ugoira zip 资源。
- 当前可选回退能力：RSS/Atom feed 发现，可从 item link 或配置正则中提取 PID。
- 当前可选回退能力：crawler 发现，可从配置的 HTTP(S) 页面中用默认或自定义 PID pattern 提取 PID。
- 产品边界保持 source type 兼容：`pixiv_app_api`、`pixiv_crawler`、`rss_feed` 继续存在，除非后续迁移计划引入 provider-neutral alias。
- 保留 `pixiv_app_api` mode params：`bookmarks` 使用 `user_id` 和 `restrict`；`user_artworks` 使用 `user_id`；`search` 使用 `word`、`search_target`、`sort` 和 `duration`；`ranking` 使用 `mode_rank` 和 `date`；`following_new` 使用 `restrict`；所有模式都可使用 `limit`。
- 保留 source cursor 为 Provider 私有 JSON。当前 cursor 对 bookmarks/user artworks 需要 offset，对 search/ranking/following-new 需要 page。
- 保留 source budget：`max_pages` 限制翻页次数，`max_artworks` 限制接收 hit 数；0 保持既有“不限制”语义。
- 保留 source filters 及其数据依赖。Stage 1 需要 tags、R18、media type、author UID、create date。Stage 2 需要 bookmark count、view count、page count、dimensions、aspect ratio、AI type、title、可用时的 byte size、可用时的 ugoira frame count。
- 保留标签翻译行为。上游提供英文翻译时应合并到已存 tag translations；缺失翻译不能阻断入库。
- 保留图片 URL 派生行为。Provider 必须返回可转成本地代理路径和派生缩略图路径的上游兼容原图 URL，或提供具有同等公开行为的资源 URL 策略。
- 保留图片代理安全边界。仅允许代理经过批准的作品资源类别，请求 query、fragment、userinfo 不得绕过校验，响应头复制仍限制在内容和缓存相关字段。
- 保留凭据归一化为独立关注点。当前实现接受浏览器 Cookie header 字符串和 JSON cookie 导出；替换实现可新增 token 凭据，但必须避免把机密放进 source params 或前端回显。
- 保留实时 source probe 语义。Probe 返回 status、checks 和 preview；`ok`、`warning`、`error`、`unsupported` 的 UI 含义不变。
- 保留管理后台配置写回行为。编辑 sources 只写允许的运行时配置字段，读取时继续隐藏疑似机密 params 和 URL query 机密。
- Provider 模块应是深模块。公开接口保持小而稳定：发现列表模式、获取详情、获取分页、获取 ugoira 元数据、校验/构建/服务图片资源、归一化凭据、探测能力。
- Orchestrator 不应依赖 Provider 实现细节。它消费 source hits、cursors 和 ingest results，不直接理解 Ajax body 或原始 Pixiv URL。
- Ingest pipeline 不应依赖列表模式响应形状。它消费规范化 detail、page、ugoira 契约。
- 当前命名可为兼容性继续保留 Pixiv 数据库和表语义，但新代码边界不应把新的 Pixiv host、Ajax path 或 cookie 特定逻辑扩散到 Provider 实现外。
- 可观测性必须包含 source ID、source type、mode、PID、run ID、上游状态或错误类别、skip reason。
- 替换 Provider 必须可按部署配置，并在当前 Pixiv 设置可热加载的范围内支持热加载。

## 测试决策

- 好测试应验证 Provider 和 orchestration 边界的外部行为，而不是私有解析 helper 的实现细节；除非解析本身就是公开契约的一部分。
- Provider 契约测试应为每个必需端点等价能力使用本地 HTTP fixtures：详情、分页、ugoira 元数据、收藏、用户作品、搜索、排行榜、关注新作。
- Provider 错误测试应覆盖作品不可用、非 2xx 状态、JSON 结构错误、凭据 provider 失败、代理配置失败和上游 envelope 错误。
- Source adapter 测试应验证每个 `pixiv_app_api` mode 如何把 params、cursor、budget 映射到预期 Provider 调用，并产出预期 PID hits。
- Ingest pipeline 测试应验证规范化 detail/page/ugoira 数据能写出与当前 Pixiv 实现相同的 artwork、page、tag、artwork-tag 和 ugoira 记录。
- Filter 测试应继续覆盖基于规范化 Provider 数据的 stage 1 与 stage 2 行为：tag 黑名单、必含 tag、R18 模式、媒体类型、UID 列表、时间窗、收藏/浏览阈值、页数、尺寸、长宽比、AI 排除、标题正则和 ugoira 帧数。
- Image proxy 测试应验证资源 URL 接受/拒绝、Range 与条件请求头转发、响应头白名单和流式状态透传。
- Probe 测试应验证成功预览、零结果 warning、凭据或能力缺失时的 following-new unsupported、参数校验错误和上游错误。
- Admin source CRUD 测试应验证 Provider 替换后 source params 仍被一致地校验和脱敏。
- 回归测试应复用现有 Pixiv client 测试、image proxy 测试、source adapter 测试、source probe 测试、orchestrator 测试和 ingest 测试模式。
- 集成测试应使用 mock Provider server，而不是实时 Pixiv，这样 CI 保持确定性且不需要凭据。
- 最小 live smoke test 可以作为真实 Pixiv 部署的人工可选测试保留，但不能成为常规 CI 前提。

## 不在范围内

- 重设计公开 `/setu` 响应形状、管理后台图库 UI、审核流程或数据库领域命名。
- 将现有 `pixiv_*` 表迁移为 provider-neutral 表名。
- 构建 Provider marketplace/selection 之类的新前端流程；除非只是为选择和校验替换 Provider 所需的最小配置。
- 实现 downloader 或永久资产存储模式；当前仍以既有 proxy-oriented 行为为边界。
- 解决 Pixiv 账号获取、OAuth 恢复、验证码绕过、反机器人绕过或任何凭据获取流程。
- 保证每个第三方镜像或代理都能提供全部 Pixiv 等价字段。
- 改变正常 source filter 语义、随机选择语义、NSFW 审核语义或搜索日志语义。

## 进一步说明

- 当前项目术语：“采集源”指 `sources[]` 中的一条配置；“适配器”指由 source type 选择的后端实现；“参数探测”指启用或验证 source 前执行一次真实上游访问；“轻预览”指 probe 响应里的摘要和样例 PID/作品。
- 当前 Pixiv 数据依赖远不止 PID 发现。完整替换必须覆盖详情元数据、逐页原图资源、标签翻译、ugoira zip/帧数据、图片资源服务和列表模式发现。
- 更具体的外部对接接口见同目录的 `pixiv-provider-interface.md`。
- 仓库当前 GitHub Issues 未启用且没有 `ready-for-agent` label，因此本 PRD 以项目文档形式导出。启用 Issues 后，可把本文档发布成 issue 并打上 `ready-for-agent`。
