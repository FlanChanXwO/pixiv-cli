# 开发流程

## 环境与验证

`go.mod` 声明 Go `1.26.3`。开工前检查：

```bash
go version
go test ./...
go test -race ./...
go vet ./...
sh scripts/build.sh
```

ugoira 转换还需 `ffmpeg`；缺失时普通图片可下载，ugoira 返回明确错误。

真实 Web fallback e2e 不属于默认回归，只有显式设置环境变量才联网：

```bash
PIXIV_E2E_WEB_API=1 PIXIV_WEB_API_PROXY=http://127.0.0.1:7890 go test ./test/e2e -run WebAPIFallbackReal -count=1 -v
```

## 公共 SDK 开发

公开 API 只放 `pkg/pixiv`，保持具体 `*pixiv.Client`、稳定 request/result/model、opaque `Cursor` 与 `*pixiv.Error`。不要把 CLI/MCP 类型、内部 transport 或上游 wire model 泄漏到包外；也不要为未来调用方预建大 interface。

调用方需接口时，在调用方拥有的 adapter 中按实际需要定义窄 interface。例如采集服务可定义只含 `SearchIllust` 和 `IllustDetail` 的接口，再用 `*pixiv.Client` 适配。调用方负责 source mode、budget、filters、cursor 持久化、入库和调度。

新增或变更公开 API 时：

1. 先写 external package test，确认导入方可用的行为。
2. 保持 `NewClient` 无本地 I/O；`OpenDefault` 每操作取快照。
3. 对网络、认证、资源、cursor 路由补充聚焦测试。
4. 同步 [SDK 接口](../pixiv-sdk-interface.md)、README、架构、CHANGELOG 和知识图谱。

## 配置、日志与运行

`auth.json` 与 `config.toml` 位于 `os.UserConfigDir()/pixiv/`，写入权限为 `0600`。可通过：

```bash
pixiv config set log_level debug
pixiv config set log_format json
PIXIV_LOG_LEVEL=warn PIXIV_LOG_FORMAT=text pixiv search "初音ミク"
```

有效日志等级：`trace`、`debug`、`info`、`warn`、`error`；格式：`text`、`json`。SDK 只有传入 `Options.Logger` 才记录日志；MCP stdout 为 JSON-RPC，所有诊断写 stderr。

代理优先级是 `--proxy` > `https_proxy`/`HTTPS_PROXY` > `config.toml`。不要记录 token、cookie、完整 URL 或查询内容。

## 文档与 Git

用户可见的 CLI、MCP、配置、SDK 或安全变化写入 `CHANGELOG.md` 的 `[Unreleased]`。API/结构变更同步 README、相关 `docs/`、ADR 与两份 `.understand-anything` 图谱。不要提交 token、下载内容、数据库、缓存、日志或本机配置。

提交前至少运行与改动匹配的测试及：

```bash
git diff --check
```
