# v1.0.0 网络配置与服务路由

状态：设计已确认，RC-6 至 RC-7 已实施并通过配置/transport 聚焦回归。确认日期：2026-08-04；实施记录：2026-08-08。

## 结论

Pixiv、FANBOX native 与 FlareSolverr browser 是不同网络消费者：

```text
pixiv-cli --Pixiv transport---------> Pixiv API / OAuth / resources
pixiv-cli --FANBOX native transport-> FANBOX API / downloads
pixiv-cli --direct control----------> FlareSolverr service
FlareSolverr --browser upstream-----> FANBOX homepage
```

CLI runtime 保留现有全局代理作为 fallback，并允许 Pixiv/FANBOX 分别显式覆盖。FlareSolverr service
与其 browser upstream 继续使用独立配置，不从 native 路径猜测或继承。该设计不增加代理库存、文件
导入、账号代理绑定、健康检查或任何自动轮换。

## TOML

```toml
# 通用 fallback，并继续供 update 等非产品请求使用。
[network]
https_proxy = "http://127.0.0.1:7890"

# 可选；只影响 Pixiv API、OAuth 与 Pixiv 第一方资源。
[pixiv.network]
proxy_url = ""

# 可选；只影响 FANBOX native API、页面与第一方资源。
[fanbox.network]
proxy_url = "http://127.0.0.1:7890"
user_agent = "Mozilla/5.0 ..."

# 显式、默认关闭的 challenge recovery。
[fanbox.flaresolverr]
url = "http://127.0.0.1:8191"
proxy_url = "http://host.docker.internal:7890"
```

`pixiv.network.proxy_url` 与 `fanbox.network.proxy_url` 是三态配置：

- key 不存在：继承通用 fallback；
- key 存在且非空：使用该服务自己的固定代理；
- key 存在且为空字符串：该服务显式 direct，不继承通用 fallback。

Runtime 必须保留“key 不存在”和“key 存在但为空”的区别，不能在装配前都压成 Go `string` 零值。
Pixiv 路径继续接受 `http`、`https`、`socks5`、`socks5h`；FANBOX native v1.0.0 只接受无 userinfo
的 `http`/`https` CONNECT proxy。通用 fallback 被某个产品消费时仍按该产品协议校验；例如全局
SOCKS proxy 对 Pixiv 合法，但 FANBOX 必须通过显式空 service key 选择 direct，不能静默忽略。

配置文件不默认生成这些可选 table。公开 Pixiv/FANBOX SDK 都不读取 TOML 或环境变量；runtime 解析后
通过各自 `Options`/client request 显式传入。

## 解析优先级

Pixiv 与 FANBOX native 分别解析：

```text
当前命令的 --no-proxy / --proxy URL
> 对应服务的 proxy_url（包括显式空字符串）
> https_proxy / HTTPS_PROXY
> [network].https_proxy
> direct
```

- `--proxy`/`--no-proxy` 只影响当前命令所属产品的 native transport，不写配置，也不能同时使用；
  FANBOX 命令上的这两个 flag 永远不覆盖 `fanbox.flaresolverr.url`、
  `fanbox.flaresolverr.proxy_url` 或 control client 路由。
- `pixiv mcp` 使用 Pixiv 路由；`pixiv fanbox mcp` 使用 FANBOX native 路由。
- update 等非产品请求不读取服务级 key，只使用命令 override、环境与 `[network].https_proxy`。
- 服务级 key 比通用环境 fallback 更具体，因而优先；该例外必须在 CLI reference 中显式记录。
- 无效的显式值返回 `InvalidArgument`/配置错误，不 fallback 到下一层。

现有 `pixiv config` alias `https_proxy` 继续读写 `[network].https_proxy`。服务级 key 属于高级手工 TOML；
除非实施阶段确认有稳定用户需求，v1.0.0 不再增加 `pixiv_proxy`/`fanbox_proxy` alias 或环境变量。

## FANBOX native UA

`fanbox.network.user_agent` 是可选的 FANBOX-only HTTP header override：

- key 不存在或值为空时使用 binary 内置的 native baseline UA；
- 非空值应用于 SDK 创建的 FANBOX native API/页面/第一方资源请求；
- 它不影响 Pixiv App API、Pixiv OAuth、Pixiv resource transport 或 FlareSolverr browser；
- 它只改变 HTTP `User-Agent`，不改变 `tls-client` ClientHello/profile，不构成完整浏览器模拟；
- 不提供 UA pool、随机轮换、失败后 profile 猜测或自动 fallback；
- 只拒绝 CR、LF、NUL 等非法 header 字符，不增加无依据的长度限制；
- 显式 debug 诊断必须用自然语言说明 UA 来自 built-in baseline、FANBOX config 还是 FlareSolverr，
  不展示成 `key=value`。UA 不是 secret，显式 config get 可以按普通字符串返回用户保存的值；错误
  不需要拼接导致校验失败的原始 header。完整输出契约见
  [显式 debug 诊断](debug-diagnostics.md)。

