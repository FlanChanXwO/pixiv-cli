<div align="center">

# pixiv-cli

**Pixiv CLI · MCP stdio server · Go SDK**

[English](README.md) · [简体中文](README.zh-CN.md) · [日本語](README.ja.md)

<p><a href="https://github.com/FlanChanXwO/pixiv-cli/actions/workflows/ci.yml"><img alt="Quality gate" src="https://github.com/FlanChanXwO/pixiv-cli/actions/workflows/ci.yml/badge.svg"></a> <a href="https://github.com/FlanChanXwO/pixiv-cli/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/FlanChanXwO/pixiv-cli?style=flat-square"></a> <a href="go.mod"><img alt="Go" src="https://img.shields.io/github/go-mod/go-version/FlanChanXwO/pixiv-cli?style=flat-square"></a> <a href="LICENSE"><img alt="License" src="https://img.shields.io/github/license/FlanChanXwO/pixiv-cli?style=flat-square"></a> <img alt="Views" src="https://hits.sh/github.com/FlanChanXwO/pixiv-cli.svg?style=flat-square&amp;label=views"></p>

[安装](#安装) · [快速开始](#60-秒快速开始) · [使用入口](#选择使用入口) · [文档](#文档) · [参与贡献](CONTRIBUTING.zh-CN.md)

</div>

`pixiv-cli` 把 Pixiv 生态带到终端：发现作品与创作者、管理账号和作品收藏、关注创作者，并下载视觉作品。它提供独立的非官方 CLI、MCP server 和 public Go SDK，与 Pixiv Inc. 无隶属或背书关系。CLI 与 MCP server 共同调用 public Go SDK，并以 Pixiv App API 作为已认证能力的数据源；使用时请遵守 Pixiv 条款与适用法律。

## 为什么选择 pixiv-cli？

- **一致的能力面**——CLI、MCP 与 SDK 均可完成搜索、详情、排行、推荐、用户、收藏、关注、下载和 ugoira 处理。
- **只读 FANBOX 能力**——通过 `FANBOXSESSID` 登录后，可从 CLI、MCP 或 `sdk/fanbox` 查看创作者、帖子、主页/支持中 feed、标签和第一方文件资源。
- **组合式视觉作品管道**——视觉列表接入管道时自动输出 canonical NDJSON；用 `--filter` 编写有类型的本地作品筛选，并可直接传给 `download`。
- **本地账号池**——为读取型任务选择符合条件的本地账号，并在分页和下载准备阶段遵循 Pixiv 的 `Retry-After` 响应。
- **易用的账号登录流程**——运行 `pixiv auth login` 即可在浏览器完成 OAuth，随后可使用 `auth list`、`auth use` 和 `auth check` 管理和确认本地多账号。
- **四种 ugoira 输出模式**——可选择 GIF、APNG、无损 ZIP 或解压后的帧。
- **可靠且可整理的下载**——重验证 `.pixiv-cache` 元数据、续传已验证残片、重试可恢复资源失败，可选归档完整作品、写入 sidecar，并在可知总字节数时显示终端进度。
- **认证 App API 发现能力**——通过 App API 读取 R18 详情、分页、ugoira metadata 和全部 16 种排行榜。
- **实用搜索筛选**——支持分级、作品类型、AI 模式、横纵比、分辨率和版本内置的绘图工具目录。
- **直达 Pixiv 引用**——可把受支持作品 URL 直接粘贴给详情或下载；已认证的作者主页/作品页 URL 会展开为该作者的视觉作品。
- **本地多账号 OAuth**——支持浏览器登录、账号选择、refresh token rotation 和可选的跨机器 callback relay。
- **适合自动化**——typed SDK error、JSON 输出、纯净 MCP stdio、签名更新和完整结果报告。
- **显式诊断**——使用 `--debug` 可将路由、challenge 恢复、账号池和下载事件安全地实时写入 stderr；不创建日志文件。

## 安装

### 安装脚本（Windows、Linux 与 macOS）

Linux/macOS（`sh`）：

```bash
curl -fsSLo /tmp/pixiv-install.sh https://github.com/FlanChanXwO/pixiv-cli/releases/latest/download/install.sh && sh /tmp/pixiv-install.sh --add-to-path
```

Windows 命令提示符（`cmd.exe`，不依赖 PowerShell）：

```bat
curl.exe -fsSLo "%TEMP%\pixiv-install.cmd" https://raw.githubusercontent.com/FlanChanXwO/pixiv-cli/main/scripts/install.cmd && call "%TEMP%\pixiv-install.cmd" --add-to-path
```

两个脚本都会检测 AMD64/ARM64、选择最新 stable 官方 Release archive、校验发布的 SHA-256、预检暂存
binary，并在修改 PATH 前完成用户级安装。可用 `--no-path` 保持 PATH 不变，或用 `--install-dir DIR`
选择其他目录；执行前也可以先审阅下载的脚本。

随正式版本发布的安装器始终从 GitHub HTTPS 直连取得权威 `checksums.txt`。内置免费候选源只用于探测和下载平台压缩包，候选返回的 checksum 必须与直连内容一致，下载结果仍必须通过 SHA-256 校验。它只改善传输可达性，不改变 Release 身份或完整性判断。

### 让 AI Agent 安装

把下面这一段 prompt 复制给能够操作本机终端的 Codex、Claude Code、Cursor 或其他 AI Agent：

```text
请为这台机器安装 https://github.com/FlanChanXwO/pixiv-cli 的最新 stable 版本：先审阅仓库中的 scripts/install.sh 或 scripts/install.cmd，再根据检测到的操作系统与架构选择对应脚本（Windows 必须使用 cmd.exe，禁止调用 PowerShell），只下载官方 GitHub Release 资产，只有发布的 SHA-256 校验通过后才能替换文件，使用无需管理员或 root 权限的用户级目录，只把选定安装目录加入用户 PATH，缺少任何前置工具时先征求同意，绝不读取或输出 Pixiv 凭据，最后运行 pixiv version 验证，并报告安装版本及全部文件和 PATH 变更。

同时安装与该 stable 发布 tag 完全一致的 `pixiv-cli` Skill（不要跟随 main）：把该 tag 下的完整 skills/pixiv-cli/ 目录安装到用户确认的 Agent skills 目录。不要猜测 skills 路径，也不要用 main 上的 skill 内容。
```

### 通过 SkillHub 安装 pixiv-cli Skill

支持 SkillHub 的 Agent 可直接从 [SkillHub 的 `pixiv-cli` Skill 页面](https://www.skillhub.cn/skills/pixiv-cli) 安装已发布的 `pixiv-cli` Skill。每个 Skill 版本均与其指导的 CLI release 对应；命令语法始终以 `pixiv <cmd> --help` 为最终依据。

### 通过 ClawHub 安装 pixiv-cli Skill

使用 ClawHub 的 Agent 可从 [ClawHub 的 `pixiv-cli` Skill 页面](https://clawhub.ai/flanchanxwo/skills/pixiv-cli) 执行 `clawhub install pixiv-cli` 安装已发布的 `pixiv-cli` Skill；请固定到与 CLI 发布相同的 Skill 版本，不要跟随未固定的 latest。

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

# 通过隐藏输入保存 FANBOX session，然后查看 FANBOX 内容。
pixiv fanbox auth import
pixiv fanbox post 123456

# 使用 App 服务端筛选搜索。
pixiv search "初音ミク" --type illust --ai-mode exclude --resolution high
pixiv novel search "初音ミク" --rating sfw --min-text-length 1000

# 关注创作者并收藏作品。
pixiv follow add 12345678
pixiv bookmark add 123456

# 查看详情、获取推荐并下载。
pixiv detail https://www.pixiv.net/artworks/123456
pixiv recommended all --limit 10
pixiv timeline latest --type illust --limit 20
pixiv download https://www.pixiv.net/artworks/123456 --pages 1,3-5 --quality regular
pixiv download 123456 https://i.pximg.net/img-original/example.jpg --concurrency 8

# 批量下载某位创作者的全部视觉作品。
pixiv download https://www.pixiv.net/users/12345678/artworks
```

运行 `pixiv --help`，或打开[完整 CLI 参考手册](docs/zh-CN/cli-reference.md)，查看全部命令、flag、配置键、环境变量、fallback 规则和更新行为。

## 选择使用入口

### CLI

交互时默认输出可读文本；命令支持时可用 `--json` 获取机器可读输出：

```bash
pixiv ranking --mode day --json
pixiv user search "miku" --limit 10 --json
pixiv user detail 12345678
pixiv timeline latest --type illust --limit 10 --json
```

### MCP

诊断默认关闭；显式启用后只将安全事件写入 stderr，MCP stdout 仍为 JSON-RPC。

显式启动 stdio server。stdout 只用于 JSON-RPC；tool 运行失败会以 `isError=true` 的 structured result 返回。默认不创建项目级或每日日志文件。

```bash
pixiv mcp
# 可选的安全诊断写入 stderr，不改变 MCP stdout。
pixiv --debug mcp
# FANBOX tools 使用独立的 runtime credential 选择。
pixiv --debug fanbox mcp
```

[MCP tool 契约](docs/zh-CN/mcp-tools.md)记录了 tools、参数、structured output 和认证行为。
MCP 固定状态、错误和展示文本使用英文；Pixiv 元数据及用户提供的文本保持原文。

### Go SDK

Public SDK 显式接收 credential，不读取 CLI 的本地账号库。下面示例只为避免把 secret 写入源码而从进程环境读取 refresh token；应用应自行保存 `Open` 返回的 rotation 后 credential：

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func main() {
	ctx := context.Background()
	client, _, err := pixiv.Open(ctx, os.Getenv("PIXIV_REFRESH_TOKEN"))
	if err != nil {
		log.Fatal(err)
	}

	result, err := client.SearchArtworks(ctx, pixiv.SearchArtworksRequest{Word: "初音ミク"})
	if err != nil {
		log.Fatal(err)
	}
	for _, artwork := range result.Items {
		fmt.Printf("%d %s\\n", artwork.ID, artwork.URL)
	}
}
```

SDK import path 为 `github.com/FlanChanXwO/pixiv-cli/sdk/pixiv`。`sdk/fanbox` 显式接收 `FANBOXSESSID`，支持 Firefox 148 native 路由，以及可选的服务级 proxy、user-agent 和仅 challenge 使用的 FlareSolverr。`Download`/`DownloadAll` 使用有文档依据的新手默认值；`DownloadWith`/`DownloadAllWith` 可控制路径、命名、页码、质量与并发。[SDK 指南](docs/zh-CN/sdk.md)说明模型、cursor、资源、错误和调用方职责。

## 认证与 token 安全

推荐使用 `pixiv auth login` 完成配置。它把原始 Pixiv App OAuth refresh token 按 UID 保存在本地账号 store。

### 账号信息免责声明

账号名称、UID、会员状态提示和当前本地账号选择来自本地存储与 Pixiv 响应，仅用于操作便利；它们不构成账号归属、权限或当前 Pixiv 状态的证明。请在 Pixiv 核对重要账号信息，并且只管理已获授权的账号。

macOS、Windows 与桌面 Linux 上执行 `pixiv` 命令会为已安装的 binary 准备当前用户的 `pixiv://` callback handler。服务器同时配置 `login_relay_public_url` 和 `login_relay_listen_addr` 后，`pixiv auth login` 会输出一次性远程 handoff URL。打开该 URL 会直接转交给已安装的桌面 handler，由它启动 OAuth 并将 callback 回传服务器。因此远程登录需要安装 pixiv-cli 的桌面端；项目不再提供确认页或复制 callback 的表单。详见 [CLI 参考手册](docs/zh-CN/cli-reference.md#获取-refresh-token)。

```bash
pixiv auth list
pixiv auth use 12345678
pixiv auth check
```

## 文档

| 文档 | 用途 |
| --- | --- |
| [CLI 参考手册](docs/zh-CN/cli-reference.md) | 命令、flag、认证、配置、fallback、下载和更新 |
| [Go SDK](docs/zh-CN/sdk.md) | Public client、模型、分页、资源和 typed error |
| [MCP tools](docs/zh-CN/mcp-tools.md) | Tool schema 与输出语义 |
| [架构](docs/maintainers/architecture.md) | 包边界和运行流程 |
| [开发流程](docs/maintainers/development.md) | 工具链、测试、构建和发布 |
| [更新日志](changelog/README.zh-CN.md) | 用户可感知变化 |

## 参与贡献

欢迎提交 bug、文档修复、测试和聚焦功能。发起 pull request 前请阅读 [CONTRIBUTING.zh-CN.md](CONTRIBUTING.zh-CN.md)；较大或影响公开接口的变更请先讨论。

## 许可证

[MIT](LICENSE) © FlanChanXwO
