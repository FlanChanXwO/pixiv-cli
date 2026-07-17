# ADR 0010：HTTP client 超时与 context 生命周期

## 状态

已采纳。

## 背景

Pixiv transport 曾有三种互相矛盾的默认值：App API、OAuth 与本地代理快照把
`http.Client.Timeout` 固定为 60 秒，Web API 和资源读取则直接复用全局
`http.DefaultClient`。前者的计时覆盖连接、redirect 和 response body 读取，会在
资源仍正常流式传输时终止整个请求；后者又允许进程内其他代码修改共享 client。

项目没有产品、协议或平台证据要求所有 Pixiv 请求必须在 60 秒内完成，也没有证据
支持新增另一项全局 timeout 配置。固定值既不能区分 API JSON 与媒体流，也会与公开
方法已经接受的 `context.Context` 形成两套生命周期来源。

## 决策

- SDK 默认不设置 `http.Client.Timeout`；一次请求的总生命周期由其
  `context.Context` 的取消或 deadline 控制。
- `NewClient` 在未提供 `Options.HTTPClient` 时创建一个专用的零 timeout client，
  并把同一 client 交给 App API、Web API、OAuth、账号与资源 transport。
- 调用方显式提供 `Options.HTTPClient` 时，SDK constructor 保留同一指针及其
  `Timeout`、`Transport`、cookie jar 和 redirect policy，不修改调用方对象。调用方若
  需要整请求 timeout，应在该 client 或每次调用的 context 上明确设置。资源请求仍按
  既有安全契约在逐请求副本上禁用 cookie、包装 redirect 校验，不会反向修改原 client。
- `OpenDefault` 未显式注入 client 时，operation snapshot 仍按本地代理配置克隆
  `http.DefaultTransport`；只移除整请求固定 timeout，不改变标准 transport 既有的
  dial、TLS handshake、idle connection 等阶段性策略。
- 内部 App API、Web API、OAuth 与 resource constructor 单独使用时，也采用专用的
  零 timeout client；不得裸用全局可变的 `http.DefaultClient`。
- context 取消与 deadline 继续保留 Go 的 `errors.Is` 语义。资源 response body 的
  生命周期属于调用方；读取完成、关闭 body 或取消 context 之前，SDK 不施加隐藏的
  整请求截止时间。

## 未采用的方案

- 保留固定 60 秒：缺少客观依据，且会误伤合法的长时间资源流。
- 把 60 秒改成可配置默认值：仍然为所有请求建立同一种不准确的总时限，并增加没有
  已知产品需求的配置面。
- 继续使用 `http.DefaultClient`：进程内全局 mutation 会改变 SDK 行为，无法为一个
  `Client` 实例提供稳定策略。

## 后果

- 默认请求不会因为 SDK 内部固定时限而中断；调用方必须用 context 表达每次操作的
  deadline，或显式注入带 timeout 的 client。
- CLI、MCP 与外部 Go 程序通过同一 public SDK 获得一致策略；显式 client 仍控制
  代理、timeout 与基础 transport 行为，resource 层既有的 cookie/redirect 安全边界不变。
- 网络连接阶段仍受 Go 标准 transport 的既有保护，不把“无整请求 timeout”误解为
  移除所有 transport 阶段约束。
