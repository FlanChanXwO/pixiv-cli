# Documentation Guidelines

文档按读者、语言与稳定性分层，避免把所有内容塞进根目录或为每种 locale 机械复制内部文档。

## 目录职责

- `README.md`、`README.zh-CN.md`、`README.ja.md`：GitHub 项目入口，分别为英文、简体中文与日语。
- `docs/en/`：面向用户的英文公开接口文档，也是 public contract 的 canonical source。
- `docs/zh-CN/`：简体中文公开接口文档，行为语义须与英文对应。
- `docs/ja/`：日语公开接口文档；只创建已完成翻译的文件，不用英文占位冒充日语。
- `docs/maintainers/`：架构、开发流程、ADR 与 Agent 协作规则；每篇只保留一个 canonical 版本。
- `docs/index.md`：语言与维护者文档总导航。
- `docs/*.md`、`docs/agents/` 的旧路径：兼容导航 stub，不再承载权威内容；`docs/adr/` 与 `docs/superpowers/` 已移除，权威 ADR 仅在 `docs/maintainers/adr/`。
- `CONTRIBUTING.md` / `CONTRIBUTING.zh-CN.md`：GitHub 可发现的贡献入口；新增 locale 只在有真实贡献者需求时进行。
- `CONTEXT.md`：Pixiv/MCP 领域词汇与关系；不要放通用工程流程。
- `CHANGELOG.md`：用户和集成方可感知的变化。
- `AGENTS.md`：agent 的短主规则和路由。
- `.github/copilot-instructions.md`：Copilot 短提示。
- `.agents/skills/`：高频 agent 工作流。

## Locale 规则

- locale 目录使用 BCP 47 tag：`en`、`zh-CN`、`ja`；日语无需无意义地写成 `ja-JP`。
- 英文 public contract 先更新，同一变更中同步已有翻译；允许自然改写，不得产生不同命令、参数、安全语义或限制。
- 某语言尚未翻译的文档在 `docs/index.md` 明确链接到英文，不创建内容为英文却标为该 locale 的文件。
- root README 的语言切换必须互相链接；新增语言时同步安装器 README 测试与文档导航。
- code、command、flag、path、package、schema field、error code 与 wire value 保持原始英文标识。

## 更新规则

- 项目定位、安装、快速开始、接口入口或用户必须先知道的安全提醒变化时，同步三语 README。
- CLI command、flag、账号、配置、环境变量、fallback、更新或输出契约变化时，同步 `docs/en/cli-reference.md`、`docs/zh-CN/cli-reference.md` 与 `docs/ja/cli-reference.md`。
- SDK API、model、error 契约写 `docs/en/sdk.md` 与 `docs/zh-CN/sdk.md`；MCP tool/schema/wire 语义写 `docs/en/mcp-tools.md` 与 `docs/zh-CN/mcp-tools.md`。
- 内部边界变化写 `docs/maintainers/architecture.md`；只有长期、难以逆转且未来维护者需要背景的取舍才写 `docs/maintainers/adr/`。
- 测试、构建与发布流程写 `docs/maintainers/development.md`；贡献者入口要求变化时同步现有 CONTRIBUTING locale。
- 术语冲突或领域概念变化才更新 `CONTEXT.md`。
- 纯内部重构、测试补充与文档清理通常不需要 changelog；新增用户语言入口需要记录。

## 验证

完成后检查三语导航、仓库内相对链接、locale 文件存在性、Markdown fence、`git diff --check`，并运行与文档约束相关的聚焦测试。