当 Client 已持有 solver 返回的 `cf_clearance` 时，solver user agent 必须覆盖 config UA，直至该 solver
state 因新 challenge 或 Client 回收而失效：

```text
solver user agent > fanbox.network.user_agent > built-in native UA
```

Firefox 148 是当前锁定的 native TLS profile/baseline，不是 Cloudflare bypass 保证。自定义 UA 也不能
把该结论提升为保证。

## FlareSolverr 的两个地址

`fanbox.flaresolverr.url` 与 `fanbox.flaresolverr.proxy_url` 含义不同：

- `url`：pixiv-cli control client 如何访问 FlareSolverr `/v1` service；
- `proxy_url`：FlareSolverr 进程中的浏览器如何访问 FANBOX 首页。

`url` 必须是无 userinfo/query/fragment/非根 path 的 absolute `http`/`https` service root；SDK 把空
path 与 `/` 规范化为同一形式并追加 `/v1`。因此外部 reverse proxy 必须在独立 host 的根路径暴露
service；v1.0.0 不接受 path prefix 或在 URL 中携带鉴权。

control client 使用独立 direct transport，不读取 `HTTP_PROXY`/`HTTPS_PROXY`、native service proxy 或
solver upstream proxy。v1.0.0 不增加第四个 `service_proxy_url`；外部 service 必须能从 pixiv-cli
所在网络直接访问。

当前 challenge 路线使用 FlareSolverr `request.get`，因此 solver upstream 只承诺官方该命令支持的
无认证 `http://`、`socks4://` 或 `socks5://` URL；带 userinfo/用户名密码的值在构造阶段拒绝。
FlareSolverr 把认证代理能力放在 `sessions.create`，但 v1.0.0 不为此创建 persistent/ephemeral browser
session。官方同时警告不要把 service 直接暴露到互联网：
[FlareSolverr README](https://github.com/FlareSolverr/FlareSolverr)。

## 部署示例

### 宿主机 pixiv-cli + Docker FlareSolverr

```toml
[fanbox.network]
proxy_url = "http://127.0.0.1:7890"

[fanbox.flaresolverr]
url = "http://127.0.0.1:8191"
proxy_url = "http://host.docker.internal:7890"
```

两个 proxy URL 可以从不同 network namespace 指向同一宿主机代理。SDK 不把 loopback 自动改写为
`host.docker.internal`；Linux Docker 也不得假定该名称天然存在。

### 同一 Compose network

```toml
[fanbox.network]
proxy_url = "http://proxy:7890"

[fanbox.flaresolverr]
url = "http://flaresolverr:8191"
proxy_url = "http://proxy:7890"
```

### 外部私网 service

```toml
[fanbox.flaresolverr]
url = "https://flaresolverr.internal.example"
proxy_url = "http://proxy.remote.internal:7890"
```

远程 service 应位于私网、VPN 或受保护的反向代理之后。若反向代理还要求当前 client 不支持的鉴权
header，v1.0.0 明确不兼容，不通过 URL userinfo 或日志中的 secret 临时绕过。

如果 Cloudflare 将 clearance 与出口绑定，native FANBOX 与 solver browser 需要由操作者配置到相容
出口。两条 URL 可以不同，但 SDK 无法从地址推断公网出口，也不增加探针、自动继承或改写。远程
solver direct、native 本地 direct 具有不同出口时，replay 只能按真实结果成功或返回 challenge，文档
不得保证可用。

## 测试门禁

- 全局 fallback、两个服务级 override、显式空值 direct 与命令 override 的完整优先级矩阵。
- FANBOX `--proxy`/`--no-proxy` 只覆盖 native transport，绝不改变 solver service/upstream 两个值。
- 配置解析保留 absent/empty/value 三态，并覆盖同一全局值对 Pixiv/FANBOX 协议支持不同的失败路径。
- Pixiv/FANBOX/MCP/update 只消费各自允许的配置，不出现跨产品继承。
- FANBOX config UA 只改变 native header；solver state 存续时 solver UA 优先。
- native proxy、service URL 与 solver upstream proxy 三者不互相继承。
- Docker/外部部署只做地址传递 contract test；普通 CI 不要求真实容器或远程 service。
- 无效 proxy/service URL 与失败路径不泄露 userinfo、Cookie 或 solver response body；service root
  的 scheme/host/path/query/fragment 矩阵和 `/v1` 构造有独立测试。非法 UA 不得进入 HTTP header，
  错误使用静态上下文而非拼接原始输入。
- debug on/off 不改变 proxy/UA 解析结果、transport 或 solver 调用次数；诊断可显示去除 userinfo 的
  proxy 地址与实际 UA，但不得显示 raw query、credential 或 solver response body。

## 非目标

- 不增加代理池、账号代理字段、代理文件导入、轮询、随机选择、健康评分或自动切换。
- 不承诺 FANBOX 能通过任意代理访问，也不把 Pixiv 使用代理描述为必要条件。
- 不提供任意 header map、Pixiv UA override、browser profile selector 或 UA/TLS 自动耦合。
- 不自动启动、发现、更新、认证或管理 FlareSolverr/Docker。
