# v1.0.0 FANBOX challenge 与 FlareSolverr 路由

状态：设计已确认，RC-6 至 RC-8 已实施并通过离线/合成聚焦回归；一次性 real solver acceptance 已于 2026-08-08 使用固定 image digest 通过，genuine challenge recovery 仍不作普通 CI 门禁。确认日期：2026-08-04；实施记录：2026-08-08。

## 结论

FANBOX 的 native 请求路径以 Go 进程内的 `tls-client` Chrome 146 TLS profile、内置 HTTP User-Agent Firefox 148 baseline 作为当前 baseline；该
选择不构成长期通过 Cloudflare 的保证。调用方可以显式覆盖 FANBOX native `User-Agent` header，但
这不会改变 TLS profile。FlareSolverr 是显式配置、默认关闭的可选 challenge recovery，不是全量
HTTP 代理：

1. 原始 API 或文件请求先走 native transport。
2. 只有响应被严格识别为 Cloudflare challenge 时，才允许调用已配置的 FlareSolverr。
3. FlareSolverr 只匿名访问 `https://www.fanbox.cc/`，取得 solver user agent 与单个
   `cf_clearance`；它不得收到 `FANBOXSESSID`、帖子 URL、API URL、文件 URL 或请求 body。
4. SDK 把用户原有的单个 `FANBOXSESSID`、新的 `cf_clearance` 与 solver user agent 交给 native
   `tls-client` 重放原请求。API JSON 与文件字节始终不经过 FlareSolverr。
5. FlareSolverr 未配置、求解失败或一次重放后仍遇到 challenge 时，返回真实分类，不再隐藏降级。

因此用户仍只管理一个 FANBOX session。SDK 不要求用户导入 Cloudflare Cookie 集合，也不维护多账号
Cookie 池。

## 采用该路线的证据

2026-08-03 至 2026-08-04 的授权本机验证得到以下结果：

- Firefox 148 profile 加单一有效 `FANBOXSESSID` 曾直接取得合法 `post.info`，并从详情发现真实
  file attachment；同一 session 传播到 `downloads.fanbox.cc` 后完整读取的字节数与详情声明一致。
- 该成功只证明 native 路径可作为首选，不足以保证所有出口、时间和 Cloudflare 状态都不会再次触发
  challenge。普通 Go TLS、header/cipher 近似及其他 browser profile 在同轮验证中出现过 HTTP 403。
- 固定 `ghcr.io/flaresolverr/flaresolverr:v3.5.0`、image digest
  `sha256:139dfee1c6f89249c8d665d1333a42e8ec74ec0a86bc6bb1c8461e10d3a66a47` 的本机容器可以在不接收
  session 的前提下，从 FANBOX 首页返回 user agent 与 `cf_clearance`。
- 将这两个值交给 Firefox 148 `tls-client`，再由 native transport 携带原有 session 请求正确
  `post.info`，取得了合法 JSON。Chrome 146 profile 在同一恢复材料下也曾成功；这些都是特定日期、
  目标、账号与网络出口下的观测结果；当前 follow-up 依据同一网络出口重新登录后的矩阵将 Chrome 146 选为 baseline，不能推出它始终有效。
- FlareSolverr 自己转发正确 API endpoint 时未稳定返回所需 JSON；标准 Go transport 即使复用 solver
  Cookie 与 user agent 也仍曾返回 403。故不能把 FlareSolverr 当作全量代理，也不能只复制 Cookie
  而继续使用普通 Go TLS。
- solver response 曾包含 14 个 Cookie；只使用 `cf_clearance` 可以成功，额外混入 Cloudflare-family
  Cookie 反而出现过 403。SDK 必须忽略其余 Cookie。
- 把 `FANBOXSESSID` 交给 FlareSolverr 会使该值进入 INFO 日志。匿名首页求解不会产生该泄露面，因此
  production integration 禁止把 session 传入 solver。
- 本机求解通常耗时约 9–14 秒，并出现过一次约 31 秒后失败。实现不增加固定 deadline 或自动重试
  上限；取消与时限只来自调用方 context。

这些结果是可行性 evidence，不是上游永久保证。验证产生的容器、日志、probe 和响应均已清理；正文、
密码信息、signed URL 与下载内容未进入仓库。

