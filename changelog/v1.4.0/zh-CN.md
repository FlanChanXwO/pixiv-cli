# v1.4.0 — 2026-11-21

## 破坏性变更

- Go SDK 改为绑定账号的引用与对象模型。将 `Open`/`OpenWith`/`NewWith` 替换为 `pixiv.New(CredentialStore, Options)` 或 `fanbox.New(sessionID, Options)`，通过 `Get(ctx)` 加载引用，网络操作显式传入 `ctx`。列表迁移至 `sdk.Page[T]`、`Next(ctx)` 与不透明游标；解码后的对象需要重新绑定 client。错误、媒体和日志分别使用 `sdk.Error` / `Reason`、产品媒体对象及当前 `slog` 默认 logger。以图搜图迁移至 `sdk/saucenao` / `sdk/ascii2d`，选定页保存使用 `sdk/download`。模块与 import 根路径保持原样。 ([#273](https://github.com/FlanChanXwO/pixiv-cli/pull/273), [#277](https://github.com/FlanChanXwO/pixiv-cli/pull/277), [#278](https://github.com/FlanChanXwO/pixiv-cli/pull/278), [#279](https://github.com/FlanChanXwO/pixiv-cli/pull/279), [#280](https://github.com/FlanChanXwO/pixiv-cli/pull/280), [#281](https://github.com/FlanChanXwO/pixiv-cli/pull/281), [#282](https://github.com/FlanChanXwO/pixiv-cli/pull/282), [#284](https://github.com/FlanChanXwO/pixiv-cli/pull/284), [#285](https://github.com/FlanChanXwO/pixiv-cli/pull/285), [#286](https://github.com/FlanChanXwO/pixiv-cli/pull/286), [#287](https://github.com/FlanChanXwO/pixiv-cli/pull/287), [#288](https://github.com/FlanChanXwO/pixiv-cli/pull/288), [#289](https://github.com/FlanChanXwO/pixiv-cli/pull/289), [#290](https://github.com/FlanChanXwO/pixiv-cli/pull/290))

- `pixiv config get/set/unset` 改用已声明的 TOML 路径，例如将 `download_path` 改为 `download.path`。配置值必须符合声明的 TOML 类型；遇到行号与键的校验错误时，请移除数字和布尔值外的引号。配置文件路径保持原样。 ([#274](https://github.com/FlanChanXwO/pixiv-cli/pull/274))

- 升级后请重新发起 MCP 查询，旧 `next_cursor` 会被拒绝。续页绑定首批结果的账号，HTTP 调用需要保持相同账号选择。Gallery 图片标识改为包含 `artwork_id`、`page_index`、`quality` 或 `user_id` 的可读对象。 ([#284](https://github.com/FlanChanXwO/pixiv-cli/pull/284), [#285](https://github.com/FlanChanXwO/pixiv-cli/pull/285), [#286](https://github.com/FlanChanXwO/pixiv-cli/pull/286))

- Linux 二进制现在依赖 glibc，包括普通命令。本地 ONNX Runtime 推理需要 glibc 2.28 或更新版本。 ([#299](https://github.com/FlanChanXwO/pixiv-cli/pull/299))

## 新增

- 新增需主动启用的向量入库与作品语义搜索。依次运行 `pixiv vector setup`、`pixiv vector sync` 和 `pixiv vector search INPUT`；启用后普通作品读取会记录元数据。搜索自动识别文字、本地 JPEG/PNG/GIF 图片及 HTTP(S) 图片 URL，默认返回 30 个作品，并在限时内补算；`--no-sync` 可跳过补算。漫画索引首页，其他作品索引全部页面。 ([#296](https://github.com/FlanChanXwO/pixiv-cli/pull/296), [#297](https://github.com/FlanChanXwO/pixiv-cli/pull/297), [#298](https://github.com/FlanChanXwO/pixiv-cli/pull/298), [#303](https://github.com/FlanChanXwO/pixiv-cli/pull/303))

- 向量配置支持本地 SigLIP2 256/512 CPU 模型、显式 `--device gpu` 及 CPU/GPU 兼容性探测，还支持通过 Gemini 兼容接口使用 Gemini Embedding 2。配置保存前会验证文字与图片兼容性。云端推理会向配置的服务上传查询内容和入库图片；本地索引可包含私密收藏与 R18 作品。 ([#297](https://github.com/FlanChanXwO/pixiv-cli/pull/297), [#299](https://github.com/FlanChanXwO/pixiv-cli/pull/299), [#300](https://github.com/FlanChanXwO/pixiv-cli/pull/300))

- `vector sync` 默认扫描公开及私密收藏，分批执行并支持断点续跑，提供 `--restrict`、`--content-type`、`--rating` 与完整 `--reconcile`。公开和私密 worker 共享至少 0.2 秒的 Pixiv 请求间隔，用户配置更大时优先生效。对账完整成功后清理旧收藏关系，已有作品与向量继续保留。使用 `vector add ID_OR_URL...` 或标准输入作品 NDJSON 可立即添加指定作品。 ([#303](https://github.com/FlanChanXwO/pixiv-cli/pull/303))

- 新增 MCP `pixiv_vector_search`，复用 CLI 的本地向量索引。向量搜索需要已配置的模型，HTTP 查询图片通过授权媒体引用提供。 ([#301](https://github.com/FlanChanXwO/pixiv-cli/pull/301))

- SDK 新增带操作上下文的作品观察器，可将作品记录接入应用管理的存储。 ([#295](https://github.com/FlanChanXwO/pixiv-cli/pull/295))

- CLI 帮助、提示、人类可读结果与诊断自动跟随系统语言选择英文或简体中文，登录/OAuth 页面跟随 `Accept-Language`。可用 `env LC_ALL=C pixiv ...` 指定英文，或 `env LC_ALL=zh_CN.UTF-8 pixiv ...` 指定中文。机器输出字段名与错误码保持原样。 ([#302](https://github.com/FlanChanXwO/pixiv-cli/pull/302))

## 变更

- 本地账号改存 `auth.json`。首次使用账号的命令在初始化账号文件时导入已有 `pixiv-cli.db`；已有有效当前格式 `auth.json` 时继续使用。旧数据库内容与修改时间保持原样。 ([#276](https://github.com/FlanChanXwO/pixiv-cli/pull/276))

- 新建 `config.toml` 包含英文注释与默认值。`config set reverse_search.saucenao_api_key VALUE` 支持位置参数，`config get` 直接输出其生效值，包括环境变量覆盖值，原有隐藏行为移除。 ([#274](https://github.com/FlanChanXwO/pixiv-cli/pull/274))

- Ugoira GIF/APNG 编码改用标准库编解码器，转换后的画面可能有细微差异。 ([#275](https://github.com/FlanChanXwO/pixiv-cli/pull/275))

## 修复

- FANBOX 创作者列表遍历上游全部页面。下载保留图片、附件扩展名，并将完整正文、文章块和压缩包密码等信息写入私有权限的 `post.json`。附件文件名避让元数据文件及文件系统别名；受限帖子保留元数据并返回错误。 ([#294](https://github.com/FlanChanXwO/pixiv-cli/pull/294))

- `pixiv fanbox tags` 与 MCP `fanbox_creator_tags` 正确读取当前 `featuredTags` 响应，修复 `missing FANBOX tag list` 错误。 ([#293](https://github.com/FlanChanXwO/pixiv-cli/pull/293))

- 等待账号初始化的请求现在响应取消操作。 ([#291](https://github.com/FlanChanXwO/pixiv-cli/pull/291))

- Ugoira GIF 转换在帧内存将超过 1 GiB 时提前报错。取消 ascii2d 搜索后，solver 会话清理在限定时间内结束，过大的 FlareSolverr 响应会被拒绝。Pixiv 媒体 URL 拒绝非默认端口。 ([#292](https://github.com/FlanChanXwO/pixiv-cli/pull/292))

**完整变更：** [v1.3.0...v1.4.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.3.0...v1.4.0)

<!-- internal: [#283](https://github.com/FlanChanXwO/pixiv-cli/pull/283) [#304](https://github.com/FlanChanXwO/pixiv-cli/pull/304) -->
