# v1.0.0 显式调试诊断

状态：设计已确认，RC-9 已实施并通过 diagnostics、CLI、MCP、solver 并发聚焦回归；一次性真实 solver acceptance 已于 2026-08-08 通过并记录于最终验证报告。确认日期：2026-08-04；实施记录：2026-08-08。

## 目标与结论

v1.0.0 增加单一根命令 flag `--debug`，用于在一次 CLI 命令或 MCP server 进程中实时观察配置、
账号选择、网络路由、challenge recovery、下载与最终错误链。它弥补默认不创建项目级日志时的现场
诊断缺口，但不恢复旧 logging subsystem。

锁定行为如下：

- 默认关闭，只能由当前 invocation 的 `--debug` 显式开启；不增加 config key 或环境变量。
- 诊断实时写入 stderr；正常 CLI 结果仍写 stdout，MCP stdout 始终只承载 JSON-RPC。
- 不自动创建 `logs/`、每日文件或其他持久化产物，也不提供 `--debug-file`。需要保存时由用户自行
  使用 shell 重定向，例如 `2>debug.log`。
- 不提供日志等级、重复 `-v`、raw trace、JSON event stream 或可选 formatter。
- 每行使用明确的 `[业务域 + 子系统]` 开头，随后是本地时间与完整英文句子；不使用 `debug`、日志
  level 或 `key=value` 作为展示格式。
- debug 只观察执行过程，不改变账号选择、路由、重试、FlareSolverr 触发条件、下载或错误分类。
- 公开 Go SDK 不新增 logger、debug writer 或 diagnostics API；外部 SDK caller 默认仍然安静。

旧 `[logging]`、`log_level` 与 `PIXIV_LOG_LEVEL` 保持删除状态，不因本设计恢复。设计写入前 README
声称 MCP 会创建每日日志的旧说明属于文档错误，本轮已同步纠正；正式 CLI reference 只在功能实现时
加入 `--debug`，不能提前把计划写成当前可用能力。

## CLI 与 MCP 表面

`--debug` 是根命令 persistent flag，普通 CLI、Pixiv MCP 与 FANBOX MCP 都继承：

```bash
pixiv --debug illust detail 123456
pixiv illust detail 123456 --debug
pixiv --debug mcp
pixiv --debug fanbox mcp
```

flag 只影响该进程。没有自动持久化；用户可以显式保存 stderr：

```bash
pixiv --debug fanbox posts 12221352 2>debug.log
```

诊断通道正常时，开启 debug 不改变命令的 stdout shape、MCP tool schema、structured result、
`isError` 语义或正常退出状态；stderr writer 自身失败是下文明确的 process-level 例外。

`auth export` 的无 `--output` 成功路径是既有机器接口：stdout 只能是 raw token 或 bundle，stderr
必须为空。它仍接受根 flag 的正常语法解析，但 command boundary 不创建 diagnostic scope，成功和失败
都不输出 debug lifecycle；`--output` 形式同样保持整个 export command 的一致行为。隐藏 OAuth callback
也继续遵守既有无 stdout/stderr 契约。这里不拒绝全局 flag，也不增加新的 secret 处理分支。

参数解析错误发生在 diagnostic scope 创建前。有效 `--debug` 与 unknown option 同时出现时，只输出
一行 `error: unknown option '--name'` 并按 usage error 退出，不生成 `[Pixiv CLI]` 或其他 lifecycle
事件。`--` 之后的 token 仍是位置值；完整严格解析契约以
[严格 unknown-option 解析](strict-cli-argument-parsing.md)为准。

## 行格式与模块名

固定展示格式为：

```text
[Module name] HH:MM:SS Complete English sentence.
```

模块名必须足以独立判断产品与子系统。禁止只使用 `[MCP]`、`[Network]`、`[Download]`、
`[Account pool]` 或 `[Configuration]`。首批稳定名称包括：

```text
[Pixiv CLI]
[FANBOX CLI]
[Pixiv MCP server]
[FANBOX MCP server]
[Pixiv account pool]
[Pixiv authentication]
[FANBOX authentication]
[Pixiv authentication database]
[FANBOX authentication database]
[Pixiv configuration]
[FANBOX configuration]
[Pixiv network]
[FANBOX network]
[FANBOX FlareSolverr]
[Pixiv download]
[FANBOX download]
```

共享实现仍按当前 operation 的业务域选择展示名称，不能因为内部复用一个 helper 就退化成泛化前缀。
正文描述 operation、资源、阶段和结果，不把字段表伪装成日志。例如：