## 公开 SDK 配置

`sdk/fanbox` 的连接选项扩展为：

```go
type Options struct {
	HTTPClient    *http.Client
	ProxyURL      string
	UserAgent     string
	FlareSolverr  *FlareSolverrOptions
}

type FlareSolverrOptions struct {
	URL      string
	ProxyURL string
}
```

- `FlareSolverr == nil` 是默认值，表示完全不调用外部 solver。
- `FlareSolverr.URL` 是显式的 service root，例如 `http://127.0.0.1:8191`。它必须是 absolute
  `http`/`https` URL，包含非空 host，且不得包含 userinfo、query、fragment 或非根 path；空 path 与
  `/` 等价，SDK 规范化后只追加 `/v1`。v1.0.0 不支持 path-prefix reverse proxy 或 service auth；
  非法值返回 `InvalidArgument`，错误不回显原值。
- `FlareSolverr.ProxyURL` 只进入 FlareSolverr `request.get` 的 upstream proxy 参数；该路线不支持代理
  用户名/密码，含 userinfo 的值返回 `InvalidArgument`。
- `UserAgent` 为空时使用内置 native UA；非空时只覆盖 FANBOX native HTTP header，不改变
  `tls-client` profile。CR、LF、NUL 等非法 header 字符返回 `InvalidArgument`，不添加固定长度限制。
- FANBOX native `Options.ProxyURL`、FlareSolverr service URL、FlareSolverr upstream proxy 是三个独立
  值，不自动继承、探测或改写。
- `HTTPClient` 仍是调用方拥有的 FANBOX native 高级注入点。SDK 不静默替换注入 transport；生产
  恢复能力以 SDK 创建的 Firefox transport 为基准，受控测试可以通过注入 transport 验证状态机。
- Go → FlareSolverr service 使用独立、无 ambient proxy 的 control client，只服从调用方 context；
  不复用 `HTTPClient`、native `ProxyURL`、solver upstream proxy、`HTTP_PROXY`/`HTTPS_PROXY`。如未来
  需要给 service connection 本身配置 proxy，必须新增独立显式 option，不能偷用现有三个值。
- 构造器仍不联网。格式错误的显式 URL 在构造阶段返回 `InvalidArgument`，且错误文本不回显
  proxy userinfo 或完整配置值。

CLI/MCP runtime 通过显式 TOML 把配置组装到上述 option：

```toml
[fanbox.network]
proxy_url = "http://127.0.0.1:7890"
user_agent = "Mozilla/5.0 ..."

[fanbox.flaresolverr]
url = "http://127.0.0.1:8191"
proxy_url = "http://host.docker.internal:7890"
```

缺少整个 `[fanbox.flaresolverr]` table 即关闭 solver；`[fanbox.network]` 也不默认生成。v1.0.0
不增加 FlareSolverr 环境变量、CLI flag、默认 URL、自动发现、自动启动、Docker 管理或
host/container 地址转换。公开 SDK 自身不读取 `config.toml`。全局与服务级 proxy、FANBOX UA、
Docker/外部部署和解析优先级的唯一契约见
[网络配置与服务路由](network-routing.md)。

## 请求状态机

### 正常路径

- Client 尚无 solver state 时，以 Chrome 146 profile、配置或内置的 native UA 和用户的单个
  `FANBOXSESSID` 发起请求。
- Client 已有有效 solver state 时，native transport 同时使用缓存的 solver user agent 与
  `cf_clearance`；solver UA 优先于配置 UA，这不会再次调用 FlareSolverr。
- 成功响应及普通上游错误沿现有路径返回，不能因为配置了 FlareSolverr 就先经过或镜像到 solver。

### Challenge 识别

只有已验证信号才进入恢复状态机：

- 现有 response header/body 命中 `cf-chl`、`cf_chl` 或 `Cf-Mitigated`；或
- 预期 JSON 的 FANBOX API、预期二进制的 FANBOX 第一方资源返回 HTTP 403、`Content-Type` 为
  `text/html`，且同时存在 `Server: cloudflare` 或 `Cf-Ray`。

