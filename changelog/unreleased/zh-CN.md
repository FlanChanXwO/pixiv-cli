# 未发布

> 此处用于准备发布说明。先审计目标 tag 范围，再直接编写下一版双语 Markdown；审计中的每个 PR 或 direct commit 都要出现在两种语言中。

## 配置与诊断

- 恢复统一的 `[logging].level`/`[logging].format` 配置及 `PIXIV_LOG_LEVEL`、`PIXIV_LOG_FORMAT` 覆盖；`debug`
  诊断只写 stderr，MCP stdout 继续保持 JSON-RPC。
- `pixiv config` 依据同一份 schema 管理 logging、下载目录、请求节奏、代理和账号池配置；首次生成的
  `config.toml` 由 schema 元数据自动生成，且不会覆盖已有文件。

## 配置与诊断

- 恢复统一的 `[logging].level`/`[logging].format` 配置及 `PIXIV_LOG_LEVEL`、`PIXIV_LOG_FORMAT` 覆盖；`debug`
  诊断只写 stderr，MCP stdout 继续保持 JSON-RPC。
- `pixiv config` 依据同一份 schema 管理 logging、下载目录、请求节奏、代理和账号池配置；首次生成的
  `config.toml` 由 schema 元数据自动生成，且不会覆盖已有文件。