```text
[FANBOX MCP server] 12:21:18 Started request 7 for tool fanbox_get_post.
[FANBOX network] 12:21:18 Request 7 is retrieving post 12221352 through the native transport.
[FANBOX network] 12:21:19 Cloudflare challenged request 7 with HTTP 403.
[FANBOX FlareSolverr] 12:21:19 Request 7 requires fresh Cloudflare clearance.
[FANBOX FlareSolverr] 12:21:31 Clearance was acquired; request 7 will be replayed natively.
[FANBOX MCP server] 12:21:34 Request 7 completed successfully in 16.2 seconds.
```

## 内部组件与依赖方向

目标增加聚焦的 `internal/diagnostics` package；它保持协议与产品 DTO 无关，并包含四个职责明确的
单元：

1. **Event contract**：一组封闭、类型化的诊断事件及其安全字段；不接受任意 map、raw header、body
   或预格式化 transport dump。
2. **Sink**：接收事件的最小接口；关闭 debug 时使用 no-op sink。
3. **Scope**：把 sink、业务域、adapter 类型与可选 MCP 本地请求编号绑定到当前 context。
4. **Narrative presenter**：统一添加模块名和时间，把 typed event 渲染为英文完整句子，并把整行同步
   写向注入的 `io.Writer`。

bootstrap 是唯一生产组装点。CLI 根命令解析 `--debug` 后创建 stderr presenter；CLI command 或 MCP
request 创建 operation scope，application、账号调度、SDK 内部 adapter、FlareSolverr 与 downloader
只上报 typed event。不得使用 `slog.Default`、package-level 可变 logger 或公开 SDK option 注入输出
目的地。

公开 SDK operation 的既有 context 可以携带仅仓库内部识别的 scope。这样 CLI/MCP 能看到 transport
阶段，外部 SDK caller 不需要也不能依赖该内部机制，公开方法签名与默认输出都不变化。

## 事件流与并发关联

普通 CLI 一次只有一个顶层 operation，不增加无意义的 request ID。并行下载使用作品 ID、帖子 ID、
文件名或目标路径区分各项。

两个 MCP server 分别维护进程内递增的本地请求编号。编号只关联 stderr 中同一请求的开始、内部事件
和结束；不输出原始 JSON-RPC ID，不写回 response，也不跨进程持久化。tool 名称保留原始 code-id。

presenter 以完整行为单位串行写入，避免 goroutine 的字符互相穿插；事件产生后立即写出，不缓存到
operation 结束。并发事件按实际到达顺序显示，不伪造全局因果顺序。模块名与请求编号或资源标识共同
提供可追踪性。

## 记录内容

开启 debug 后覆盖成功与失败的安全生命周期，而不是只在最终失败时输出一行：

- **配置**：实际读取的配置来源、相关功能开关、proxy/UA 来自默认值还是显式配置。
- **账号与调度**：候选数量、所选非 secret UID、策略、冻结、排除、恢复和 exhaustion 原因。
- **网络**：逻辑 operation、native/App API/solver 路由、去除认证信息后的 proxy 地址、实际 UA、
  HTTP 状态与耗时；不打印 raw URL query。
- **Cloudflare/FlareSolverr**：challenge 识别、开始匿名求解、取得 clearance、native replay 与最终
  结果；不显示 solution Cookie。
- **下载**：正在解析的作品或帖子、发现的资源数量、目标路径、单项与总体成功/失败阶段。
- **MCP**：明确的 Pixiv/FANBOX server、tool 名称、本地请求编号、开始、结束和 structured failure。
- **错误**：失败模块、阶段、直接原因与必要的受控 cause chain；不重复打印同一个底层错误。

UID、作品 ID、帖子 ID、HTTP status、用户选择的下载目标路径、去除 userinfo 的 proxy 地址与实际 UA
均不是 credential，在显式 debug 中可直接显示。不能为省事把这些诊断价值高的普通字段全部隐藏。

## 必要的 secret 边界

以下数据不得进入 typed event、renderer、错误或测试失败 diff：

- Pixiv access/refresh token、FANBOXSESSID、Cookie、Set-Cookie、Authorization；
- proxy userinfo、用户名与密码；
- signed query、完整 header、request/response body 与下载内容；
- FlareSolverr response body、solution Cookie 与 `cf_clearance` value；
- browser profile path/content、数据库中的加密 secret 与可能携带凭据的原始 argv。

transport/library error 不能未经检查直接 dump。若错误含 URL，只能展示移除 userinfo 和 query 后的
scheme/host/path；业务 adapter 应优先产生 operation、status、stable reason 与安全 cause 等 typed
字段。这里不增加任意文本长度限制、截断规则或把普通标识符全部模糊化。

## 错误与输出通道故障

- 未开启 debug 时，既有输出和退出码完全不变。
- 普通 CLI 失败时，模块事件说明路径，command boundary 仍只输出一次既有 `error: ...` 并保留原
  非零退出码。
- 可预期的配置、认证、网络、上游与文件错误不附加 Go stack trace；真正 panic 保留现有 runtime
  行为，不能伪装成业务错误。
