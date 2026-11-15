# 未发布

> Release-prep 暂存区：审计目标 tag 范围后直接写入下一版双语说明。审计范围内的每个 PR 或 direct commit 都需出现在两种语言中；仅内部改动列在末尾的 `internal` 注释里，渲染 Release 正文时不会显示。

## 破坏性变更

- Go SDK 重写为绑定账号的引用与对象模型：Pixiv 使用 `pixiv.New(CredentialStore, Options)`，FANBOX 使用 `fanbox.New(sessionID, Options)`，替换 `Open`/`OpenWith`/`NewWith`。通过引用的 `Get` 加载元数据，在返回对象上调用操作，所有网络操作显式传入 `ctx`。Pixiv 按 revision 比较并持久化刷新凭据，账号选择与存储由调用方负责。列表使用绑定查询/账号的 `sdk.Page[T]`、`Next(ctx)` 与不透明游标，解码后的对象/页在继续操作前重新取得 client 绑定。日志改用当前 `slog` 默认 logger，取代构造选项中的 logger。错误统一为 `sdk.Error` / `Reason` 并保留错误链，通用资源包装改用产品 `Image`/`File` 操作。以图搜图改用独立的 `sdk/saucenao`、`sdk/ascii2d`，选定 Pixiv 页的保存与部分下载报告改用 `sdk/download`。模块与 import 根路径保持原样。

## 变更

- 本地账号改存 `auth.json`；首次初始化账号文件时自动迁移已有 `pixiv-cli.db`；已有有效的当前格式 `auth.json` 时直接使用，保持原有账号内容；旧数据库保留原位，内容与修改时间保持原样。
- `pixiv config get/set/unset` 改用 TOML 路径，例如将 `download_path` 改为 `download.path`。命令可管理已声明的配置字段，包括服务网络、登录和默认账号设置。配置文件中的现有路径保持一致，废弃键继续被忽略。
- 新建的 `config.toml` 使用带英文注释和默认值的模板，已有文件保持原样。
- `pixiv config set reverse_search.saucenao_api_key VALUE` 支持通过参数传入 API key；`config get` 直接输出生效值，包括环境变量覆盖的值，原来的 `<redacted>` 隐藏行为已移除。
- 使用标准库编解码器精简 Ugoira GIF/APNG 动图编码，输出画面可能有细微差异。

## 修复

- Ugoira 转 GIF 时，全部帧所需内存超过 1 GiB 会在编码前报错，不再耗尽内存。
- 取消 ascii2d 搜索后，不再无限等待无响应的 FlareSolverr 关闭会话；过大的 FlareSolverr 响应会被拒绝。
- 拒绝在媒体主机上指定非默认端口的 Pixiv 媒体 URL。
