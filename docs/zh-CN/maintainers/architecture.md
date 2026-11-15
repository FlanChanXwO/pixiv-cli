# 架构说明

[English](../../en/maintainers/architecture.md) | 简体中文 | [文档索引](../../index-zh-CN.md)

```mermaid
flowchart LR
    ENTRY["cmd/pixiv<br/>唯一二进制入口"] --> CLI["internal/cli<br/>命令与生命周期"]
    CLI -->|"stdio / HTTP"| MCP["internal/mcpserver<br/>Pixiv / FANBOX 工具"]
    CLI --> SDK["公开 SDK<br/>sdk/pixiv · sdk/fanbox"]
    MCP --> SDK
    SDK --> API["sdk/pixiv/internal/api · sdk/fanbox/internal/api<br/>传输、资源策略与凭据"]
    SDK --> OAUTH["sdk/pixiv/internal/api/oauth<br/>PKCE 与 token 交换"]
    CLI --> SHARED["internal<br/>跨子系统机制"]
    MCP --> SHARED
    CLI --> STATE["config / storage<br/>配置与本地状态"]
    CLI -.->|"仅组合根"| RSA["internal/reversesearch"]
    RSA --> RSSDK["sdk/saucenao · sdk/ascii2d"]

    subgraph boundary["不可跨越的边界"]
        RULE["CLI / MCP 从不直连协议适配器<br/>共享机制不承载产品协议语义"]
    end
```

> [!IMPORTANT]
> CLI 与 MCP 只能通过公开 SDK 与 owner-local 窄端口获得业务能力。生产组装可以依赖具体实现，但不得向命令层暴露 locator、runtime graph 或协议适配器。