- MCP tool 失败仍通过 JSON-RPC structured result 返回并设置 `isError=true`；debug 文本只在 stderr。
- stdio/server 生命周期错误属于 MCP runtime，不伪装成某个 tool error。

stderr 自身写入失败时不静默吞掉，也不自动切换文件：presenter 保留第一次错误。已经开始的业务操作
不因该错误被中途取消；普通 CLI 在 operation 结束时返回非零，业务同时失败时仍以业务错误为主并
附带诊断通道错误。MCP 不污染 stdout 或改写当前 tool result，而是把该错误交给 server runtime 的
既有终止路径，由宿主通过进程状态发现。没有重试、固定等待或其他输出 fallback。

## 测试与本机验证

普通 CI 只增加小型 deterministic tests，不启动 Docker、真实 FANBOX、真实 credential 或
FlareSolverr：

- 未启用时零诊断且不创建文件；启用时只写 stderr；
- 使用固定 clock 覆盖少量代表性 narrative sentence 和明确模块名；
- Pixiv/FANBOX CLI 与两个 MCP server 的模块不会混淆；
- MCP stdout 保持可解析的纯 JSON-RPC，本地请求编号关联开始、内部阶段与结束；
- secret、signed query、proxy userinfo 与 solver clearance 不出现在 buffer 或失败 diff；
- `auth export` 即使收到根 `--debug`，仍保持成功 stderr 为空且 stdout byte-for-byte 遵守既有
  raw-token/bundle 契约；
- 有效 `--debug` 与 unknown option 并存时只输出单一 usage error，不初始化 presenter 或业务依赖；
- failing writer 不取消已开始的业务 operation，并按 CLI/MCP runtime 契约暴露；
- debug on/off 不改变路由、账号调度、retry、solver 调用次数或最终业务错误；
- 不维护庞大整段日志 golden，也不增加 CI service topology。

FlareSolverr recovery 实现完成后，维护者按既有 runbook 在本机使用固定 image digest 做一次真实
protocol 验证：synthetic native challenge、真实匿名首页 solve、synthetic native replay，使用非
secret dummy session 并检查 debug 只报告状态与路由。该次结果是 v1.0.0 发布前只需取得一次的
implementation acceptance evidence，不进入普通 CI，也不要求每个 RC 重复。若授权环境能稳定触发
genuine challenge，可再用有效本机 session 和明确帖子验证真实 post/file 路径；没有自然 challenge
不算失败。所有临时重定向文件与容器日志在 secret 扫描后删除。

## 文档与发布边界

设计阶段：

- 本文是 debug 行为的 canonical maintainer spec；总设计与相关分卷只链接本文，不复制完整规则。
- 纠正 README 中旧的每日/project log 描述，但不宣传未实现的 `--debug`。
- 在 verification 与 release-prep runbook 中记录测试和一次性本机验证入口。
- 后续实施步骤只写入 [RC 后续改动索引](rc-follow-up-index.md)所指向的新计划，不追加到已执行的
  `implementation-plan.md`。

实现阶段同步：

- `docs/en/cli-reference.md`、`docs/zh-CN/cli-reference.md`、`docs/ja/cli-reference.md`；
- 三语 README 中必要的快速诊断入口；
- 两种语言已有的 MCP 文档、maintainer development/architecture 文档；
- `skills/pixiv-cli/` troubleshooting 与用户可感知的 release-note 声明。

## 非目标

- 不恢复 daily log、retention、log rotation、logs directory 或历史 logging config。
- 不增加 `--trace`、raw HTTP/TLS dump、JSON diagnostics、verbosity ladder 或 stack trace mode。
- 不记录完整 API URL、header、body、Cookie 或文件内容。
- 不让 debug 成为 SDK public API、数据库表、配置项、MCP tool 或 JSON-RPC 字段。
- 不因开启 debug 增加 retry、timeout、fallback、账号切换或 FlareSolverr 请求。

## 验收标准

1. 默认安静且不创建日志文件；只有当前 invocation 的 `--debug` 会开启诊断。
2. 每行以明确的 `[业务域 + 子系统]` 开头，随后为时间和可读英文句子。
3. CLI stdout、MCP JSON-RPC、业务选择与 primary error 不被诊断文本污染；只有 stderr writer 自身
   故障按已定义的 process-level 契约改变最终进程状态。
4. Pixiv/FANBOX、CLI/MCP、账号、网络、solver 与下载阶段可以清楚区分和关联。
5. 成功与失败链均实时可见，writer failure 也不会被静默隐藏。
6. credential、Cookie、signed query、body、proxy secret 与 clearance 不泄露。
7. 没有自动 logs directory、旧 logging config、额外 runtime service 或隐藏业务 fallback。
