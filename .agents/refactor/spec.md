# pixiv-cli 重构规格说明

本文件完整取代此前的重构计划草稿，作为本次重构的唯一依据。本文件和各 goal 语句（同目录的 `goals.md`）随仓库版本管理，执行时以 `origin/main` 上的版本为准；G34 删除整个 `.agents/refactor/` 目录。第 6 节的代码示例只用来固定职责划分、数据流向和失败处理方式；具体签名以实施时的现有代码为准。

## 1. 目标

1. 减少包的数量和类型层数，让从 CLI/MCP 经 SDK 到远端协议的调用链可以顺序读完。
2. 去除重复的样板代码、函数变体和只做转发的中间层。
3. 依赖总量净减少，并替换已停止维护或用途很窄的库；仓库始终保持 CGO-free。
4. 公开 SDK 每个实体只保留一种公开类型。module 路径保持 v1，SDK 的破坏性变更在 v1 的次版本中发布。
5. 增加结构化日志能力，日志不得混入 CLI 和 MCP 的 stdout。

第 4、5 条明确列出的变化之外，CLI 的命令树、flag 和输出格式，以及 MCP tool 的名称、schema 和错误语义，都保持不变。

## 2. 现状基线

数据来自只读统计。

| 区域 | 包数 | 生产代码行数 | 结构特征 |
|---|---|---|---|
| `internal/services` | 59 | 15.9k | `pixiv/endpoint/**` 有 28 个包，`fanbox/endpoint/**` 有 10 个包，调用方都只有 `sdk/*` |
| `internal/mcpserver` | 67 | 11.5k | 每个 tool 单独一个包，大多 20–100 行 |
| `sdk` | 3 | 8.5k | `models.go` 和 `dto.go` 是两套平行的公开类型 |

其他现状：

- **样板重复：** 36 个包各自声明了 `Transport` 接口；`mapUser`、`continuation` 这类函数一共定义了 86 处。
- **转换链路长：** 一次请求要依次经过 endpoint 的 `responseDTO`、endpoint 领域类型、`pixiv.Artwork`、`pixiv.ArtworkDTO`，最后才到 `record.Record`。
- **协议包：** `appapi`、`oauth`、`protocol`、`resource` 的生产调用方只有 `sdk/pixiv`；`e2e/sdk_pixiv_live_mutation_test.go` 直接使用 `appapi` 原始客户端，`services/pixiv/account/login_test.go` 使用 `protocol.OAuthRedirectURI`。
- **构建：** release、容器和 smoke 构建都用 `CGO_ENABLED=0`。
- **二进制：** 链接 51 个模块，体积 41 MB。
- **测试：** 全仓库测试 61k 行，生产代码 68.8k 行；`scripts/` 与 `tools/` 中测试约 8000 行，代码约 6600 行。

## 3. 设计决策

| 编号 | 决策 |
|---|---|
| D1 | module 路径保持 `github.com/FlanChanXwO/pixiv-cli`，不升 `/v2`。公开 SDK 允许破坏性变更，在 v1 的次版本中发布，只在 changelog 中列出，不提供弃用过渡、旧名别名或迁移文档（D13）；更新器、安装路径和 import 路径都不变 |
| D2 | 仓库永久保持 CGO-free，所有依赖都必须是纯 Go |
| D3 | 持久化使用 GORM，驱动用 `github.com/ncruces/go-sqlite3` 及其 `gormlite`；schema migration 继续沿用现有的 SQL 文件和 ledger |
| D4 | 日志使用标准库 `log/slog`。公开 SDK 唯一的日志入口是 `Options.Logger *slog.Logger`，构造时交给传输与协议层（`pixivapi`、`fanboxapi`）；SDK 业务 `Client` 不持有 logger 字段，也不写日志。内部业务对象从 ctx 取得 logger（细则见 6.9） |
| D5 | 包按职责划分，不按端点或 tool 划分；每个实体只有一种公开类型 |
| D6 | 可选参数用 spec 结构体或方法表达，不再新增 `WithX` 一类函数变体 |
| D7 | 上游协议实现（传输、协议约定、资源下载、OAuth）按产品放在模块级 `internal/pixivapi`、`internal/fanboxapi`。模块级 `internal/` 对模块外同样不可见；真实环境写入测试需要绕过 SDK、直接使用原始客户端做独立回读与清理，`pixivaccount` 的测试也引用协议常量，放在模块级可以让这些调用方都直接 import |
| D8 | 拆成 45 个 goal，按依赖分四个阶段，阶段内的线路并行；每个 goal 一个命名 worktree、一个 PR，验收通过后合并进 main（见 7.0） |
| D9 | 声明按使用范围放置。包内多个文件共用的实体和常量，集中放在声明文件里；只服务一个行为的声明，和使用它的函数放在同一个文件（细则见 3.1）。名字说明职责，不用 `Deps`、`Data`、`Manager` 一类空泛名，不建依赖袋（细则见 3.1.2） |
| D10 | 具有业务含义的标识符使用具名常量；有取值集合的标识符使用具名类型（细则见 3.2） |
| D11 | 每个横切关注点只有一个归属层和一种传入方式；业务对象可以持有基础设施，但只能以“一个已封装好的依赖”形式持有，不自己实现横切逻辑（细则见 3.3） |
| D12 | 测试只保留长期契约，结构只为生产需要而存在；判定标准与典型例子见 3.4，执行者按标准处理全部文件，不依赖穷举清单 |
| D13 | 不做内部兼容：旧入口、旧名别名、source-compatible 包装、为旧版本自身组件保留的分支一律删除，SDK 不加 `Deprecated` 过渡。只保留两类：上游格式本身的差异，以及让用户本机已有的数据库和配置文件仍能打开所需的读取逻辑（细则见 3.4.2 的 P-l） |
| D14 | 每个包只在一个文件里写标准库风格的包文档，其他文件开头不写注释；注释在 `sdk/` 用英文、其余用简体中文（细则见 3.5、3.5.1） |

### 3.1 声明放置规则（D9）

| 声明的使用范围 | 放置位置 | 示例 |
|---|---|---|
| 包内多个文件共用 | 集中放在声明文件中 | `sdk/pixiv/models.go`（公开实体）、`wire.go`（响应结构）、`storage/database/rows.go`（行结构）、`errors.go`（错误码） |
| 只服务一个行为 | 和使用它的函数放在同一个文件，写在函数前面 | `tools/user.go` 中的 `userDetailIn`；`pixivcmd/list.go` 中的 `ArtworkList` |
| 类型自身的方法（`String`、`Valid`、`MarshalJSON` 等） | 和类型放在同一个文件 | `sdk/resource.go` 中的 `Resource.MarshalJSON` |

禁止事项：
- 不按“一个类型一个文件”拆分。
- 不建立 `types.go`、`consts.go` 这类没有主题的汇总文件。

### 3.1.1 文件命名

| 情形 | 规则 | 示例 |
|---|---|---|
| 单文件包 | 使用 `包名/包名.go` | `cli/ranking/ranking.go` |
| 多文件包的入口 | `包名.go` 存放包的主入口：主类型、构造函数或 cobra 命令定义；其余文件按主题命名 | `cli/search/search.go` 定义 `pixiv search`，`filters.go` 存放筛选逻辑 |
| 只有包注释的文件 | 改名为 `doc.go`，或把包注释合并到入口文件 | `update/release/release.go`（2 行）→ `doc.go` |
| 入口文件已经包含多个主题 | 按主题拆分，`包名.go` 只保留入口 | `media/downloader/downloader.go`（1097 行）、`fanbox/protocol/protocol.go`（863 行） |
| 文件名 | 说明内容，不用 `support.go`、`deps.go`、`helpers.go`、`util.go`、`common.go`、`types.go` | `cli/auth/support.go`、`deps.go` 按内容改名或并入 |

`包名.go` 的含义是“从这里开始读这个包”，不能用作杂物文件。

### 3.1.2 类型与标识符命名

`internal/` 中的包、文件、类型、函数和字段都可以改名；SDK 公开名的修改集中在 A2（见 7.0）。名字要说明它在领域里**是什么、负责什么**，而不是它在代码结构中的位置或机制。

| 规则 | 不这样写 | 这样写 |
|---|---|---|
| 不用空泛的类型名：`Deps`、`Dependencies`、`Data`、`Info`、`Facade`、`Manager`、`Helper`、`Util`、`Common`、`Base`、`Impl`、`Wrapper` | `deps.Data`、`fanbox.Data`、`pixiv.Facade`、`downloader.Manager` | `pixivcmd.Runner`（运行 Pixiv 列表命令）、`command.Env`（命令的执行环境）、`session.Pixiv`；`Manager` 按实际职责改名 |
| 不建“依赖袋”：构造函数只接收自己用到的依赖。参数多到需要一个结构体，通常说明职责过多，先拆职责；确需结构体时，按它的角色命名 | 每个命令包各有一个 `Dependencies` 结构体，由 root 一次填满再传下去 | `download.New(env command.Env, s *session.Pixiv)` |
| 不用“函数字段 + 同名转发方法”，函数字段不加 `Fn` 后缀；需要替换的行为用具体类型或窄接口（3.3） | `PromptSecretFn func(string) (string, error)` 加 `func (d Data) PromptSecret` | `func (e Env) PromptSecret(message string) (string, error)` |
| 包名已经给出的上下文不再重复 | `session.PixivSession`、`filename.FilenameData`、`reversesearch.AggregatorDependencies` | `session.Pixiv`、`filename.Fields`、按职责拆成构造参数 |
| 同一种能力在各处使用同一个名字 | `UsageError` / `Usage`、`PrintJSON` / `WriteJSON` 并存 | 统一为 `command.Env` 上的一个名字 |

**新代码的补充规则（2026-10-26 起）：** 以下规则只约束新增的代码；现有名字不为此改名，SDK 公开名也不再为此破坏一次。

