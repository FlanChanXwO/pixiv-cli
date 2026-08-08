# v1.0.0 Pixiv / Pixiv FANBOX 重写设计

状态：初始整体重写已实施；2026-08-04 以后的增量设计见
[RC 后续改动索引](rc-follow-up-index.md)，实施顺序见
[RC follow-up 实施计划](rc-follow-up-implementation-plan.md)。

## 目标

v1.0.0 是公开 Pixiv Go SDK 的最后一次整体破坏性重写，同时首次在统一 `/sdk` 树下提供
独立的 Pixiv FANBOX package、模型、CLI 与 MCP 能力。发布后，公开 package、模型、错误、分页、
凭据与资源契约进入兼容性保护；后续版本允许增加能力和替换内部实现，不再进行
同等级的大规模 SDK 重构。

## 已锁定决策

- v1.0 不提供旧 SDK compatibility layer。
- Pixiv SDK 只使用 App API；删除 `internal/services/pixiv/webapi`、匿名 Web API、Web fallback
  及其配置、环境变量、路由和专项测试。仅保留旧配置键的迁移墓碑，使显式残留配置返回
  `removed_setting`，并允许 CLI 将其清除。
- v1 顶层公开 SDK 目录统一为 `/sdk`：package `sdk` 提供 `Page`、`Cursor`、`Error`、`Resource`，
  `/sdk/pixiv` 与 `/sdk/fanbox` 分别提供两个产品 Client、凭据、模型与 operation。旧 `/pixiv` 在
  v1.0.0 删除，不新增顶层 `/fanbox`。
- package path 不加 `/v1`；`v1.0.0`、`v1.0.2`、`v1.1.0` 等版本由根 Go module 的不可变 SemVer tag
  选择。`/sdk/pixiv` 与 `/sdk/fanbox` 在整个 v1 系列保持兼容且不再移动。
- Pixiv Client 与 FANBOX Client 都不读取浏览器、本地账号库或 `config.toml`。
- Pixiv refresh token 与 FANBOX session 是两套独立凭据，不转换、不互用、不 fallback。
- FANBOX 浏览器导入在 v1.0 首发，但属于协议无关的内部 platform adapter，不进入公开 SDK。
- `pixiv mcp` 与 `pixiv fanbox mcp` 是两个独立的 stdio server。
- 高级批量下载由 application/downloader 编排；核心 SDK 只冻结资源读取与单资源安全保存。
- Pixiv URL 使用公开的 typed `Reference` 纯本地解析；不让调用方依赖 URL 正则或协议 DTO。
- 两产品的第一方媒体使用 `Resource` 双轨暴露：`URL` 与安全 `RequestHeaders` 支持不落盘直连/反代，
  product-scoped `ResourceRef` 支持 SDK 重新解析、校验与读取；第三方 embed 只保留 canonical link。
- addressable entity 保留上游稳定 ID；可发布内容固定 UTC `PublishedAt` 与可选 `UpdatedAt` 语义，
  不合成 identity 或时间。
- ugoira metadata 固定为 archive quality/ref 与有序 frame/delay 列表，不公开 ZIP URL。
- 所有鉴权状态进入 `~/.pixiv-cli/pixiv-cli.db`；普通配置继续使用 `config.toml`。
- SQLite 使用单数表名：`pixiv_account`、`fanbox_account`、`schema_migration`。
- 默认账号由 `config.toml` 管理；未设置时使用 `sort_order` 最小的已保存账号。
- Pixiv 账号池的 `config.toml` 只保存 `enabled` 与 `strategy`；账号是否参加调度由
  `pixiv_account.schedulable` 表达，不建立 blacklist/whitelist 或独立 membership 表。
- Sketch 不创建空实现、占位 package、表或公开 API。
- `codex/fanbox` 只作为协议知识和 fixture 来源，不直接 merge。
- Pixiv 公开能力以固定版本的 PixivPy `AppPixivAPI` 做产品能力基线，但采用 Go 风格模型、分页、错误和
  request struct，不追求方法签名兼容，也不把项目描述为 PixivPy 的 Go 重写。
- `sdk`、`sdk/pixiv` 与 `sdk/fanbox` 公开 package 的 Go documentation 使用英文；至少覆盖 package
  comment 以及所有导出 type、function、method、const、var 和需要独立解释的导出 field。内部实现
  注释仍可按项目默认使用中文，不能把中文说明留在公开 GoDoc 中。
- 跨产品错误分类的公开名称使用 `Reason`、`Error.Reason`、`ReasonOf` 与 `IsReason`；常量直接命名为
  `InvalidArgument`、`ChallengeRequired` 等，不保留与错误含义无关的 `Code` 前缀。稳定 wire value
  仍是 `invalid_argument`、`challenge_required` 等既有字符串。
