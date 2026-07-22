<div align="center">

# pixiv-cli

**Pixiv CLI · MCP stdio server · Go SDK**

[English](README.md) · [简体中文](README.zh-CN.md) · [日本語](README.ja.md)

<p><a href="https://github.com/FlanChanXwO/pixiv-cli/actions/workflows/ci.yml"><img alt="Quality gate" src="https://github.com/FlanChanXwO/pixiv-cli/actions/workflows/ci.yml/badge.svg"></a> <a href="https://github.com/FlanChanXwO/pixiv-cli/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/FlanChanXwO/pixiv-cli?style=flat-square"></a> <a href="go.mod"><img alt="Go" src="https://img.shields.io/github/go-mod/go-version/FlanChanXwO/pixiv-cli?style=flat-square"></a> <a href="LICENSE"><img alt="License" src="https://img.shields.io/github/license/FlanChanXwO/pixiv-cli?style=flat-square"></a> <img alt="Views" src="https://hits.sh/github.com/FlanChanXwO/pixiv-cli.svg?style=flat-square&amp;label=views"></p>

[安装](#安装) · [快速开始](#60-秒快速开始) · [使用入口](#选择使用入口) · [文档](#文档) · [参与贡献](CONTRIBUTING.zh-CN.md)

</div>

`pixiv-cli` 是独立开发的非官方第三方工具，为用户、Coding Agent 和 Go 应用提供一致的 Pixiv 能力，与 Pixiv Inc. 无隶属或背书关系。CLI 与 MCP server 共同调用 public Go SDK，并以 Pixiv App API 作为已认证能力的数据源；使用时请遵守 Pixiv 条款与适用法律。

## 为什么选择 pixiv-cli？

- **一致的能力面**——CLI、MCP 与 SDK 均可完成搜索、详情、排行、推荐、用户、收藏、关注、下载和 ugoira 处理。
- **App API 优先**——配置 refresh token 后始终走已认证 App 路径；App 失败不会静默回落 Web。
- **认证 R18 读取**——详情、分页、ugoira metadata 和全部 16 种排行榜都走 App API；无法取得 original 时会诚实使用已验证的 medium ugoira ZIP。
- **实用搜索筛选**——支持分级、作品类型、AI 模式、横纵比、分辨率和动态绘图工具。
- **本地多账号 OAuth**——支持浏览器登录、账号选择和 refresh token rotation，不读取浏览器 Cookie 或 profile。
- **适合自动化**——typed SDK error、JSON 输出、纯净 MCP stdio、签名更新且不隐藏截断结果。
- **有限匿名访问**——没有 token 且启用 fallback 时，受支持的只读操作可以使用 Web API。

## 安装

### 安装脚本（Windows、Linux 与 macOS）

Linux/macOS（`sh`）：

```bash
curl -fsSLo /tmp/pixiv-install.sh https://raw.githubusercontent.com/FlanChanXwO/pixiv-cli/main/scripts/install.sh && sh /tmp/pixiv-install.sh --add-to-path
```

Windows 命令提示符（`cmd.exe`，不依赖 PowerShell）：

```bat
curl.exe -fsSLo "%TEMP%\pixiv-install.cmd" https://raw.githubusercontent.com/FlanChanXwO/pixiv-cli/main/scripts/install.cmd && call "%TEMP%\pixiv-install.cmd" --add-to-path
```

两个脚本都会检测 AMD64/ARM64、选择最新 stable 官方 Release archive、校验发布的 SHA-256、预检暂存
binary，并在修改 PATH 前完成用户级安装。可用 `--no-path` 保持 PATH 不变，或用 `--install-dir DIR`
选择其他目录；执行前也可以先审阅下载的脚本。

Linux 兼容性说明：v0.3.0 archive 在 Ubuntu 24.04 上链接，可能要求 glibc 2.39，因此不能在 Debian 12
运行。v0.4.2 已从构建侧修复：Linux 资产以 glibc 2.35 为最高依赖并经过 ABI 门禁。安装器会在
替换现有安装前预检 staged binary，并明确报告不兼容。

### 让 Coding Agent 安装

把下面这一段 prompt 复制给能够操作本机终端的 Codex、Claude Code、Cursor 或其他 Coding Agent：

```text
请为这台机器安装 https://github.com/FlanChanXwO/pixiv-cli 的最新 stable 版本：先审阅仓库中的 scripts/install.sh 或 scripts/install.cmd，再根据检测到的操作系统与架构选择对应脚本（Windows 必须使用 cmd.exe，禁止调用 PowerShell），只下载官方 GitHub Release 资产，只有发布的 SHA-256 校验通过后才能替换文件，使用无需管理员或 root 权限的用户级目录，只把选定安装目录加入用户 PATH，缺少任何前置工具时先征求同意，绝不读取或输出 Pixiv 凭据，最后运行 pixiv version 验证，并报告安装版本及全部文件和 PATH 变更。

同时安装与该 stable 发布 tag 完全一致的产品 skill（不要跟随 main）：把该 tag 下的完整 skills/pixiv-cli/ 目录安装到用户确认的 Agent skills 目录。不要猜测 skills 路径，也不要用 main 上的 skill 内容。
```

### Homebrew（macOS 与 Linux 推荐）

```bash
brew install FlanChanXwO/tap/pixiv-cli
```

后续更新使用：

```bash
brew update
brew upgrade pixiv-cli
```

### Go

请使用精确的已发布 tag。源码安装需要 Go、cgo、C linker 和仓库中与目标平台匹配的 Rust static library。

```bash
go install github.com/FlanChanXwO/pixiv-cli/cmd/pixiv@vX.Y.Z
```

### Release 压缩包或源码构建

从 [GitHub Releases](https://github.com/FlanChanXwO/pixiv-cli/releases) 下载受支持的压缩包，或构建当前 checkout：

```bash
sh scripts/build.sh
```

直接下载提供 checksum 与签名 manifest。平台与信任细节见 [CLI 参考手册](docs/zh-CN/cli-reference.md#安装与构建)。

## 60 秒快速开始

```bash
# 通过浏览器 OAuth 保存 Pixiv 账号。
pixiv auth login

# 使用 App 服务端筛选搜索。
pixiv search "初音ミク" --type illust --ai-mode exclude --resolution high

# 查看详情、获取推荐并下载。
pixiv detail 123456
pixiv recommended all --limit 10
pixiv download 123456 --pages 1,3-5 --quality regular
```

运行 `pixiv --help`，或打开[完整 CLI 参考手册](docs/zh-CN/cli-reference.md)，查看全部命令、flag、配置键、环境变量、fallback 规则和更新行为。

## 选择使用入口

### CLI

交互时使用人类可读输出；命令支持时可用 `--json` 获取机器可读输出：

```bash
pixiv ranking --mode day --json
pixiv user detail 12345678
pixiv search-options "初音ミク"
```

### MCP

显式启动 stdio server。stdout 只用于 JSON-RPC。操作摘要写入用户 state 目录 `pixiv/logs` 的按日 JSONL（默认保留 7 天），终端默认无日志痕迹。

```bash
pixiv mcp
```

[MCP tool 契约](docs/zh-CN/mcp-tools.md)记录了 tools、参数、structured output 和认证行为。
MCP 固定状态、错误和展示文本使用英文；Pixiv 元数据及用户提供的文本保持原文。

### Go SDK

```go
client, err := pixiv.OpenDefault(pixiv.Options{})
if err != nil {
    // 处理本地认证或配置失败。
}
result, err := client.SearchIllust(ctx, pixiv.SearchIllustRequest{Word: "初音ミク"})
```

导入 `github.com/FlanChanXwO/pixiv-cli/pixiv`。[SDK 指南](docs/zh-CN/sdk.md)说明模型、cursor、资源、错误和调用方职责。

## 认证与 token 安全

推荐使用 `pixiv auth login` 完成配置。它把原始 Pixiv App OAuth refresh token 按 UID 保存在本地账号 store；`PHPSESSID` 等浏览器 Cookie 会被拒绝，也不会转换为 App 凭据。

```bash
pixiv auth list
pixiv auth use 12345678
pixiv auth check
```

v0.4.2 以 `auth import`/`auth export` 取代 `auth add`/`auth token`，已删除的名称和 `--token` 均无 alias。导入时优先使用无参隐藏输入或 raw stdin；位置参数会暴露在 argv/shell history。只有不带 `--output` 的 `pixiv auth export [UID]` 与 `pixiv auth export --all` 可以向 stdout 输出 secret；需要文件时使用 `--output` 写私有 bundle。bundle 是未加密的 point-in-time backup，不是 live sync，token rotation 后可能 stale。不要把 secret 输出粘贴到聊天、日志、shell history、issue 或 Agent transcript；其他 stdout/stderr、JSON、MCP result、日志和错误均不得暴露 refresh token。完整 import/export 与文件保护契约见 [CLI reference](docs/zh-CN/cli-reference.md#获取-refresh-token)。

## 文档

| 文档 | 用途 |
| --- | --- |
| [CLI 参考手册](docs/zh-CN/cli-reference.md) | 命令、flag、认证、配置、fallback、下载和更新 |
| [Go SDK](docs/zh-CN/sdk.md) | Public client、模型、分页、资源和 typed error |
| [MCP tools](docs/zh-CN/mcp-tools.md) | Tool schema 与输出语义 |
| [架构](docs/maintainers/architecture.md) | 包边界和运行流程 |
| [开发流程](docs/maintainers/development.md) | 工具链、测试、构建和发布 |
| [更新日志](CHANGELOG.md) | 用户可感知变化 |

## 参与贡献

欢迎提交 bug、文档修复、测试和聚焦功能。发起 pull request 前请阅读 [CONTRIBUTING.zh-CN.md](CONTRIBUTING.zh-CN.md)；较大或影响兼容性的变更请先讨论。

## 许可证

[MIT](LICENSE)
