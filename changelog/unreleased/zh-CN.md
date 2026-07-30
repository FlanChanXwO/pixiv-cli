# 未发布

> 此处是 release-prep 暂存区。功能 PR 在模板中提供分类和摘要；经审核的 release-prep plan 会将这些来源归并到下一个版本说明。

## 变更

- 仅文档的 Pull Request 与 `main` push 现只运行文档契约检查，不再运行完整 Linux 质量门禁和六平台已打包二进制 smoke；任何源码、依赖、脚本或 workflow 改动仍会执行完整验证集。
- 数据命令现在使用已选定的本地账号或可选的手工账号池；拒绝按命令选择 UID/token，并忽略 `PIXIV_REFRESH_TOKEN`。其流式 `--ndjson` Record 具有稳定的 `id`、`type`、`url` 字段，`filter` 与下载/收藏/关注动作可消费同一协议。
- `pixiv config` 现在只管理 `download_path`、`filename_template`、`https_proxy`；高级设置仍由用户在私有 TOML 中手工维护。
- 文本模式的数据命令现在会报告 stdout 写入失败，不再在部分结果写出后误报成功。

## 新增

- Ugoira 下载可通过 `--ugoira-format apng` 生成 APNG；默认格式仍为 GIF。
- 公开 SDK 与 MCP 的实体读取 tool（含 feed 与 MyPixiv）提供规范 Record；MCP 文本现在仅为短摘要，不再重复实体 payload。
- 已发布的 release tag 现在会同时向 ClawHub 与 SkillHub 发布产品 skill。ClawHub workflow 会校验不可变 release 来源、在无凭据下 dry-run，并只在最后发布步骤注入 `CLAWHUB_TOKEN`。