- FANBOX 当前以 native `tls-client` Firefox 148 作为 baseline，但不承诺该 profile 永久通过
  Cloudflare。FANBOX-only UA 可显式覆盖 native header；它不改变 TLS profile，solver state 存续时由
  solver user agent 优先。FlareSolverr 是显式配置、默认关闭的可选 challenge recovery：只有严格
  识别的 Cloudflare challenge 才匿名求解首页并把单个 `cf_clearance`/solver user agent 交回 native
  transport 重放；API、帖子与文件内容不经过 solver。
- CLI network 保留全局 proxy fallback，并允许 Pixiv/FANBOX native 分别覆盖；FlareSolverr service
  URL 与 browser upstream proxy 独立、显式且不继承。没有代理池、账号绑定或自动换出口。
- CLI、Pixiv MCP 与 FANBOX MCP 使用单一显式 `--debug` 输出实时叙述式 stderr 诊断；默认不输出、
  不创建日志文件、不恢复旧 logging config，也不改变 MCP stdout、SDK public API 或业务路由。
- CLI 对所有未注册 option 使用统一的严格解析：stdout 为空、usage exit code 为 `2`，stderr 只输出
  `error: unknown option '--name'`；不透传、忽略、猜测或自动纠正未知 option。
- Web API 并非被判定为不可用；它仍适合浏览器同源或显式 Cookie 的局部能力。v1 删除的是不稳定、
  难以认证且会模糊错误来源的服务端 Web 后端与自动 fallback。未来只有在 App API 明确缺少必要能力时，
  才能把 Web 能力作为独立、显式鉴权、无 fallback 的 additive package 重新提案。

## 分卷

- [公开 SDK 契约](public-sdk.md)
- [网络配置与服务路由](network-routing.md)
- [FANBOX challenge 与 FlareSolverr 路由](fanbox-challenge-routing.md)
- [PixivPy App API 能力兼容矩阵](pixivpy-parity.md)
- [SDK 使用伪代码](sdk-usage-pseudocode.md)
- [鉴权、浏览器导入与 SQLite](auth-browser-storage.md)
- [Pixiv 账号调度](account-pool-scheduling.md)
- [authdb 设计审查（2026-08-04）](authdb-design-review-2026-08-04.md)
- [CLI、MCP、下载与内部边界](cli-mcp-download.md)
- [显式 debug 诊断](debug-diagnostics.md)
- [严格 unknown-option 解析](strict-cli-argument-parsing.md)
- [RC 后续改动索引](rc-follow-up-index.md)
- [RC follow-up 实施计划（已批准并实施，RC-11 自动门禁已完成，Pixiv/solver evidence 已通过）](rc-follow-up-implementation-plan.md)
- [测试、迁移与发布门禁](verification-release.md)
- [最终验证操作手册](release-prep-runbook.md)
- [初始实施顺序与检查点（历史、已执行）](implementation-plan.md)
- [环境就绪审计](environment-readiness.md)
- [Pixiv App API only 与 Web API 删除](web-api-removal.md)
- [公开 SDK package 布局与版本管理](sdk-package-layout-versioning.md)

## 非目标

- 不把 FlareSolverr 变成全量代理、默认依赖、自动启动的 sidecar 或 Cookie pool；未配置时不调用，
  配置后也只有 challenge recovery 会访问它。生产 recovery 实现必须在 v1.0.0 前取得一次本机 protocol
  acceptance，但 solver 服务可用性和 genuine challenge 成功不是普通 CI 或后续 RC 的重复条件。
- 不增加代理池、代理文件导入、账号代理绑定、自动换 IP、Pixiv UA override 或 UA 随机轮换。
- 不把第三方 embed 递归交给其他下载器。
- 不提供 FANBOX Cookie export、MCP 认证 tool 或任意 Cookie header 输入。
- FANBOX comments 与 plan detail 不属于 v1 公共能力。
- 不把配置、下载 archive、缓存、请求日志、使用统计或浏览器 profile 写入鉴权数据库。
- 不承诺未列入 v1.0 inventory 的实验 endpoint 或反向工程 DTO 为公开契约。

## v1 兼容策略

公开 API inventory 在 release candidate 阶段冻结。v1.0.0 发布后，CI 以该版本为
baseline 运行 Go public API compatibility 检查；删除、改名、收窄类型、改变零值语义、
改变错误分类或破坏 cursor 解码均视为不兼容。浏览器 schema、协议 DTO、SQLite
repository、下载调度和 MCP runtime wiring 保持内部实现，可以在不改变公开契约时持续维护。

v0.x 的 `github.com/FlanChanXwO/pixiv-cli/pixiv` 继续由不可变旧 tag 获取；v1 不在新源码中保留
compatibility package。同一个消费者 build 只能选择根 module 的一个版本，因此不能同时 import
v0 `/pixiv` 与 v1 `/sdk/pixiv`；迁移指南必须要求一次切换。不同项目可以分别固定任意旧 tag。
