# Pixiv MCP Server 文档

## 项目定位

`pixiv-mcp-server` 是一个 Go 版 Pixiv CLI 和 MCP stdio server。CLI 面向脚本/终端使用；`pixiv mcp` 通过 MCP tools 向客户端提供 Pixiv 搜索、浏览、推荐、排行榜、用户信息、收藏、关注、下载、token refresh 和缩略图获取能力。无 refresh token 时，默认对搜索、详情、排行、用户搜索、下载和缩略图获取启用匿名 Pixiv web/ajax API fallback。

## 文档目录

- [架构说明](architecture.md)：入口、Pixiv/config/utils 包边界、运行流程和关键约束。
- [开发流程](development.md)：本地环境、测试、构建、运行配置和 Git 注意事项。
- [MCP 工具](mcp-tools.md)：当前注册的 tools 与参数概览。
- [ADR 0001](adr/0001-cli-thin-controller-and-bootstrap.md)：CLI thin controller、application services 与 bootstrap 分层决策。
- [ADR 0002](adr/0002-utils-and-common-boundaries.md)：`utils/*` 与 `common/constants` 的边界规则。

## 快速命令

```bash
go test ./...
go build -o pixiv ./cmd/pixiv
```

CLI 示例：

```bash
pixiv auth login main
pixiv search "初音ミク" --json
pixiv download 123456
```

MCP 运行示例：

```bash
PIXIV_REFRESH_TOKEN=... \
DOWNLOAD_PATH=./downloads \
FILENAME_TEMPLATE="{author} - {title}_{id}" \
./pixiv mcp
```

真实 token 写在 inline 环境变量里也可能进入 shell history；长期使用建议通过 MCP client 的私密环境配置或本地账号管理。

stdout 保留给 MCP JSON-RPC；日志写入 stderr。
