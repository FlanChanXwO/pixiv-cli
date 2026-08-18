---
name: pixiv-cli-docs
description: Maintain pixiv-cli README, locale contracts, maintainer documentation, contribution guidance, and documentation checks. Use when adding, changing, reviewing, or routing repository documentation; use pixiv-cli-release-notes for versioned bilingual changelog or release-prep work.
---

# pixiv-cli Docs

新增、修改或审查本仓库文档。文件职责与路由表以本文件的
[文档路由](#文档路由) 章节为准。

## 流程

1. 读取 `AGENTS.md`、本 SKILL 和目标文件，确定内容应落在哪个 locale、maintainer 文件、README 或贡献者入口。
2. 按目标 locale 写作；命令、路径、包名、schema field、error code 和 code-id 保持英文。更新 public contract 时先改 canonical English，再同步已有 Simplified Chinese 语义。
3. 检查命令、参数、输出、安全边界和代码实现是否一致；涉及 CLI/MCP/SDK/config/安装或 release 行为时，同时检查对应测试、`AGENTS.md`、PR 模板和产品 `skills/pixiv-cli/`。
4. 同一规则只写一处，其他位置用链接路由，不复制大段内容；历史计划、验收报告和 release evidence 不因当前流程变化而改写。
5. 完成后检查相对链接、locale 导航、Markdown fence、文件存在性和语义一致性，并运行：

   ```bash
   go test ./scripts/tests/documentation -count=1
   git diff --check
   ```

## 文档路由

拿不准写到哪里时，先看读者。用户需要查到的接口放在 locale 文档里；只有维护代码的人会用到的内容，放进 `docs/en/maintainers/`（英文基准）和 `docs/zh-CN/maintainers/`（中文）。

### 每个位置管什么

| 位置 | 适合放的内容 |
| --- | --- |
| `README.md`、`README.zh-CN.md` | 项目介绍、安装、快速开始和最先要看到的安全提醒 |
| `docs/en/` | 英文公开接口，也是接口定义的基准 |
| `docs/zh-CN/` | 与英文行为一致的中文说明 |
| `docs/en/maintainers/`、`docs/zh-CN/maintainers/` | 架构、开发流程、能力边界和协作规则 |
| `docs/index.md` | 英文文档站入口 |
| `docs/index-zh-CN.md` | 中文文档站入口 |
| 两份 `CONTRIBUTING` | 提交 Issue 和 PR 前需要知道的事 |
| `changelog/` | 每个版本真正发生了什么 |
| `AGENTS.md` | agent 开工前要读的短规则 |
| `.agents/skills/` | 可以照着执行的项目流程 |

> [!IMPORTANT]
> 同一条规则只留一份。其他地方给链接，不要换个说法再抄一遍。

### 两种语言怎么同步

- locale 目录使用 BCP 47 tag，目前是 `en` 和 `zh-CN`。
- 公开接口先确认英文定义，再同步中文。中文不必逐句直译，但不能改掉命令、参数、schema、安全语义或限制。
- 暂时没有翻译时，从 `docs/index.md` 链到英文。不要复制一份英文内容再把它放进中文目录。
- code、command、flag、path、package、schema field、error code 和 wire value 保持英文。

### 找到该同步的文档

| 变化 | 一起更新 |
| --- | --- |
| 项目定位、安装、快速开始 | 两份 README |
| CLI command、flag、账号、配置、环境变量或输出 | 两份 CLI reference |
| SDK API、model、error | 两份 SDK 文档 |
| MCP tool、schema、wire 语义 | 两份 MCP tools 文档 |
| 包职责或依赖方向 | `docs/en/maintainers/architecture.md`、`docs/zh-CN/maintainers/architecture.md` |
| 测试、构建或发布流程 | `docs/en/maintainers/development.md`、`docs/zh-CN/maintainers/development.md` |
| 贡献方式 | 两份 CONTRIBUTING |
| 领域术语 | `docs/zh-CN/maintainers/architecture.md` 的「领域词汇」 |

> [!NOTE]
> 普通 PR 不写版本说明。准备发布时先审计 tag 范围，再直接写 `changelog/vX.Y.Z/en.md` 和 `zh-CN.md`。纯内部改动也不能漏掉，统一放在 `Maintenance`。

### 提交前看一眼

确认链接能打开、两种语言没有说出不同的行为、Markdown fence 成对，然后运行：

```bash
go test ./scripts/tests/documentation -count=1
git diff --check
```

## 发布说明

- PR 正文只说明改动、验证结果和检查清单；不要在功能 PR 中预写最终版本说明。
- 版本化英文/简体中文 release notes、来源审计、tag 发布或历史 GitHub Release body 同步，转交 `pixiv-cli-release-notes` 技能；它负责 `audit`、直接编写 Markdown、`validate` 和 `sync-history` 的顺序。
- 纯内部重构、测试补充和文档清理在发布准备时归入 `Maintenance`；用户可见文档、命令、API、安全或兼容性变化同步必要的双语 contract。

## 约束

- 不把长篇架构说明写回 `AGENTS.md`。
- 不把未翻译内容伪装成 locale 文档。
- 不在普通 PR 中为纯内部整理单独写 changelog；发布准备时仍要覆盖它的来源。