- Go 代码以 [Google Go Style Guide](https://google.github.io/styleguide/go/)（Guide、Style Decisions、Best Practices）为基线，本规格与它冲突时以本规格为准。代码要让只有 Go 基础的工程师也能读懂：清楚优先于简短，不炫技。
- 新类型和字段也不用 `Kind`、`Type`（作为万能区分字段时）、`Item`、`Object`、`Misc` 做名字或后缀，改用说明具体角色的词，例如 `Phase`、`Mode`、`Source`、`Status`、`Registry`、`Store`。上游协议本身的字段名（例如 Pixiv 的 `type`）在映射层保持原样。
- 不用缩写：`Cfg`、`Mgr`、`Svc`、`Msg`、`Req`、`Resp`、`Tmp`、`Val` 都写完整单词。只在几行内使用的局部变量（如 `ctx`、`err`、`ok`、`i`、`n`、`req`）和一两个字母的方法接收者按 Go 惯例使用短名；字段、导出符号和作用域较大的变量仍写完整单词。`ID`、`URL`、`HTTP` 等首字母缩写词保持全大写。
- 包名用简短的小写名词，不用 `util`、`common`、`base`、`types`。

**已确认的起点**（实施时按规则检查所在阶段触及的全部类型，不限于此表）：

| 现有名字 | 位置 | 处理 |
|---|---|---|
| `Dependencies` ×6、`Deps` ×2、`Data` ×2 | `cli/commands/pixiv/{recommended,user,search,detail,download,auth}`、`pixiv.go`（包 `deps`）、`fanbox/fanbox.go`、`services/pixiv` | 改为构造参数；留下的共用能力进入 `command.Env`、`pixivcmd.Runner`（A6） |
| `Facade` ×3 | `services/pixiv`、`services/fanbox`、`services/reversesearch` | 前两个改为 `session.Pixiv`、`session.Fanbox`（A7 只移动了路径，改名由 G05 完成）；第三个按职责改名（G18） |
| `Manager`、`DownloadManager`、`LoginManager` | `media/downloader`、`mcpserver/pixiv/accounts` | 按实际职责改名（G24、G25） |
| `AggregatorDependencies`、`FilenameData` | `services/reversesearch`、`media/downloader/filename` | 见上表 |

### 3.1.3 新代码放在哪里

重构之后的新功能按下面的顺序决定位置，第一条成立就停下：

1. 只属于一个命令 → 该命令的目录 `cli/<命令路径>/`。
2. 被多个命令共用：只有 Pixiv 命令用 → `cli/internal/pixivcmd`；只有 FANBOX 命令用 → `cli/internal/fanboxcmd`；所有命令都用 → `cli/internal/command`。
3. MCP tool → `mcpserver/<产品>/tools/` 中对应领域的文件。
4. 公开的 Pixiv/FANBOX 能力 → `sdk/<产品>/` 对应领域的文件；上游协议细节 → `pixivapi`、`fanboxapi`。
5. 以上都不是 → 新建一个按职责命名的包。

**检验：** 放进去之后，所在包、文件、类型的名字如果不再能概括它们的全部内容，就是放错了位置，应该新建或拆分，而不是扩大原有名字的含义。不建立 `common`、`shared`、`util`、`misc` 这类按“性质”命名、什么都能放的位置。

### 3.2 常量规则（D10）

具有业务含义的标识符一律使用具名常量，不在代码中直接写字符串字面量。有取值集合的标识符使用具名类型，让编译器检查类型。

| 类别 | 形式 | 放置位置 |
|---|---|---|
| 配置键（别名） | `settings.Key` 类型常量，例如 `settings.KeyLogLevel` | `internal/config/settings/keys.go` |
| 枚举取值（日志级别、输出格式、restrict、作品类型等） | 具名类型 + 常量，例如 `settings.LogLevelDebug`、`pixiv.RestrictPublic` | 与该类型放在同一文件 |
| 远端 API 路径、query/form 参数名、请求头名 | 导出常量，例如 `pixivapi.PathUserFollower`、`pixivapi.ParamUserID`（所在包位于 `internal/` 下，模块外部无法引用） | `internal/pixivapi/api.go`、`internal/fanboxapi/api.go` |
| MCP tool 名称 | 未导出常量，例如 `toolUserDetail` | 与该 tool 的注册代码放在同一文件 |
| Go 代码中引用的数据库列名 | 未导出常量，例如 `colCredentialRevision` | `storage/database/rows.go`，紧挨行结构 |
| JSON 输出键（struct tag 以外），例如列表的 `illusts`、`manga` | 未导出常量 | 与使用它的输出代码放在同一文件 |
| 环境变量名 | 仍由 `env` 标签声明；代码中需要单独引用时定义常量 | `keys.go` |

**数值与其他有含义的值（2026-10-26 起）：** 长度、上限、超时、间隔、退出码、文件名、地址和服务名等有含义的值同样写成命名常量；契约规定的值放在负责执行它的包里。同一个值只定义一次，别处引用常量或从它推导（例如字符串形式用 `strconv.Itoa` 生成），不再写一遍字面量。不需要命名的：0、1 这类不带业务含义的值，以及标准库约定的参数（如 `net.Dial("tcp", ...)`）。这条规则约束新增和触及的代码，现有字面量的统一清理不在注释 goal 内。

可以直接写字面量的场景：
- struct tag：Go 语法要求它必须是字面量。
- 日志消息、错误文本、格式化串。
- SQL migration 文件，以及 `Raw` 查询中的 SQL 正文。
- 测试中的输入数据。测试断言标识符时，仍然引用生产代码的常量。

**配置键的一致性：** 配置键的声明来源仍然是 `RuntimeConfig` 字段上的 `alias` 标签，`keys.go` 中的常量必须与之一一对应。由一个表驱动测试遍历全部 `Key` 常量，确认每个都能通过 `SettingSpecByAlias` 解析。这个测试保护的是实际契约：常量拼错时，程序运行时会找不到配置项。

**程序内读取配置：** 程序内部通过 `RuntimeConfig` 的字段读取配置（例如 `cfg.LogLevel`），不经过别名字符串。按别名读写的 `Store.Get/Set/Unset` 只用于 `config get/set/unset` 命令，别名来自用户输入。

### 3.3 横切关注点的归属（D11）

“业务对象不含任何基础设施字段”只在日志上能做到。HTTP、资源生命周期、锁、凭据持久化这类能力，业务对象在运行时必须用到，无法完全剥离。可以推广的原则是：

1. 每个横切关注点只有一个归属层，其他层不重复实现。
2. 业务对象持有基础设施时，只持有一个已经封装好的依赖（例如一个 transport、一个 repository），不持有构成它的零件（`http.Client`、`selfHTTP`、`sync.Mutex`、缓存 map）。
3. 请求范围内的信息（取消、调用关联、日志）通过 `context.Context` 传递，不放进字段。
4. 只为测试存在的注入点不出现在导出 API（导出字段、构造函数变体）中；确有需要时保留为未导出字段，由同包测试设置。

| 关注点 | 归属层 | 传入方式 | 业务对象中的形态 |
|---|---|---|---|
| 日志、诊断事件 | `pixivapi`、`fanboxapi`（请求与内部决策）、执行边界（最终结果） | SDK：构造时经 `Options.Logger` 传给传输与协议层；内部：`ctx` | 业务 `Client` 无字段 |
| 取消、超时、调用关联 | 执行边界 | `ctx` | 方法参数 |
| HTTP client、代理、重试、Retry-After、请求间隔 | `internal/pixivapi`、`internal/fanboxapi` | 构造时传入一次 | 一个 `app` 字段 |
| 签名 URL 的运行时缓存、资源域名白名单 | `pixivapi`（资源下载属于传输层） | 同上 | 无字段 |
| 资源释放（`Close`、`CloseIdleConnections`） | 创建者；`lifecycle` 包提供 Lease | 构造函数返回关闭函数或 Lease | 无额外字段 |
| 数据库连接、文件锁、事务 | `storage/database` 内部 | 方法内部 `withLock` | 业务只看到 repository 接口 |
| 凭据轮换的持久化 | `session` | `session.Use` 回调 | SDK 调用方不处理凭据 |
| 配置 | 组合根读取一次 | 以具名字段的值传入构造函数 | 值字段，不持有 `settings.Store` |
| stdout 输出（文本/JSON/NDJSON） | CLI 命令层 | 命令持有 writer | 业务返回数据，不写 writer |

**仍然允许持有基础设施的对象：** 本身就是基础设施的包不受“业务对象”这一限制，例如 `pixivapi`、`fanboxapi`、`storage/*`、`media/downloader`、`mcpserver/pixiv/mediaproxy`。它们的职责就是并发、缓存、文件和网络，规则只要求它们对外暴露的接口窄、内部零件不外泄。

**按此原则需要调整的现有结构：**

```go
// sdk/pixiv：重构前，传输零件散落在业务 Client 上
type Client struct {
	app          *pixivapi.Client
	opts         Options
	httpClient   *http.Client   // 传输零件
	selfHTTP     bool           // 传输零件：是否由 SDK 自己创建，决定能否关闭
	userID       int64
	userName     string
	expiresAt    time.Time
	cursorInstance string
	resourceMu   sync.RWMutex   // 传输零件：签名 URL 缓存的锁
	resourceURLs map[string]string
}

// 重构后：传输零件归 pixivapi 所有，Client 只剩一个传输依赖和身份。
// 原 opts 中构造后仍被读取的选项在构造时交给 pixivapi（核实：目前只有 ResourcePolicy.AllowedHosts 一项，属于资源下载）。
type Client struct {
	app *pixivapi.Client // 拥有 http.Client、是否自建、签名 URL 缓存、资源白名单

	// 当前账号身份；游标与资源引用都与它绑定。
	userID         int64
	userName       string
	expiresAt      time.Time
	cursorInstance string
}

func (c *Client) CloseIdleConnections() { c.app.CloseIdleConnections() }
```

```go
// internal/session（原 services/pixiv Facade）：重构前
type Facade struct {
	accounts       AccountService
	poolFactory    PoolFactory
	gate           RotationGate
	loadPoolConfig ConfigLoader
	// 只为测试观察生命周期而存在；生产代码固定调用 SDK 的 CloseIdleConnections。
	closeClient func(*sdkpixiv.Client) error
}

// 重构后：删除只为测试存在的导出注入点
type Pixiv struct {
	accounts       AccountService // 窄端口：选择账号、提交轮换
	poolFactory    PoolFactory
	gate           RotationGate
	loadPoolConfig ConfigLoader // 保留：每次调用读取最新配置，长时间运行的 MCP 进程依赖这一点
}
```

核实结果：
- `closeClient` 在 Pixiv 侧通过导出的 `Dependencies.CloseClient` 注入，测试位于外部测试包 `pixiv_test`；FANBOX 侧通过只给测试用的导出构造函数 `NewFacadeWithCloseClient` 注入（同时也是一个 `WithX` 变体）。两者都从导出 API 中删除；仍需观察关闭行为的测试改为同包测试，设置未导出字段。
- `loadPoolConfig` 由 `composition.go` 注入，每次调用都会经 `runtimeConfig()` 重新读取配置文件；MCP 进程依赖这一点感知配置修改，因此保留加载函数。

### 3.4 测试与设计的判定标准（D12）

本节给出判定标准和典型例子，不列全部清单。执行者对每个 PR 范围内的**全部**测试文件和导出符号逐一套用标准；第 7 节各阶段列出的名单只是已核实的起点。

#### 3.4.1 测试

**删除**（满足任一条）：

| 编号 | 标准 | 例外 |
|---|---|---|
| T-a | 断言文件文本（YAML、Dockerfile、Markdown、Go 源码），而不是执行代码观察行为 | 文本本身就是安全契约、且无法通过执行检查时保留，例如 action 必须固定完整 SHA |
| T-b | 与另一个测试断言同一契约 | 保留经由最公开边界（CLI/MCP 输出、SDK 方法、`Snapshot.Effective` 这类对外入口）的那一个 |
| T-c | 测试的是测试辅助代码：真实环境测试的环境变量解析、fixture 构造、测试目标查找 | — |
| T-d | 防止已删除的东西再出现（历史防回退） | — |
| T-e | 已完成迁移的对比或特征测试；文件名带 `red`、`baseline`、`parity` 一类过程标记的通常属于此类 | — |
| T-f | 测试 getter、纯转发、字段投影，或断言常量等于它自己 | — |
| T-g | 共用同一个实现的同族端点（例如走同一个 `userPreviews` 的 user 列表端点）逐个重复同一组响应校验 | 每族保留一个正常用例和一张失败模式表（缺字段、类型错误、空列表、`next_url` 异常等）；只有某个端点有自己的分支时，才为它单独保留用例。取代 A3、A4 中“原有断言全部保留”的要求（自 G35 起） |
| T-h | 同一契约在多层重复断言 | 按下方的分层表，只在负责该契约的那一层保留 |

**每类契约只在一层断言（T-h）：**

| 契约 | 断言所在的层 | 其他层不再断言 |
|---|---|---|
| 上游响应到模型的映射、响应校验 | SDK（`sdk/pixiv`、`sdk/fanbox`） | CLI、MCP 不再逐字段检查映射结果 |
| 文本、JSON、NDJSON 输出外形，flag 解析，退出码，stdin 输入 | CLI | — |
| tool schema、结构化输出外形、`isError`、注册与鉴权、stdio 纯净 | MCP | — |
| 凭据与签名 URL 不泄露、持久化与 revision、取消与错误传播 | 该机制所在的边界 | 不在调用方重复 |

**预期规模：** 2026-10-24 统计，生产代码 68.4k 行、测试 69.7k 行。G35–G38 完成后，测试预期约为生产代码的一半（3.5–4 万行）。这是预期结果，不是门槛：明显高于此比例时，在 PR 中说明原因。

#### 3.4.4 测试重写（G44，2026-10-25 起取代 3.4.1 的逐条删减）

G35–G38 后测试仍有 52.7k 行（生产代码 65.2k），并被合并进少数大文件（最大的 4716 行）。维护者决定：删除全部针对代码和行为的测试，只按下面的清单重写，测试点到为止。CI 的检查不在删除范围内：workflow 及其步骤（Quality gate 的 test、vet、race、build、licensebundle 检查，platform-smoke、container-smoke、release 中的各项检查）保持不变；workflow 按名称或包路径单独运行的测试原样保留，不删除、不重写，也不计入下面的上限。

**上限：**

| 项目 | 上限 |
|---|---|
| 单个测试文件 | 200 行（含导入与辅助函数） |
| 单个测试函数 | 40 行；表驱动时表格不超过 8 行用例 |
| 全仓库测试文件 | 25 个 |
| testdata | 只用于数据库旧版本样本与少量 golden 文件 |

不新建测试辅助包、fixture 构造器或 mock 层级；不为测试导出生产符号。

**清单（只测这些，每项一个代表性用例加必要的失败用例）：**

| 契约 | 位置 |
|---|---|
| 凭据不泄露：账号与 Cookie 类型在 `%v`、`%+v`、`%#v`、slog 中不输出凭据；日志不含 query、token、Cookie、签名 URL；MCP tool 输出不含签名 URL 与 Cookie | `logging`、账号包、`mcpserver` |
| 账号存储：revision 冲突、事务回滚、v1.2.x 数据库可打开、文件权限；原子替换与文件锁 | `storage/*` |
| 凭据轮换后新 token 写入存储 | `pixivaccount` 或 `session` |
| 上游响应异常时返回错误而不是空结果；取消向上传播 | `sdk/pixiv`、`sdk/fanbox` |
| 对外契约快照：MCP `tools/list` 一份 golden；CLI 一个代表性命令的 text、JSON、NDJSON 输出；用法错误的退出码；MCP stdio stdout 只有 JSON-RPC；HTTP 必须带 bearer | `mcpserver`、`cli` |
| 配置来源优先级（环境变量、文件、默认值）与显式空值 | `config/settings` |
| 自更新：校验失败时不覆盖已有二进制 | `selfupdate` |

**原样保留、不计入上限的 CI 测试**（以合并时 workflow 中的引用为准重新核对）：`scripts/citest` 整个包（platform-smoke、container-smoke、release 运行）；`scripts/release` 的测试（release.yml 运行）；platform-smoke 点名的 `TestPixivBinaryPackagedSmoke`（`cmd/pixiv`）、`TestOpenOnWindowsAcceptsForwardSlashFileURI`（`storage/database`）、`TestAuthURLCallback`、`TestAuthURLHandlerInstall`、`TestNormalCLIInvocationEnsuresPersistentHandlerWithoutBlockingCommand`（`internal/cli`）；`internal/cli/auth/loginhelper` 与 `internal/media/ugoira/...` 的测试（platform-smoke 按包路径运行，作为各平台的原生证据）。

真实环境测试（`live_test.go`、`live_mutation_test.go`）删除，不重写；真实环境验证改为手动。

**保留**（满足任一条，且没有被另一个测试在更公开的边界覆盖）：凭据与密钥不泄露；数据完整性与持久化（事务、revision、文件原子替换）；CLI、MCP、SDK 对外输出契约；取消与错误传播（不把失败报告成成功）；发布产物的正确性（校验和、归档命名）。能真实执行的检查优先于字符串检查。

**处理方式：** 删除；去重合并为一个；或移到对应生产文件的 `x_test.go`（测试布局规则）。移动时不搬运重复断言。

**典型例子：**

```go
// 删除，T-b：internal/config/settings/tags_red_test.go
// 通过反射读取字段标签来断言 OutputJSON 声明了 default:"false"、RequestInterval 声明了 default:"0s"……
func TestDefaultTagDistinguishesEmptyFalseZeroAndAbsent(t *testing.T) { /* requireTagged(t, tags, "OutputJSON") … */ }

// 保留：contract_test.go 经由对外入口断言同一契约，测的是用户可观察的结果（B1 后入口改为 Store 方法，断言不变）
func TestDefaultDeclarationDistinguishesFalseZeroAndEmpty(t *testing.T) {
	state, err := config.LoadSnapshotAt(filepath.Join(t.TempDir(), "config.toml"))
	value, err := state.Effective(test.alias)
	assert.Equal(t, test.want, value.Value) // false、0s 是默认值；未声明默认值的保持 unset
}
```

```go
// 删除，T-a：scripts/tests/containerrelease。container-smoke 会真实构建并运行镜像
func TestDockerfileSetsWorkdirToWork(t *testing.T) {
	if !strings.Contains(readDockerfile(t), "WORKDIR /work") { t.Fatal("Dockerfile must set WORKDIR /work") }
}

// 删除，T-d：scripts/tests/releasepublishers
func TestReleaseNoLongerPublishesHomebrewInline(t *testing.T) {
	for _, forbidden := range []string{"render_homebrew_formula", "deploy_homebrew_tap" /* … */} { /* strings.Contains(release.yml) */ }
}
```

| 典型例子 | 判定 |
|---|---|
| `TestReverseSearchE2EConfigIsDisabledByDefault`（测试真实环境测试自己的环境变量解析） | 删除，T-c |
| `e2e/ugoira_*_test.go`（与已移除的 Rust 编码器逐帧对比） | 删除，T-e |
| `TestToolOutputValuesDoNotLeakResourceTransportCanary`（经真实 handler 断言签名 URL、Cookie 不进入 MCP 输出） | 保留：凭据不泄露 |
| `TestInstallShChecksumFailurePreservesExistingBinary`（真实执行 `install.sh`，校验失败时不覆盖旧二进制） | 保留：真实执行，防止数据丢失 |
| `TestSelectPixivMutationAccountRequiresExplicitNonDefault`（写入测试不得使用默认账号） | 保留：保护真实账号 |

#### 3.4.2 生产代码中的过度设计与迂回设计

判断一个结构是否必要，问三个问题：去掉它，哪个调用方需要多写代码？它防止了什么具体错误？读者要多跳几处才能看懂一次调用？前两问答不出、第三问的答案大于零时，按下表处理。

| 编号 | 标准 | 处理 |
|---|---|---|
| P-a | 导出符号只被测试引用（`sdk/` 的公开 API 除外） | 改为未导出，或删除后让测试走生产代码实际使用的入口 |
| P-b | 包级可变变量，由导出的 `SetXxx` 在测试中替换 | 改为结构体上的未导出字段，由同包测试构造 |
| P-c | 只转发到默认实例的包级函数，以及 `At`、`WithX`、`WithFileStore` 一类入口变体（D6） | 删除，调用方直接使用唯一入口 |
| P-d | 同一接口或映射函数在多个包中各写一份（例如 36 个 `Transport`）；同一套方法或机制在多个类型上各写一份（例如三个带凭据的账号结构体各自实现 `String`、`GoString`、`Format` 来脱敏） | 合并为一份（D5）；机制收归到它保护或描述的那个字段类型、值类型上（例如凭据字段使用一个自我脱敏的类型），外层类型不再各写一遍 |
| P-e | 平行的类型层，例如 DTO 与模型 | 合并为一种类型（6.2） |
| P-f | 自制的标准库或已有依赖的替代品（自制诊断系统对应 `slog`，`FirstNonEmpty` 对应 `cmp.Or`） | 改用标准库或已有依赖 |
| P-g | 没有任何生产调用方的代码或工具 | 删除 |
| P-h | 注释引用仓库中不存在的文档编号（`§28`、`R14`） | 改为直接写出原因，或删除 |
| P-i | 中转层：只在结构相同的类型之间转换，或原样转发调用，自身不增加校验、不改变语义 | 删除中转层，两侧共用一个类型或直接调用 |
| P-j | 碎片化：一个概念被拆进多个必须一起读才能看懂的小包或小文件，例如十几行的 `main` 加一个 `internal` 实现包 | 合并为一个包或文件 |
| P-k | 为假想扩展预留的间接层：同一层内只是照抄某个具体类型方法集的接口、每个包各声明一份的相同接口、只注册一项的注册表、没有第二种取值的选项；测试替身不算第二个实现，测试改用 httptest、临时目录这类真实边界 | 去掉间接层，直接使用具体类型。例外：由使用方定义、只含其所需方法、用来切断对存储/网络/系统依赖的窄端口（例如 `session` 的 `AccountService`、MCP 的 `toolhost.SDKPorts`）是 AGENTS.md 规定的边界，保留 |
| P-l | 兼容层：旧入口与新入口并存、旧名别名、source-compatible 包装、为旧版本自身组件（旧页面、旧流程、旧端口）保留的分支、已删除配置键的墓碑、以往改名遗留的输入别名 | 删除，只保留新入口。例外：上游（Pixiv/FANBOX API、浏览器 Cookie）格式本身的差异不是兼容层；让用户已有的数据库、配置文件仍能打开的读取逻辑保留 |
| P-m | 过度防御：对内部调用方已经保证的前提重复校验（同一参数在公开方法、私有实现、映射函数中各校验一次）；对构造时一定会设置的字段或依赖判 nil；Go 本身已经安全的判断（读 nil map、对 nil 切片取 `len` 或遍历）；遇到不可能出现的状态时悄悄降级、返回零值或吞掉错误；没有共享修改风险的防御性拷贝；“以防万一”的 `recover` | 删除；校验只留在最外层的那一次。确需防住的内部不变量，让它立即失败（返回明确错误或 panic），不伪装成成功。例外：信任边界上的校验（用户输入、CLI/MCP 参数、上游响应、文件与数据库内容、环境变量）、公开 SDK 的参数校验、凭据的防御性拷贝（`RefreshTokenCopy` 这类所有权边界）、并发与取消相关的检查，保留 |

边界：以上标准只调整结构，不删除用户可见的功能。功能取舍（例如 `pixiv auth` 登录链路的范围）不在本次范围内（第 8 节）。

**典型例子（P-i）：** `internal/cli/commands/pixiv/pixiv.go`（包名 `deps`）与 `internal/listing` 各定义了一个字段完全相同的 `Request`，命令代码中有 20 处 `listing.Request(request)` 转换。合并为 `pixivcmd.Request` 一个类型，转换全部删除（5.3）。同类还有第 2 节的四层转换链：endpoint `responseDTO` → endpoint 领域类型 → `pixiv.Artwork` → `pixiv.ArtworkDTO` → `record.Record`，重构后只剩 wire 结构 → 模型 → record 两步。

**典型例子（P-j + T-f）：** `internal/cli/invocation` 整个包只有 19 行，保存三个标准流；另有一个测试断言构造函数原样保存了传入的三个字段。包并入 `cli/streams.go`，测试删除。

```go
// internal/cli/invocation/invocation.go（整个包）
type Streams struct{ In io.Reader; Out, Err io.Writer }
func NewStreams(in io.Reader, out, errOut io.Writer) Streams { return Streams{In: in, Out: out, Err: errOut} }

// invocation_test.go：T-f，断言字段等于它自己
if streams.In != in || streams.Out != out || streams.Err != errOut { t.Fatalf(…) }
```

**典型例子（P-c + P-d）：** CLI 中同一组“命令支撑”能力写了多份，名字还各不相同：

| 能力 | root `app`（`cli/root.go`） | Pixiv `deps.Data` | FANBOX `fanbox.Data` |
|---|---|---|---|
| 用法错误 | `UsageError` | `Usage` | `UsageError` |
| 位置参数校验 | `RequireExactArgs` 等 | `ExactArgs`、`MinArgs`、`MaxArgs` | `RequireExactArgs` 等 |
| JSON 输出 | `PrintJSON` | `WriteJSON` | `PrintJSON` |
| 终端提示 | `PromptSecret` 等 | — | `PromptSecret` 等 |
| 输入绑定 | — | `BindTextValue`、`BindNoInput` 等 | `BindTextValue`、`BindNoInput` |

FANBOX 的版本还是转发：root 把自己的方法作为函数字段注入，`Data` 的同名方法检查字段为 nil 就 panic，否则原样调用。

```go
// 重构前：internal/cli/commands/fanbox/fanbox.go
type Data struct {
	Reader          io.Reader
	Writer          io.Writer
	WrapUsage       func(error) error
	CanPromptFn     func() bool
	PromptSecretFn  func(string) (string, error)
	PromptConfirmFn func(string, bool) (bool, error)
	// ……服务工厂、浏览器、配置
}

func (d Data) PromptSecret(message string) (string, error) {
	if d.PromptSecretFn == nil {
		panic("fanbox secret prompt is not configured")
	}
	return d.PromptSecretFn(message)
}
```

```go
// 重构后：internal/cli/internal/command/env.go
// Env 是所有命令共用的执行环境，由 root 构造一次，作为参数传给各命令的构造函数。
type Env struct {
	In       io.Reader
	Out, Err io.Writer
}

func (e Env) UsageError(err error) error
func (e Env) ExactArgs(count int, usage string) cobra.PositionalArgs // MinArgs、MaxArgs 同理
func (e Env) PrintJSON(value any) error
func (e Env) CanPrompt() bool
func (e Env) PromptSecret(message string) (string, error)            // PromptConfirm 同理
func (e Env) BindTextValue(cmd *cobra.Command, minArgs, maxArgs, fillPosition int)
func (e Env) BindNoInput(cmd *cobra.Command)

// 子命令的构造函数只接收自己用到的依赖，不再经过一个依赖袋
// internal/cli/fanbox/download/download.go
func New(env command.Env, s *session.Fanbox) *cobra.Command

// internal/cli/fanbox/auth/auth.go
func New(env command.Env, accounts *fanboxaccount.Service, browser browsercookies.Provider) *cobra.Command

// internal/cli/internal/fanboxcmd：只剩 FANBOX 子命令共用的函数，没有聚合类型
func ProxyOverride(cmd *cobra.Command) (*string, error)
func UseClient(cmd *cobra.Command, s *session.Fanbox, use func(*fanbox.Client) error) error
```

**典型例子（P-k + P-d）：** 每个 endpoint 包都声明了自己的 `Transport` 接口（共 36 个），生产实现只有 `appapi.Client` 一个；测试替身也可以换成 httptest 服务器。端点并入 SDK 后直接持有 `*pixivapi.Client`（6.1）。

**典型例子（P-l）：** SDK 中同一操作有新旧两个公开入口，旧入口注释写明为 source-compatible 而保留。删除旧入口及其请求类型，私有实现中为区分两个入口而传入的 `operation` 参数随之删除。

```go
// sdk/pixiv/ops_mutation.go：重构前
func (c *Client) AddArtworkBookmark(ctx context.Context, request AddArtworkBookmarkRequest) error {
	return c.addArtworkBookmark(ctx, request, "AddArtworkBookmark")
}

// AddBookmark bookmarks one artwork. It is retained as a source-compatible
// wrapper around AddArtworkBookmark's implementation.
func (c *Client) AddBookmark(ctx context.Context, request AddBookmarkRequest) error {
	return c.addArtworkBookmark(ctx, AddArtworkBookmarkRequest{ArtworkID: request.ArtworkID, Restrict: request.Restrict, Tags: request.Tags}, "AddBookmark")
}
// RemoveBookmark 同理

// 重构后：只有 AddArtworkBookmark、RemoveArtworkBookmark；AddBookmarkRequest、RemoveBookmarkRequest 删除
```

P-l 的判定参照：

| 现有代码 | 判定 |
|---|---|
| `sdk/pixiv/ops_mutation.go` 的 `AddBookmark`、`RemoveBookmark` | 删除 |
| `media/downloader/filename` 的 `Generate`（注释称“兼容入口”，生产代码已改用 `GenerateChecked`） | 删除 |
| `mcpserver/pixiv/internal/runtime`（改名后为 `toolhost`）的 `Execute` 中“兼容未注入 Read 的原重放端口” | 删除，只保留当前端口 |
| `cli/commands/pixiv/auth/login.go` 中“保留本地旧页面的字段兼容”，以及“`expectedChallenge` 为空时视为不约束（兼容旧流程）” | 删除。登录页面由同一个二进制提供，不存在旧页面；后者还放宽了校验 |
| `config/settings` 中已删除配置键的墓碑（`removed_setting`） | 删除。需确认文件中残留的旧键在加载时被忽略、不导致启动失败 |
| `searchfilter` 中 `illustration` 作为 `illust` 的别名 | 删除，写入 changelog |
| `storage/database/migrate.go` 的 `legacyInitialMigrationChecksum` | 保留：旧版本创建的数据库仍需能打开 |
| endpoint `timeline` 中读取上游 `ai_type` 字段的 `LegacyAIType`；`browsercookies/chromium` 中旧版加密格式 | 保留：上游格式差异，不是本项目的兼容层 |

**典型例子（P-m）：**

```go
// internal/cli/internal/input/pipeline.go：调用方总是传入 cobra 命令，读 nil map 本身安全，两个判断都多余
func CodecOf(command *cobra.Command) Codec {
	if command == nil || command.Annotations == nil {
		return NoInput
	}
	value := command.Annotations[AnnotationKey]
	// ……
}

// internal/cli/internal/command/env.go：Env 由 root 构造，WrapUsage 一定被设置；
// 判 nil 只会在装配写错时成立，而那时它悄悄丢掉用法错误的退出码，把装配 bug 伪装成正常
func (e Env) UsageError(err error) error {
	if err == nil || e.WrapUsage == nil {
		return err
	}
	return e.WrapUsage(err)
}

// 重构后：前提由构造保证，函数只做自己的事
func CodecOf(command *cobra.Command) Codec {
	codec, _, ok := strings.Cut(command.Annotations[AnnotationKey], ":")
	// ……
}
```

| 现有代码 | 判定 |
|---|---|
| 上面两个函数中的判断 | 删除，P-m |
| `pixivaccount.RefreshTokenCopy`、`fanboxaccount.SessionIDCopy` | 保留：凭据的所有权边界 |
| `media/ugoira/jpeg` 中照搬标准库 `image/jpeg` 的 `panic("unreachable")` | 保留：内部不变量立即失败 |
| SDK 公开方法入口的参数校验（例如 `UserID <= 0`） | 保留：公开契约；同一参数在私有实现中的再次校验删除 |

**典型例子（P-a + P-b）：**

```go
// 重构前：internal/cli/commands/pixiv/auth/loginhelper/install_linux.go
var linuxExecutablePath = os.Executable

// SetLinuxExecutablePath 覆盖本机 executable 解析；返回恢复函数。
func SetLinuxExecutablePath(resolve func() (string, error)) func() {
	original := linuxExecutablePath
	linuxExecutablePath = resolve
	return func() { linuxExecutablePath = original }
}

// 测试（外部测试包）
t.Cleanup(loginhelper.SetLinuxExecutablePath(func() (string, error) { return "/tmp/pixiv cli", nil }))
```

```go
// 重构后：依赖是安装器自身的字段，测试不再修改全局状态
type linuxInstaller struct {
	executablePath func() (string, error)
	runXDGMime     func(context.Context, ...string) error
}

func newLinuxInstaller() linuxInstaller {
	return linuxInstaller{executablePath: os.Executable, runXDGMime: runXDGMime}
}

// 同包测试
installer := linuxInstaller{executablePath: fakeExecutable, runXDGMime: recordCalls}
err := installer.install(ctx)
```

改为同包测试的目录，按测试布局规则在中英文 `development.md` 中登记例外和原因。

#### 3.4.3 执行要求

- **确认零调用方：** 用 gopls 的 references，或 `grep -rw` 加编译检查，确认删除的符号没有非测试调用方。
- **PR 中的说明表：** 列出每个删除或合并的测试和符号，格式如下：

  | 测试或符号 | 判定标准编号 | 处理 | 仍覆盖该契约的测试 |
  |---|---|---|---|

- **改动后检查：** 删除测试后运行该包的测试和 `go vet ./该包`，不跑全量（第 9 节）。保留下来的测试不得修改断言来迁就重构。

### 3.5 包文档与文件开头（D14）

2026-10-26 起取代此前“每个文件写 1–3 行头注释”的要求。

**Go 包文档：** 每个包只在一个文件里写包文档，通常是 `doc.go` 或包的入口文件 `name/name.go`，紧贴 `package` 那一行。写法参照 Go 标准库：

- 第一句以 `Package x` 开头（`main` 包例外，见下一条），说清这个包做什么；再空一行（`//`）分段写使用范围、不负责什么，以及相关内容在哪里。不写维护者、创建日期这类元信息。
- 有多个文件或多个主要类型的包，在包文档里加一段“包的组成”，用文档链接（如 `[Client]`、`[Session]`）列出主要类型和入口；读者可以在 godoc 中直接跳转，改名时 gopls 也会提示链接失效。
- `main` 包的包文档不以 `Package main` 开头，而以命令名开头，例如“pixiv 命令启动 pixiv-cli。”
- 同一个包的其他文件不再写包文档；已有的重复包文档合并进一处。

**其他 Go 文件开头不写注释：** 紧贴 `package` 的注释会被 Go 并进包文档，与 `package` 隔一行的文件说明又容易随重构过期。文件名要让人一眼看出内容；一个文件需要靠注释才说得清，通常说明它该改名或调整拆分。这类问题记录下来交给代码 goal，注释 goal 不改文件名。

**指令与特殊文件：**
- 有 `//go:build` 时，build tag 放在第一行，空一行后再写包文档或 `package`。cgo 注释、`//go:embed`、`//go:generate` 等指令保留原来的位置。
- 生成的文件保留生成器写的头部，不手工补写。
- `_test.go`、JSON 等不支持注释的格式、`testdata/`、`third_party/` 不加文件说明。
- Shell、PowerShell、`.cmd` 脚本在 shebang 之后、workflow YAML 在 `name:` 之前、`frontend` 的 HTML/JS/CSS 在文件顶部，各用一行注释说明用途。

**例子：**

```go
// Package pixiv 提供 Pixiv App API 的 Go 客户端：作品、小说、用户、收藏与评论。
//
// 包的组成：[Client] 承载全部端点，由 [New] 构造。上游协议细节在 internal/pixivapi，不对外公开。
package pixiv
```

（`sdk/` 的注释用英文，上例只演示结构；语言见 3.5.1。）

```go
//go:build windows

package loginhelper
```

（同一个包里不写包文档的文件：build tag 之后空一行直接是 `package`。）

```sh
#!/bin/sh
# 为单个目标平台构建 pixiv 二进制，注入版本号，并对 Linux 产物运行 linuxabi 检查。
```

**现状（ff5f22cf）：** 368 个生产 Go 文件、91 个包目录；139 个文件带与 `package` 隔开的文件头注释，需要删除或把有用的内容并入包文档；88 个文件带包文档，其中 6 个包有不止一处包文档。

### 3.5.1 注释写什么

注释遵循 `pixiv-cli-code-commenting` 技能，只写名字和结构表达不出来的东西：调用约定、原因、不变量、副作用、所有权。没有这类内容就不写；写了就用平常说话的语气。

**语言：**
- `sdk/` 的注释（包括文档注释）用英文，它们会显示在 pkg.go.dev 上。
- 其余代码（`cmd/`、`internal/`、`scripts/`、`.github/`、`frontend/`）的注释用简体中文；已有的英文注释在注释整理时改写成中文。协议字段、标识符、错误原文保留英文，不在句子中夹杂可翻译的英文术语。
- 程序输出（日志、错误字符串、CLI 输出、MCP 结果文本）一律用英文，与注释语言无关。

**文档注释：** 导出的类型、函数、方法、常量和变量都写文档注释，`internal/` 也不例外。注释以标识符开头，后面紧跟它做什么、保证什么，必要时写清副作用、错误、取消、并发和零值的含义。私有函数和方法在名字与签名已经说清楚时不写；有前置条件、调用顺序要求或不明显的取舍时再写。

不要写成“X 是……”的定义句，也不要用“该函数”“此方法”“用于”开头：

| 不这样写 | 原因 | 这样写 |
|---|---|---|
| `// New 创建一个带 opaque FANBOX session 的账号值，并复制输入凭据。` | 前半句复述函数名与签名 | `// New 会复制 sessionID，调用方之后修改原切片不影响账号。` |
| `// Account 是一个已保存的 FANBOX 账号记录。session ID 是 opaque credential。` | “X 是 Y”的定义句，夹杂英文术语 | `// Account 对应本机保存的一个 FANBOX 账号；session ID 只能通过 SessionIDCopy 取得副本。` |
| `// FANBOX session 不与 Pixiv refresh token 共享类型或生命周期。` | 翻译腔 | `// FANBOX 的 session 和 Pixiv 的 refresh token 分开存放、分开轮换。` |
| `// String 返回字符串表示。` | 没有信息 | 删除；如果它承担脱敏，写清楚：`// String 只输出账号摘要，日志和 %v 不会打印出凭据。` |

**类型与字段：** 类型注释写它代表什么，以及使用者必须遵守的不变量。字段只在名字和类型看不出含义时才写，例如单位、零值或缺省的意义、可选性、由哪把锁保护、所有权和生命周期；短说明放在字段同一行的末尾。

```go
type Budget struct {
	mu   sync.Mutex
	used int64 // 已占用字节，由 mu 保护
	max  int64
}
```

**step 注释：** 函数里有几个明确的阶段时，在每段逻辑前用 `// 1. ...`、`// 2. ...` 写出这一段的目的或顺序要求。编号的是阶段，不是语句；简单函数、getter 和转发函数不编号。注释紧贴它描述的代码段，不在函数开头写目录；调整逻辑时同步改编号。

**让 Go 基础工程师读得懂：**
- 用到 `select`、channel 关闭、`sync/atomic`、锁的先后顺序、goroutine 交接等写法时，在旁边用一行注释说明为什么这样写。
- 状态机、账号池、OAuth 流程、MCP 会话这类复杂模块，在包文档或类型注释里用一段多行注释讲清整体模型：有哪些状态、谁拥有什么、按什么顺序发生。之后的局部注释不再重复。
- 不逐行复述代码，不给每个赋值、`if err != nil` 或 return 加注释，不写装饰性分隔线。
- 不承诺代码没有提供的保证，例如线程安全、原子性、幂等或可重试。

**写现状，不写进度：** 注释和错误消息只写代码当前的行为、限制和原因，不写 goal、task、工作项或设计文档的编号，也不写“某阶段实现”“后续补齐”这类进度。尚未支持的能力直接写“尚不支持 X”。临时实现用 `shortcut: <限制>，<替换条件>`，替换条件写成条件，例如“上游提供稳定 ID 后替换”。

**适用方式：** 新增文件完整遵守 3.5、3.5.1；修改已有文件时，补齐本次触及部分的注释。全库补齐由 G29–G33 按目录完成，不夹带在功能改动里。只改注释时检查内容、格式和引用即可；涉及 `//go:build`、`//go:embed`、`//go:generate` 等会被工具读取的注释时，验证实际的使用方。

## 4. 依赖规格

| 处理 | 模块 | 替代方案与依据 | 链接模块变化 |
|---|---|---|---|
| 移除 | `knadh/koanf/v2` 及其 parser、maps | 现在只用作点分路径 map；改为直接用 `pelletier/go-toml/v2`，配合已有的 `tomledit` | −6 |
| 移除 | `modernc.org/sqlite` 及其闭包 | 由 D3 的驱动替代 | 约 −9 |
| 移除 | `AlecAivazis/survey/v2` | 最后一次发布在 2022-12，只在 `prompt.go` 里用到 4 种提示；改用 `golang.org/x/term` 和 `bufio` | −4 |
| 移除 | `go-resty/resty/v2` | 只当作 HTTP 客户端用，重试逻辑项目自己实现了；改用 `net/http` | −1 |
| 整理 | `go.mod` | 运行 `go mod tidy`：修正直接依赖上的 `// indirect` 标记，并删除未使用的条目 | 0 |
| 新增 | `gorm.io/gorm` v1.31.2，连带 `jinzhu/inflection`、`jinzhu/now` | D3 | +3 |
| 新增 | `ncruces/go-sqlite3` v0.35.x、`gormlite`、`go-sqlite3-wasm`、`julianday` | D2、D3；SQLite 经 wasm2go 翻译成纯 Go | 约 +4 至 +6 |
| 保留 | `tls-client`、`fhttp`、`cobra`、`pflag`、`x/term`、`x/net`、`x/sys`、`fsnotify`、`tomledit`、MCP `go-sdk`、`pkg/browser`、`testify`（仅测试）、`yaml.v3`（仅脚本） | 都有实际用途 | — |

预计二进制链接的模块数降到 37 个左右，以实测为准。`gormlite` 和 go-sqlite3 v0.35 的版本组合，以编译和测试结果为准。

## 5. 目标目录结构

顶级目录 `cmd/`、`sdk/`、`internal/` 保持不变。`internal/` 内部按职责重新组织，包名允许调整；撤销 `services/`、`shared/`、`utils/` 这类按“性质”归类的汇总目录，每个包的名字直接说明它负责什么。

标记：`=` 内容不变，`~` 内容收敛，`+` 新增，`←` 从别处并入或改名。

### 5.1 sdk/

```text
sdk/                                        =  Cursor、Page、Resource、错误
sdk/pixiv/
├── client.go            ~  Open/OpenWith、Options、Client（原 pixiv.go）
├── artwork.go           ←  作品：详情、相关、排行、推荐、系列、时间线、趋势
├── artwork_search.go    ~
├── novel.go             ←  小说：详情、正文、排行、推荐、系列、搜索、时间线
├── user.go              ←  用户：详情、各类用户列表、作品与小说列表
├── bookmark.go          ←  收藏读取与标签
├── comment.go           ←  评论与 stamp
├── mutation.go          =  收藏/关注/评论写操作
├── models.go            ~  唯一的公开实体类型（dto.go 删除）
├── wire.go              +  未导出的响应结构与唯一一份映射
├── cursor.go errors.go login.go login_urls.go credentials.go
└── reference.go resource.go validation.go ugoira.go novel_parse.go doc.go   =
sdk/fanbox/
├── client.go creator.go post.go models.go wire.go                         ~ / ←
└── cursor.go errors.go reference.go request.go resource.go doc.go          =
```

### 5.2 internal/

```text
internal/
├── cli/                        CLI：目录结构与命令树一一对应（根命令 pixiv 就是 cli/ 本身）
│   ├── root.go composition.go execution.go sigpipe_*.go   =
│   ├── streams.go              ←  cli/invocation
│   ├── auth/ auth/loginhelper/ ←  pixiv auth
│   ├── search/ download/ bookmark/ comment/ detail/ follow/
│   ├── mypixiv/ ranking/ recommended/ series/ timeline/ user/   ←  pixiv <命令>
│   ├── mcp/                    ←  pixiv mcp（原 commands/pixiv/mcp，它同时服务两个产品）
│   ├── config/  update/        ←  pixiv config、pixiv update
│   ├── fanbox.go               ←  只定义 pixiv fanbox 命令组：持久 flag --proxy/--no-proxy、挂载三个子命令（约 30 行）
│   ├── fanbox/auth/ fanbox/download/ fanbox/post/   ←  pixiv fanbox <命令>
│   └── internal/               不是命令；只放命令之间共用的支撑代码
│       ├── command/            ←  commands/lifecycle.go：所有命令共用的生命周期声明
│       ├── input/              ←  cli/pipeline + utils/parse：stdin 输入编解码与参数解析
│       ├── pixivcmd/           ←  commands/pixiv/pixiv.go（原包名 deps）+ listing：Pixiv 内容命令共用的执行与列表入口
│       ├── fanboxcmd/          ←  commands/fanbox/internal/listing
│       └── resolver/           ←  shared/resolver：只被 CLI 命令使用
│
├── mcpserver/                  MCP 传输与生命周期                          =
│   ├── auth/                   =
│   ├── pixiv/  accounts/ gallery/ mediaproxy/ internal/toolhost/ tools/   ←  internal/runtime 改名
│   └── fanbox/ internal/toolhost/ tools/
│
├── pixivapi/                   ←  services/pixiv/{appapi,protocol,resource}：App API 传输、协议约定、资源下载
│   └── oauth/                  ←  services/pixiv/oauth
├── fanboxapi/                  ←  services/fanbox/{protocol,resource}：协议、Cloudflare 挑战、资源 URL 策略
├── pixivaccount/               ←  services/pixiv/account（原包名 pixiv）
│   └── pool/                   ←  services/pixiv/pool
├── fanboxaccount/              ←  services/fanbox/account（原包名 fanbox）
├── session/                    ←  services/pixiv、services/fanbox 的 Facade：按账号打开 SDK client 并提交凭据轮换
├── loginrelay/                 ←  services/pixiv/account/loginrelay + loginpage
├── reversesearch/              ←  services/reversesearch
│   └── ascii2d/ saucenao/ assembly/
│
├── config/
│   ├── paths/                  =
│   └── settings/               ~  go-toml 解析、Store 方法、keys.go
├── storage/
│   ├── database/               ~  GORM + gormlite
│   ├── atomicfile/             ←  storage/file/atomic + storage/file/replace
│   ├── filelock/               ←  storage/file/lock
│   └── secretfile/             ←  storage/file/secret
│
├── media/
│   ├── downloader/             ~  并入 downloader/parallel、utils/uri
│   ├── filename/               ←  media/downloader/filename
│   └── ugoira/  ugoira/jpeg/   =
├── browsercookies/             =  chromium/ firefox/ safari/ secret/ sqliteio/ system/
├── selfupdate/                 ←  internal/update 改名；installer/ process/ release/ source/
│
├── logging/                    ←  取代 shared/diagnostics 与 cli/diagnostics：slog Handler、ctx 调用信息、原因树
├── lifecycle/                  ←  shared/lifecycle
├── httpclient/                 ←  shared/network：按代理配置构造 http.Client
├── pagination/                 ←  shared/pagination + shared/traversal
├── record/                     ←  shared/record（并入切片转换）
├── searchfilter/               ←  shared/searchfilter + utils/date
├── plaintext/                  ←  utils/text 中的 SafeLine、SafeLines、HTMLPlainText
└── version/                    ←  shared/buildinfo + releaseversion
```

`utils/text` 中的 `FirstNonEmpty`、`DefaultString` 删除，调用处改用标准库 `cmp.Or`。

### 5.3 迁移对照

| 原路径 | 新路径 | 原因 |
|---|---|---|
| `services/pixiv/endpoint/**`、`services/fanbox/endpoint/**` | 并入 `sdk/{pixiv,fanbox}` | 只被 SDK 使用（A3、A4） |
| `services/pixiv/{appapi,protocol,resource}` | `pixivapi` | 同一个上游协议；`resource.Client` 改名为 `ResourceClient`，重复的 `ErrMalformedResponse` 只保留一个（D7） |
| `services/pixiv/oauth` | `pixivapi/oauth` | 与 `appapi` 有 7 个同名导出（`Client`、`New`、`WithHTTPClient` 等），保持独立子包 |
| `services/fanbox/{protocol,resource}` | `fanboxapi` | 无同名冲突 |
| `services/pixiv/account` | `pixivaccount` | 原包名 `pixiv` 与目录不符；与 FANBOX 账号包有 17 个同名导出且错误文案不同，两者是按产品并行的实现，不合并 |
| `services/fanbox/account` | `fanboxaccount` | 同上 |
| `services/pixiv/pool` | `pixivaccount/pool` | Pixiv 账号池 |
| `services/pixiv`、`services/fanbox`（Facade） | `session` | 职责是“按账号打开 SDK 会话”；同名的 `Facade`、`NewFacade` 改为 `session.Pixiv`、`session.NewPixiv`、`session.Fanbox`、`session.NewFanbox` |
| `services/pixiv/account/loginrelay/**` | `loginrelay` | CLI 和 MCP 共用的独立协议，不属于账号存储 |
| `services/reversesearch/**` | `reversesearch/**` | 撤销 `services/` 层 |
| `storage/file/{atomic,replace}` | `storage/atomicfile` | 都是原子落盘原语 |
| `storage/file/{lock,secret}` | `storage/{filelock,secretfile}` | 去掉 `file/` 中间层；`secretfile` 与 `browsercookies/secret` 区分 |
| `shared/diagnostics`、`cli/diagnostics` | `logging` | C1 中由 slog 取代自制的 Event/Sink/Scope |
| `shared/{pagination,traversal}` | `pagination` | 同属分页算法 |
| `shared/{lifecycle,record,searchfilter}`、`shared/network` | `lifecycle`、`record`、`searchfilter`、`httpclient` | 撤销 `shared/` 层；`network` 改为表达职责的名称 |
| `shared/resolver` | `cli/internal/resolver` | 只被 CLI 命令使用 |
| `shared/buildinfo`、`releaseversion` | `version` | 同属版本标识 |
| `utils/{date,parse,text,uri}` | `searchfilter`、`cli/input`、`plaintext`、`media/downloader` | 撤销 `utils/` 层，并入主要调用方 |
| `cli/commands/pixiv/<命令>` | `cli/<命令>` | 这些命令直接挂在根命令 `pixiv` 下，目录层级与命令层级一致 |
| `cli/commands/pixiv/mcp` | `cli/mcp` | `pixiv mcp` 同时服务 Pixiv 与 FANBOX，不属于 Pixiv 产品目录 |
| `cli/commands/fanbox/**` | `cli/fanbox/**` | 对应 `pixiv fanbox` 命令组 |
| `cli/commands/{config,update}` | `cli/{config,update}` | 对应根下命令 |
| `internal/update` | `internal/selfupdate` | 与命令包 `cli/update` 同名，现状需要别名 `updateapp` |
| `mcpserver/{pixiv,fanbox}/internal/runtime` | `…/internal/toolhost` | 遮蔽标准库 `runtime` |
| `cli/commands/pixiv/pixiv.go`（包名 `deps`）+ `internal/listing` | `cli/internal/pixivcmd` | 两包的 `Request` 字段完全相同，合并后删除 20 处 `listing.Request(request)` 转换 |
| `cli/{invocation,pipeline}`、`cli/commands/lifecycle.go` | `cli/streams.go`、`cli/internal/input`、`cli/internal/command` | 前者只有 19 行；其余是支撑代码，不是命令 |
| `e2e/` | `cmd/pixiv`、`sdk/{pixiv,fanbox}`、`internal/{reversesearch,browsercookies}` 下的测试文件 | 测试与被测代码放在一起；按测试对象拆分（A8） |
| `media/downloader/{parallel,filename}` | 前者并入 `downloader`，后者改为 `media/filename` | `parallel` 只被 downloader 使用；`filename` 还被 MCP 使用 |

**命名约束：**
- `internal/cli/` 下除 `internal/` 以外的每个目录都对应一个命令，目录路径就是命令路径。例如 `cli/fanbox/post` 对应 `pixiv fanbox post`。命令组目录只容纳子命令时可以不含 Go 文件，组本身在上一级定义。
- `mcpserver/` 与 `sdk/` 按产品分组：Pixiv 和 FANBOX 在这两处是对等的两个产品。CLI 中 FANBOX 是根命令下的子命令组，所以两者的目录形状不同，这是有意为之。
- 目录名与包名一致。
- `internal/` 根目录保持平铺，不按产品建 `internal/pixiv/`、`internal/fanbox/` 分组：分组后要么包名重复（两个 `api`、两个 `account`，而 `storage/database`、`cli`、`session` 会同时 import 两个产品的包，必须写别名），要么路径重复（`internal/pixiv/pixivapi`）。产品前缀已经让同一产品的包在目录列表中相邻。
- 包名不与 SDK 的 `pixiv`、`fanbox`、与同一调用方会一起 import 的内部包、或常用标准库包（`runtime`、`log`、`context` 等）重名，调用方不需要写 import 别名。按此要求：
  - `cli/fanbox/` 只作为子命令目录，不放 Go 文件。`pixiv fanbox` 命令组在 `cli/fanbox.go`（包 `cli`）中定义；原 `commands/fanbox/fanbox.go`（251 行）拆开：命令组定义留在 `cli/fanbox.go`；`Data` 中通用的命令支撑（参数校验、提示、JSON 输出、输入绑定）并入 `cli/internal/command`，FANBOX 子命令共用的函数（`ProxyOverride`、client 租约）并入 `cli/internal/fanboxcmd`；账号服务、浏览器等依赖由各子命令的构造函数直接接收，不再有 `Data` 聚合类型（见 3.1.2、3.4.2 的例子）。现状中该文件以包名 `fanbox` import SDK 的 `fanbox`，同名遮蔽。
  - `internal/update` 改名为 `internal/selfupdate`，与命令包 `cli/update` 区分；现状中命令包以别名 `updateapp` import 它。
  - `mcpserver/{pixiv,fanbox}/internal/runtime` 改名为 `toolhost`（承载 tool 的 App、`AddTool`、`Read`），不遮蔽标准库 `runtime`。

**包数变化（生产代码）：**

| 区域 | 重构前 | 重构后 |
|---|---|---|
| `sdk` | 3 | 3 |
| `internal/services` | 59 | 0（撤销；改为 `pixivapi`、`pixivapi/oauth`、`fanboxapi`、`pixivaccount`、`pixivaccount/pool`、`fanboxaccount`、`session`、`loginrelay`、`reversesearch/**`，共 12 个） |
| `internal/mcpserver` | 67 | 11 |
| `internal/shared` + `internal/utils` + `releaseversion` | 14 | 0（并入 8 个按职责命名的包或其调用方） |

### 5.4 scripts/

`tools/` 与 `scripts/` 都是 CI 和发布流程使用的 Go 工具，统一放在 `scripts/`；顶层 `tools/`、`ci/` 撤销。每个工具一个目录、一个 `package main`，测试与之同目录。

```text
scripts/
├── install.sh  install.cmd                 =  README 中公开的 raw URL，路径不能变
├── build.sh  build-platform.sh  package-release.sh  classify-change-scope.sh  test-*.sh   =
├── ci/                                     ←  顶层 ci/platforms.json
├── licensebundle/  releaseassets/  releasenotes/  homebrewformula/  homebrewrecovery/
├── nativeevidence/  linuxabi/              ←  scripts/cmd/X + scripts/internal/X 合并
├── release/  platformmatrix/  prmeta/  verification/   ←  tools/*（verification 的命令白名单随之移动）
├── internal/
│   ├── releasecontract/                    ~  并入 releasenotesrender（35 行）
│   └── verificationpolicy/                 ←  tools/internal/verificationpolicy
└── citest/                                 ←  scripts/tests/* 中保留的测试合并为一个包
```

| 原路径 | 处理 | 依据 |
|---|---|---|
| `scripts/{cmd,internal}/publicapi` | 删除 | 没有 workflow 或脚本调用，也没有 golden 文件；公开 API 基线改用 `go doc -all`（第 9 节） |
| `browser-evidence.yml`、`scripts/{cmd,internal}/browsernativeevidence`、`scripts/tests/browserevidence`、`ci/browser-fixtures.json`、`scripts/install-browser-sqlite.ps1` | 删除 | 只能手动触发；共运行 13 次，其中 9 次失败，最后一次在 2026-09-22 |
| `homebrew-prepublish-verify.yml` | 删除 | 只能手动触发；共运行 6 次，最后一次在 2026-07-29 |
| PR 治理（`pr-metadata`、`pr-verification`、`pr-invalid-close`、`pr-triage`、`auto-assign`、`prmeta`、`verification`、`verificationpolicy`） | 保留，只移动目录 | — |
| `.github/ci-change-scope.gitignore` | 保持原位 | 只由 `classify-change-scope.sh` 读取，留在 `.github/` 不影响整理 |

包数：`scripts/` 与 `tools/` 合计 32 个 Go 包 → 14 个。

## 6. 代表性代码

### 6.1 SDK：endpoint 并入，同族端点共用一个实现

**重构前：** 每个端点各有一个包，包里有自己的 `Transport` 接口、`Request`、`Result`、`Client` 和映射函数；SDK 的 `Client` 持有约 30 个子 client。

```go
// internal/services/pixiv/endpoint/user/followers/followers.go（following、mypixiv、related、search 结构相同）
type Transport interface {
	GetJSON(context.Context, string, url.Values, any) error
}
type Request struct{ UserID int64; Restrict string; Offset int }
type Result struct{ Items []user.Preview; NextOffset int; HasNext bool }
type Client struct{ transport Transport }

func (c *Client) List(ctx context.Context, request Request) (Result, error) { /* 拼 query、解码、校验、映射 */ }

// sdk/pixiv/ops_user.go
list, err := c.userFollowers.List(ctx, userfollowers.Request{UserID: request.UserID, Restrict: string(restrict), Offset: offset})
if err != nil {
	return sdk.Page[UserPreview]{}, classifyAppError(err, "UserFollowers")
}
return c.userPreviewPage("UserFollowers", query, list.Items, list.NextOffset, list.HasNext)
```

**重构后：** 公开方法只负责参数校验和拼 query，同族端点共用一个私有实现。

```go
// sdk/pixiv/user.go

// UserFollowers lists the users following one user.
func (c *Client) UserFollowers(ctx context.Context, req UserFollowersRequest) (sdk.Page[UserPreview], error) {
	const op = "UserFollowers"
	if req.UserID <= 0 {
		return sdk.Page[UserPreview]{}, newError(op, sdk.InvalidArgument, "user ID must be positive")
	}
	restrict, err := normalizeFollowingRestrict(op, req.Restrict)
	if err != nil {
		return sdk.Page[UserPreview]{}, err
	}
	query := url.Values{
		pixivapi.ParamUserID:   {itoa(req.UserID)},
		pixivapi.ParamRestrict: {string(restrict)},
	}
	return c.userPreviews(ctx, op, pixivapi.PathUserFollower, query, req.Cursor)
}

// userPreviews 是所有 user_previews 列表端点的共同实现，端点之间只有路径和 query 不同。
func (c *Client) userPreviews(ctx context.Context, op, path string, query url.Values, cursor sdk.Cursor) (sdk.Page[UserPreview], error) {
	// 1. 游标与本次 query 绑定，防止把别的列表的游标用到这里。
	offset, err := c.continuationPositiveOffset(op, query, cursor)
	if err != nil {
		return sdk.Page[UserPreview]{}, err
	}
	request := maps.Clone(query)
	if offset > 0 {
		request.Set(pixivapi.ParamOffset, strconv.Itoa(offset))
	}

	// 2. 解码并校验上游响应；不完整的响应一律视为 MalformedResponse。
	var raw userPreviewsWire
	if err := c.app.GetJSON(ctx, path, request, &raw); err != nil {
		return sdk.Page[UserPreview]{}, classifyAppError(err, op)
	}
	items, err := raw.items(c)
	if err != nil {
		return sdk.Page[UserPreview]{}, classifyAppError(err, op)
	}

	// 3. 下一页游标从 next_url 中的 offset 生成。
	next, err := c.nextOffsetCursor(op, query, raw.NextURL)
	if err != nil {
		return sdk.Page[UserPreview]{}, err
	}
	return sdk.Page[UserPreview]{Items: items, Next: next}, nil
}
```

```go
// internal/pixivapi/api.go

// App API 路径。
const (
	PathUserFollower  = "/v1/user/follower"
	PathUserFollowing = "/v1/user/following"
	PathUserMyPixiv   = "/v1/user/mypixiv"
	// ……其余端点
)

// query/form 参数名。
const (
	ParamUserID   = "user_id"
	ParamRestrict = "restrict"
	ParamOffset   = "offset"
	// ……
)
```

```go
// sdk/pixiv/models.go

// Restrict 是收藏、关注列表的可见范围。
type Restrict string

const (
	RestrictPublic  Restrict = "public"
	RestrictPrivate Restrict = "private"
)
```

```go
// sdk/pixiv/wire.go

// userPreviewsWire 是 /v1/user/{follower,following,mypixiv,related,...} 的共同响应结构。
type userPreviewsWire struct {
	Users   pixivapi.RequiredList[userPreviewWire] `json:"user_previews"`
	NextURL *string                              `json:"next_url"`
}

func (w userPreviewsWire) items(c *Client) ([]UserPreview, error) {
	if !w.Users.Present || !w.Users.Valid {
		return nil, pixivapi.MalformedResponse()
	}
	items := make([]UserPreview, len(w.Users.Items))
	for i, v := range w.Users.Items {
		if v.User.ID <= 0 {
			return nil, pixivapi.MalformedResponse()
		}
		items[i] = UserPreview{User: c.mapUser(v.User)}
	}
	return items, nil
}
```

响应结构不同的端点，例如 blocked，保留自己的 wire 类型，只和其他端点共用 `mapUser` 等映射函数。

**Client 结构：**

```go
// 重构前：约 30 个子 client 字段，构造时逐个 New(app)
type Client struct {
	app             *pixivapi.Client
	artworkBookmark *artworkbookmark.Client
	artworkDetail   *artworkdetail.Client
	// ……共 29 个
	userSearch      *usersearch.Client
	stamps          *stamps.Client
	// ……
}

// 重构后：只持有一个 transport 和身份字段（完整形态见 3.3）
type Client struct {
	app *pixivapi.Client // 拥有 HTTP client 和签名 URL 缓存
	// userID、userName、expiresAt、cursorInstance
}
```

### 6.2 SDK：每个实体只有一种公开类型

**重构前：** `Artwork` 和 `ArtworkDTO` 字段完全相同，只是后者带 json tag，之间靠 `ToArtworkDTO` 等函数逐字段复制，另外还有 `ToTagDTO`、`ToImageResourceDTO` 等 20 多个转换函数。

**核实结果（逐字段比对全部 DTO 与模型）：**
- `sdk/pixiv` 的 30 个 DTO、`sdk/fanbox` 的 16 个 DTO 与模型字段逐一对应。差异只有三类：
  - 9 处 `Resource` 字段的类型不同（DTO 用 `*sdk.ResourceDTO`，模型用 `sdk.Resource`）。
  - `CommentAccessControlDTO` 自带 `MarshalJSON`。
  - `sdk.PageDTO` 的 `Next` 是 string，模型里是 `Cursor`；而零值 `Cursor` 的 `MarshalJSON` 会直接报错。
- FANBOX 的 `Creator` 通过嵌入 `CreatorSummary` 得到 `id`、`name`、`icon`，JSON 展开后与 `CreatorDTO` 一致。
- 转换函数会把 nil 切片变成空切片（例如 `tags` 输出 `[]` 而不是 `null`），还会深拷贝指针字段。
- `mcpserver/pixiv/mediaproxy/reads.go` 的私有内存缓存嵌入了 `sdk.Resource`、`pixiv.ImageResource` 等公开类型，并且依赖完整 JSON（含 URL）做序列化。

**重构后：**
- 实体类型直接带 json tag。
- “不输出运行时 URL”的保证放在 `sdk.Resource` 自身。这样包含 `Resource` 的 9 个实体都自动安全，不需要逐个实现 `MarshalJSON`。
- 唯一需要保留 URL 的私有缓存，改为显式写出自己需要的字段。

```go
// sdk/resource.go

// Resource 是运行时资源定位信息。URL、RequestHeaders 只供进程内使用；
// JSON 只输出不透明引用，任何实体序列化时都不会带出定位信息。
type Resource struct {
	Ref                 ResourceRef
	URL                 string
	RequestHeaders      map[string]string
	ExpiresAt           *time.Time
	RequiresCredentials bool
}

func (r Resource) MarshalJSON() ([]byte, error) {
	if r.Ref.IsZero() {
		return []byte("null"), nil // 与原 ToResourceDTO 返回 nil 时的输出一致
	}
	return json.Marshal(struct {
		Ref                 ResourceRef `json:"ref"`
		RequiresCredentials bool        `json:"requires_credentials,omitempty"`
	}{r.Ref, r.RequiresCredentials})
}
```

```go
// sdk/page.go

// MarshalJSON 把游标输出为字符串；最后一页输出空串，与原 PageDTO 一致。
func (p Page[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Items []T    `json:"items"`
		Next  string `json:"next"`
	}{p.Items, p.Next.String()})
}
```

```go
// sdk/pixiv/models.go

type Artwork struct {
	ID          int64         `json:"id"`
	Title       string        `json:"title"`
	Tags        []Tag         `json:"tags"` // 映射层保证非 nil，输出 [] 而不是 null
	Cover       ImageResource `json:"cover"`
	Pages       []ArtworkPage `json:"pages,omitempty"`
	// ……其余字段及 tag 与原 ArtworkDTO 一一对应
}

type ImageResource struct {
	Resource sdk.Resource `json:"resource"` // 由 Resource.MarshalJSON 保证安全
	Variant  string       `json:"variant"`
	Width    int          `json:"width"`
	Height   int          `json:"height"`
}
```

```go
// internal/mcpserver/pixiv/mediaproxy/cache.go

// cachedResource 是进程内缓存的私有格式，需要保留定位信息，因此显式列出字段，
// 不嵌入 sdk.Resource（否则会继承它的安全 MarshalJSON，丢失 URL）。
type cachedResource struct {
	Ref                 *sdk.ResourceRef  `json:"ref,omitempty"`
	URL                 string            `json:"url"`
	RequestHeaders      map[string]string `json:"request_headers,omitempty"`
	ExpiresAt           *time.Time        `json:"expires_at,omitempty"`
	RequiresCredentials bool              `json:"requires_credentials,omitempty"`
}
```

`sdk.ResourceDTO`、`sdk.PageDTO` 和全部 `XxxDTO`、`ToXxxDTO` 一起删除。`CommentAccessControlDTO.MarshalJSON` 移到 `CommentAccessControl`。原来 DTO 转换的生产调用点约 20 处（CLI 13 个文件、MCP 6 个文件、`shared/record`），全部改为直接使用模型。

**调用方：**

```go
// 重构前
rec, err := record.RecordFromArtworkDTO(pixiv.ToArtworkDTO(artwork))
records, err := records.FromArtworks(items) // mcpserver/pixiv/internal/records

// 重构后
rec, err := record.RecordFromArtwork(artwork)
recs, err := record.FromArtworks(items) // internal/record
```

必须保留的安全测试：给 `Resource` 填入 URL 和请求头，分别序列化 `Artwork`、`Novel` 内容块、`UgoiraMetadata` 和 FANBOX `Post`，断言输出中既没有 URL，也没有请求头。`mediaproxy` 缓存需要一个往返测试，确认 URL 在缓存读出后仍然存在。

### 6.3 MCP：每个产品一个 tools 包

**重构前：** 每个 tool 单独一个包，`pixiv.go` 里依次调用 45 次 `xxx.Register(app, server)`。

```go
// internal/mcpserver/pixiv/tools/user_detail/user_detail.go
package user_detail

func Register(app *runtime.App, server *mcp.Server) { runtime.AddTool(app, server, &mcp.Tool{ /* … */ }, /* … */) }
type In struct{ UserID int64 `json:"user_id"` }
func handleUserDetail(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.UserDetail, error)
```

**重构后：** handler 是 `pixivTools` 的方法，按领域放进不同文件；注册表集中在一处。

```go
// internal/mcpserver/pixiv/tools/register.go
package tools

type pixivTools struct{ app *toolhost.App }

// Register 按领域注册全部 Pixiv tools；可选端口不存在时，对应的 tool 不注册。
func Register(app *toolhost.App, server *mcp.Server, ports Ports) {
	t := pixivTools{app: app}
	if ports.Accounts != nil {
		t.registerAccounts(server, ports)
	}
	if ports.Media != nil {
		t.registerGallery(server, ports.Media)
	}
	if ports.Downloads != nil {
		t.registerDownload(server)
	}
	t.registerArtworks(server)
	t.registerBookmarks(server)
	t.registerComments(server)
	t.registerNovels(server)
	t.registerUsers(server)
	t.registerSearch(server)
	t.registerTimelines(server)
	t.registerReverseSearch(server)
}
```

```go
// internal/mcpserver/pixiv/tools/user.go

const (
	toolUserDetail    = "pixiv_user_detail"
	toolUserArtworks  = "pixiv_user_artworks"
	toolUserNovels    = "pixiv_user_novels"
	toolUserRelations = "pixiv_user_relations"
)

func (t pixivTools) registerUsers(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{
		Name:         toolUserDetail,
		Annotations:  readOnlyRemote,
		Description:  "Get a user's complete profile through the authenticated Pixiv SDK.",
		InputSchema:  closedObject(map[string]any{argUserID: positiveInteger("Positive Pixiv user ID.")}, []string{argUserID}),
		OutputSchema: userDetailOutputSchema(),
	}, t.userDetail)
	// toolUserArtworks、toolUserNovels、toolUserRelations 依次注册
}

// userDetailIn 的 json 标签必须与 argUserID 一致（struct tag 只能是字面量）。
type userDetailIn struct {
	UserID int64 `json:"user_id" jsonschema:"required positive Pixiv user ID"`
}

// userDetail 把 SDK 返回的完整 envelope 封装为单条 user record。
func (t pixivTools) userDetail(ctx context.Context, _ *mcp.CallToolRequest, in userDetailIn) (*mcp.CallToolResult, userDetailOut, error) {
	if in.UserID <= 0 {
		return userDetailError(fmt.Errorf("%s must be a positive integer", argUserID))
	}
	detail, err := toolhost.Read(t.app, ctx, func(ctx context.Context, c *pixiv.Client) (pixiv.UserDetail, error) {
		return c.User(ctx, pixiv.UserRequest{UserID: in.UserID})
	})
	if err != nil {
		return userDetailError(err)
	}
	rec, err := record.RecordFromUserDetail(detail)
	if err != nil {
		return userDetailError(err)
	}
	return userDetailResult(rec)
}
```

`readOnlyRemote` 这类 annotations 组合定义为包级变量；每个 tool 的 annotations 取值和原来完全一致。多个 tool 共用的参数名（`argUserID`、`argCursor` 等）定义在 `schema.go`。

### 6.4 CLI：列表入口收敛

**重构前：** 4 个公开变体，参数逐个累加。

```go
func (a Runner) RunPooledIllustList(ctx, request, plan, jsonOut, ndjson bool, heading string, fetch, print) error
func (a Runner) RunPooledIllustListWithHeading(ctx, request, plan, jsonOut, ndjson bool, heading func() string, fetch, print) error
func (a Runner) RunPooledIllustListWithKey(ctx, request, plan, jsonOut, ndjson bool, jsonKey string, heading func() string, fetch, print) error
func (a Runner) RunPooledIllustListWithKeyPostFilter(ctx, request, plan, jsonOut, ndjson bool, jsonKey string, heading func() string, fetch, postFilter, print) error
```

**重构后：** 只有一个入口，可选项放进 spec 结构体，零值即默认值。

```go
// internal/cli/internal/pixivcmd/list.go

// Format 是列表的输出格式，由 --json、--ndjson 解析得到；两者互斥。
type Format int

const (
	FormatText Format = iota
	FormatJSON
	FormatNDJSON
)

// JSONKey 是列表 JSON 输出的顶层键。
type JSONKey string

// ArtworkList 描述一次作品列表的读取和输出方式。
type ArtworkList struct {
	// Heading 在池内账号确定后才求值，因为标题可能依赖当前账号身份。
	Heading func() string
	// Key 为空时使用 JSONKeyIllusts；少数命令使用不同的键，例如 JSONKeyManga；这是现有输出契约。
	Key     JSONKey
	Fetch   func(*pixiv.Client, context.Context, sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error)
	// PostFilter 可选：分页窗口确定之后再做本地筛选，不会为了凑满结果向后扫描。
	PostFilter func([]pixiv.Artwork) []pixiv.Artwork
	Print      func(items []pixiv.Artwork, start int) error
}

func (a Runner) RunArtworks(ctx context.Context, request Request, plan Plan, format Format, list ArtworkList) error

// 列表 JSON 输出的顶层键；已发布的输出契约，不能改名。
const (
	JSONKeyIllusts JSONKey = "illusts"
	JSONKeyManga   JSONKey = "manga"
	JSONKeyNovels  JSONKey = "novels"
	JSONKeyUsers   JSONKey = "user_previews"
)

// Static 把固定标题包装成 Heading。
func Static(heading string) func() string { return func() string { return heading } }
```

```go
// internal/cli/ranking/ranking.go
return a.runner().RunArtworks(cmd.Context(), request, plan, format, pixivcmd.ArtworkList{
	Heading: pixivcmd.Static(fmt.Sprintf("%s ranking", opts.mode)),
	Fetch:   fetch,
	Print:   printRanking,
})
```

`RunNovels` 和 `RunUsers` 采用同样的形态。三者共享的分页、spool 和提交逻辑合并为一个私有泛型函数 `runList[T]`。

### 6.5 配置：只保留 Store 方法，用 go-toml 替代 Koanf

**入口收敛：**

```go
// 重构前：包级函数、At 变体、WithFileStore 变体和 Store 方法并存
settings.LoadSnapshot()
settings.LoadSnapshotAt(path)
settings.LoadSnapshotAtWithFileStore(path, store)
settings.SetConfigValue(path, alias, value)
settings.SetConfigValueWithFileStore(path, alias, value, store)
settings.ReadPixivDefaultUserID()        // 包级函数，转发到 DefaultStore()
settings.DefaultStore().ReadPixivDefaultUserID()

// 重构后：只有 Store 方法；测试直接构造 Store{Files: fake}
store := settings.DefaultStore()
snap, err := store.Current()
uid, ok, err := store.ReadPixivDefaultUserID()

// 程序内部读取配置：直接用具名类型字段，不经过别名
if snap.Runtime.LogLevel == settings.LogLevelDebug { … }

// config set 命令：别名来自用户输入，属于数据，不是魔法字符串
result, err := store.Set(settings.Key(args[0]), args[1])
```

**配置键与枚举常量：**

```go
// internal/config/settings/keys.go

// Key 是配置别名，与 RuntimeConfig 字段上的 alias 标签一一对应。
type Key string

const (
	KeyDownloadPath       Key = "download_path"
	KeyHTTPSProxy         Key = "https_proxy"
	KeyLogLevel           Key = "log_level"
	KeyLogFormat          Key = "log_format"
	KeyWebFallbackEnabled Key = "web_fallback_enabled"
	KeyAccountPoolAccounts Key = "account_pool_accounts"
	// ……其余全部别名
)
```

```go
// internal/config/settings/config.go

// LogLevel 是日志级别配置的取值。
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"  // C1 新增
	LogLevelError LogLevel = "error" // C1 新增
)

// LogFormat 是诊断输出格式配置的取值。
type LogFormat string

const (
	LogFormatText LogFormat = "text"
	LogFormatJSON LogFormat = "json"
)

type RuntimeConfig struct {
	// ……
	LogLevel  LogLevel  `config:"logging.level" alias:"log_level" env:"PIXIV_LOG_LEVEL" default:"info" cli:"true" example:"true"`
	LogFormat LogFormat `config:"logging.format" alias:"log_format" env:"PIXIV_LOG_FORMAT" default:"text" cli:"true" example:"true"`
}
```

**使用处：**

```go
// 重构前
switch alias {                         // values.go
case "log_level":
	return normalizeLogLevel(value)
}
if runtime.LogLevel != "debug" { … }  // cli/execution.go
if _, err := s.Effective("web_fallback_enabled"); err != nil { … }

// 重构后
switch key {
case KeyLogLevel:
	return parseLogLevel(value)        // 校验取值属于 LogLevel 常量集合
}
if runtime.LogLevel != settings.LogLevelDebug { … }
if _, err := s.Effective(KeyWebFallbackEnabled); err != nil { … }
```

`SettingSpecByAlias`、`Effective`、`Get`、`Set`、`Unset` 的参数类型都改为 `Key`。`schema.go` 已按字段的 `reflect.Kind` 判定值类型；B1 中需确认赋值处使用 `SetString` 这类按 Kind 赋值的方法，使 `LogLevel` 这类以 string 为底层类型的具名类型可以直接绑定。`keys.go` 的一致性由 3.2 中的表驱动测试保证。

**解析部分：**

```go
// internal/config/settings/snapshot.go

// 1. 文件内容在这里一次性解析，环境变量在这里一次性捕获，之后的读取都基于这份快照。
var file map[string]any
if err := toml.Unmarshal(body, &file); err != nil {
	return Snapshot{}, configFileError(path, err)
}

// lookup 按声明的配置路径（例如 "pixiv.network.https_proxy"）读取嵌套表。
// 第二个返回值区分“键不存在”和“显式写成零值”。
func lookup(table map[string]any, configPath []string) (any, bool) {
	for i, key := range configPath {
		value, ok := table[key]
		if !ok {
			return nil, false
		}
		if i == len(configPath)-1 {
			return value, true
		}
		if table, ok = value.(map[string]any); !ok {
			return nil, false
		}
	}
	return nil, false
}
```

稀疏写回继续用 `tomledit`，不改动。

### 6.6 HTTP：resty 改为 net/http

```go
// internal/pixivapi/client.go

func (c *Client) doJSON(ctx context.Context, method, rawURL string, opts requestOptions, out any) error {
	target, err := url.Parse(rawURL)
	if err != nil {
		return Transport(err)
	}
	if len(opts.Query) > 0 {
		target.RawQuery = opts.Query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), nil)
	if err != nil {
		return Transport(err)
	}
	for name, value := range opts.Headers {
		req.Header.Set(name, value)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Transport(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Transport(err)
	}

	// 以下状态码、Retry-After 和解码规则与原 resty 实现完全一致。
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		retryAfter, present := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
		return HTTPStatusWithRetryAfter(resp.StatusCode, retryAfter, present)
	}
	if len(bytes.TrimSpace(body)) == 0 || json.Unmarshal(body, out) != nil {
		return MalformedResponse()
	}
	return nil
}
```

上面的代码只示意 resty 到 net/http 的对应关系。B2 中发送与重试统一收敛为 6.9 的 `do`（重试循环）与 `send`（单次发送），`doJSON` 只负责构造请求和解码响应，C1 再在 `do`、`send` 中加入日志。

实施时要核对两点：resty 在没有设置 User-Agent 时会填入自己的默认值，必须确认 `apiHeaders()` 已经显式设置了 User-Agent；原来 rawURL 自带的 query 与 `opts.Query` 的合并方式也要保持一致。

### 6.7 终端提示：survey 改为 x/term

```go
// internal/cli/auth/prompt.go

func terminalPromptSecret(in io.Reader, w io.Writer, message string) (string, error) {
	fd, err := terminalFD(in) // 非终端时返回与现在相同的错误
	if err != nil {
		return "", err
	}
	for {
		writePrompt(w, "%s: ", message)
		raw, err := term.ReadPassword(fd) // 不回显
		writePrompt(w, "\n")
		if err != nil {
			return "", err
		}
		if value := strings.TrimSpace(string(raw)); value != "" {
			return value, nil
		}
		writePrompt(w, "value cannot be empty\n")
	}
}

func terminalPromptSelect(in io.Reader, w io.Writer, message string, options []string) (string, error) {
	if len(options) == 0 {
		return "", errors.New("no available options")
	}
	reader := bufio.NewReader(in)
	for i, option := range options {
		writePrompt(w, "  %d) %s\n", i+1, option)
	}
	for {
		line, err := readLine(reader, w, fmt.Sprintf("%s [1-%d]: ", message, len(options)))
		if err != nil {
			return "", err
		}
		if n, convErr := strconv.Atoi(line); convErr == nil && n >= 1 && n <= len(options) {
			return options[n-1], nil
		}
		writePrompt(w, "enter a number between 1 and %d\n", len(options))
	}
}
```

Input 和 Confirm 也用 `readLine` 实现：空输入时采用默认值，Confirm 接受 `y/yes/n/no`。提示写到哪个 writer，与现在保持一致。

### 6.8 持久化：GORM + gormlite

**连接：** migration 仍然在 `*sql.DB` 上执行，GORM 只负责之后的查询和写入。

```go
// internal/storage/database/database.go

import (
	_ "github.com/ncruces/go-sqlite3/driver" // 注册 database/sql 驱动 "sqlite3"
	"github.com/ncruces/go-sqlite3/gormlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func Open(appDataDir string) (*DB, error) {
	// …目录校验与权限处理保持不变…
	sqlDB, err := sql.Open("sqlite3", dsnWithPragmas(path))
	if err != nil {
		return nil, fmt.Errorf("database: open database: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)

	orm, err := gorm.Open(gormlite.OpenDB(sqlDB), &gorm.Config{
		Logger:                 logger.Discard, // SQL 和参数一律不进入日志
		SkipDefaultTransaction: true,           // 事务边界由仓储方法显式声明，与现在的 autocommit 一致
		NamingStrategy:         schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("database: open orm: %w", err)
	}
	db := &DB{sql: sqlDB, orm: orm, path: path}
	// 首次连接和 migration 在侧车锁内完成：application_id 校验和 migrate(sqlDB) 都不变。
	// …
}

func dsnWithPragmas(path string) string {
	query := url.Values{}
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "synchronous(FULL)")
	query.Add("_pragma", "secure_delete(ON)")
	query.Add("_pragma", "trusted_schema(off)")
	query.Add("_pragma", "busy_timeout(0)") // 显式写出，与现有行为一致
	// ncruces 直接把 URI 交给 SQLite 解析：Windows 下写成 file:C:/...，不加 "///"。
	return "file:" + filepath.ToSlash(path) + "?" + query.Encode()
}
```

**行结构：** 新文件 `rows.go`，只在包内使用。

```go
// internal/storage/database/rows.go

// pixivAccountRow 对应 pixiv_account 表的一行；所有列都显式映射，不依赖 GORM 的隐式约定。
type pixivAccountRow struct {
	UserID             int64 `gorm:"primaryKey;autoIncrement:false"`
	SortOrder          int64
	Username           string
	RefreshToken       []byte
	CredentialRevision int64
	PremiumStatus      *int64 // NULL 表示未知，0/1 表示否/是
	PremiumCheckedAt   *int64
	PoolFrozenUntil    *int64
	PoolLastSelected   bool
	CreatedAt          int64 `gorm:"autoCreateTime:false"` // 由调用方写 Unix 秒
	UpdatedAt          int64 `gorm:"autoUpdateTime:false"`
	Schedulable        bool  `gorm:"->"` // 只读；由 migration 0003 维护
}

const tablePixivAccount = "pixiv_account"

// Go 代码中引用到的列名；gorm 标签与 SQL migration 中的列名必须与之一致。
const (
	colUserID             = "user_id"
	colSortOrder          = "sort_order"
	colRefreshToken       = "refresh_token"
	colCredentialRevision = "credential_revision"
	colUpdatedAt          = "updated_at"
)

func (pixivAccountRow) TableName() string { return tablePixivAccount }

func (r pixivAccountRow) account() pixivaccount.Account {
	a := pixivaccount.New(r.UserID, r.Username, r.RefreshToken)
	a.SortOrder = r.SortOrder
	a.CredentialRevision = r.CredentialRevision
	a.PremiumStatus = intPtrToBoolPtr(r.PremiumStatus)
	// …其余字段一一赋值
	return a
}
```

**查询：** 原来的 `scanPixivAccounts`、`scanPixivAccount`、`rowScanner` 和列常量全部删除。

```go
// ListPixiv 返回全部 Pixiv 账号，按 sort_order 升序排列。
func (d *DB) ListPixiv(ctx context.Context) ([]pixivaccount.Account, error) {
	return withDatabaseValue(ctx, d, func() ([]pixivaccount.Account, error) {
		var rows []pixivAccountRow
		if err := d.orm.WithContext(ctx).Order(colSortOrder).Find(&rows).Error; err != nil {
			return nil, err
		}
		accounts := make([]pixivaccount.Account, len(rows))
		for i, row := range rows {
			accounts[i] = row.account()
		}
		return accounts, nil
	})
}
```

**条件更新：**

```go
// RotatePixivCredentials 只在 revision 仍等于 expectedRevision 时写入新凭据，
// 防止旧请求覆盖已经轮换过的凭据。
func (d *DB) RotatePixivCredentials(ctx context.Context, userID, expectedRevision int64, refreshToken []byte) error {
	return d.withLock(ctx, func() error {
		if userID <= 0 || expectedRevision <= 0 || len(refreshToken) == 0 {
			return errors.New("database: invalid rotation input")
		}
		result := d.orm.WithContext(ctx).Model(&pixivAccountRow{}).
			Where(colUserID+" = ? AND "+colCredentialRevision+" = ?", userID, expectedRevision).
			Updates(map[string]any{ // 用 map 指定列，零值和 NULL 也会写入
				colRefreshToken:       refreshToken,
				colCredentialRevision: gorm.Expr(colCredentialRevision + " + 1"),
				colUpdatedAt:          time.Now().UTC().Unix(),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 0 {
			return nil
		}

		// 没有行被更新：账号可能已删除，也可能已被另一请求轮换。
		var count int64
		if err := d.orm.WithContext(ctx).Model(&pixivAccountRow{}).Where(colUserID+" = ?", userID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return pixivaccount.ErrNotFound
		}
		return pixivaccount.ErrCredentialConflict
	})
}
```

`SelectPixiv` 这类调度查询如果含复杂条件，使用 `d.orm.Raw(...).Scan(&rows)` 保留原 SQL，只省掉手写的扫描代码。

### 6.9 日志：slog

**分层规则：**

| 层 | 是否记录日志 | 记录内容 | logger 从哪里来 |
|---|---|---|---|
| SDK 业务 `Client` 及其方法 | 否；只在 ctx 中登记操作名 | — | — |
| 内部业务对象：`pixivaccount`、`fanboxaccount`、`session`、`storage/*` | 只在恢复点记录 WARN，例如账号池冻结或切换账号 | 恢复动作与原因；失败本身通过带原因链的错误返回 | `logging.From(ctx)`，不持有字段 |
| SDK 传输与协议层：`pixivapi`、`pixivapi/oauth`、`fanboxapi` | 是 | 每次请求的结果；token 刷新、限流等待、请求节流、Cloudflare 挑战与 solver 等内部决策（见下表） | 构造时由 SDK 的 `Options.Logger` 传入；这些对象本身是基础设施 |
| 应用层：CLI 命令、MCP tool runtime | 是 | 操作完成时的 INFO 结果摘要，例如下载数量 | `logging.From(ctx)` |
| 执行边界：CLI 根命令、MCP `AddTool` 包装 | 是 | 操作终止时记录一次 ERROR，附完整原因树 | 装配时注入 |

业务对象不持有 logger 字段。需要诊断的信息主要在传输层（请求是否发出、返回了什么状态）和执行边界（最终成功还是失败、原因是什么），业务层负责把原因完整地放进返回的错误。业务恢复点从 `logging.From(ctx)` 取得 logger；这与现有 `diagnostics.Emit(ctx, …)` 的传递方式相同，C1 时逐处替换。

**取代现有诊断系统：** `shared/diagnostics` 的 `Event`、`Sink`、`Scope` 和 `cli/diagnostics` 的渲染器，是一套按 `log_level`/`log_format` 输出的自制结构化日志。C1 用 slog 整体取代它们：`Scope` 对应 `logging.WithCall`，`Sink` 对应 `slog.Handler`，`Event` 的 Module/Kind/Operation/Count/Duration 对应日志属性。不保留两套并行机制。

**SDK：唯一的日志入口是 `Options.Logger`。** 公开 SDK 只接受一个标准库 `*slog.Logger`，不定义自己的日志接口。职责划分：

| 角色 | 职责 | 不做的事 |
|---|---|---|
| `Options.Logger` | 构造时交给 `pixivapi`、`pixivapi/oauth`、`fanboxapi`，之后不再保存在别处 | — |
| `pixivapi`、`fanboxapi`（基础设施） | 持有 logger；在实际发出请求、做出内部决策的那一处记录日志 | 不记录 query、请求头、响应体、token、Cookie、签名 URL |
| SDK 业务 `Client` 及其方法 | 在入口把操作名放进 ctx（与错误中的 `op` 是同一个常量） | 不持有 logger 字段，不写日志 |
| 执行边界 | 记录最终成功或失败 | 不重复记录传输层已记录的请求 |

**记录的内容：**

| 事件 | 级别 | 位置 | 属性 |
|---|---|---|---|
| 每次请求的结果 | DEBUG；429、5xx 为 WARN | `send` | method、host、path、status、duration、attempt、sdk_operation |
| access token 过期，自动刷新后重试 | DEBUG | 重试循环 | sdk_operation |
| refresh token 被轮换 | INFO | `oauth` | 不含 token 值 |
| 按 Retry-After 等待后重试 | WARN | 重试循环 | wait |
| 请求间隔节流实际发生等待 | DEBUG | 节流处 | wait |
| FANBOX 判定为 Cloudflare 挑战 | WARN | `fanboxapi` | host、path |
| solver 开始、完成；带 clearance 重放 | DEBUG；完成为 INFO | `fanboxapi` | duration |

失败本身不在这里记录 ERROR：错误连同原因链返回给调用方，由执行边界记录一次。

```go
// sdk/pixiv/client.go

type Options struct {
	// ……现有字段

	// Logger 接收 SDK 的诊断日志：每次请求的结果，以及 token 刷新、限流等待、请求节流等内部决策。
	// nil 时不输出。日志不包含 query、请求头、响应体、token 或签名 URL。
	Logger *slog.Logger
}

func OpenWith(ctx context.Context, refreshToken string, opts Options) (*Client, string, error) {
	log := cmp.Or(opts.Logger, slog.New(slog.DiscardHandler))
	httpClient, selfHTTP := opts.httpClient()
	app := pixivapi.New(pixivapi.Config{
		HTTPClient:   httpClient,
		OwnsClient:   selfHTTP,
		Logger:       log,
		AllowedHosts: opts.ResourcePolicy.AllowedHosts,
		// ……其余配置不变
	})
	// ……OAuth 刷新、身份校验与现在一致；oauth 客户端使用同一个 log
	return &Client{app: app /* , 身份字段 */}, rotated, nil
}

// 公开方法在入口登记操作名，传输层的日志据此带上 sdk_operation。
func (c *Client) UserFollowers(ctx context.Context, req UserFollowersRequest) (sdk.Page[UserPreview], error) {
	const op = "UserFollowers"
	ctx = pixivapi.WithOperation(ctx, op)
	// ……与 6.1 相同
}
```

```go
// internal/pixivapi/client.go

type Client struct {
	http    *http.Client
	session Session
	log     *slog.Logger // 永不为 nil
	// ……签名 URL 缓存、资源白名单
}

// do 是所有 App API 请求唯一的发送循环；重试规则（401 刷新一次、有效 Retry-After 重试一次、
// disableRetryAfterRetry）与现状完全一致，日志记录在做出决策的地方。
func (c *Client) do(ctx context.Context, req request) (*http.Response, error) {
	for attempt := 1; ; attempt++ {
		if err := c.pace(ctx); err != nil { // 实际等待时记录 DEBUG
			return nil, err
		}
		resp, err := c.send(ctx, req, attempt) // 记录本次请求结果
		if err != nil {
			return nil, Transport(err)
		}
		switch {
		case resp.StatusCode == http.StatusUnauthorized && attempt == 1:
			resp.Body.Close()
			c.log.LogAttrs(ctx, slog.LevelDebug, "Access token expired; refreshing", operationAttr(ctx))
			if err := c.session.Refresh(ctx); err != nil {
				return nil, err
			}
		case resp.StatusCode == http.StatusTooManyRequests && attempt == 1 && !c.disableRetryAfterRetry:
			wait, ok := parseRetryAfter(resp.Header.Get(HeaderRetryAfter), time.Now())
			if !ok {
				return resp, nil
			}
			resp.Body.Close()
			c.log.LogAttrs(ctx, slog.LevelWarn, "Rate limited; retrying after wait", operationAttr(ctx), slog.Duration(logging.AttrWait, wait))
			if err := sleep(ctx, wait); err != nil {
				return nil, err
			}
		default:
			return resp, nil
		}
	}
}

// send 发出一次请求并记录结果。只记录 host 和 path。
func (c *Client) send(ctx context.Context, req request, attempt int) (*http.Response, error) {
	start := time.Now()
	resp, err := c.http.Do(req.build(ctx))
	attrs := []slog.Attr{
		slog.String(logging.AttrMethod, req.method),
		slog.String(logging.AttrHost, req.url.Host),
		slog.String(logging.AttrPath, req.url.Path),
		slog.Duration(logging.AttrDuration, time.Since(start)),
		slog.Int(logging.AttrAttempt, attempt),
		operationAttr(ctx),
	}
	switch {
	case err != nil:
		c.log.LogAttrs(ctx, slog.LevelDebug, "Request failed", append(attrs, slog.Any(logging.AttrError, err))...)
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError:
		c.log.LogAttrs(ctx, slog.LevelWarn, "Request rejected by upstream", append(attrs, slog.Int(logging.AttrStatus, resp.StatusCode))...)
	default:
		c.log.LogAttrs(ctx, slog.LevelDebug, "Request completed", append(attrs, slog.Int(logging.AttrStatus, resp.StatusCode))...)
	}
	return resp, err
}
```

```go
// internal/pixivapi/operation.go

type operationKey struct{}

// WithOperation 把 SDK 操作名放进 ctx；只供日志属性使用，不影响请求。
func WithOperation(ctx context.Context, op string) context.Context {
	return context.WithValue(ctx, operationKey{}, op)
}

func operationAttr(ctx context.Context) slog.Attr {
	op, _ := ctx.Value(operationKey{}).(string)
	return slog.String(logging.AttrSDKOperation, op)
}
```

`fanboxapi` 采用同一形态：logger 由 `fanbox.Options.Logger` 传入；请求结果在其发送函数中记录；挑战判定、solver 和重放在各自的决策处记录（替换现有的 14 处诊断事件）。

**属性键：** 所有日志属性键是 `internal/logging` 的导出常量（`AttrMethod`、`AttrHost`、`AttrPath`、`AttrStatus`、`AttrDuration`、`AttrAttempt`、`AttrWait`、`AttrSDKOperation`、`AttrError`、`AttrCount`），`pixivapi`、`fanboxapi` 和执行边界共用，JSON 日志的字段名就是这些常量的值。`sdk_operation` 是 SDK 方法名；`operation` 是 CLI 命令路径或 MCP tool 名，由执行边界写入，两者不混用。

**删除的部分：** `sdk/pixiv/pixiv.go` 的 `diagnosticRoundTripper` 与 `withDiagnosticTransport`（请求记录改在 `pixivapi.send` 中）；`fanboxapi` 中的 `diagnostics.Emit` 调用（改为上表中的 slog 记录）。

**为什么 SDK 用 Options 而内部包用 ctx：** 公开 SDK 的使用方无法使用 `internal/logging` 的 ctx 键，所以 SDK 在构造时接收 logger；内部业务对象没有构造参数这一负担，按 3.3 从 ctx 取得。两处是同一个 logger 实例；MCP 调用 ID 等关联信息由 Handler 从 ctx 读取，因此 SDK 内的日志同样带有调用关联。

**共享组件：Handler 与 context。**

```go
// internal/logging/logging.go

type contextKey struct{}

// callInfo 是一次 CLI 命令或 MCP tool 调用的关联信息，由执行边界写入 ctx。
type callInfo struct {
	operation string
	callID    string // 只有 MCP 并发调用时才有值
}

// WithCall 把调用信息和 logger 放进 ctx；同一次调用的所有日志都会附带这些信息。
func WithCall(ctx context.Context, logger *slog.Logger, operation, callID string) context.Context

// From 返回 ctx 中的 logger；没有时返回丢弃所有输出的 logger，调用方无需判空。
func From(ctx context.Context) *slog.Logger
```

```go
// internal/logging/handler.go

// textHandler 的输出格式：
//   2026-10-15 12:21:18.013 DEBUG --- internal/pixivapi/logging.go:31 : Request completed method=GET path=/v1/search/illust status=200 duration=212ms
//   2026-10-15 12:21:18.247 ERROR --- internal/cli/execution.go:120 : Command failed operation=pixiv download
//       caused by: connect: connection refused
type textHandler struct {
	mu    *sync.Mutex
	w     io.Writer // 只能是 stderr
	level slog.Leveler
	attrs []slog.Attr
}

func (h *textHandler) Enabled(_ context.Context, level slog.Level) bool { return level >= h.level.Level() }

func (h *textHandler) Handle(ctx context.Context, r slog.Record) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %-5s --- %s : %s",
		r.Time.Format(timeLayout), r.Level, sourcePath(r.PC), r.Message)
	writeCallInfo(&b, ctx)     // operation=…、call_id=…（从 ctx 读取）
	writeAttrs(&b, h.attrs, r) // key=value；error 属性展开为 "caused by:" 行
	b.WriteByte('\n')
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, b.String())
	return err
}

// sourcePath 通过 runtime.CallersFrames 取得真实调用位置并去掉 module 前缀。
func sourcePath(pc uintptr) string

// causes 遍历 Unwrap() error 和 Unwrap() []error（errors.Join）的全部分支，每个原因单独一行。
func causes(err error) []string
```

**执行边界：** 只在这一处记录终止 ERROR。

```go
// internal/cli/execution.go
ctx = logging.WithCall(ctx, logger, cmd.CommandPath(), "")
err := cmd.ExecuteContext(ctx)
if err != nil {
	logging.From(ctx).LogAttrs(ctx, slog.LevelError, "Command failed", slog.Any(logging.AttrError, err))
}

// internal/mcpserver/pixiv/internal/toolhost：AddTool 统一包装每个 handler
ctx = logging.WithCall(ctx, app.logger, toolName, callID)
result, out, err := handler(ctx, req, in)
// err != nil 或 result.IsError 时记录一次 ERROR，然后再转换为结构化结果
```

**应用层：结果摘要。**

```go
// internal/cli/download/download.go
logging.From(ctx).InfoContext(ctx, "Download completed", logging.AttrCount, len(saved))
```

**装配：** 在 `composition.go` 中进行。

```go
logger, err := logging.New(stderr, cfg.LogLevel, cfg.LogFormat) // LogFormatJSON 时使用 slog.NewJSONHandler
if err != nil {
	return err
}
// logger 交给三处：SDK 的 Options.Logger（传输与协议层）、CLI 执行边界、MCP toolhost；内部业务对象从 ctx 取得。
```

## 7. 实施阶段

第 7 节的工作项按 7.0 的 goal 执行。

### 7.0 并行执行与 goal

A1、B2 等是工作项的名称。执行单位是 **goal**：一个 goal 对应一个命名的 worktree、一个分支、一个 PR，验收通过后合并进 main。共 45 个 goal，分四个阶段：阶段 0、3 内部按依赖执行，阶段 1、2 内的线路可以并行。各 goal 的完整语句见同目录的 `goals.md`。

```text
阶段 0（串行）  G01 → G02 → G03 → G04 → G05
阶段 1（并行）  L1 SDK      G06 → G07 → G08 → G09
                L2 MCP      G10
                L3 CLI      G11 → G12
                L4 配置     G13 → G14
                L5 传输     G15
                L6 存储     G16
                L7 真实测试 G17
                L8 叶子包   G18
                L9 脚本/CI  G19 → G20 → G21      ← 不依赖阶段 0
阶段 2          G23–G27 与 G22（日志）按下表依赖并行 → G28（扫尾）
阶段 3          G35–G38（测试精简）并行 → G44（测试重写）→ G39、G40、G42（逐包审查与修正）并行，G41 在 G39、G40 之后 → G45（遗留问题修正）与 G29、G33 并行 → G30–G32（注释整理）并行 → G34（收尾）
                G43（面向用户的 changelog）现在就可以开始；G41、G42、G34 都依赖它
```

2026-10-23 调整：G19 不再等待发布 workflow 的状态；G20 把 S2 扩大为 scripts 与 CI 的全面精简（删除不再需要的工具和 workflow、能用 sh 写清楚的工具改为 sh、撤销 scripts/internal、按用途重排目录，细则见 `goals.md`）；G21、G33 照常执行；frontend/ 的注释整理并入 G32。G09、G23–G27 只等待与其代码冲突的 goal；G24、G26 先于 G22 合并，G22 在它们的结果上改造日志。依赖以下表和 `goals.md` 为准。

| Goal | worktree 名称 | 工作项 | 依赖（须已合并） | 负责的目录 |
|---|---|---|---|---|
| G01 | `refactor-g01-tidy` | A1 | — | `go.mod`、`go.sum` |
| G02 | `refactor-g02-move-shared` | A7①：撤销 `shared/`、`utils/`、`storage/file/`，`releaseversion` 并入 `version`，`update` 改名为 `selfupdate`，MCP `internal/runtime` 改名为 `toolhost` | G01 | 全仓库（纯移动） |
| G03 | `refactor-g03-move-services` | A7②：撤销 `services/`（`endpoint/**` 暂留原处） | G02 | 全仓库（纯移动） |
| G04 | `refactor-g04-move-cli` | A7③：CLI 目录按命令路径重排 | G03 | `internal/cli/`（纯移动） |
| G05 | `refactor-g05-sdk-types` | A2，以及 SDK 公开 API 的全部删除与改名（T6 中 SDK 的部分）；补做 G03 推迟的 `session` 合包与改名 | G04 | `sdk/`、`record`、`internal/session`、各调用方 |
| G06 | `refactor-g06-sdk-artwork` | A3：artwork 端点并入 `sdk/pixiv` | G05 | `sdk/pixiv`、`services/pixiv/endpoint/artwork/**`、`pixivapi/api.go`（只新增常量） |
| G07 | `refactor-g07-sdk-novel` | A3：novel 端点 | G06 | 同上，novel 部分 |
| G08 | `refactor-g08-sdk-user` | A3：user 及其余 Pixiv 端点；删除 `services/pixiv/endpoint/` | G07 | 同上，其余部分 |
| G09 | `refactor-g09-sdk-fanbox` | A4 | G06 | `sdk/fanbox`、`services/fanbox/endpoint/**`；完成后 `services/fanbox/` 消失（`services/` 在 G08、G09 都合并后消失） |
| G10 | `refactor-g10-mcp-tools` | A5 | G05 | `internal/mcpserver/` |
| G11 | `refactor-g11-cli-commands` | A6 | G05 | `internal/cli/`；例外：`composition.go` 中构造命令依赖的部分 |
| G12 | `refactor-g12-cli-prompt` | B3 | G11 | `internal/cli/auth` 的提示代码、`go.mod` |
| G13 | `refactor-g13-config-tests` | T1 | G05 | `internal/config/settings` 的测试 |
| G14 | `refactor-g14-config-toml` | B1，含配置墓碑（T6） | G13 | `internal/config/`、`composition.go`、`cli/config/` |
| G15 | `refactor-g15-transport` | B2 | G05 | `internal/pixivapi/`、`internal/fanboxapi/`（`api.go` 的新增常量除外） |
| G16 | `refactor-g16-storage` | B4 | G05 | `internal/storage/`；`pixivaccount`、`browsercookies` 测试中的驱动名 |
| G17 | `refactor-g17-live-tests` | A8 | G05 | `e2e/`（删除）与各处新建的 `live_test.go`、`cmd/pixiv/main_test.go`；例外：`release.yml` 的 e2e job、`platform-smoke.yml` 的测试路径 |
| G18 | `refactor-g18-leaf-packages` | T5：`media/ugoira`、`browsercookies/*`、`selfupdate/*`、`reversesearch/*`、`loginrelay` | G05 | 这些包 |
| G19 | `refactor-g19-scripts-move` | S1 | — | `scripts/`、`tools/`、`ci/`、`.github/` |
| G20 | `refactor-g20-scripts-slim` | S2（范围扩大） | G19 | 同上 |
| G21 | `refactor-g21-workflow-refs` | T4：脚本与 workflow 中的编号引用 | G20 | 同上 |
| G22 | `refactor-g22-logging` | C1 | G07、G08、G09、G10、G11、G14、G15、G24、G26 | `internal/logging`、`pixivapi`、`fanboxapi`、`toolhost`、`cli/execution.go`、`composition.go`、SDK `Options` |
| G23 | `refactor-g23-shared-mechanisms` | T5：`lifecycle`、`pagination`、`record`、`searchfilter`、`httpclient`、`plaintext`、`version` | G08、G14、G19 | 这些包及其调用处 |
| G24 | `refactor-g24-media` | T5：`media/downloader`、`media/filename` | G05 | 这些包 |
| G25 | `refactor-g25-mcp-runtime` | T5：MCP 的 `auth`、`accounts`、`gallery`、`mediaproxy` | G05 | 这些包 |
| G26 | `refactor-g26-accounts` | T5：`pixivaccount`、`pixivaccount/pool`、`fanboxaccount`、`session` | G05 | 这些包 |
| G27 | `refactor-g27-cli-auth` | T5 与 T2：`cli/auth`、`loginhelper` 的注入点 | G12 | 这些包 |
| G28 | `refactor-g28-sweep` | T2 重新扫描、T3、T4（scripts、.github 以外）、T6 其余（含 `illustration` 别名） | G22–G27 | 全仓库剩余部分 |
| G29 | `refactor-g29-headers-sdk` | C3 包文档与注释整理（3.5、3.5.1）：`sdk/`、`cmd/` | G39–G42 | 只改注释 |
| G30 | `refactor-g30-headers-cli` | C3 包文档与注释整理（3.5、3.5.1）：`internal/cli/` | G39–G42、G45 | 只改注释 |
| G31 | `refactor-g31-headers-mcp` | C3 包文档与注释整理（3.5、3.5.1）：`internal/mcpserver/` | G39–G42、G45 | 只改注释 |
| G32 | `refactor-g32-headers-internal` | C3 包文档与注释整理（3.5、3.5.1）：`internal/` 其余目录与 `frontend/` | G39–G42、G45 | 只改注释 |
| G33 | `refactor-g33-headers-scripts` | C3 包文档与注释整理（3.5、3.5.1）：`scripts/`、`.github/` | G21、G39–G42 | 只改注释 |
| G35 | `refactor-g35-tests-sdk` | 测试精简（3.4.1，含 T-g、T-h） | G28 | `sdk/`、`pixivapi`、`fanboxapi` 的测试 |
| G36 | `refactor-g36-tests-mcp` | 测试精简 | G28 | `internal/mcpserver/` 的测试 |
| G37 | `refactor-g37-tests-cli` | 测试精简 | G28 | `internal/cli/`、`cmd/` 的测试 |
| G38 | `refactor-g38-tests-internal` | 测试精简 | G28 | `internal/` 其余目录的测试 |
| G39 | `refactor-g39-audit` | 逐包审查与修正：`sdk/`、`pixivapi`、`fanboxapi`；SDK 公开 API 与 v1.2.1 比对 | G20、G28、G35–G38、G44 | 这些包（修正以小 PR 提交） |
| G40 | `refactor-g40-audit-mcp` | 逐包审查与修正：`internal/mcpserver/`；`tools/list` 与 v1.2.1 比对 | G20、G28、G35–G38、G44 | 同上 |
| G41 | `refactor-g41-audit-cli` | 逐包审查与修正：`internal/cli/`、`cmd/`；CLI 输出与 v1.2.1 比对；产出 changelog 草稿 | G20、G28、G35–G40、G44、G43 | 同上 |
| G42 | `refactor-g42-audit-internal` | 逐包审查与修正：`internal/` 其余目录、`scripts/`；凭据脱敏收归一个类型；发布链路核对 | G20、G28、G35–G38、G44、G43 | 同上 |
| G44 | `refactor-g44-tests-rewrite` | 测试重写（3.4.4）：删除全部测试，按清单与上限重写；更新 `pixiv-cli-test` 与测试布局文档 | G35–G38 | 全仓库的测试、`pixiv-cli-test`、`development.md` 的测试章节 |
| G43 | `refactor-g43-changelog` | 面向用户的 changelog：新规则、`scripts/release` 的 notes 校验、重写全部历史版本 | — | `changelog/`、`scripts/release` 的 notes 部分、`pixiv-cli-release-notes` |
| G45 | `refactor-g45-leftovers` | 遗留问题修正：NDJSON 退出码、`input` 包的全局状态、重复的收藏过滤、mediaproxy 固定参数、D10 字面量 | G39–G44 | `internal/cli/`、`internal/mcpserver/`、`internal/searchfilter` |
| G34 | `refactor-g34-finish` | C2；删除 `.agents/refactor/` | G29–G33、G39–G43、G45 | 文档、skill |

每个 goal 顺带对自己触及的目录做 T2、T3、T6，并对触及的测试套用 3.4。

**每个 goal 的提交要求：**

1. **worktree 与分支：** 开始前确认依赖的 goal 都已合并进 `origin/main`，否则标为阻塞并结束。然后从最新 `origin/main` 创建 `git worktree add -b refactor/<worktree 名称去掉 refactor- 前缀> /Users/flanchan/.codex/worktrees/<worktree 名称>/pixiv-cli origin/main`。
2. **提交：** 按逻辑单元分次 commit；纯移动、改名与逻辑改动不放在同一个 commit 里。commit message 按 `pixiv-cli-commit-message` skill 编写。
3. **验收后提交 PR：** goal 的验收要点全部有证据后，运行一次第 9 节的完整检查，按 `pixiv-cli-pr` skill 编写 PR 描述：使用模板，用 `prmeta` 校验，并列出 changelog 条目、删除或合并的测试及依据、行数与包数变化。push 后向 main 创建 PR。
4. **合并：** 当前 head 的必需检查全部通过，机器人评审意见逐条核实（成立的修复，不成立的回复理由），G06–G09、G10、G16 合并前额外用 `code-review-expert` 审查；然后 squash 合并，标题带 PR 编号。合并后删除远端分支和 worktree。
5. **与其他 goal 的冲突：** 只修改自己负责的目录。阶段 1 中被其他线路调用的导出 API 保持不变；确需修改时，由负责该包的 goal 一并修改调用处。PR 前 rebase 到最新 main；`go.mod`、`go.sum`、许可证文件冲突时采用 main 的版本后重新生成，不手工合并。
6. **无法合并时：** 不强推、不绕过分支保护；在 PR 和汇报中写明阻塞原因并停止。
7. **changelog：** G01–G33 不修改 changelog，只在 PR 描述中写“changelog 条目”；G34 汇总进 PR 描述，作为 release notes 草稿。
8. **spec 与事实不符：** 不修改设计决策。选择不改变对外行为的最保守做法继续，并把假设写进工作记录和 PR 描述；无法在假设下继续的部分标为阻塞。
9. **工作文件：** `goal-N/`、临时探针、基线比对输出不提交，执行前加入 `.git/info/exclude`。除 G34 外不修改 `.agents/refactor/`。
10. **smoke workflow 引用的路径：** PR 上的 Platform smoke 与 Container smoke worker 由 `pr-metadata.yml` 按 main 上的 workflow 定义派发，却检出 PR head 的代码。移动或删除 `platform-smoke.yml`、`container-smoke.yml` 引用的路径时，先合并一个过渡 PR，让对应步骤按检出的代码树中实际存在的路径选择新路径或旧路径（PR #163）；再合并主 PR；最后用一个小 PR 删除回退（PR #164）。已知涉及 G17、G19。

**真实账号测试：** 允许使用本机 Homebrew 安装的 pixiv-cli（v1.2.1）中已登录的 Pixiv 账号运行真实环境测试，规则如下：

| 用途 | 可用账号 |
|---|---|
| G06–G09、G05 的读取测试 | `pikman1`（128042145） |
| G15、G22 的读取测试 | `jjkjk`（128042194） |
| G17 的读取测试 | `josh`（128042265） |
| G17 的写入测试（收藏、关注、评论，测试会自行清理） | `davi`（128042328），以非默认账号身份显式指定 |

- 禁止使用 `Flan`（25649510）。默认账号 `Remi`（127975236）留给用户日常使用，测试不用。
- 用 `PIXIV_E2E_READ_USER_ID` 等测试自带的选择方式指定账号；不直接打开或查询账号数据库，不在日志、PR 或输出中出现 token。
- 只有 G16 改动存储层，它不得用真实账号库运行测试，只能用临时目录中的数据库。
- FANBOX 会话、以图搜图（会上传图片）和浏览器凭据的真实请求仍未授权。
- 遇到 429 不重试、不换账号，记录后停止该测试。

**授权范围：** 已授权：每个 goal 的 commit、push、创建 PR，验收和检查通过后 squash 合并到 main，以及上表的真实账号测试。未授权：发布版本、打 tag、修改分支保护、force push、引入第 4 节以外的依赖。

### A 组：组织结构

| PR | 内容 | 对应示例 | 验收要点 |
|---|---|---|---|
| A1 | `go mod tidy` | — | 只改动 `go.mod`、`go.sum` |
| A2 | 公开类型合一，并一次完成 SDK 全部公开 API 的删除与改名（T6 中 SDK 的部分，见 7.0）：删除 `dto.go`、`resource_dto.go`、`page_dto.go`；`Resource`、`Page` 增加 `MarshalJSON`；`mediaproxy` 缓存改为显式字段；映射层保证非 nil 切片；`record.RecordFrom*` 改名，切片转换并入 `record` | 6.2 | CLI 的 JSON/NDJSON 输出和 MCP 结构化输出逐字节一致；6.2 中的安全测试与缓存往返测试 |
| A3 | Pixiv endpoint 并入 `sdk/pixiv`，重复的映射函数合并为一份（现状：`mapUser` 27 份共 10 种写法，`continuation` 21 份各不相同，`mapImageURLs` 18 份共 3 种写法，`mapArtwork` 10 份共 6 种写法，`mapNovel` 9 份共 4 种写法）。`continuation` 改为“每个端点一条 `Spec`（路径、允许的 query 键）+ 一个解析函数”。可按 artwork、novel、user 分 2–3 次提交 | 6.1 | 35 个 endpoint 测试文件（3718 行）中的每条响应校验断言都保留，改写成按端点族组织的表驱动测试；写法之间的行为差异逐项列在 PR 中并给出取舍（见附录 A） |
| A4 | FANBOX endpoint 并入 `sdk/fanbox` | 6.1 | 同 A3 |
| A5 | MCP tools 每个产品合成一个包：需要重命名的同名声明有 `Register` ×45、`In` ×29、`handle` ×15（Pixiv），以及 `Register` ×9、`In` ×2、`handle` ×9（FANBOX）；`commentProperties`、`validateSeriesID`、`bookmarkCursor` 等重复声明合并为一份；端口类型沿用现有的 `toolhost.SDKPorts`；同步修订 AGENTS.md、`pixiv-cli-mcp-tool` skill、`architecture.md` 和 `mcp-tools.md` 的中英文版本 | 6.3 | `tools/list` 输出与重构前完全一致 |
| A6 | CLI 去重：命令支撑能力收敛为 `cli/internal/command.Env` 一个具体类型（3.4.2 的例子；root `app`、`deps.Data`、`fanbox.Data` 三份合一，函数字段加 nil panic 的转发删除）；各命令的 `Dependencies`、`Deps`、`Data` 依赖袋改为构造参数（3.1.2）；`cli/fanbox.go` 只保留命令组定义；列表入口收敛（`RunPooled*` 4 个变体）；输出辅助函数收敛到 `pixivcmd`，现状为 `printNovels` 7 份完全相同 + 1 份变体，`printArtworks` 8 份共 5 种写法，`printUserPreviews` 4 份共 3 种写法，`writeJSONLine` 3 份相同，`newJSONArraySpool` 3 份共 3 种写法，`textBool` 4 份相同；`bindCommonFlags`、`bindTextValue` 各 4 处 | 6.4 | `--help` 与各命令文本、JSON、NDJSON 输出完全一致；有变体的输出函数只合并输出相同的部分 |
| A7 | `internal/` 目录重组（按 5.3 对照表；在 A2–A6 之前执行），分为 G02、G03、G04 三个 goal：<br>① 撤销 `shared/`、`utils/`、`storage/file/`，`releaseversion` 并入 `version`；`internal/update` 改名为 `selfupdate`；MCP 的 `internal/runtime` 改名为 `toolhost`；<br>② 撤销 `services/`，建立 `pixivapi`、`pixivapi/oauth`、`fanboxapi`、`pixivaccount`、`pixivaccount/pool`、`fanboxaccount`、`session`、`loginrelay`、`reversesearch`；<br>③ 撤销 `cli/commands/` 层，CLI 目录按命令路径重排（含 `mcp` 移到根下），支撑代码移入 `cli/internal/{command,input,pixivcmd,fanboxcmd,resolver}`<br>每个 PR 同步修订：`scripts/build-platform.sh` 中 ldflags 注入的 `internal/shared/buildinfo.Version` 路径（不改会导致版本号静默变回默认值）；`.github/workflows/platform-smoke.yml` 中的 `go test ./internal/cli/commands/pixiv/auth/loginhelper`；AGENTS.md 中引用旧路径的边界条款（`cli/root.go`、`services/reversesearch/assembly`、共享机制归属）；中英文 `architecture.md`（各约 75 处路径）与 `development.md`（各约 38 处）；`pixiv-cli-native`、`pixiv-cli-mcp-tool` skill | 5.2、5.3 | 只有移动、改名和 import 改写，不改逻辑；`--help` 和 `tools/list` 输出完全一致；`go list ./internal/...` 中目录名与包名一致；`pixiv --version` 输出注入的版本号 |
| A8 | 撤销 `e2e/`，测试搬到被测代码旁边，搬家时按 `pixiv-cli-test` 的准入规则精简（清单见下方“A8 的要求”） | — | 保留的真实环境测试在未设置开关变量时全部跳过，`go test ./...` 仍然离线；`TestPixivBinaryPackagedSmoke` 在 platform-smoke 中通过；保留测试的断言与搬家前一致 |


**A8 的要求**（下列名单是按 3.4.1 核实的起点；搬家时对全部文件套用同一标准）：

`e2e/` 现有 15 个文件、5854 行，混合了四类互不相关的测试。按测试对象拆分，搬家与精简在同一个 PR 中完成。

| 去向 | 搬入内容 | 精简 |
|---|---|---|
| `cmd/pixiv/main_test.go` | 编译二进制后执行的测试：`TestPixivBinaryReportsRootVersion`、`TestPixivBinaryPackagedSmoke`、`TestPixivBinaryMCPCancelsOnSignals`、`TestPixivBinaryUnifiedStdioWithoutOwner`、`TestPixivBinaryHTTPOAuthDiscovery` | `isolatedEnv` 与 `isolatedVersionProcessEnv` 两个环境隔离 helper 合并为一个；`TestPixivBinaryHTTPOAuthDiscovery` 只保留进程级装配的断言，`internal/mcpserver/auth/http_test.go` 已覆盖的 discovery 文档字段不再重复断言 |
| `sdk/pixiv/live_test.go`、`live_mutation_test.go`（外部测试包 `pixiv_test`） | Pixiv SDK 真实读取与写入测试；写入测试继续使用 `internal/pixivapi` 原始客户端做独立回读 | 三个打开客户端的函数（`openPixivE2EClient`、`openRealPixivLiveClient`、`openRealPixivMutationSession`）合并为一个；artwork 与 novel 成对的流程（收藏写入、清理、查找目标、评论写入、回复、对账）各合并为一份，以目标描述结构区分；`containsArtworkID` 等 5 个查找函数改用 `slices.ContainsFunc`；自定义 `min` 改用内置函数 |
| `sdk/fanbox/live_test.go`（`fanbox_test`） | FANBOX SDK 真实读取、Cloudflare solver 测试 | — |
| `internal/reversesearch/live_test.go` | 以图搜图真实请求与 MCP solver 会话复用测试 | — |
| `internal/browsercookies/live_test.go` | 本机浏览器 Cookie 读取测试 | — |

**删除：**

| 内容 | 行数 | 依据 |
|---|---|---|
| `ugoira_{corpus,encode,quality,replay,sample}_test.go` 全部 | 1338 | 用于 #147 从 Rust 编码器迁移到 Go 时的对比验证，依赖迁移前编译的冻结基线二进制和私有样本，属于迁移期的特征测试。编码器的长期契约已由 `internal/media/ugoira` 自身的 15 个测试文件和 `testdata/` 参考数据覆盖（GIF/APNG 回放、量化、抖动、Lanczos3、JPEG 解码像素） |
| 只测试 e2e 辅助代码本身的离线测试：环境变量解析（`TestReverseSearchE2EConfig*` ×5、`TestFanboxE2ETargets*`、`TestFanboxPostTarget*`、`TestSelectLiveReadAccount*` ×4）、目标查找与 ID 解析（`TestFirstNovelWithComments*` ×2、`TestParseNovelSeriesRecoveryID*`、`TestFollowFanboxPostPageAllowsTerminalPage`、`TestNativeBrowserNamesRejectInvalidInput`）、评论探测分类（`TestCommentTarget*` ×6、`TestDecodeCommentAccessControl*` ×2、`TestCommentReplyReadbackUsesDedicatedEndpoint`、`TestBookmarkTagReadBackAllowsUntaggedMutation`）、测试进程装配（`TestRegisterRealReverseSearchE2ECleanup*`、`TestStartRealReverseSearchMCPServer*`、`TestIsolatedVersionProcessEnvDisablesAutomaticUpdate`） | 约 500 | 不保护产品契约；被测的辅助函数仍随真实环境测试保留 |
| `TestPixivBinaryRejectsRemovedVersionCommand` | 18 | 只保证已删除的 `version` 子命令不再出现，不是对用户的契约 |

**保留的离线测试**（保护真实账号不被测试本身损坏，符合准入规则中的“凭据与数据完整性”）：`TestSelectPixivMutationAccountRequiresExplicitNonDefault`（写入测试不得使用默认账号）、`TestLiveMutationResultMustStopAfterUncertainWrite`（写入结果不确定时停止）、`TestPixivE2ERotationUsesStoredRevision`（轮换后的凭据必须落库）、`TestArtworkCommentReplyWithCreatedParentUsesWritableTargetAndCleansBothWrites`、`TestReplyCleanupReadTreatsOnlyNotFoundAsAbsent`、`TestLiveCommentMutationKeepsWriteReadNotFoundStrict`、`TestLiveCommentMutationRejectsPostDeletePresence`、`TestLiveCommentMutationUsesSeparatePostDeleteRead`、`TestValidateRequiredCommentMutationEvidenceRequiresEveryPublicMutation`（写入后的清理必须确认完成）。

**同步修改：**
- `release.yml`：删除只离线运行 `./e2e` 的 “SDK E2E contract gate” job 及其下游的 `needs: e2e`。搬家后这些测试由 validate 中的 `go test ./...` 运行。
- `platform-smoke.yml`：`go test ./e2e -run '^TestPixivBinaryPackagedSmoke$'` 改为 `./cmd/pixiv`。
- 中英文 `development.md`：真实环境测试命令中的 `./e2e` 改为新路径；删除 “Offline ugoira parity” 中的采集、对比和 Rust 校准步骤，其中描述生产编码器实现的段落（JPEG 解码器、Lanczos 核、APNG 写入）移到 “Ugoira Go encoder” 一节；关于 release 运行离线 SDK E2E gate 的说明随 job 一起删除；在 “Test file layout” 中登记 `live_test.go`、`live_mutation_test.go` 这一命名例外（按开关变量启用的真实环境测试，没有单一的生产文件归属）。
- `pixiv-cli-test` skill：`TestRealPixivSDKRead` 等测试的位置说明。

预计 `e2e/` 的 5854 行精简为约 3500 行测试，以实际 diff 为准。

### S 组：scripts 与 tools

S1 由 G19 执行；S2 由 G20 执行，2026-10-23 扩大为 scripts 与 CI 的全面精简，以 `goals.md` 中 G20 的语句为准。S2 精简后 `licensebundle` 的生成结果逐字节不变。

| PR | 内容 | 验收要点 |
|---|---|---|
| S1 | 按 5.4 移动、合并和删除。同步修改全部 workflow 中的 `go run`/`go test` 路径、`scripts/test-build-platform.sh` 中的 `linuxabi` 路径、AGENTS.md（`ci/platforms.json`）、`pixiv-cli-ci`、`pixiv-cli-native`、`pixiv-cli-test` skill、中英文 `architecture.md` 与 `development.md`；`development.md` 中 `scripts/internal/*` 的 7 条同包测试例外删除（`package main` 的测试必然同包） | 各 workflow 引用的路径都存在；`go test ./scripts/...` 通过；在一次 release 全部发布成功之后合并（见第 10 节） |
| S2 | 精简：`licensebundle` 删除暂存、备份、回滚的发布逻辑和 `bundleFileOps` 注入点，改为删除旧目录后直接写入，`--check` 生成到临时目录再比对；`licensebundle --check` 从 7 处减为 2 处（`ci.yml` 的 Quality、`release.yml` 的 `build`）；测试按下方规则精简 | `licensebundle` 生成结果与精简前逐字节一致；PR 中逐项列出删除的测试及依据 |

**S2 的测试规则**（3.4.1 在 scripts 中的具体应用）：
- 只断言 YAML、Dockerfile 文本的测试删除。workflow 实际运行和 container-smoke 构建运行镜像才是这些配置的证据。
- 执行真实代码的保留：`installers`（真实执行安装脚本）、`changescope_test.go`（真实执行分类脚本）、`dispatch_prefilter_test.go`（真实调用 `prmeta`）。
- 供应链与凭据边界保留：`workflowpins`（action 必须固定完整 SHA）；`containerrelease` 中的固定 base digest、非 root 运行、不内嵌密钥或状态、只使用受信 release 产物，共 4 个。
- 防历史回退的测试删除，例如 `TestReleaseNoLongerPublishesHomebrewInline`、`TestDockerfileExists`。
- 各工具自身的单元测试只保留“出错会悄悄产出错误发布物”的部分：校验和、归档命名、formula 版本、release 正文渲染。

### T 组：测试与测试注入点

按 3.4 的标准处理全仓库。全仓库测试 61k 行，生产代码 68.8k 行。下表是已核实的起点和规模，不是完整清单。

| PR | 范围 | 已核实的起点 |
|---|---|---|
| T1 | `internal/config/settings` 的测试 | 测试 3012 行，生产代码 1794 行。`tags_red_test.go`、`baseline_test.go`、`contract_test.go` 中，同一契约各被断言 2–3 次（3.4.1 的第一个例子）；这三个文件和 `advanced_test.go` 都没有对应的生产文件 |
| T2 | 只为测试存在的导出符号（P-a、P-b） | 49 个导出函数只被测试引用，其中 `loginhelper` 有 16 个 `SetXxx`（3.4.2 的例子），`auth` 有 2 个；其余多是生产代码已不再使用的旧入口 |
| T3 | 没有对应生产文件的测试 | 35 个文件、10220 行，主要在 `mcpserver/pixiv`（测试 7693 行，生产代码 132 行） |
| T4 | 悬空的编号引用（P-h） | 22 个文件中共 96 处，主要在 `.github/workflows`、`scripts/tests`、`tools/` |
| T5 | A2–A6 只移动、没有重写的全部包（例如 `media/*`、`browsercookies/*`、`selfupdate/*`、`storage/*`、`lifecycle`、`pagination`、`record`、`searchfilter`、`httpclient`、`mcpserver/{auth,pixiv/accounts,pixiv/gallery,pixiv/mediaproxy}`、`loginrelay`、`reversesearch/*`、`cli/auth`），按 3.4.2 全部标准检查生产代码，按 3.4.1 检查测试 | 未逐一检查；执行时按目录拆成多个 PR |
| T6 | 兼容层（P-l、D13） | 3.4.2 的 P-l 判定参照表列出了已确认的位置；生产代码中带“兼容”“旧”“历史”“legacy”字样的注释约 150 处，按 P-l 逐一判定 |

各 T 项由哪个 goal 执行，见 7.0。

### B 组：依赖替换

| PR | 内容 | 对应示例 | 必须保持的行为 |
|---|---|---|---|
| B1 | Koanf 换成 go-toml/v2；入口收敛为 Store 方法；引入 `settings.Key` 常量与 `LogLevel`、`LogFormat` 等具名枚举类型，替换现有的别名和取值字面量 | 6.5 | 来源优先级、显式空值、脱敏、稀疏写回、校验时机 |
| B2 | resty 换成 net/http | 6.6 | 请求头（User-Agent 已由 `protocol.AppHeaders` 显式设置，不依赖 resty 默认值）、form 请求的 `Content-Type: application/x-www-form-urlencoded`（resty 自动添加，需手动设置）、`Failure` 映射、Retry-After、认证重试 |
| B3 | survey 换成 x/term | 6.7 | 密码不回显、默认值、非 TTY 时的报错（`in`、`out` 都必须是终端）、提示写到 `out`；Select 从方向键选择改为编号选择，属于可见的交互变化，写入 changelog |
| B4 | 存储层换成 GORM + gormlite | 6.8 | 见下方说明 |

**B4 的要求：**

- **不变的部分：** 数据库路径、`application_id`、单数表名、UID 主键、Unix 时间；SQL migration、ledger、checksum，以及遇到未知版本时拒绝；`_pragma` 设置、单连接、文件权限、侧车锁和事务回滚。
- **禁止使用：** `AutoMigrate`、`gorm.Model`、软删除、隐式时间戳。
- **驱动名：** ncruces 注册的驱动名是 `"sqlite3"`。使用 modernc 驱动名 `"sqlite"` 的 5 个文件一起修改：`storage/database/database.go`，以及 `browsercookies/{chromium,firefox,sqliteio}` 和 `services/pixiv/account/reads_watch` 的测试。
- **单连接：** 连接池上限为 1，任何未关闭的 `Rows`、`QueryRow` 未 `Scan` 都会造成死锁（原型验证中复现过）；review 时逐处检查。
- **共享依赖升级：** 引入后 `golang.org/x/sys` 由 v0.41 升到 v0.48，`golang.org/x/text` 由 v0.32 升到 v0.42，影响 tls-client 等现有依赖，需要完整回归。
- **许可证：** 运行 `licensebundle` 重新生成 `third_party/licenses/` 与 `THIRD_PARTY_LICENSES.md`。`go-sqlite3-wasm` 根目录是 MIT-0，并声明翻译代码保留原作者版权与原许可证，需人工确认这些声明被完整收录。

### C 组：新功能

| PR | 内容 | 对应示例 |
|---|---|---|
| C1 | slog 日志：用 `internal/logging` 取代 `diagnostics`，涉及 11 个文件中的 27 处 `diagnostics.Emit`/`Event` 和 2 处 scope 安装（`cli/execution.go`、`composition.go`）；删除 Event/Sink/Scope 与 CLI 渲染器；SDK 两个 `Options` 增加 `Logger`；请求与内部决策的记录放进 `pixivapi`、`fanboxapi` 的发送与重试处，删除 `diagnosticRoundTripper` 和 `fanboxapi` 的 `diagnostics.Emit`；公开方法入口登记 `sdk_operation`；CLI 与 MCP 执行边界接入。沿用现有配置键 `log_level`（`logging.level`，`PIXIV_LOG_LEVEL`）和 `log_format`（`logging.format`，`PIXIV_LOG_FORMAT`）；`LogLevel` 在现有 `info`、`debug` 之外新增 `warn`、`error`；stderr 诊断输出的格式变为 6.9 的文本/JSON 格式，属于对外可见的变化，写入 changelog。按 AGENTS.md 执行 Red/Green | 6.5、6.9 |
| C2 | 删除 `.agents/refactor/`；`docs/{en,zh}/sdk.md` 改为描述新 API（不写迁移对照）；汇总 G01–G33 PR 描述中的 changelog 条目，作为下个版本 release notes 的草稿写进本阶段 PR 描述（`changelog/vX.Y.Z/` 只在单独授权的发布准备中创建）；通用工程规则（命名与放置、不做内部兼容、测试删除标准、按改动范围验证、注释规则）已在 PR #159 写入 `pixiv-cli-develop`、`pixiv-cli-test`、`pixiv-cli-code-commenting`、`pixiv-cli-review`；C2 只补充与本次重构结果相关的内容：新目录对应的放置路径、D7 的协议包位置、日志归属（6.9），并把第 6 节的示例整理到 `references/go-examples.md` | — |
| C3 | 按 3.5、3.5.1 整理注释：每个包一处标准库风格的包文档，删除其他文件的头注释，补齐导出符号的文档注释，统一注释语言（规则写在 `pixiv-cli-code-commenting`） | 3.5 |

**C1 的日志约束：**
- CLI 的数据 stdout 和 MCP stdio 的 stdout 中不得出现日志。
- 不记录 token、Cookie、响应体或签名 URL。

文档跟随各个 PR 的实际改动同步更新。

## 8. 非目标

以下事项另行立项，不在本次范围内：

- 配置一次性完整加载与提前校验
- 账号删除与默认 UID 的提交顺序，以及相应的错误类型
- README 的整体重写
- 移除 tls-client
- PR 治理体系（`/test` 验证、PR 模板校验、无效 PR 自动关闭等）的去留：本次只移动目录
- `pixiv auth` 登录链路（含 loginhelper、远程登录接力、SSH 隧道，生产代码 4593 行）的功能取舍；本次只处理 T2 的测试注入点

## 9. 验证

验证按改动范围选择，不以测试数量为目标。测试只是证据之一；编译、输出比对和阅读 diff 同样是证据，很多改动用后者更直接。

**改动过程中：** 只跑受影响的部分，不跑全量。

| 改动 | 做什么 |
|---|---|
| 任何 Go 改动 | `go build ./...`（移动、改名、改签名时编译就是主要证据） |
| 改了某个包的逻辑 | 只跑该包及其直接调用方的测试，例如 `go test ./internal/pixivapi ./sdk/pixiv -count=1`；需要时用 `-run` 只跑相关用例 |
| 纯移动、改名、import 改写 | 编译通过即可，不跑测试 |
| 影响 CLI/MCP 对外输出 | 只采集受影响命令的输出或 `tools/list` 做比对 |
| 只改注释、文档 | `gofmt -l`、`git diff --check` |

同一处改动的测试通过后，除非又有新改动或失败，不重跑、不扩大范围。

**提交 PR 前：** 每个阶段只跑一次完整检查，作为合并证据。

```bash
go test ./... -count=1
go vet ./...
sh scripts/build.sh
go run ./scripts/cmd/licensebundle --check   # S1 之后为 ./scripts/licensebundle；只在依赖变化的阶段运行
git diff --check
```

`go test -race ./...` 只在涉及并发、账号生命周期、持久化或共享运行时的 goal 运行：G05–G10、G15、G16、G22，以及动到 `session`、`pixivaccount/pool`、`storage/*`、`media/downloader` 的 T5。

**各阶段的附加验证：**

| 适用范围 | 验证内容 |
|---|---|
| G01–G16 | 每个 goal 开始前，在当时的 `origin/main` 上采集递归的 `pixiv --help` 输出和 MCP `tools/list` 的 JSON，存到不提交的工作目录；goal 结束时重新采集并逐项比对，只允许出现该 goal 明确列出的变化 |
| G05–G09 | 公开 API 只出现预期中的变化：goal 开始前在 `origin/main` 上运行 `go doc -all ./sdk ./sdk/pixiv ./sdk/fanbox` 生成基线，goal 结束时比对；G05 在 PR 中列出全部变化，G06–G09 不应出现变化 |
| B1–B4 | 记录 `go version -m` 显示的链接模块数和二进制体积；用 `go mod why -m` 确认被移除的模块已不在依赖图中 |
| B4 | 用 v1.2.x 生成的数据库文件，验证可以直接打开和迁移；覆盖 revision 冲突、事务回滚，以及 false/0/空串/NULL 的写入；Windows 路径在 CI 中实际打开；在现有六平台 CI 矩阵上完成交叉编译和 smoke 测试 |
| A8 | 未设置开关变量时运行 `go test ./cmd/pixiv ./sdk/... ./internal/reversesearch ./internal/browsercookies -count=1 -v`，确认真实环境测试全部为 SKIP、其余通过；PR 中逐项列出删除的测试及依据 |
| A3、A5、B4 | 提交前用 `code-review-expert` 审查 |

每个阶段 PR 的说明需分别列出生产代码、测试、文档的行数变化，以及包数变化。

## 10. 风险

| 风险 | 应对 |
|---|---|
| B4 更换驱动影响已有用户的数据库 | 合并前必须用旧版本生成的数据库通过兼容测试 |
| B4 单连接下遗漏关闭 `Rows` 导致死锁 | review 逐处检查；原型中已复现过一次 |
| B4 升级 `x/sys`、`x/text` 影响现有依赖 | 完整回归，并在六平台 CI 上运行 |
| A2 合并类型后 JSON 输出出现差异，或泄露运行时 URL | 逐字节比对输出；`Resource.MarshalJSON` 必须有安全测试，`mediaproxy` 缓存必须有往返测试 |
| A3 合并有差异的映射函数时改变了某个端点的行为 | 差异逐项列在 PR 中；用响应 fixture 做前后输出比对 |
| S1 移动工具路径后，重新发布或恢复发布在移动之前打的 tag 会失败：发布 workflow 使用默认分支上的定义，却在 release 当时的 checkout 中执行 `go run ./tools/release` 等旧路径 | 在一次 release 全部发布成功之后再合并 S1；进行中的 PR 需要 rebase |
| A7 移动 `buildinfo` 后 ldflags 路径失效，版本号静默变回默认值 | 同一 PR 修改 `scripts/build-platform.sh`，并验证 `pixiv --version` |
| B2 中 resty 的默认行为被遗漏 | 改动前后抓取请求头进行比对；form 请求显式设置 `Content-Type` |
| SDK 的破坏性变更发布在 v1 次版本中，执行 `go get -u` 的 SDK 使用方会遇到编译错误 | changelog 列出；SDK 使用方通常固定版本，不提供兼容过渡（D13）。版本号和发布时间需单独授权 |

## 附录 A：可行性核实（2026-10-22）

以下结论来自对当前 `main`（`ef4accb9`）的只读检查，以及在仓库外构建的临时原型；没有修改仓库。

| 编号 | 核实项 | 结论 | 对 spec 的影响 |
|---|---|---|---|
| F1 | DTO 与模型是否逐字段对应 | 是，例外见 6.2；转换会把 nil 切片转成空切片 | A2 改为在 `Resource`、`Page` 上实现 `MarshalJSON`，映射层保证非 nil |
| F2 | 有没有代码依赖带 URL 的 `Resource` JSON | 只有 `mediaproxy` 的进程内缓存 | 缓存改为显式字段，并补往返测试 |
| F3 | 协议包是否只被 SDK 使用 | 生产代码是；`e2e` 的真实环境测试、`account/login_test.go` 也在用 | D7 改为模块级 `internal/pixivapi`、`internal/fanboxapi` |
| F4 | endpoint 中重复的函数是否逐字相同 | 否：`mapUser` 10 种写法，`mapArtwork` 6 种，`continuation` 21 种（端点参数不同，部分版本没有正数校验和 nil 处理） | A3 要求列出差异并逐项给出取舍 |
| F5 | 同族端点的响应结构是否一致 | `user_previews` 6 个端点一致（blocked 兼容 `users`），`illusts` 6 个、`novels` 7 个、`comments` 2 个端点各自一致 | 6.1 的“同族共用函数”成立 |
| F6 | MCP tools 合包时的命名冲突 | `Register` ×54、`In` ×31、`handle` ×24；tool 测试只有 7 个文件 | A5 可行，工作量是机械重命名 |
| F7 | “download 与 recommended 参数解析重复” | 不成立；实际的重复是输出与 flag 辅助函数 | A6 改写 |
| F8 | 两个账号包能否合并 | 不能：17 个同名导出，错误文案不同 | 改为 `pixivaccount`、`fanboxaccount` |
| F9 | 协议包之间能否合并 | `appapi`+`protocol`+`resource` 可以；`oauth` 与 `appapi` 有 7 个同名导出 | `pixivapi` + 子包 `pixivapi/oauth` |
| F10 | CI 是否依赖 `internal/` 路径 | 变更分类器不依赖；`build-platform.sh` 的 ldflags 与 `platform-smoke.yml` 的测试路径依赖 | 写入 A7 |
| F11 | 配置绑定能否支持具名 string 类型 | 能：赋值使用 `Convert(fieldType)` | D10 的 `LogLevel` 等类型可行 |
| F12 | 代码中硬编码的配置别名 | 只有 `web_fallback_enabled`、`account_pool_accounts` 和 `values.go` 的 switch 分支 | `keys.go` 只定义被代码引用的键 |
| F13 | 账号池配置能否改为构造时传值 | 不能：MCP 长时间运行时依赖每次重新读取 | 保留 `loadPoolConfig` |
| F14 | `Client.opts` 在构造后的用途 | 只有 `ResourcePolicy.AllowedHosts`（资源下载） | 移入 `pixivapi` |
| F15 | resty 的隐式行为 | User-Agent 已显式设置；form 请求的 `Content-Type` 由 resty 自动添加 | B2 需手动设置 |
| F16 | survey 的流与中断处理 | 提示写到 `out`；没有代码对 Ctrl-C 做特殊处理 | B3 可直接替换；Select 交互方式会变化 |
| F17 | gormlite v0.34 + go-sqlite3 v0.35.6 + gorm v1.31.2 | `CGO_ENABLED=0` 下编译运行通过：4 个 pragma 生效，busy_timeout 为 0（与现状一致，DSN 带 `_pragma` 时 ncruces 不设默认值），零值和 NULL 写入、revision 条件更新、事务回滚、CHECK 约束错误都符合预期；windows/amd64、windows/arm64、linux/arm64、darwin/amd64 交叉编译通过 | B4 可行；`x/sys`、`x/text` 会升级 |
| F18 | 依赖 modernc 驱动名 `"sqlite"` 的代码 | 5 个文件 | 写入 B4 |
| F19 | 现有诊断系统的规模 | 11 个文件中有 27 处 `Emit`/`Event`，其中 SDK 侧 18 处（`sdk/pixiv/pixiv.go` 4 处、FANBOX 协议 14 处）；SDK 已有 `diagnosticRoundTripper`；重试、刷新、Retry-After 决策集中在 `appapi` 的 `*WithRetry` 函数中 | C1 把请求与决策记录放进 `pixivapi`、`fanboxapi` 的发送与重试处 |
| F20 | 公开 API 清单 | `scripts/cmd/publicapi` 存在，但没有 golden 文件，CI 也没有调用 | 删除该工具（G19），基线改用 `go doc -all` |
| F21 | 模块路径改为 `/v2` 的影响 | 已发布的 v1.x 更新器无法通过 `go install` 升级到 v2 | 不升 `/v2`（D1），更新器与安装路径保持不变 |

| F22 | `e2e/` 的组成与能否拆分 | 15 个文件、5854 行：二进制测试 545 行、SDK 真实环境测试 3444 行、以图搜图 422 行、ugoira 迁移对比 1338 行、浏览器 105 行；约 730 行是测试 e2e 辅助代码本身的离线测试；ugoira 对比测试依赖迁移前的冻结基线，生产编码器已有自身的参考数据测试 | A8 |

| F23 | `scripts/`、`tools/` 工具的调用方 | `publicapi` 没有调用方；`browsernativeevidence`、`homebrew-prepublish-verify` 只由很少运行的手动 workflow 使用；发布 workflow 会 checkout release 当时的 commit，在其中运行工具 | 5.4、G19；第 10 节风险 |
| F24 | 测试与测试注入点 | 49 个导出函数只被测试引用；35 个测试文件没有对应的生产文件；`config/settings` 同一断言重复 2–3 次；96 处编号引用指向不存在的文档 | T 组（G18、G23–G28） |

**未核实（实施时确认）：**
- `go-sqlite3-wasm` 中“保留原许可证”的具体条目能否被 `licensebundle` 完整收录。
- Windows 上 `file:C:/…` 形式的 DSN 能否实际打开数据库（交叉编译已通过，但没有在 Windows 上运行）。
- A3 中各映射函数写法之间的具体字段差异（只确认了差异存在，还没有逐一列出）。