| 读者 / 关注点 | 跳转 |
| --- | --- |
| 从入口到 SDK 的调用链 | [总体流程](#总体流程) |
| 页面源码与嵌入 | [前端源码与页面嵌入](#前端源码与页面嵌入) |
| 某个包能做/不能做什么 | [包职责](#包职责) |
| MCP server、OAuth、媒体代理、Gallery 任务 | [MCP server 架构](architecture-mcp.md) |
| Release 信任根与签名 | [Release assets 与信任边界](#release-assets-与信任边界) |
| 本地状态、配置、路径权限 | [internal/account](#internalaccount)、[internal/config](#internalconfig)、[internal/storage](#internalstorageatomicfilefilelocksecretfile) |
| 硬约束清单 | [已知约束](#已知约束) |

## 总体流程

```mermaid
flowchart TD
    M["cmd/pixiv/main.go"] --> R["internal/cli.Run"]
    R --> H{"参数"}
    H -->|无| HELP["CLI help"]
    H -->|"auth · config · update · search · timeline · detail<br/>ranking · recommended · user · bookmark · follow · download"| CMD["CLI 模式"]
    H -->|"--version"| V["独立的只读 flag"]
    H -->|fanbox| FB["FANBOX 模式"]
    H -->|mcp| MCP["统一 stdio（默认）/<br/>Streamable HTTP（--port）"]
    CMD & FB & MCP --> RES["按 owner 构造生产资源"]
    RES --> DB["~/.pixiv-cli/auth.json<br/>（本地账号）"]
    RES --> CFG["~/.pixiv-cli/config.toml<br/>+ 公开环境变量覆盖"]
```

- `auth import` 处理直接导入 token 或恢复 bundle；`auth export` 处理本地 secret 快照。`pixiv fanbox mcp` 已移除。
- Windows 路径为 `%USERPROFILE%\.pixiv-cli\`。首次访问账号时创建 `auth.json`；存在本地 `pixiv-cli.db` 时自动导入，旧数据库保持原样。
- 没有匿名 Web fallback：内容需要已认证的本地账号或显式凭据；残留的 `web_fallback_enabled` 属于未声明键，加载时忽略。

## 前端源码与页面嵌入

| 源码目录 | 负责 | 嵌入方式 |
| --- | --- | --- |
| `frontend/mcp-apps/gallery/` | Gallery HTML/CSS/JS | CSS/JS 内联 |
| `frontend/pages/login/` | 登录/回调/结果模板与样式 | 保持分离，逐字节保留 |
| `frontend/pages/oauth/` | 同意页 HTML/CSS | CSS 内联 |

纯资源包 `frontend` 嵌入精确路径（不含 fixture），启动时用标准库组装一次页面；页面行为通过相应浏览器/MCP 宿主手动检查。Go owner 保留 MCP 注册、模板渲染/数据/转义、CSP/媒体 origin、canonical action、CSRF 与授权。当前与历史资源 URI 均受支持。干净构建与带版本的 `go install` 无需生成步骤；运行时只读嵌入字节。Homebrew 模板保持原 owner。

## 包职责

### `cmd/pixiv`

生成 `pixiv` 二进制的 `main` 包；没有业务逻辑，只调用 `internal/cli.Run` 并返回退出码。

### `internal/cli`

CLI 组合根。`internal/cli` 只有一个 `root.go` 和命令目录；与 MCP 或另一产品共用的代码都放在模块级 owner：

| 文件 | 负责 |
| --- | --- |
| `root.go` | `Run`、Cobra 命令树、全局 flag、按需启动 hook、退出码映射、stderr logger 启动、broken-pipe 策略、生产装配与反向搜索 ports；绑定反搜配置与 HTTP 连接 |

职责：Cobra 命令树/help/flag；文本/JSON 输出；`auth import` 输入适配（位置参数 = opaque token；无参数时 TTY 隐藏输入，非 TTY 按首个非空白字节区分 token 与严格 bundle；通过管道/重定向恢复，没有 `--file`）；`auth export` secret 输出适配（无 `--output`：原始 token + 换行，或 `--all` 的 bundle；有 `--output`：私有文件 + 不含 secret 的摘要）；`--page`/`--limit` 解析（逻辑计划在 `internal/pagination`）；`auth login` 的 loopback OAuth、浏览器与 TTY；`pixiv mcp` 分发；根 `--version` 与 `pixiv update`（已移除的 `version` 子命令在解析时返回 unknown command）；只写 stderr 的只读自动更新提示。

- **关闭：** `lifecycle.CloseState` 登记 `Run` 打开的资源；调用方在关闭前停止资源生产者并完成注册；互斥锁保护列表，不保护迟到的注册；closer 不得向同一 scope 注册或递归关闭它。关闭在命令完成后进行；MCP stdio 等待进行中的 handler；关闭错误被合并。
- **Owner：** `internal/cli` 只有一个 `root.go` 与命令目录，没有 `internal/` 子目录，也没有测试文件。`root.go` 通过显式工厂与窄端口把生产依赖接到命令 owner，并逆序关闭已登记的资源。没有跨命令 locator、bootstrap 构造器或 `internal/cli/runtime`。CLI 契约测试放在 `cmd/pixiv`，通过 `cli.Run` 做黑盒测试。
- **命令包：** 根级 `internal/cli/{config,mcp,update,whoami}`；Pixiv `internal/cli/{auth,bookmark,comment,detail,download,follow,mypixiv,ranking,recommended,search,series,timeline,user}`；FANBOX `internal/cli/fanbox/{auth,browse,download}`。组的代理 flag 由 `internal/cli/fanbox` 拥有。Pixiv 命令接收 `environment.PixivExecutor` 并使用 `*pixiv.Client`，列表输出使用 `record.RunSDKList`。FANBOX 命令接收延迟 SDK lease 工厂，调用 `fanbox.UseClient`。流与输入输出支持由 `environment.Env` 提供。
- **共享 helper 位于模块级 owner：** `internal/environment`（流、stdin codec、flag、usage/生命周期标注与 Pixiv 执行端口）、`internal/pagination`（逻辑 page/limit 预算）、`internal/record`（稳定输出值、资源引用与列表输出）、`internal/display`（终端渲染）、`internal/pixiv/target`（目标解析）。`internal/account` 提供存储、轮换门、lease 与读会话，`root.go` 将它们与配置组合。这些包不导入 `internal/cli` 或 `internal/mcpserver`。
- **`--version` / 自动更新：** stdout 恰为 `pixiv <version>`，stderr 为空，不检查更新。自动更新只在普通业务命令成功后运行，MCP、help、`--version`、`update`、所有 `auth export`、bundle 形式 `auth import`、标记为离线的命令（例如 `whoami`）与 dev 构建跳过；选择 stable Release，用户缓存 24 小时节流，最多等 3 秒；失败只作为 stderr warning——不改变退出码，不泄漏到 JSON stdout 或 MCP JSON-RPC。
- **Windows 启动清理** 待更新文件属于潜在写操作：所有 `auth export` 调用在 Cobra 解析前识别并跳过它。根 bool 解析变更时，手动核对 `--help=false` 等重复覆盖形式仍遵守 secret 导出边界。

### `internal/urlscheme`

负责 `pixiv://` URL scheme：平台 handler 安装、回调分发、持久化 handler 清单、一次性远程 hand-off 私有状态与远程回调 client。`internal/cli/auth` 通过它安装按需 handler，并保留 OAuth、loopback HTTP、浏览器、TTY 与 relay server 编排。中继的 HTTP 协议留在 `internal/loginrelay`。

| 平台 | 机制 |
| --- | --- |
| Darwin | 内嵌 Swift、`Info.plist`、LaunchServices |
| Windows | 当前用户注册表/class 启动 |
| 桌面 Linux | XDG desktop entry + `gio` |
| 无头 Linux | 不注册 handler；relay server 仍可用 |

只有精确的 `pixiv://account/login` 与 `pixiv://account/remote-login` 进入本轮；活动 loopback 优先；远程回调只交给活动的一次性 hand-off；其他 URL 交给清单中持久化的 handler。桌面状态只保存当前 hand-off 的 relay origin、会话 ID 与 capability；server 不保存设备记录；SDK 不暴露这些状态。远程回调只接受同一 relay base 上的一次性结果 URL；`internal/cli` 打开不敏感的最终页面并等待交换。

### 横切关注点

每个关注点只有一个归属层，其他层不重复实现。

1. 每个关注点只有一个归属层和一种传入方式。
2. 业务对象可以持有基础设施，但只持有一个已经封装好的依赖（一个 transport、一个 repository），不持有构成它的零件（`http.Client`、`selfHTTP`、`sync.Mutex`、缓存 map）。
3. 请求范围内的信息（取消、调用关联、日志）通过 `context.Context` 传递，不放进字段。
4. 只为测试存在的注入点不出现在导出 API 中。同包测试可以设置未导出字段。

| 关注点 | 归属层 | 传入方式 | 业务对象中的形态 |
| --- | --- | --- | --- |
| 日志、诊断事件 | SDK 产品/协议 owner（请求与决策）；执行边界（最终结果） | 当前 slog 默认 logger；应用调用信息通过 `context.Context` | 无 logger 字段 |
| 取消、超时、调用关联 | 执行边界 | `context.Context` | 方法参数 |
| HTTP client、代理、重试、Retry-After、请求间隔 | `sdk/pixiv/internal/api`、`sdk/fanbox/internal/api` | 构造时传入一次 | 一个 transport 字段 |
| 媒体域名白名单与重定向 | `sdk/pixiv/internal/api`、`sdk/fanbox/internal/api` | 构造设置与已校验元数据 | 无额外策略字段 |
| 资源释放（`Close`、`CloseIdleConnections`） | 创建者；`lifecycle` 包提供 `Lease` | 构造函数返回关闭函数或 `Lease` | 无额外字段 |
| 账号快照、文件锁、原子更新 | `internal/account` | `Store` 方法持锁重读 `auth.json` | 一个 store 依赖 |
| 本地凭据轮换的持久化 | `internal/account` | 绑定账号的 `pixiv.CredentialStore` | 离线构造 client，刷新凭据先保存再请求内容 |
| 配置 | 组合根读取一次 | 以具名字段的值传入构造函数 | 值字段，不持有 `settings.Store` |
| stdout 输出（文本/JSON/NDJSON） | CLI 命令层 | 命令持有 writer | 业务返回数据 |

基础设施包 `sdk/pixiv/internal/api`、`sdk/fanbox/internal/api`、`internal/storage/*`、`internal/pixiv/saving`、`internal/mcpserver/pixiv/mediaproxy` 拥有并发、缓存、文件与网络机制，对外暴露窄接口，内部零件保持私有。

### 日志入口

| 位置 | 负责 |
| --- | --- |
| `internal/logging/` | CLI/MCP handler 与 ctx 调用信息；文本格式为 `[HH:MM:SS.mmm] [operation] [LEVEL] [上级目录/文件:行号]`，按终端着色、转义文本、缩进多行值；JSON 带 `source`；内部恢复点使用 `logging.From(ctx)` |
| `internal/cli/root.go` | 启动 stderr logger、`slog.SetDefault` 与清理后恢复；JSON 以 ERROR、文本以 DEBUG 记录 `Command failed`，保留 stderr 错误输出 |
| `internal/cli/mcp/runtime.go`、`internal/mcpserver/toolrun/toolrun.go` | MCP logger 端口与 `Scope` / `AddTool` 调用绑定；错误或 `isError` 结果记录一次受控失败摘要 |
| `sdk/pixiv/internal/api/`（含 `oauth/`）、`sdk/fanbox/internal/api/` 等 SDK 包 | 在请求/决策处使用当前 slog 默认 logger；SDK 自持属性常量，保留协议键名 |
| `internal/netdiagnostic/` | 共用的 typed transport 属性提取：`op`、`net`、IP `address` 与已知 `errno` 名，省去原始错误文本与 URL |

`operation` 表示 CLI 命令路径或 MCP 工具名，`call_id` 区分并发工具调用，`source` 标明日志位置。SDK 请求事件还可能携带 `sdk_operation`。CLI/MCP 日志只写 stderr，数据 stdout 与 JSON-RPC 通道保持纯净。SDK 依赖图独立于 `internal/logging` 与终端处理。

### 共享机制

| 包 | 负责 | 不得 |
| --- | --- | --- |
| `internal/version` | linker 注入的 `Version`（默认 `dev`；只有 `dev` 构建拒绝自更新） | — |
| `internal/record` | CLI/MCP 共享的稳定 Pixiv record 投影：新绑定对象投影为现有输出值，固定 `id`/`type`/`url`，JSON 规范化、版本元数据清理、基于 record 字段的本地筛选；CLI 的 NDJSON 消费与逐行诊断、列表 JSON 暂存和合并后的 Pixiv/FANBOX 列表输出 | 依赖 CLI、MCP 或适配器；变成杂物箱（MCP schema 与输出信封留在 `internal/mcpserver/pixiv/internal/{records,outputs}`） |
| `internal/environment` | CLI 执行环境、流、stdin codec、flag、usage/生命周期标注；`PixivRequest` 与 `PixivExecutor` 描述应用执行边界 | 产品协议、账号存储；导入 `internal/mcpserver` |
| `internal/display` | 只供 CLI 的终端文本：实体列表行、详情卡片、`用户名 (UID)`、上游转义；并入原 `plaintext` | JSON/NDJSON 输出；只服务一个命令的渲染 |
| `internal/pixiv/searchfilter` | 作品本地分级/内容类型规范化，客户端侧 `x_restrict` 与类型匹配，隐藏原始值的 cursor 上下文 | 把分级映射为未确认的上游 `x_restrict` 参数；持有 client、凭据或适配器（字段可用性与已确认的 wire 参数由各命令负责） |
| `internal/lifecycle` | 与协议无关的生命周期、`Lease`、`Attempt`、`CloseState` | 负责账号选择、凭据或重放策略 |
| `internal/pagination` | 单流与有序聚合流的逻辑分页：筛选先于全局 skip/limit 预算，`OneBatch` 在首个匹配批次停止，批次截断通过来源位置 checkpoint 续读；`StreamState` = 当前流 + 各流 cursor（owner 与查询/账号/子类型绑定一起编码） | 产品语义 |
| `internal/pagination` | 单流/聚合读取的可重入执行生命周期，安全重放前清除未提交结果 | 负责命令筛选装配 |
| `internal/logging` | 应用 slog handler、ctx 调用信息与执行属性键；SDK 请求键由 SDK owner 自持 | 日志文件、响应体、Cookie、token、query、签名 URL 或任意 cause 转储 |

**账号池读取：** `root.go` 提供 `environment.PixivExecutor`，MCP 经 `SDKPorts.ExecuteSDK` 使用同一执行边界。回调同时返回提交状态与错误。`WriteSDK` 在进入回调时提交，避免跨账号重放结果未知的写入。命令 owner 负责流式输出、分页与部分交付：JSON 暂存到成功后输出，可见的文本/NDJSON 会提交当前尝试；`pagination` 在安全重放前清除未提交结果。

**稳定输出：** `internal/record` 拥有输出 DTO 与资源引用 codec。`pixiv_output.go` 和 `pixiv_user_output.go` 在本次尝试内投影 SDK 对象，`pixiv_resource.go` 保留 CLI 资源身份。Gallery 使用 `RecordFromMediaArtwork`。可用媒体地址留在 SDK 对象或私有媒体缓存，适配器选择安全字段公开。`internal/pixiv/target` 经 SDK 引用解析，拥有应用目标描述符。

### 业务会话、账号与账号池

- `internal/account` 拥有 Pixiv/FANBOX 有序快照、凭据持久化、登录完成、轮换门、读会话缓存与 Pixiv 账号池选择/冻结/重放。调用方提供目录、网络选项与池策略。
- `internal/cli/root.go` 直接组合 `Store`、`Gate` 和 `ReadSessions`，解析配置/默认账号、构造 SDK lease 并登记清理。默认选择与本地导入/导出/恢复编排位于 `internal/cli/auth` 和 `internal/cli/fanbox/auth`；MCP 接收组合根的具体函数端口。
- `Store.OpenSDKClient` 提供 `sdk/pixiv` client。绑定账号的 `CredentialStore.Load` 读取最新 revision 并固定创建时间，`CompareAndSwap` 比较两者后保存；revision 冲突返回 `false, nil`。OAuth 在存储锁外执行，持久化失败会停止内容访问。
- 读会话缓存有效的访问凭据与统一失效代次，在复用、发布及每次内容请求前检查已保存的 revision 与账号创建时间；旧代次的失败只作用于该代次。
- 根执行器单独交出所选 UID；加载凭据前 client 的 `UserID()` 可能为零。缺省目标在每次尝试内解析。stdio `ResolveSDKAccount` 保留请求供池调度，HTTP 使用持久化的 `ResolveAccount` 快照。绑定对象在本次操作内使用。
- `account.Selection.Mode` 使用正数 UID 的 `SelectionFixed` 或 `SelectionPool`。固定账号的读写保持身份，安全且尚未提交的池读取可重放。账号池关闭时，应用 auth 选择配置默认账号或首个存储账号。FANBOX 保持独立凭据/生命周期，采用固定账号。
- CLI/MCP 适配器拥有筛选、record、schema 与输出。下载使用 `internal/pixiv/saving → sdk/download`，以图搜图使用 `sdk/saucenao` 和 `sdk/ascii2d`。SDK 的依赖图独立于本地账号存储。

**登录 relay** —— `internal/loginrelay` 同时负责纯渲染器（消费 `frontend` 模板/CSS，不依赖 CLI、账号库、浏览器或 listener）与可复用的 HTTP handler/session：接收 OAuth 会话校验器并交付一次回调；从不监听、交换或持久化。CLI 保留 listener/TLS/终端编排。宿主 mux 可剥离前缀后挂载；生产 MCP 将其挂在 canonical base + `/pixiv-login/`，位于相同的 Host/Origin 检查之后，但以 hand-off 证明而非 MCP bearer 认证。转义前缀在 URL 与分发中保留。已取消的 relay 会话返回 HTTP 410，并在接受已校验回调前再次检查取消。回调 allowlist、启动响应 wire 类型与结果 URL header 位于 `loginrelay`，由 CLI relay 与桌面 helper 复用。

**MCP 登录 relay**（`accounts.Login`） —— 单一当前登录：重复启动复用待处理工作，restart 取消并等待旧尝试，关闭时等待交换/持久化。完成时保留不含 secret 的 account-saved/selection-updated 标记（含部分失败）。组合根把它绑定到同一 SDK 登录会话，保存凭据但不改 CLI 默认，再单独写 MCP 选择。账号服务在多次 restart 间复用；每次完成的 SDK 登录释放其空闲 HTTP 连接（显式代理/直连时，组合根在每个退出路径关闭其 client 的空闲连接）。

### SDK 协议内部实现

仅供 SDK 使用的上游细节位于 `sdk/<产品>/internal/`，CLI/MCP 经公开 SDK 调用。产品领域文件拥有端点请求、wire 响应校验与映射；应用和 SDK 实际共用的机制放在模块级 `internal/`。

| 位置 | 负责 |
| --- | --- |
| `sdk/pixiv/internal/api/api.go`、`transport.go` | App 请求、认证头、状态处理与节流；幂等读取仅在首次 429 带有效 `Retry-After` 时重试一次；修改请求发送一次 |
| `sdk/pixiv/internal/api/routes.go`、`profile.go`、`failure.go`、`required.go` | 路径/参数/请求头常量、请求 profile、脱敏失败与字段存在性解码 |
| `sdk/pixiv/internal/api/resource.go` | 媒体域名策略、资源读取与重定向校验 |
| `sdk/pixiv/internal/api/oauth/` | PKCE、token 交换/轮换与 refresh token 校验 |
| `sdk/fanbox/internal/api/` | 会话请求头、身份、原生 HTTP、挑战恢复、媒体读取与 URL 策略；端点/query 常量在 `json.go`，请求头/媒体策略在 `protocol.go` / `media.go` |
| `sdk/pixiv/`、`sdk/fanbox/` 领域文件 | 绑定对象、端点请求组装、wire 响应校验/映射与绑定查询/账号的分页；Pixiv 凭据/刷新在 `credentials.go` / `refresh.go` |
| `sdk/internal/cloudflare/`、`sdk/internal/cursor/` | SDK 共用的浏览器 TLS/solver 机制与不透明游标编码 |

Pixiv 每次请求从当前凭据快照构造协议 client。`RequiredList` / `RequiredObject` 区分缺失字段与显式 `null`（`Present` 为 true、`Valid` 为 false）。领域代码处理解码失败并检查完整性；复用外层响应结构前重置状态。

### reverse-search service 例外

`internal/cli/root.go` 读取配置并创建标准 HTTP 连接。CLI 与 MCP 通过顶层 `internal/reversesearch` 加载来源、合并 Pixiv 结果；服务请求交给 `sdk/saucenao` 与 `sdk/ascii2d`。

```mermaid
flowchart LR
    COMP["internal/cli/root.go<br/>（配置与 HTTP 连接）"] --> TOP["internal/reversesearch<br/>（来源读取 + 结果合并）"]
    CMDS["internal/cli/*"] --> TOP
    MCPS["internal/mcpserver/*"] --> TOP
    TOP --> SN["sdk/saucenao"]
    TOP --> A2["sdk/ascii2d"]
```

- Service 返回领域结果；CLI/MCP 适配器在输出边界投影 canonical record。
- 它把普通文件或 HTTP(S) 来源加载为一个私有快照、计算哈希，并在完成后删除。来源策略有意允许任意可读普通文件与私有、loopback、link-local URL；单 owner OAuth 下只授权可信 connector——路径指 MCP server 文件系统，URL 使用 server 网络。只有来源类型/哈希、安全的 provider 摘要/错误、领域证据与 canonical `artwork`/`user` record 可以发布。
- 仅查询 ascii2d 时，在读入图片字节前按 `ascii2d.MaxImageBytes` 拒绝超限快照。SauceNAO 与 `all` 按 SDK 的 `Image []byte` 契约在内存中保留来源字节；SauceNAO 上传时把 multipart 封装与原始字节串联读取，避免再复制一份整图请求体。
- 三个网络面：标准来源/SauceNAO client、专用 ascii2d 浏览器 client、FlareSolverr JSON 控制 client。服务代理只能覆盖 ascii2d；命令级覆盖同时作用于两个原生 client；否则标准 client 走全局路由。ascii2d UA 与 `Chrome_146` TLS profile 及匹配的 Chromium hints 配对（非 Chromium UA 省略 hints）。FlareSolverr 上游代理只用于浏览器 `sessions.create`；控制流量保持独立；图片始终使用原生 ascii2d multipart。ascii2d SDK 在进程内缓存通行状态，并负责每次查询的浏览器连接与 solver 会话清理；状态不持久化。

### `internal/account`

- `Store` 拥有 `auth.json`，标识为 `schema: "pixiv-cli.auth"`、`version: 1`。有序 `pixiv`/`fanbox` 数组保存各产品的 UID 身份、明文凭据与正数 `revision`。数组顺序即显示顺序；新账号追加，更新保留原位置。Pixiv `premium` 可为空，时间均为 UTC；`pixiv_pool_last_user_id` 记录唯一的上次池选择（0 表示无历史选择）。
- 首次访问缺失文件或 schema 缺失/不同的合法 JSON 时执行迁移 1：存在 `pixiv-cli.db` 则导入，否则写空账号文件。非法 JSON、自有 schema 损坏、更新的版本均返回错误并保留原文。有效的当前格式文件成为后续读取的权威。
- 迁移仅通过 `database/sql` 与已有 `ncruces/go-sqlite3` 驱动只读导入，检查 `application_id=0x50495843`、schema 版本 1–3，以及连续且与 `user_version` 一致的迁移账本。导入保留账号顺序、UID、名称、revision、会员与池状态；旧库内容和修改时间保持原样，文件保留原位。
- 每次读取和更新均持有 `auth.json.lock` 并重读文件。更新校验完整新快照，再经 `storage/secretfile` 原子替换私有文件。迁移额外持有 `pixiv-cli.db.lock`，顺序始终为 JSON 锁在先。锁覆盖本地快照与写入；OAuth 和内容请求在锁外执行。可取消的锁等待沿用 `storage/filelock` 的取消语义。
- 凭据轮换先在锁外执行 OAuth，再比较存储中的 revision 与创建时间，将新 token 与递增的 revision 原子保存。revision 冲突时，SDK 加载较新的已保存凭据；持久化失败会在内容访问前停止本次操作。
- `ReadSessions` 在缓存复用前、轮换 client 发布前、每次内容请求前检查 revision 与创建时间。创建时间区分同一 UID 删除后重建的账号。发现凭据替换/删除时作废对应代际；仅元数据变化保留原代际。账号缓存通过这些读取保持一致，无文件系统 watcher。
- Unix 账号目录/文件权限为 `0700`/`0600`；Windows 原子替换沿用共享存储 ACL 策略。普通 JSON、fmt、slog 中的凭据值保持脱敏，仅在文件编码和显式凭据流程展开。auth export 本地读取已保存凭据，保持 token 与配置原值。

### `internal/config`

| 存储 | 保存 |
| --- | --- |
| `auth.json` | 有序 Pixiv/FANBOX 账号身份、凭据、revision 与池状态；Unix `0600` |
| `config.toml` | 全局键，含 `pixiv.auth.default_user_id`、`fanbox.auth.default_user_id`；Unix `0600`。无默认账号时由应用 auth 按数组顺序选第一个账号 |

#### 原生 TOML 配置

`Config` 使用嵌套结构体与原生 `toml` 标签。`Store.Current` 先取 `Default()`，用 `go-toml/v2` 解码文件，再显式应用环境变量并校验结果。缺失字段保留默认值，未知文件键被忽略，每次读取返回独立的配置值。文件值遵循声明的 TOML 类型；解码器在应用环境变量覆盖前报告类型错误。

- `environment.go` 显式赋值支持的变量；存在即生效，包括空字符串。小写 `https_proxy` 优先于 `HTTPS_PROXY`。公开列表见[环境变量](../cli-reference.md#环境变量)。
- 服务代理与 User-Agent 指针区分缺失和显式空值。服务代理缺失时沿用全局路由，空值表示直连。FlareSolverr 组任一字段出现时，URL 须为非空值。
- `values.go` 将 TOML 叶子路径映射到 `Config` 字段指针，并负责字段校验；`Keys()` 提供 CLI 帮助列表。`config get/set/unset` 接受这些路径，包括服务和默认账号设置。`get` 直接输出生效值（含 API key）；`set` 接受值参数或 stdin，并校验目标字段。
- `document.go` 通过 `tomledit` 编辑目标值，保留该键的注释及其他键（含废弃键）；支持普通段、点分键与 inline table，写入前检查 TOML 语义与被编辑的 solver 组，保留逐项修复其他配置错误的能力。`unset` 在 section 清空后仍保留其注释。
- `default.toml` 通过 `go:embed` 内嵌，首次创建时原样写出。英文注释包含可选服务、solver 与账号示例；模板解码测试与 `Default()` 对比。
- 默认账号读写方法保留在此。独立读取方法只校验所选 UID，跳过无关设置的校验；`Config` 中的可选 UID 字段使用 `*int64`。`AccountPool` 仅含 `Enabled` 与 `Strategy`，可调度成员、冻结与调度标记保留在 `internal/account`。
- `paths.go` 负责应用管理路径、`AppDataDirName` 与 Unix 私有权限常量。`HTTPClient` 按所选代理构造独立 transport，空值表示直连。配置解析与 storage 包通过端口连接：组合根将 `secretfile.ConfigFiles` 注入 `FileStore`，复用已有私有文件写入协议。

新增设置时，同步更新 `Config` 字段与原生标签、`Default()`、CLI 字段映射及相关校验、内嵌模板与双语公开参考。仅为支持的变量增加显式环境赋值；按[测试提交规则](coding-standards.md#测试提交规则)复用适用的优先级/空值测试。

| 文件 | 职责 |
| --- | --- |
| `config.go` | 配置类型、原生标签与默认值 |
| `store.go` | 文件端口、内嵌模板创建与有效配置读取 |
| `environment.go` | 显式环境赋值与覆盖检测 |
| `values.go` | CLI 字段路径、格式化、规范化与取值规则 |
| `document.go` | TOML 定位与稀疏修改 |
| `auth.go` | 默认账号读写 |
| `paths.go` | 应用管理路径与私有权限常量 |
| `http.go` | 按配置的代理构造 HTTP transport |

**原子写入**（`FileStore`，由 `internal/storage/secretfile.ConfigFiles` 实现）：

```mermaid
flowchart LR
    T["目标目录中随机名临时文件"] --> W["写入全部内容"] --> S["文件 Sync"] --> C["关闭"] --> R["替换目标"] --> D["同步目标目录<br/>+ 新建目录 叶→根"]
```

- Unix 主动把父目录/文件收紧为 `0700`/`0600`。若本次调用新建了目录，提交后按叶→根顺序同步目标目录与每个新目录的外层父目录，使文件条目与新目录条目都落入持久化边界；已有目录只同步目标目录。
- 失败：替换前（写入/Sync/关闭）失败时保留旧目标；普通替换失败与可恢复的部分完成失败会保留/恢复旧目标并清理临时文件。目录同步失败仍会尝试其余目录并合并错误——替换已提交，旧文件可能已不存在。部分完成后的恢复本身失败时，调用方收到合并错误，目标可能暂时缺失，同目录的旧内容恢复备份与新内容临时文件都保留以便人工恢复（此时"无临时残留"不适用）。其他临时文件清理失败与主错误一起返回。
- Windows：替换前同样 Sync 并关闭；目标存在 → 使用带同目录唯一恢复备份的 `ReplaceFileW`；首次创建 → 不覆盖的 `MoveFileEx`。`ERROR_UNABLE_TO_MOVE_REPLACEMENT` 保持目标/源的原名；`ERROR_UNABLE_TO_MOVE_REPLACEMENT_2` 尝试恢复已移到备份的旧目标，失败则保留备份/源。成功替换后备份清理失败属于已提交错误。新文件继承父目录 ACL，替换保留目标 ACL；不声称收紧 ACL，也不提供与 POSIX mode 或目录 fsync 等价的保证。

`update.check_enabled`（默认 `true`），只控制自动检查，不影响用户主动执行的 `pixiv update`。`logging.level` / `logging.format`；默认 `info`/`text`，`debug` 增加请求与决策记录，`json` 选择 JSON 记录；环境变量按次覆盖。

### `internal/browsercookies`

只读浏览器 Cookie provider；不属于公开 SDK；不依赖 FANBOX、Pixiv、CLI、MCP 或账号库。

| 部分 | 作用 |
| --- | --- |
| 根包（core） | 与协议无关；固定的 `CookieQuery`；按安全 profile 标识发现浏览器目录；返回脱敏的 `Secret`；`sqlite.go` 通过系统 `sqlite3` 只读查询 cookie 数据库；不导入任何 provider（只导入根包不会注册浏览器） |
| `chromium` | 显式的 Chrome/Edge provider；从平台 profile 根目录读取 Local State/Cookies；支持 v10/v11 GCM 与旧 CBC；`secret_*.go` 实现 macOS Keychain、Windows 当前用户 DPAPI、Linux 通过固定属性的 `secret-tool` 访问 Secret Service |
| `firefox` | 解析 `profiles.ini` |
| `safari` | `Cookies.binarycookies`，仅 macOS |

需要全部浏览器的调用方空导入 `chromium`、`firefox` 与 `safari` 完成注册，再调用根包 `New`。

未知 Chromium 衍生浏览器不做模糊识别；多个 profile 且未选择时明确失败。缺少工具、权限、锁、schema 漂移与解密错误映射为稳定错误；Cookie、密钥、绝对路径与命令输出从不进入日志、错误、JSON 或 MCP。各平台 native 证据须按 release-prep 手册在对应主机采集；离线 fixture 与交叉编译只证明代码/格式契约。

### `internal/selfupdate`

薄根包（协调器 + 自动检查 API + 不经 shell 的命令执行器），向 CLI 组合根重新导出 `internal/selfupdate/{source,release,installer}` 的类型。

| 安装来源 | 更新行为 |
| --- | --- |
| Homebrew | 通过可执行文件符号链接与 keg `INSTALL_RECEIPT.json` 区分 stable/beta；切换时先卸载冲突 formula，失败时显式尝试恢复原 formula |
| `go install` | 必须同时匹配 Go build info **与**实际 `GOBIN`/`GOPATH/bin` 可执行文件；安装所选的精确 tag |
| Release 二进制 | 选择平台压缩包 → 校验 Ed25519 `checksums.json` → 校验 SHA-256 → 精确 `pixiv --version` 预检 → 同目录原子替换 |

- GitHub Releases API 是唯一查询后端；排除草稿；stable 在校验前排除预发布；ETag/缓存用于节流并原子持久化。对通道内已发布 Release 强制 canonical SemVer——任一非法 tag 都 fail closed 并报告（绝不跳过而选旧版本）。
- 签名、checksum、HTTP、压缩包、替换与权限错误从不伪装成"没有更新"。Release 安装的失败语义是保护边界，不是临时降级。
- **信任根**（`internal/selfupdate/installer`）：key ID 与公钥常量位于 `release_installer.go`；`internal/cli/update/production.go` 为安装器构造 key ID → 公钥映射，调用方无法污染；`composition.go` 直接调用生产更新构造。离线安装器测试验证带有效签名但 archive checksum 错误的下载保留原二进制；私钥不在源码或配置中；只读检查不需要它；这层装配不能替代每个版本的发布验收。`internal/bootstrap` 已在 v1 删除。

### `scripts/`

| 位置 | 内容 |
| --- | --- |
| `scripts/build/`、`scripts/ci/`、`scripts/release/`、`scripts/pr/` | 构建/打包脚本与平台注册表、必需 CI 脚本、发布工具、受信 PR 工具；Go 工具均为一目录一个 `package main`，测试与之同目录 |
| `scripts/citest/` | 真实执行安装器、变更范围分类、验证预过滤、workflow action pin 与容器发布契约的测试 |
| `scripts/build/platforms.json` → `scripts/build/matrix.sh` | 供 workflow 使用的 runner/能力元数据；workflow 测试验证行为/安全边界，而不是第二份 YAML 策略模型 |

### 公开 SDK 包

模块路径保持 `github.com/FlanChanXwO/pixiv-cli`。SDK 破坏性变更在 v1 次版本中发布并列入 changelog，更新器、安装与 import 根路径保持原样。用法见 [SDK 参考](../sdk.md)。

| 包 | 导出 |
| --- | --- |
| `sdk` | `Page[T]`、带 text/JSON codec 的不透明 `Cursor`、保留 context/cause 链的 `Error` 与 `Reason` |
| `sdk/pixiv` | `New(CredentialStore, Options)`、`BeginLogin`、绑定实体引用/对象、列表、修改操作、URL 解析、图片与 Ugoira 归档 |
| `sdk/fanbox` | `New(sessionID, Options)`、会话验证、创作者/投稿/动态引用与对象、列表、条件媒体流与原子保存 |
| `sdk/saucenao`、`sdk/ascii2d` | 独立的 `Search(ctx, Options)` 图片字节查询、provider 结果与部分失败信息 |
| `sdk/download` | Pixiv 按页批量保存、模板、GIF/APNG 编码与部分结果报告 |

FANBOX 默认使用 Chrome 146 TLS 与 Firefox 148 UA。会话 Cookie 仅随经过校验的 API/首页身份请求和 `downloads.fanbox.cc` 请求发送，第三方 CDN 与 solver 控制请求省略凭据。solver 控制直连，浏览器代理显式设置，仅挑战进入 solver。

调用方拥有凭据、账号选择、来源加载、预算、筛选、cursor 持久化与输出。网络操作显式接收 context，SDK 事件使用当前默认 slog logger。SDK JSON 保留元数据和可用媒体地址，移除运行时绑定；输出适配器自行选择公开数据。本地 auth 备份归应用 auth，保持时间点 secret 快照契约。

### `internal/mcpserver`

见 [MCP server 架构](architecture-mcp.md)。每个产品的 tools 只有一个 `tools` 包：`register.go` 保存唯一的 `Register` 与原有的端口门控注册顺序，各领域文件（`accounts.go`、`media.go`、`bookmarks.go` 等）保存该领域工具的名称、annotations、schema 与 `<tool>Handler` 方法。共享端口仍放在 `internal/mcpserver/{pixiv,fanbox}/internal/toolhost`。

### `internal/pixiv/saving`

展开 CLI 与 stdio MCP 下载来源，把逐页保存结果汇总成既有的作品报告。范围字符串由调用方解析，构造页下标前校验边界。`sdk/download` 负责文件发布、命名模板与动图编码；输入的资源身份通过绑定账号的 SDK 对象重新解析。

作品 ID 去重后排序。每件作品在首个失败页停止，报告保留已发布文件与提示。目录 `{num}` 使用第 0 页，文件名 `{num}` 使用实际选中的页。未设置目录模板时，单页文件写入下载根目录，多页与动图按作品建立子目录。目录模板适用于所有作品，包含单页作品。静态扩展名优先采用受支持的响应 MIME，再嗅探图片字节；响应未声明 MIME 时可使用已知 URL 扩展名。Ugoira 使用原质量，不接受显式页选择，输出 GIF 或 APNG。

### `internal/media/ugoira`

负责 `Format`、有序帧输入、`Encoder`、ZIP 校验/解码、缩放、GIF 量化/抖动、APNG 容器组装与原子发布。下载器消费 `Encoder` 契约，不得重复编码或发布。JPEG/PNG 解码与 GIF/PNG 编码使用标准库；本包保留 RGB Wu 量化、Floyd–Steinberg 抖动、双线性缩放与 APNG 动画块。[开发流程](development.md#ugoira-go-encoder)说明缓冲区归属与帧预算。六目标执行证据来自平台 smoke worker，与运行时编码器分离。

## Release assets 与信任边界

```mermaid
flowchart LR
    B["六目标 CGO0 构建"] --> A["releaseassets：压缩包<br/>（.tar.gz / Windows .zip，<br/>只含 pixiv / pixiv.exe）"]
    A --> F["finalize：checksums.txt<br/>+ Ed25519 checksums.json（key ID）"]
    F --> D["草稿 Release，校验资产集"]
    D --> P["发布（受保护的 release env）"]
    P --> HB["4 个原生 staging tap 安装<br/>pixiv --version"]
    HB --> TAP["最终受保护任务：<br/>用 tap deploy key 推送 formula"]
```

- 项目 `LICENSE` 留在源码仓库。Windows Git Bash 缺少 `zip` 时使用预装的 `7z`；两条分支产生相同成员；缺少归档工具直接失败。
- `release.yml` 在受保护的 `release` Environment 中签名/发布，最小权限，Action 固定完整 SHA。发布任务把已校验的 `release/checksums.txt` 交给渲染器；stable/预发布分别生成唯一的 `pixiv-cli` / `pixiv-cli-beta` formula。四个原生 macOS/Linux runner 从隔离的本地 staging tap 安装（tap 限定名，验证 `pixiv --version`），从不使用或写入公开 tap；Homebrew 6 所需的信任只写入 runner 本地 staging tap 的临时信任库。
- 只有最终受保护任务能读取独立的 tap deploy key，并只推送对应 formula。tap 凭据与 release 密钥是不同的信任域；tap key 只进入最后的推送步骤，不能替代 Ed25519 信任根（`internal/selfupdate/installer/release_installer.go`）。formula 模板只使用已校验六目标 checksum 中的 macOS/Linux 资产；stable 与 beta 互相冲突，都安装 `pixiv`。
- 草稿 URL 无法安装，因此先公开 Release 再跑四架构门禁；失败时保留已公开的 Release 待处理，不写入 formula。v0.3.0 是首个走此路径的 tag；之后每个 tag 都必须通过同样的门禁。
- 没有 Apple notarization 或 Windows Authenticode：Gatekeeper/SmartScreen 提示须对照已验证的 Release、checksum 与签名记录处理，绝不静默绕过。

详细门禁见[发布](release.md)。

### `internal/storage/{atomicfile,filelock,secretfile}`

这些包负责原子写入/替换、文件锁与私有文件写入。路径/权限使用方依赖 `internal/config`，`secretfile.ConfigFiles` 实现其文件端口。配置解析由 `internal/config` 负责，该包的实现仅通过端口使用 storage。

### 测试与 helper owner

与协议无关的 helper 放在调用方：正整数解析与 stdin codec 在 `internal/environment`，字符串安全与终端渲染在 `internal/display`，URL 路径/file URI 在 `internal/pixiv/saving`，按月截断在 `internal/pixiv/searchfilter`。

| 迁出的职责 | Owner |
| --- | --- |
| 文件名清理、模板展开 | `internal/media/filename` |
| 下载 ID 去重、媒体 MIME 类型 | `internal/pixiv/saving` |
| refresh token 校验 | `sdk/pixiv/internal/api/oauth` |
| 路径、权限 | `internal/config` |
| 原子写入、替换 | `internal/storage/atomicfile` |
| 文件锁 | `internal/storage/filelock` |
| secret 导出写入 | `internal/storage/secretfile` |

## 已知约束

> [!WARNING]
> 不可协商。任何新增的超时、截断、数量上限、重试上限、静默 fallback 或隐藏降级都必须有证据、注释、测试或文档，否则视为违规。

- `sdk/pixiv/internal/api`、其 `oauth` 子包与资源传输使用调用方/SDK 注入的 HTTP client；默认 client 专属于当前 SDK client，没有固定的整请求超时；取消/截止时间经 context 传播；显式 client 保留调用方策略。
- `pixiv mcp` 默认 stdio；显式 `--port` 启用 HTTP 并要求 owner 状态已初始化；单独运行 `pixiv` 显示 help。用 `--port`、`--host`、`--base-url` 配置 HTTP，而不是已移除的地址设置（[启动](../cli-reference.md#mcp-http-server)）。
- 没有持久化账号导入/导出的 MCP 工具；会话范围的 MCP auth 工具与 wire 契约不变。
- 凭据以明文存于 `auth.json`；Unix 账号文件权限为 `0600`。拥有当前用户文件访问权限的人仍可读取；禁止自动备份。保留的旧数据库同样含有明文凭据。
- `config.toml` 首次创建时写出包含默认值的内嵌注释模板；后续编辑只修改目标键。