普通 JSON 403 仍是 `Forbidden`；401/session envelope 仍是 `CredentialsExpired`；其他状态继续使用
既有分类。实现不为了识别 challenge 新增无依据的 body 截断、等待时限或静默 fallback。

### 求解与重放

1. 若原请求使用了缓存 solver state 又触发 challenge，先使该 state 失效。
2. 同一个 `fanbox.Client` 内并发到达的 challenge 合并为一次匿名首页求解；共享求解的取消 ownership
   见下文，等待者服从各自 context。
3. 向 FlareSolverr 发送 `request.get`，目标固定为 `https://www.fanbox.cc/`。只有配置的
   FlareSolverr upstream proxy 可以随该请求发送。
4. 在写入 Client state 前验证 solver 成功状态：user agent 必须非空并符合 HTTP header field-value
   语法；response 中必须恰好有一个名为 `cf_clearance` 的 Cookie，其 value 非空并符合 HTTP cookie
   value 语法。重复 clearance、CR/LF/NUL 或其他非法 control/header/cookie byte 都使整个 response
   成为 `MalformedUpstreamResponse`；其他 Cookie 全部丢弃，不参与数量或内容校验。
5. 如 response 提供该 Cookie 的 expiry，则按上游 expiry 失效；未提供 expiry 时只保留到下一次
   challenge 主动失效或 Client 被回收，不臆造 TTL。
6. 使用新的 state 由 native transport 重放原请求一次。每个原请求最多一次 fresh solve 和一次
   replay；replay 再遇 challenge 即返回 `ChallengeRequired`。

调用方再次调用 operation 是新的请求周期，可以重新求解。SDK 不在一个请求中无限求解、轮询或
重放。

### 共享求解与取消

共享求解不能直接绑定第一个触发 challenge 的 caller context，否则 leader 取消会错误终止仍在等待的
其他请求：

- coordinator 为该次 solve 创建独立 context，并记录仍活跃的 waiter；每个 caller 只用自己的
  context 等待结果；
- 某个 waiter 取消时立即返回它自己的 `context.Canceled`/`context.DeadlineExceeded` 并注销自己，
  不影响仍活跃 waiter；leader 与 follower 遵循同一规则；
- 最后一个 waiter 注销时，coordinator 取消正在进行的 FlareSolverr request，不让无消费者的 solve
  和 goroutine 继续；
- 只有 solve 完成时仍存在活跃 waiter，且 response 通过完整校验，结果才能发布并缓存；全员取消后
  与 response completion 竞态得到的结果直接丢弃；
- 全员取消并清理后到达的新 challenge 开始新的 solve，不复用已经取消的结果。

该 ownership 不增加内部固定 deadline；上限只来自各 caller context 以及“无 waiter 即取消”的真实
生命周期。

## Proxy 拓扑

配置必须表达真实网络拓扑，不按“宿主机能访问”推断“容器内也相同”：

```text
pixiv-cli --native ProxyURL--> FANBOX API / downloads
pixiv-cli --direct control----> FlareSolverr service URL
FlareSolverr --ProxyURL-------> FANBOX homepage
```

- native proxy 只影响 native API/资源 transport。
- solver service URL 只决定独立 direct control client 如何访问 FlareSolverr；不读取 ambient proxy。
- solver proxy 只决定浏览器进程如何访问 FANBOX。
- 三者可以相同、不同或部分为空；SDK 不做继承。
- Docker Desktop 场景可显式使用 `host.docker.internal`，但 SDK 不把 loopback 自动改成该地址。
- 如果 Cloudflare 将 clearance 与出口绑定，操作者需要显式让 native FANBOX 与 solver browser 使用
  相容出口；SDK 既不能从 URL 推断公网出口，也不通过额外探针自动验证或改写配置。

本机验证中，即便几条路径观察到相同公网出口，是否成功仍有差异；因此测试必须断言配置传递，而不能
只比较出口 IP。外部 service 必须能被 control client 直接访问；v1.0.0 不增加
`service_proxy_url`。完整部署示例与 FlareSolverr 不应暴露公网的边界见
[网络配置与服务路由](network-routing.md)。

## 错误映射

本路线不新增 solver 专用 `Reason`：

