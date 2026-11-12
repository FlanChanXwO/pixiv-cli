# 未发布

> Release-prep 暂存区：审计目标 tag 范围后直接写入下一版双语说明。审计范围内的每个 PR 或 direct commit 都需出现在两种语言中；仅内部改动列在末尾的 `internal` 注释里，渲染 Release 正文时不会显示。

## 变更

- 本地账号改存 `auth.json`；首次初始化账号文件时自动迁移已有 `pixiv-cli.db`；已有有效的当前格式 `auth.json` 时直接使用，保持原有账号内容；旧数据库保留原位，内容与修改时间保持原样。
- `pixiv config get/set/unset` 改用 TOML 路径，例如将 `download_path` 改为 `download.path`。命令可管理已声明的配置字段，包括服务网络、登录和默认账号设置。配置文件中的现有路径保持一致，废弃键继续被忽略。
- 新建的 `config.toml` 使用带英文注释和默认值的模板，已有文件保持原样。
- `pixiv config set reverse_search.saucenao_api_key VALUE` 支持通过参数传入 API key；`config get` 直接输出生效值，包括环境变量覆盖的值，原来的 `<redacted>` 隐藏行为已移除。
- 使用标准库编解码器精简 Ugoira GIF/APNG 动图编码，输出画面可能有细微差异。
