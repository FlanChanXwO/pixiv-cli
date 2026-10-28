# 编码手册

[English](../../en/maintainers/coding-standards.md) | 简体中文 | [文档索引](../../index-zh-CN.md)

本手册是 pixiv-cli Go 代码写法和审查规则的唯一出处。维护 skill 只说明流程，规则一律回到这里；[AGENTS.md](../../../AGENTS.md) 负责任务路由与硬约束。结构归属见[架构说明](architecture.md)，工具链、测试布局与发布步骤见[开发流程](development.md)。

## 风格基线

- 以 [Google Go Style Guide](https://google.github.io/styleguide/go/)（Guide、Style Decisions、Best Practices）为基础。与本手册冲突时，以本手册为准。
- 使用 `go.mod` 声明的 Go 版本，用 `gofmt` 格式化。
- 代码要让只有 Go 基础的工程师读懂：清楚优先于简短，不炫技，不为未来需求预留抽象。

## 命名与术语

包、文件、类型、函数和字段按它在领域里是什么、负责什么命名，不按它在代码结构中的位置命名。`internal/` 中的名字可以随时修改；SDK 公开名一旦改名就是破坏性变更，需要写入 changelog。

| 规则 | 不这样写 | 这样写 |
| --- | --- | --- |
| 不用空泛的名字 | `Deps`、`Dependencies`、`Data`、`Info`、`Facade`、`Manager`、`Helper`、`Util`、`Common`、`Base`、`Impl`、`Wrapper` | `pixivcmd.Runner`、`command.Env`、`session.Pixiv`；`Manager` 按实际职责改名 |
| 不建依赖袋 | 每个命令一个 `Dependencies` 结构体，由组合根填满再向下传 | `download.New(env command.Env, session *session.Pixiv)`；依赖过多的 owner 先拆职责 |
| 不用“函数字段 + 同名转发方法”，函数字段不加 `Fn` 后缀 | `PromptSecretFn func(string) (string, error)` 加 `func (d Data) PromptSecret` | `func (e Env) PromptSecret(message string) (string, error)`；需要替换的行为用具体类型或窄接口 |
| 不重复包名已经给出的上下文 | `session.PixivSession`、`filename.FilenameData` | `session.Pixiv`、`filename.Fields` |
| 同一种能力只用一个名字 | `UsageError` 与 `Usage`、`PrintJSON` 与 `WriteJSON` 并存 | 统一为 `command.Env` 上的一个名字 |

以下补充规则只约束新增代码（2026-10-26 起）。现有名字不为此改名，SDK 公开名也不再为此破坏一次。

- 新类型和字段不用 `Kind`、`Type`（作为万能区分字段时）、`Item`、`Object`、`Misc` 做名字或后缀，改用说明具体角色的词：`Phase`、`Mode`、`Source`、`Status`、`Registry`、`Store`。上游协议本身的字段名，例如 Pixiv 的 `type`，在映射层保持原样。
- 不用缩写：`Cfg`、`Mgr`、`Svc`、`Msg`、`Req`、`Resp`、`Tmp`、`Val` 都写完整单词。只在几行内使用的局部变量（`ctx`、`err`、`ok`、`i`、`n`、`req`）和一两个字母的方法接收者按 Go 惯例使用短名。`ID`、`URL`、`HTTP` 等首字母缩写词保持全大写。
- 包名用简短的小写名词，不用 `util`、`common`、`base`、`types`。目录名与包名一致。避开与公开 SDK 包、常用标准库包（`runtime`、`log`、`context`）或同一调用方已导入的包重名；需要 import 别名就说明命名有问题。

## 声明放置与文件命名

| 声明的使用范围 | 放置位置 | 示例 |
| --- | --- | --- |
| 包内多个文件共用 | 按主题命名的声明文件 | `sdk/pixiv/models.go`（公开实体）、`sdk/pixiv/artwork_wire.go`（作品响应结构）、`sdk/pixiv/response.go`（多个领域共用的响应结构）、`internal/storage/database/rows.go`（行结构）、`errors.go`（错误码） |
| 只服务一个行为 | 和使用它的函数放在同一个文件，写在函数前面 | `internal/mcpserver/pixiv/tools/users.go` 中的 `userArtworksIn`；`internal/cli/internal/pixivcmd/list.go` 中的 `ArtworkList` |
| 类型自身的方法（`String`、`Valid`、`MarshalJSON` 等） | 和类型放在同一个文件 | `sdk/resource.go` 中的 `Resource.MarshalJSON` |

- 不按“一个类型一个文件”拆分。
- 不建立 `types.go`、`consts.go` 这类没有主题的汇总文件。

| 情形 | 规则 | 示例 |
| --- | --- | --- |
| 单文件包 | 使用 `包名/包名.go` | `internal/cli/ranking/ranking.go` |
| 多文件包的入口 | `包名.go` 存放主入口：主类型、构造函数或 cobra 命令定义；其余文件按主题命名 | `internal/cli/search/search.go` 定义 `pixiv search`，`filters.go` 存放筛选逻辑 |
| 只有包注释的文件 | 改名为 `doc.go`，或把包注释合并到入口文件 | — |
| 入口文件已经包含多个主题 | 按主题拆分，`包名.go` 只保留入口 | — |
| 其他文件 | 文件名说明内容，不用 `support.go`、`deps.go`、`helpers.go`、`util.go`、`common.go`、`types.go` | `internal/cli/auth/` 下的文件按职责命名 |

`包名.go` 的含义是“从这里开始读这个包”，不用作杂物文件。

新代码按下面顺序决定位置，第一条成立就停下：只服务一个命令，放进该命令目录；被同一产品的命令共用，或所有命令共用，放进对应的共享 CLI 包；MCP tool 放进该产品的 `tools` 包；公开能力放进 `sdk/<产品>/` 的领域文件，上游协议细节放进 `internal/pixivapi` 或 `internal/fanboxapi`；以上都不是，新建一个按职责命名的包。放进去之后，所在包、文件或类型的名字如果不再能概括全部内容，就是放错了位置。不建立 `common`、`shared`、`util`、`misc` 这类位置。

## 命名常量

具有业务含义的标识符使用具名常量，不在代码中直接写字符串字面量；有取值集合的标识符使用具名类型，让编译器检查类型。

| 类别 | 形式 | 放置位置 |
| --- | --- | --- |
| 配置键（别名） | `settings.Key` 常量，例如 `settings.KeyLogLevel` | `internal/config/settings/keys.go` |
| 枚举取值（日志级别、输出格式、restrict、作品类型等） | 具名类型 + 常量，例如 `settings.LogLevelDebug`、`pixiv.RestrictPublic` | 与该类型放在同一文件 |
| 上游 API 路径、query/form 参数名、请求头名 | 导出常量，例如 `pixivapi.PathUserFollower`、`pixivapi.ParamUserID`；包位于 `internal/` 下，模块外部无法引用 | `internal/pixivapi/api.go`、`internal/fanboxapi/api.go` |
| MCP tool 名称 | 未导出常量，例如 `toolUserDetail` | 与该 tool 的注册代码放在同一文件 |
| Go 代码中引用的数据库列名 | 未导出常量，例如 `colCredentialRevision` | `internal/storage/database/rows.go`，紧挨行结构 |
| JSON 输出键（struct tag 以外），例如 `illusts` | 未导出常量 | 与使用它的输出代码放在同一文件 |
| 环境变量名 | 仍由 `env` 标签声明；代码中需要单独引用时定义常量 | `keys.go` |

长度、上限、超时、间隔、退出码、文件名、地址和服务名等有含义的值同样写成命名常量。契约规定的值放在负责执行它的包里。同一个值只定义一次，别处引用常量或从它推导（例如字符串形式用 `strconv.Itoa` 生成）。没有业务含义的 `0`、`1` 等值，以及标准库契约要求的参数（如 `net.Dial("tcp", ...)`）不用命名。这条规则约束新增和触及的代码，现有字面量的统一清理属于独立工作。

可以直接写字面量的场景：

- struct tag：Go 语法要求它必须是字面量。
- 日志消息、错误文本、格式化串。
- SQL migration 文件，以及 `Raw` 查询中的 SQL 正文。
- 测试中的输入数据。测试断言标识符时，仍然引用生产代码的常量。

配置键：配置键的声明来源仍然是 `RuntimeConfig` 字段上的 `alias` 标签，`keys.go` 中的常量必须与之一一对应。由一个表驱动测试遍历全部 `Key` 常量，确认每个都能通过 `SettingSpecByAlias` 解析；常量拼错时，程序运行时会找不到配置项。程序内部通过 `RuntimeConfig` 的字段读取配置（例如 `cfg.LogLevel`），不经过别名字符串。按别名读写的 `Store.Get/Set/Unset` 只用于 `config get/set/unset` 命令，别名来自用户输入。

```go
// internal/pixivapi/api.go

// App API 路径。
const (
	PathUserFollower  = "/v1/user/follower"
	PathUserFollowing = "/v1/user/following"
)

// 查询与表单参数名。
const (
	ParamUserID   = "user_id"
	ParamRestrict = "restrict"
)
```

```go
// sdk/pixiv/models.go

// Restrict is the visibility scope of a bookmark or following list.
type Restrict string

const (
	RestrictPublic  Restrict = "public"
	RestrictPrivate Restrict = "private"
)
```

## 只在边界检查、不过度防御

判断一个结构是否必要，问三个问题：去掉它，哪个调用方需要多写代码？它防止了什么具体错误？读者要多跳几处才能看懂一次调用？前两问答不出、第三问的答案大于零时，按下表处理。

| 编号 | 标准 | 处理 |
| --- | --- | --- |
| P-a | 导出符号只被测试引用（`sdk/` 的公开 API 除外） | 改为未导出，或删除后让测试走生产代码实际使用的入口 |
| P-b | 包级可变变量，由导出的 `SetXxx` 在测试中替换 | 改为结构体上的未导出字段，由同包测试构造 |
| P-c | 只转发到默认实例的包级函数，以及 `At`、`WithX`、`WithFileStore` 一类入口变体 | 删除，调用方直接使用唯一入口 |
| P-d | 同一接口或映射函数在多个包中各写一份；同一套方法或机制在多个类型上各写一份，例如三个带凭据的账号结构体各自实现 `String`、`GoString`、`Format` 来脱敏 | 合并为一份，并把机制收归到它保护或描述的那个字段类型、值类型上，例如凭据字段使用一个自我脱敏的类型，外层类型不再各写一遍 |
| P-e | 平行的类型层，例如 DTO 与模型 | 合并为一种类型 |
| P-f | 自制的标准库或已有依赖的替代品，例如自制诊断系统对应 `slog`、`FirstNonEmpty` 对应 `cmp.Or` | 改用标准库或已有依赖 |
| P-g | 没有任何生产调用方的代码或工具 | 删除 |
| P-h | 注释引用仓库中不存在的文档编号 | 改为直接写出原因，或删除 |
| P-i | 中转层：只在结构相同的类型之间转换，或原样转发调用 | 删除中转层，两侧共用一个类型或直接调用 |
| P-j | 碎片化：一个概念被拆进多个必须一起读才能看懂的小包或小文件 | 合并为一个包或文件 |
| P-k | 为假想扩展预留的间接层：同一层内照抄某个具体类型方法集的接口、每个包各声明一份的相同接口、只注册一项的注册表、没有第二种取值的选项 | 去掉间接层，直接使用具体类型。测试替身不算第二种生产实现；通过 `httptest` 或临时目录测试真实边界。由使用方定义、只含其所需方法、用来切断存储/网络/系统依赖的窄端口（例如 `session` 的 `AccountService`、MCP `toolhost.SDKPorts`）是真实边界，保留 |
| P-l | 兼容层：旧入口与新入口并存、旧名别名、source-compatible 包装、为旧版本自身组件保留的分支、已删除配置键的墓碑、以往改名遗留的输入别名 | 删除，只保留新入口。上游（Pixiv/FANBOX API、浏览器 Cookie）格式本身的差异不是兼容层；让用户已有的数据库、配置文件仍能打开的读取逻辑保留 |
| P-m | 过度防御：对内部调用方已经保证的前提重复校验；对构造时一定会设置的字段或依赖判 nil；Go 本身已经安全的判断（读 nil map、对 nil 切片取 `len` 或遍历）；遇到不可能出现的状态时悄悄降级、返回零值或吞掉错误；没有共享修改风险的防御性拷贝；“以防万一”的 `recover` | 删除，校验只留在最外层的那一次。确需防住的内部不变量，让它立即失败（返回明确错误或 panic），不伪装成成功 |

这些标准只调整结构，不删除用户可见的功能。功能范围由对应功能负责人决定。

校验留在信任边界：用户输入、CLI/MCP 参数、上游响应、文件与数据库内容、环境变量。公开 SDK 入口的参数校验、定义所有权的凭据副本（例如 `RefreshTokenCopy`），以及并发与取消相关的检查同样保留。

不做内部兼容：新入口取代旧入口时，在同一次改动里删除旧入口。SDK 公开 API 的破坏性变更写入 changelog，不提供 `Deprecated` 过渡。

可选参数通过 request/spec 结构体或方法表达，不再新增 `WithX` 一类入口变体。

限额、超时、重试、截断和降级只在有明确需求、平台约束、既有契约或可复现故障时添加。写明触发条件，用最小证据覆盖，失败时如实报告，不静默丢弃有效数据。尚未支持的能力直接写成不支持。

```go
// sdk/resource.go: JSON encoding omits URL, request headers, and expiry metadata.

// MarshalJSON emits only the opaque reference and the credential requirement,
// never URL, request headers, or expiry metadata.
func (r Resource) MarshalJSON() ([]byte, error) {
	if r.Ref.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(struct {
		Ref                 ResourceRef `json:"ref"`
		RequiresCredentials bool        `json:"requires_credentials,omitempty"`
	}{r.Ref, r.RequiresCredentials})
}
```

```go
// internal/cli/internal/command/env.go：Env 由组合根构造，WrapUsage 一定存在；
// 判 nil 只会在装配写错时成立，却会把用法错误的退出码悄悄丢掉。
func (e Env) UsageError(err error) error { return e.WrapUsage(err) }
```

## 程序输出

日志、错误字符串、CLI 输出和 MCP 结果文本一律用英文，与注释语言无关。机器输出通道、HTTP 授权与如实返回结构化错误属于 [AGENTS.md](../../../AGENTS.md) 的硬约束。

## 包文档与注释

注释只补充名字与结构表达不出来的信息：调用约定、意图、限制、不变量、所有权和有意义的流程阶段。共享契约保留在权威位置；其他调用方需要时链接过去，不重复长篇说明。

### 包文档

每个包只在一个文件里写包文档，通常是 `doc.go` 或入口文件 `包名/包名.go`，紧贴 `package` 那一行。写法参照 Go 标准库：

- 第一句以 `Package x` 开头，说清这个包做什么。`main` 包以命令名开头，例如 `pixiv 命令启动 pixiv-cli。`
- 空一行（`//`）后写使用范围、不负责什么，以及相关内容在哪里。不写维护者、创建日期这类元信息。
- 有多个文件或多个主要类型的包，在包文档里加一段“包的组成”，用文档链接（`[Client]`、`[Session]`）列出主要类型和入口。godoc 可以直接跳转，改名时 gopls 会提示链接失效。
- 同一个包的其他文件开头不写注释。紧贴 `package` 的注释会被并进包文档，与 `package` 隔一行的文件说明又容易随重构过期。需要靠注释才说得清的文件，应该改名或调整拆分。

指令与特殊文件：

- `//go:build` 放在第一行，空一行后再写包文档或 `package`。cgo 注释、`//go:embed`、`//go:generate` 保留原来的位置。
- 生成的文件保留生成器写的头部。
- 保留许可证声明与生成文件来源；生成输出需要变化时修改生成器，不手工改生成结果。
- `_test.go`、不支持注释的格式、`testdata/`、`third_party/` 不加文件说明。
- Shell、PowerShell、`.cmd` 脚本在 shebang 之后用一行说明用途；workflow YAML 在 `name:` 之前；`frontend` 的 HTML/JS/CSS 在文件顶部。

```go
// Package pixiv provides a Go client for the Pixiv App API: artworks, novels, users, bookmarks, and comments.
//
// [Client] carries every endpoint and is built by [New]. Upstream protocol details live in internal/pixivapi.
package pixiv
```

```go
//go:build windows

package urlscheme
```

```sh
#!/bin/sh
# 为单个目标平台构建并可选打包 pixiv；发布身份与打包规则由 scripts/release 与 package.sh 持有。
```

### 注释语言

- `sdk/` 的注释（包括文档注释）用英文，它们会显示在 pkg.go.dev 上。
- `cmd/`、`internal/`、`scripts/`、`.github/`、`frontend/` 的注释用简体中文。触及到语言不对的注释时改写成规定的语言；未触及代码的批量翻译属于独立工作。
- 协议字段、标识符、错误原文保留英文，不在中文句子里夹杂可翻译的英文术语。

### 文档注释

导出的类型、函数、方法、常量和变量都写文档注释，`internal/` 也不例外。注释以标识符开头，后面紧跟它做什么、保证什么；必要时写清副作用、错误、取消、并发、所有权和零值/空值/缺失的含义。私有函数和方法在名字与签名已经说清楚时不写；有前置条件、调用顺序要求或不明显的取舍时再写。

不要写成定义句（“X 是……”），也不要用“该函数”“此方法”“用于”开头：

| 不这样写 | 原因 | 这样写 |
| --- | --- | --- |
| `// New 创建一个带 opaque FANBOX session 的账号值，并复制输入凭据。` | 前半句复述函数名与签名 | `// New 会复制 sessionID，调用方之后修改原切片不影响账号。` |
| `// Account 是一个已保存的 FANBOX 账号记录。session ID 是 opaque credential。` | 定义句，夹杂英文术语 | `// Account 对应本机保存的一个 FANBOX 账号；session ID 只能通过 SessionIDCopy 取得副本。` |
| `// FANBOX session 不与 Pixiv refresh token 共享类型或生命周期。` | 翻译腔 | `// FANBOX 的 session 和 Pixiv 的 refresh token 分开存放、分开轮换。` |
| `// String 返回字符串表示。` | 没有信息 | 删除；如果它承担脱敏，写清楚：`// String 只输出账号摘要，日志和 %v 不会打印出凭据。` |

类型与字段：类型注释写它代表什么，以及使用者必须遵守的不变量。字段只在名字和类型看不出含义时才写，例如单位、零值或缺失的意义、可选性、由哪把锁保护、所有权和生命周期；短说明放在字段同一行的末尾。

```go
type Budget struct {
	mu   sync.Mutex
	used int64 // 已占用字节，由 mu 保护
	max  int64
}
```

### 编号阶段

函数里有几个明确的阶段时，在每段逻辑前用 `// 1. ...`、`// 2. ...` 写出这一段的目的或顺序要求。编号的是阶段，不是语句；简单函数、getter 和转发函数不编号。

- 各阶段保持同一抽象层级，措辞平行，调整顺序后重新编号。
- 注释紧贴它描述的代码段，不在函数开头写目录。
- 优先使用扁平序列；嵌套编号需要真实存在的嵌套流程。
- 难以整理的流程可能暴露职责混杂，但不要为了缩短函数而拆分代码。

### 让 Go 基础工程师读得懂

- 用到 `select`、channel 关闭、`sync/atomic`、锁的先后顺序、goroutine 交接等写法时，在旁边用一行注释说明为什么这样写。
- 状态机、账号池、OAuth 流程、MCP 会话这类复杂模块，在包文档或类型注释里讲一次整体模型：有哪些状态、谁拥有什么、按什么顺序发生。之后的局部注释不再重复。
- 不逐行复述代码，不给每个赋值、`if err != nil` 或 return 加注释，不写装饰性分隔线。
- 不承诺代码没有提供的保证，例如线程安全、原子性、幂等或可重试。

### 意图、限制与临时实现

- 解释显而易见的替代方案为什么不合适、顺序为什么重要、后续逻辑依赖哪个不变量，或哪个外部约束要求采用当前做法。说明紧挨相关代码。
- 限额、超时、重试或降级要写清其真实需求、平台约束、既有协议或可复现故障，以及对正常成功路径的影响。不要编造测量结果，也不要用注释为任意限制背书。
- 已知时，让 TODO/FIXME 能通过真实 issue 或明确的删除条件采取行动。保留真实引用，不编造 issue、负责人、期限或假设中的后续工作。
- 注释和错误消息只写当前行为、限制和原因，不写 goal、task、工作项、设计文档编号或实现进度；不要写“某阶段实现”“后续补齐”。尚未支持的能力直接写“尚不支持 X”。临时实现使用 `shortcut: <限制>，<替换条件>`，替换条件写成条件，例如“上游提供稳定 ID 后替换”。

## 测试提交规则

长期维护的代码与行为测试只覆盖以下契约；每项保留一个代表性用例和必要的失败用例。写新测试前先复用已有检查。

| 契约 | 归属 |
| --- | --- |
| 凭据不泄露：账号与 Cookie 类型在 `%v`、`%+v`、`%#v`、slog 中不输出凭据；日志不含 query、token、Cookie、签名 URL；MCP tool 输出不含签名 URL 与 Cookie | 账号与 Cookie 类型、请求日志、MCP 输出 |
| 账号存储：revision 冲突、事务回滚、v1.2.x 数据库可打开、文件权限；原子替换与文件锁 | `internal/storage/*` |
| 凭据轮换后新 token 在返回 client 前写入存储 | `internal/pixivaccount` 或 `internal/session` |
| 上游响应异常时返回错误；取消向上传播 | `sdk/pixiv`、`sdk/fanbox` |
| 对外契约快照：MCP `tools/list` 一份 golden；一个 CLI 命令的 text、JSON、NDJSON；用法错误的退出码；stdio stdout 只有 JSON-RPC；HTTP 必须带 bearer | `internal/mcpserver`、`internal/cli` |
| 环境变量优先于文件、文件优先于默认值，包括显式空值 | `internal/config/settings` |
| 自更新校验失败时保留已有二进制 | `internal/selfupdate` |

保留凭据隐私、数据完整性与持久化、CLI/MCP/SDK 对外输出、取消与错误传播，以及发布产物校验（例如 checksum 和归档命名）测试。每项契约都在所属的对外边界断言；能实际执行行为时，优先于只检查字符串。

| 项目 | 上限 |
| --- | --- |
| 全仓库非豁免测试文件 | 25 个 |
| 单个测试文件（含导入、空行与辅助函数） | 200 行 |
| 单个测试函数 | 40 行 |
| 单个表格的用例 | 8 个 |
| `testdata` | 数据库旧版本样本与少量 golden |

- 统计所有平台与 build tag 的文件，不只当前主机选中的文件。
- 使用 `httptest`、临时目录和真实 store。不新建测试辅助包、fixture 构造器、mock 层级，不为测试导出生产符号。
- 每类契约只在自己的归属层断言；CLI 或 MCP 输出测试检查呈现契约，不再重复 SDK 映射。
- 清理测试时保持 workflow 及其步骤不变。workflow 选中的测试连同支撑代码和 fixture 原样保留，不计入文件/函数/用例上限与 `testdata` 限制。宽泛的 `go test ./...` 选择器不会豁免整个仓库。同一文件同时包含选中测试和其他测试时，原样保留选中测试、注释及其必要声明；其他测试仍按本清单处理。重写测试前按 workflow 重新核对选择器；选择器匹配不到测试时报告缺口，不宣称通过。
- 按原样保留 CI 选中的清单：整包 `scripts/citest`（platform-smoke、container-smoke 和 release 运行）、`scripts/release`（release workflow）、`internal/urlscheme`、`internal/media/ugoira/...`；点名测试 `cmd/pixiv/TestPixivBinaryPackagedSmoke` 与 `internal/storage/database/TestOpenOnWindowsAcceptsForwardSlashFileURI`。清理测试前，按当前 workflow selector 复核测试名与包路径。
- 删除重复断言、getter 与纯转发测试、测试辅助代码或测试框架本身的测试、已完成迁移的对比/特征测试，以及清单外测试。同步删除孤儿辅助函数、fixture 和只为测试存在的生产入口；保留 v1.2.x 数据库可打开的证据。真实环境测试改为手动，不保留长期测试文件。
- 清单外调查放在被忽略的 `tmp/test/_probe/`；显式运行该包，并将探测输出留在该目录中，这样普通 `go test ./...` 不会发现它。使用合成凭据；不添加长期 live 测试文件或 opt-in live harness。
- 行为变更先观察失败测试，再实现通过，最后保留回归。导入或工具链错误不算 Red。纯结构调整使用改动前后的特征测试。

测试布局、CI 豁免与手动真实环境验证见[开发流程](development.md#测试)。

## 如何应用与审查

- 新增文件完整遵守本手册。
- 修改已有文件时，补齐本次触及部分。同一次改动里不顺手重写未触及的代码。
- 全库补齐是独立任务。
- CI 通过不代表这些规则已满足。按本手册审查 diff。