| 场景 | `sdk.Error.Reason` |
|---|---|
| FANBOX UA、native/FlareSolverr 配置 URL 非法 | `InvalidArgument` |
| challenge 出现但未配置 solver | `ChallengeRequired` |
| solver 明确报告求解失败 | `ChallengeRequired` |
| replay 后仍是 challenge | `ChallengeRequired` |
| service 无法连接或 transport 不可用 | `UpstreamUnavailable` |
| response 非法、缺/非法 user agent、缺/重复/非法 `cf_clearance` | `MalformedUpstreamResponse` |

`context.Canceled` 与 `context.DeadlineExceeded` 保留在 error chain。错误、日志、CLI/MCP structured result
不得包含 Cookie、solver response body、原始 URL query、proxy userinfo 或配置正文。
显式 `--debug` 以 `[FANBOX network]` 与 `[FANBOX FlareSolverr]` 区分 native/challenge/solve/replay
阶段，只报告受控状态与安全字段；完整契约见 [显式 debug 诊断](debug-diagnostics.md)。

## 最小实现边界

- solver state 只存在于单个 `fanbox.Client` 内存中，不写 SQLite、config、文件、日志或全局 Cookie
  jar。
- 不创建 persistent FlareSolverr session，不维护 Cookie pool，不增加 Client `Close` 责任。
- 继续复用现有 FANBOX host 与 redirect 校验；API 和下载请求只向允许的第一方 host 传播所需 Cookie。
- 只保留两条必要 secret 边界：不把 `FANBOXSESSID` 交给 solver，不持久化或记录
  `FANBOXSESSID`/`cf_clearance`。不为此引入额外账号限制、文本截断或猜测式安全规则。

## 测试与验收

普通 CI 不运行 Docker、真实 FANBOX、真实 session 或 FlareSolverr。只增加小型 deterministic tests：

- native 成功时 FlareSolverr 零调用；
- challenge → 匿名首页 solve → 只提取 clearance/UA → native replay 成功；
- 普通 403 不触发 solver；
- 未配置、service unavailable、malformed response 与 replay challenge 的分类；
- native proxy、service URL 与 solver upstream proxy 不互相继承；
- config UA 只改变 native header，solver state 存续时 solver UA 优先；
- service control client 不复用 native injected client 或 ambient proxy；
- service URL 覆盖合法 root、userinfo/query/fragment/path-prefix 拒绝、`/v1` 构造与错误脱敏；
- fake solver response 覆盖重复 clearance、空/非法 cookie value、空/含 control byte 的 UA，失败时不
  cache、不 replay；
- 同一 Client 的并发 challenge 只产生一次 solve；覆盖 leader 取消而 follower 成功、follower 取消而
  leader 成功、全员取消会取消 solver request 且无 goroutine/state 残留。
- debug on/off 不改变零调用成功、单次 solve、共享 solve 或一次 replay 语义；诊断不得包含 session、
  clearance、solver response body 或 proxy userinfo。

实现完成后，维护者在本机使用固定 image digest 做一次可复现的 real-solver protocol 验证：注入的
native test transport 首次返回 synthetic challenge，真实 FlareSolverr 匿名求解 FANBOX 首页，随后
native replay 验证只收到 solver UA/clearance。构造 Client 时使用非 secret dummy session；它只进入
injected native transport，绝不发给 solver。该验证不使用真实 session，不算真实 FANBOX E2E，也
不是普通 CI 或每次 RC 的重复条件，但必须在 v1.0.0 发布前作为 implementation acceptance 完成一次。
只有真实网络自然出现 challenge 时，才额外记录 genuine recovery evidence；当前 native 成功时没有
自然 challenge 不构成失败，genuine evidence 也不替代或加重上述一次性 synthetic protocol 验收。

## 非目标

- 不把所有 FANBOX 请求转发给 FlareSolverr。
- 不把 solver 当下载器、浏览器自动化 API 或登录工具。
- 不接受完整 Cookie header、多 Cookie 配置或多个 FANBOX 账号 Cookie 组合。
- 不提供 UA pool、随机 UA、自动 browser profile 切换或 Pixiv UA override。
- 不承诺 Cloudflare 行为、FlareSolverr image 或某个 browser profile 永久有效。
- 不在失败后自动切换普通 Go transport、其他 profile、浏览器进程或未配置代理。
