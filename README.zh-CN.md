<div align="center">

# pixiv-cli

**Pixiv CLI · MCP stdio server · Go SDK**

[English](README.md) · [简体中文](README.zh-CN.md)

<p><a href="https://github.com/FlanChanXwO/pixiv-cli/actions/workflows/ci.yml"><img alt="Quality gate" src="https://github.com/FlanChanXwO/pixiv-cli/actions/workflows/ci.yml/badge.svg"></a> <a href="https://github.com/FlanChanXwO/pixiv-cli/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/FlanChanXwO/pixiv-cli?style=flat-square"></a> <a href="go.mod"><img alt="Go" src="https://img.shields.io/github/go-mod/go-version/FlanChanXwO/pixiv-cli?style=flat-square"></a> <a href="LICENSE"><img alt="License" src="https://img.shields.io/github/license/FlanChanXwO/pixiv-cli?style=flat-square"></a> <img alt="Views" src="https://hits.sh/github.com/FlanChanXwO/pixiv-cli.svg?style=flat-square&amp;label=views"></p>

[安装](#安装) · [快速开始](#60-秒快速开始) · [使用入口](#选择使用入口) · [文档](#文档) · [参与贡献](CONTRIBUTING.zh-CN.md)

</div>

`pixiv-cli` 把 Pixiv 生态带到终端：发现作品与创作者、管理账号和作品收藏、关注创作者，并下载视觉作品。它提供独立的非官方 CLI、MCP server 和 public Go SDK，与 Pixiv Inc. 无隶属或背书关系。CLI 与 MCP server 共同调用 public Go SDK，并以 Pixiv App API 作为已认证能力的数据源；使用时请遵守 Pixiv 条款与适用法律。

## 为什么选择 pixiv-cli？

- **一致的能力面**——CLI、MCP 与 SDK 均可完成关键词搜索、详情、排行、推荐、用户、收藏、关注、下载和 ugoira 处理；反向搜图接入 CLI/MCP 能力面。
- **只读 FANBOX 能力**——通过 `FANBOXSESSID` 登录后，可从 CLI、MCP 或 `sdk/fanbox` 查看创作者、帖子、主页/支持中 feed、标签和第一方文件资源。
- **组合式视觉作品管道**——视觉列表接入管道时自动输出 canonical NDJSON；用 `--filter` 编写有类型的本地作品筛选，并可直接传给 `download`。
- **本地账号池**——为读取型任务选择符合条件的本地账号，并在分页和下载准备阶段遵循 Pixiv 的 `Retry-After` 响应。
- **易用的账号登录流程**——运行 `pixiv auth login` 即可在浏览器完成 OAuth，随后可使用 `auth list`、`auth use` 和 `auth check` 管理和确认本地多账号。
- **Ugoira 输出模式**——可选择 GIF 或 APNG；文件名模板非法或渲染为空时使用稳定默认文件名，并以 warnings 保持可观测。
- **明确的下载结果**——可选择图片质量和闭区间页码，将允许的 Pixiv CDN URL 作为直链来源，并保留已完成文件、warnings 与 failures。
- **认证 App API 发现能力**——通过 App API 读取 R18 详情、分页、ugoira metadata 和全部 16 种排行榜。
- **实用搜索筛选**——支持分级、作品类型、AI 模式、横纵比、分辨率和版本内置的绘图工具目录；反向搜图支持从本地文件或 URL 查询 SauceNAO、ascii2d。
- **直达 Pixiv 引用**——可把受支持作品 URL 直接粘贴给详情或下载；已认证的作者主页/作品页 URL 会展开为该作者的视觉作品。
- **本地多账号 OAuth**——支持浏览器登录、账号选择、refresh token rotation 和可选的跨机器 callback relay。
- **适合自动化**——typed SDK error、JSON 输出、纯净 MCP stdio、签名更新和完整结果报告。

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

### Docker（Linux amd64/arm64）

官方镜像发布到 GHCR：`ghcr.io/flanchanxwo/pixiv-cli`。需要可复现部署时，请拉取精确 release：

```bash
docker pull ghcr.io/flanchanxwo/pixiv-cli:v1.2.3
```

`latest` 只跟随 stable release；prerelease tag 绝不移动 `latest`。要跟踪当前 stable release，可拉取
`ghcr.io/flanchanxwo/pixiv-cli:latest`。镜像分别为 `linux/amd64` 和 `linux/arm64` 原生构建。容器运行同一个
`pixiv` binary，并使用与其他安装方式相同的 `~/.pixiv-cli` 状态命名空间。

持久保存账号状态，并挂载下载工作区：

```bash
docker run --rm \
  -v pixiv-cli-state:/home/pixiv/.pixiv-cli \
  -v "$PWD:/work" \
  ghcr.io/flanchanxwo/pixiv-cli:v1.2.3 \
  --version
```

下载文件请配合显式输出路径放在 `/work`；它是容器工作目录，不代表独立的产品模式。

bind mount 会保留宿主目录属主。如果宿主目录不能被镜像内的 UID 1000 写入，请以宿主身份运行、给容器临时 `HOME`，并选择显式输出路径：

```bash
docker run --rm \
  --user "$(id -u):$(id -g)" \
  -e HOME=/tmp/pixiv-cli \
  -v "$PWD:/work" \
  ghcr.io/flanchanxwo/pixiv-cli:v1.2.3 \
  download URL --output /work/downloads
```

若要在同一宿主身份下持久化状态，请绑定一个你拥有的宿主目录，而不是默认 named volume：

```bash
mkdir -p "$PWD/pixiv-cli-state"
docker run --rm \
  --user "$(id -u):$(id -g)" \
  -e HOME=/tmp/pixiv-cli \
  -v "$PWD/pixiv-cli-state:/home/pixiv/.pixiv-cli" \
  ghcr.io/flanchanxwo/pixiv-cli:v1.2.3 \
  auth list
```

请通过 stdin 导入 refresh token，不要把它作为 argv 值传入。手动导入时分配 TTY，让隐藏输入提示避免回显：运行命令后粘贴 opaque token，再发送 EOF（`Ctrl-D`）。

```bash
docker run --rm -it \
  -v pixiv-cli-state:/home/pixiv/.pixiv-cli \
  ghcr.io/flanchanxwo/pixiv-cli:v1.2.3 \
  auth import
```

自动化场景请把 secret manager 的 stdout 直接管道给 stdin，使用 `-i` 而不是 `-it`。容器只写入持久状态 volume：

```bash
secret-manager print-token | docker run --rm -i \
  -v pixiv-cli-state:/home/pixiv/.pixiv-cli \
  ghcr.io/flanchanxwo/pixiv-cli:v1.2.3 \
  auth import
```

这沿用既有 `pixiv auth import` 行为，不会为 `auth login` 添加 Docker-specific OAuth callback 流程。

MCP 传输保持 `docker run --rm -i ghcr.io/flanchanxwo/pixiv-cli mcp`，stdout 仍保留给 MCP JSON-RPC。可按下方命令固定 release：

```bash
docker run --rm -i ghcr.io/flanchanxwo/pixiv-cli:v1.2.3 mcp
```

如果 MCP server 需要复用已保存账号，请追加 `-v pixiv-cli-state:/home/pixiv/.pixiv-cli`。

通过拉取新镜像升级，并使用同一 state volume 重新部署。该流程不会将 `pixiv update` 改为 container-aware。

### 让 AI Agent 安装

把下面这一段 prompt 复制给能够操作本机终端的 Codex、Claude Code、Cursor 或其他 AI Agent：

```text
请为这台机器安装 https://github.com/FlanChanXwO/pixiv-cli 的最新 stable 版本：先审阅仓库中的 scripts/install.sh 或 scripts/install.cmd，再根据检测到的操作系统与架构选择对应脚本（Windows 必须使用 cmd.exe，禁止调用 PowerShell），只下载官方 GitHub Release 资产，只有发布的 SHA-256 校验通过后才能替换文件，使用无需管理员或 root 权限的用户级目录，只把选定安装目录加入用户 PATH，缺少任何前置工具时先征求同意，绝不读取或输出 Pixiv 凭据，最后运行 pixiv --version 验证，并报告安装版本及全部文件和 PATH 变更。

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

# 对本地图片或 HTTP(S) 图片 URL 反向搜图；可输出 JSON 或 NDJSON。
pixiv search ./image.png --provider ascii2d-color --json
pixiv search https://your-image-url.example/image.png --provider all --ndjson

# 关注创作者并收藏作品。
pixiv follow add 12345678
pixiv bookmark add 123456

# 查看详情、获取推荐并下载。
pixiv detail https://www.pixiv.net/artworks/123456
pixiv recommended all --limit 10
pixiv timeline latest --type illust --limit 20
pixiv download https://www.pixiv.net/artworks/123456 --pages 1,3-5 --quality regular
pixiv download 123456 https://i.pximg.net/img-original/example.jpg

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

反向搜图属于 CLI/MCP integration；public Go SDK 保持不变。

显式启动 stdio server。stdout 只用于 JSON-RPC；tool 运行失败会以 `isError=true` 的 structured result 返回。默认不创建项目级或每日日志文件。

```bash
pixiv mcp
# FANBOX tools 使用独立的 runtime credential 选择。
pixiv fanbox mcp
```

[MCP tool 契约](docs/zh-CN/mcp-tools.md)记录了 tools、参数、structured output 和认证行为。
MCP 固定状态、错误和展示文本使用英文；Pixiv 元数据及用户提供的文本保持原文。

`reverse_search` 接受常规本地文件或 HTTP(S) URL，并可能把图片上传给第三方 provider。
可信本机 MCP client 可以请求私有文件以及私网/loopback/link-local URL，因此只应在可信 client
中运行；详见 [MCP 反向搜图契约](docs/zh-CN/mcp-tools.md#反向搜图)。
高级 reverse-search proxy、User-Agent 和 challenge-recovery 配置见
[CLI reference](docs/zh-CN/cli-reference.md)。FlareSolverr 只负责 JSON
challenge-recovery control path，绝不会收到 native ascii2d image upload。

### Go SDK

Public SDK 显式接收 credential，不读取 CLI 的本地账号库或进程环境。应用应从自己的 secret store 取得 credential，并自行保存 `Open` 返回的 rotation 后 credential：

```go
package main

import (
	"context"
	"fmt"
	"log"

	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func main() {
	ctx := context.Background()
	refreshToken := "replace-with-a-refresh-token-from-your-secret-store"
	client, _, err := pixiv.Open(ctx, refreshToken)
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

SDK import path 为 `github.com/FlanChanXwO/pixiv-cli/sdk/pixiv`。`sdk/fanbox` 显式接收 `FANBOXSESSID`，使用 Chrome 146 TLS profile 与内置 Firefox 148 HTTP User-Agent baseline 的 native 路由，以及可选的服务级 proxy、user-agent 和仅 challenge 使用的 FlareSolverr。公开 SDK 提供 typed Pixiv/FANBOX client 和 opaque resource API，而不是 CLI 批量下载 helper：`SaveResource` 接收 `ResourceRef`；Pixiv 的 `SaveResourceURL` 接收允许的 Pixiv media host 上的 HTTPS URL。两者都使用原子写入，并返回目标路径、字节大小和响应 Content-Type。[SDK 指南](docs/zh-CN/sdk.md)说明模型、cursor、资源、错误和调用方职责。

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
| [架构](docs/zh-CN/maintainers/architecture.md) | 包边界和运行流程 |
| [开发流程](docs/zh-CN/maintainers/development.md) | 工具链、测试、构建和发布 |
| [更新日志](changelog/README.zh-CN.md) | 用户可感知变化 |

## 参与贡献

欢迎提交 bug、文档修复、测试和聚焦功能。发起 pull request 前请阅读 [CONTRIBUTING.zh-CN.md](CONTRIBUTING.zh-CN.md)；较大或影响公开接口的变更请先讨论。

## 许可证

[MIT](LICENSE) © FlanChanXwO
