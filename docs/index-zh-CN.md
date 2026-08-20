# pixiv-cli 文档

[English](index.md) | 简体中文

查命令、SDK 或 MCP，从用户文档开始。准备开发、审查或发布，直接看维护者文档。

## 使用 pixiv-cli

| 想做什么 | 链接 |
| --- | --- |
| 了解项目和安装方式 | [README](../README.zh-CN.md) |
| 查 CLI 命令 | [CLI 参考](zh-CN/cli-reference.md) |
| 调用 Go SDK | [SDK](zh-CN/sdk.md) |
| 配置 MCP tools | [MCP tools](zh-CN/mcp-tools.md) |
| 提交 Issue 或 PR | [贡献指南](../CONTRIBUTING.zh-CN.md) |
| 从旧版本升级到 v1 | [迁移指南](zh-CN/v1.0.0-migration.md) |

英文文档记录公开接口的准确定义。中文可以换一种更自然的说法，但命令、参数、schema、输出和安全规则要一致。

## 维护 pixiv-cli

| 现在要处理的事 | 去这里 |
| --- | --- |
| 看清包职责和调用边界 | [架构说明](zh-CN/maintainers/architecture.md) |
| 搭环境、跑测试或准备发布 | [开发流程](zh-CN/maintainers/development.md) |
| 查看已发布变化 | [Changelog](../changelog/README.md) |

## 给自动化工具

先读根目录的 `AGENTS.md`。具体任务再进入对应 skill：

| 任务 | Skill |
| --- | --- |
| 准备 PR | `.agents/skills/pixiv-cli-pr/` |
| 诊断 CI | `.agents/skills/pixiv-cli-ci/` |
| 审查改动 | `.agents/skills/pixiv-cli-review/` |
| 整理文档 | `.agents/skills/pixiv-cli-docs/` |
| 准备发布说明 | `.agents/skills/pixiv-cli-release-notes/` |

`CLAUDE.md` 只引用 `AGENTS.md`，Copilot 也只保留短提示。长期规则不要复制到各个工具的配置里。
