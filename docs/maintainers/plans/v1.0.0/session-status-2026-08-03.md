# v1.0.0 会话状态记录（2026-08-03）

本文件记录 2026-08-03 在隔离 worktree `codex/v1-sdk-rewrite`（HEAD `7de5abf`）上完成 v1.0.0
实施、全量测试与本地真实 e2e 验收的过程、结果与阻塞点。作为下一轮计划的输入，不再复述
[总设计](design.md) 与 [实施顺序](implementation-plan.md) 已写明的契约细节。

更新说明：第 9–11 节是 2026-08-04 的追加验证、设计决策与 authdb 审查；与前文中间结论冲突时，
以后附章节及其链接的专题设计为准。

当前状态（2026-08-08）：本文是过程与基线的历史记录；RC-1 至 RC-10 及内部架构重组的当前实现和
自动门禁以 [最终验证记录](final-verification-2026-08-08.md) 为准。本文保留的失败、旧路径和“下一轮”
措辞不表示当前代码仍未修复；RC-11 的真实 SDK、native browser 与 solver evidence 仍独立待授权。

## 1. 会话范围

- 接续上一会话在 Phase 8（文档/迁移/兼容冻结）中断的位置继续。
- 目标：基于 `docs/maintainers/plans/v1.0.0/` 在隔离 worktree 完成计划实施 + 全量测试 +
  本地真实 e2e 验收；不发布版本、不碰远端仓库。

## 2. 本会话完成的工作

### 2.1 Phase 8 文档清理（Web fallback 引用）

- 修完 `docs/en|zh-CN|ja/cli-reference.md` 与 `docs/en|zh-CN/mcp-tools.md` 中残留的
  过时 Web fallback 描述，统一改写为 "Removed anonymous web fallback" 语义（含
  `removed_setting` / `pixiv config unset web_fallback_enabled` 契约）。
- 最后两处零散引用（默认 `config.toml` 清单仍含 "web fallback"）在 `cli-reference.md`
  的 en/zh 中修正。
- `skills/pixiv-cli/` 全部残留更新为 v1 无匿名 Web fallback：SKILL.md 陷阱 #1/#5、
  troubleshooting 认证语义、discover 搜索/排行、download 认证要求。

### 2.2 补齐 `removed_setting` 墓碑（代码缺口，此前文档声称但未实现）

- `internal/application/config/settings.go`：`web_fallback_enabled` 标记 `Removed`；旧配置仍显式包含时
  `Effective`/`Runtime` 返回可 `errors.Is` 匹配的 `removed_setting`；删除死字段
  `RuntimeConfig.WebFallbackEnabled`；`ValidSettingAliases`/`ParseSettingInput`/
  `SetConfigValue` 排除或拒绝该键。
- `internal/application/config/paths.go`：默认 `config.toml` 模板删除 `[web] fallback_enabled = true`。
- `internal/cli/config_cmd.go`：`config get/set web_fallback_enabled` 返回 `removed_setting`，
  `config unset` 放行墓碑键以允许清理。
- 新增测试：absent 正常、present 报错、unset 清除、CLI 三命令路径；更新
  `TestConfigOnlyManagesApprovedKeys` 与默认模板断言。

### 2.3 维护者文档 v1 同步

- `AGENTS.md`：FANBOX 能力、`sdk/pixiv`/`sdk/fanbox` 调用链、v1 SDK e2e 命令、
  removed Web fallback 规则。
- `CONTEXT.md`：authdb、`sdk/pixiv`/`sdk/fanbox`、无 webapi。
- `docs/maintainers/architecture.md`：auth.json→`pixiv-cli.db`、顶层 `pixiv`→`sdk/*`、
  删除 `webapi` 章节、下载编排移到 application/download。
- `docs/maintainers/development.md`：Web fallback e2e 命令替换为 `TestRealPixivSDKRead`/
  `TestRealFanboxSDKRead`，删除已移除 canary 测试的过时描述。

## 3. 门禁结果（当前候选 commit `7de5abf` + 未提交改动）

| 门禁 | 结果 |
|---|---|
| `go test ./...` | PASS |
| `go test -race ./...` | PASS |
| `go vet ./...` | PASS |
| `sh scripts/build.sh` | PASS |
| `pre-commit run --all-files` | PASS |
| `go test ./scripts/documentation` | PASS |
| `git diff --check` | PASS |
| `internal/browsercookies/...`（macOS 本机） | PASS |

## 4. 真实 E2E 与路由复核结果

### 4.1 自动化 E2E

| 测试 | 结果 |
|---|---|
| `PIXIV_SDK_E2E=1 go test ./e2e -run TestRealPixivSDKRead`（本机 `pixiv-cli.db` 选中账号） | **PASS**（3.1s） |
| `FANBOX_SDK_E2E=1 go test ./e2e -run TestRealFanboxSDKRead` | **FAIL**（`upstream_error: 404`） |

FANBOX 自动化 E2E 的 404 来自当前实现使用错误的 `/api/v1/` base path 和旧 operation 名称；它不能
作为 CDN 指纹拦截证据。修正路由属于下一轮代码计划，本会话没有修改 FANBOX 实现。

### 4.2 单 `FANBOXSESSID` 直连复核

经用户显式授权，使用有效 `FANBOXSESSID` 在同一网络出口重新验证。凭据只经 stdin/进程内存传递，
未写入 argv、环境变量、文件或 evidence；请求未使用 FlareSolverr 或指纹库。

| 请求 | 结果 |
|---|---|
| `https://api.fanbox.cc/post.listHome?limit=10` | HTTP 200；合法 JSON；返回 10 条帖子摘要 |
| 对上述 10 条逐一请求 `post.info` | **10/10 HTTP 403**；`text/html` Cloudflare challenge 响应 |
| 其中 4 条 `isRestricted=false` 的帖子 | 仍为 HTTP 403，排除“仅因赞助权限不足”解释 |

因此，该明显非浏览器 request profile 只能证明可读取首页帖子摘要，不能证明可读取完整帖子详情。
该结论随后由 4.4 节的完整 Firefox profile 复核更新；公开 cover/preview 可达仍不能替代附件下载
evidence。

### 4.3 通用 headless browser 对照

同一凭据写入临时 headless Edge browser context 后，`www.fanbox.cc` 能识别登录态，但上述 10 个
`post.info` 仍全部返回 HTTP 403。该结果说明“换成通用 headless browser/浏览器指纹”没有形成稳定
替代路径；它不证明 FlareSolverr 一定可行。浏览器 context、Cookie、loopback bridge 与诊断产物均
已清理，响应内容和下载内容未落盘。

### 4.4 Firefox request profile 复核

为验证 `gallery-dl` 的轻量路径，先用本机既有 Python `requests` 复刻其 Firefox headers、TLS ciphers、
正确 endpoint 和相同代理；结果仍是首页 200、10/10 `post.info` 为 403。随后改用项目已经锁定的
`tls-client` 完整 Firefox profile，不新增依赖或浏览器：

| 验证 | 结果 |
|---|---|
| Firefox 135/148 profile，请求公开 `post.info` | HTTP 200；合法 JSON；无 challenge |
| Firefox 148 profile + 单一有效 `FANBOXSESSID` | 首页 10 条详情 **10/10 HTTP 200**；全部为合法 JSON |
| 从这 10 条详情发现 `files`/`fileMap`/`images`/`imageMap` | 0；该样本恰好没有正文资源 |
| `gallery-dl` 历史公开 imageMap 测试目标 | 当前已变为 HTTP 404，不能用于下载 evidence |
| 用户显式提供的非 secret 目标 `https://www.fanbox.cc/@aak/posts/12221352` | `post.info` HTTP 200；合法 JSON；`isRestricted=false`；正文存在 1 个文本段；发现 1 个 file attachment |
| 附件请求未携带 session | HTTP 403；只读取到 2,789 字节 challenge 响应 |
| 同一单一 `FANBOXSESSID` 同时用于 `downloads.fanbox.cc` | HTTP 200；完整读取 28,544,885 字节并丢弃，与详情声明大小完全一致 |

这推翻了“正确详情必须依赖 FlareSolverr”的中间结论：在当前网络出口，完整 Firefox TLS/HTTP profile
已能直连 `post.info`，并从合法详情完成真实 file attachment 下载闭环。用户配置仍只有一个
`FANBOXSESSID`；关键是 SDK 必须把同一 session 限定传播到经校验的 `api.fanbox.cc` 与
`downloads.fanbox.cc` 请求，而不是要求多个 Cookie。正文、密码值、文件名、signed URL 和响应内容
均未写入日志或 evidence；正文关键词探针未独立确认密码语义，因此这里只证明正文 payload 可读，
不声称已解析出专用“密码字段”。一次性 Go 探针、session 和响应内容均已清理，附件只读入内存后
丢弃，仓库没有留下诊断源码或下载文件。

## 5. 既有 FlareSolverr 辅助诊断的纠偏（2026-08-03）

此前按 [release-prep-runbook](release-prep-runbook.md) 第 5 节启动的隔离措施仍有效，但该轮上游
结论使用了错误路由，不能继续作为 release evidence。

固定 `ghcr.io/flaresolverr/flaresolverr:v3.5.0` digest、loopback-only、无仓库/Keychain/profile
mount 等隔离方式没有问题，诊断后容器也已删除。问题在于请求目标本身错误：

| 原请求 | 当前正确形式 |
|---|---|
| `/api/v1/home.posts` | `/post.listHome` |
| `/api/v1/home.supporting` | `/post.listSupporting` |
| `/api/v1/post.listTaggedPosts` | `/post.listTagged` |
| `/api/v1/creator.getTags` | `/tag.getFeatured` |

此外，`post.listCreator` 要求 `limit`；creator pagination 由 `post.paginateCreator` 返回 `pageUrls`。
此前错误路由在普通 HTTP 下返回 404、经 FlareSolverr 返回 HTTP 200 + `general_error`，不能据此推出
“非浏览器流量遭 CDN 404 拦截”或“FlareSolverr 已解决正确 API”。4.4 节已经证明完整 Firefox
profile 可直连正确 `post.info`；FlareSolverr 不再承担详情可达性或发布保证。

## 6. 阻塞点

### 6.1 FANBOX 完整真实 E2E（当时的 release blocker）

当前证据分为三条，不得把手工可行性探针直接记成“生产 SDK 已全部可用”：

1. **已通过**：正确 `post.listHome` 在普通 HTTP 下仅携带有效 `FANBOXSESSID` 即可返回登录态帖子摘要。
2. **已通过**：完整 Firefox profile + 同一 session 对首页 10 条 `post.info` 返回 10/10 HTTP 200 合法
   JSON；普通非浏览器 UA、Python requests 的 Firefox header/cipher 近似和通用 headless Edge 仍为 403。
3. **手工可行性已通过**：对用户显式提供的非 secret 目标，Firefox 148 profile + 同一单一 session
   取得合法详情、发现 1 个 file attachment，并从 `downloads.fanbox.cc` 完整读取 28,544,885 字节；
   与详情声明大小一致。附件请求不带 session 时为 403，带同一个 `FANBOXSESSID` 后为 200。

这也推翻了“会话已失效”的旧结论：同一 session 能取得登录态首页数据，且真实浏览器首页识别为
已登录。轻量直连的详情与附件可行性门禁均已通过。当时 release blocker 已收敛为**生产实现仍使用
错误 route/Chrome profile、尚未按资源 host 安全传播同一 session，且 public SDK 的完整自动化真实
E2E 尚未完成**，不再是详情级 challenge 或附件可达性本身；这些代码项已在 RC-6 至 RC-8 实施，当前
public SDK 的完整真实 operation/file evidence 仍按 RC-11 单独追踪。

完整 FANBOX SDK 真实 E2E 仍不能满足发布条件，因为本轮使用的是一次性手工探针而非修复后的 public
SDK；但现有 evidence 已证明轻量路径可实施。FlareSolverr 最多作为维护者按需诊断，不提供产品
保证，也不能替代 SDK 自身的详情和附件 evidence。

### 6.2 其余未完成项（非本会话阻塞）

- 三个平台的浏览器 provider native evidence：本机只跑了 macOS；Windows/Linux 依赖 GitHub Actions
  native-evidence job（`scripts/nativeevidence` 本地 macOS 通过）。
- 会话中段的 FANBOX 浏览器导入（Chrome 登录 fanbox.cc）需要用户交互，未执行。
- `scripts/test-e2e.sh`、releaseworkflow 与 platformsmokeworkflow 中的 v0 E2E/Web fallback 环境入口已
  在 2026-08-08 清理；当前脚本只选择 `PIXIV_SDK_E2E`/`FANBOX_SDK_E2E`，Pixiv 从本地 authdb、FANBOX
  从 Keychain 读取凭据，并要求显式非 secret creator/tag/post/page targets。

## 7. 工作区状态（未提交）

当前 `codex/v1-sdk-rewrite` 分支 HEAD `7de5abf`。工作区仍包含此前的配置墓碑、测试、public docs、
维护者文档与 product skill 改动，以及未跟踪的 `.tool-versions`；不得在下一轮开始时覆盖或丢弃。
本次补充复核只更新本状态记录与 `verification-release.md`，没有修改 FANBOX 源码或测试。

交接时以 `git status --short` 为准，不在本文件维护容易过期的精确文件数。

## 8. 下一轮计划输入

下一轮先设计、再实施；本会话不预先改生产契约。已明确的目标与约束如下：

1. 公开 `sdk/fanbox` 仍只接受 `FANBOXSESSID`，不要求用户配置多 Cookie、Cookie 池或浏览器指纹。
2. 修正 API root、operation 名称、必需参数、pagination 解析和 top-level error envelope；为每一处补充
   offline contract test。
3. 生产主路径使用现有 `tls-client` 的 Firefox profile，并复刻 FANBOX 所需的 `Origin`、`Referer`、
   `Sec-Fetch-*` 与 header order；同一个 `FANBOXSESSID` 只传播到严格校验后的 FANBOX API/下载 host，
   redirect 后重新校验 host，不向任意 URL 泄露；不引入浏览器、Node/Python runtime 或 FlareSolverr
   sidecar。
4. 把本轮已通过的手工可行性路径落实到 public SDK：以单一有效 session 取得合法 `post.info`，再从
   显式非 secret 目标的详情发现真实 image/file attachment URL，完整读取并核对详情声明大小。目标
   经 E2E 环境配置传入，不把本轮 URL、内容 ID、文件名或 signed URL 硬编码进测试。
5. 真实 E2E 覆盖每个 public FANBOX operation。账号相关列表合法为空时，以状态和 schema 正确计为
   通过；creator/tag/post/resource 的非空语义覆盖使用显式、非 secret 的环境配置目标，不硬编码，
   也不运行时自动发现。
6. 资源验收区分 cover/preview 与帖子 file attachment；只有后者完成 URL 发现、响应校验和完整读取，
   且在详情声明 size 时字节数一致，才算文件下载闭环通过。
7. **已执行（2026-08-08）**：审查现有真实 E2E 的固定 60 秒 context deadline；未发现协议/平台证据，
   已改为遵循 `t.Context()` 的 caller/test cancellation，避免用任意时限掩盖长响应或 challenge。
8. 本轮当时把 FlareSolverr 仅保留为维护者按需诊断，不属于生产能力、普通测试依赖或发布成功条件；
   该结论已被第 9 节的追加验证与 challenge-only production recovery 决策取代。其结果不能改写 SDK
   失败。轻量路径的手工可行性门禁已经通过；上述设计获批并落实到 public SDK 后，再同步
   `design.md`、`implementation-plan.md`、`release-prep-runbook.md`、架构/开发文档和受影响的 public
   contract locale。

若重跑真实 Pixiv e2e：
`PIXIV_SDK_E2E=1 PIXIV_E2E_PROXY=http://127.0.0.1:7890 go test ./e2e -run TestRealPixivSDKRead -count=1 -v`。
开始下一轮前先评审并处理当前未提交改动。

## 9. 2026-08-04 FlareSolverr 追加验证与决策

本节是 8 月 3 日状态记录的追加决策。它保留前文当时的观察，但取代第 5、6.1 与 8 节中
“FlareSolverr 只作维护者诊断、不进入产品能力”的结论。

### 9.1 追加验证结果

继续使用固定 `ghcr.io/flaresolverr/flaresolverr:v3.5.0`，digest 为
`sha256:139dfee1c6f89249c8d665d1333a42e8ec74ec0a86bc6bb1c8461e10d3a66a47`。容器保持临时、无仓库/
Keychain/profile mount，并在验证后删除。

| 验证 | 结果 |
|---|---|
| FlareSolverr 直接 `request.get` 正确 `post.info` API | 未稳定返回目标 JSON；不能用作 API 全量代理 |
| persistent solver session 后再请求 API | 同样不能建立可靠的 API JSON 路线 |
| 标准 Go transport 复用 solver UA/Cookie | HTTP 403；仅复制 Cookie 不足以替代 browser-aligned TLS |
| FlareSolverr 匿名请求 `https://www.fanbox.cc/` | 返回 solver user agent 与 `cf_clearance`；未传 session |
| Firefox 148 `tls-client` + 原 session + solver UA/clearance | 正确 `post.info` 返回 HTTP 200 合法 JSON |
| clearance-only 对照 | 成功；不需要 solver 返回的其余 Cookie |
| 混入额外 Cloudflare-family Cookie 的一次 Chrome 146 对照 | HTTP 403；说明 Cookie 越多并非越可靠 |

FlareSolverr response 在一次验证中含 14 个 Cookie，但 production 只应读取单个 `cf_clearance`。曾把
session 传给 solver 的旧诊断会使其进入 INFO 日志；相关容器与日志已经删除。锁定路线改为匿名首页
求解，禁止把 `FANBOXSESSID`、API/帖子/文件 URL、request body 或下载流交给 solver。

宿主机 proxy、容器直连与容器显式经 host proxy 在本机观察到相同公网出口，但请求行为仍不同，不能
据此自动继承 proxy。native FANBOX proxy、FlareSolverr service URL 与 solver upstream proxy 保持
独立；非空值必须分别显式配置，允许 native/solver proxy 按真实拓扑留空。

### 9.2 已锁定设计

- Firefox 148 是当前 native baseline，不是永久通过 Cloudflare 的保证；成功或普通错误时
  FlareSolverr 零调用。FANBOX-only UA 可以显式覆盖 native header，但不改变 TLS profile。
- FlareSolverr 显式配置、默认关闭；只有严格识别的 Cloudflare challenge 才触发。
- solver 只匿名求解 FANBOX 首页；API 与文件继续由 native transport 请求。
- 每个 Client 只在内存缓存 solver user agent 与单个 clearance；使用上游 expiry，不臆造 TTL。
- 同 Client 的并发 challenge 合并求解；每个原请求最多一次 fresh solve 与一次 replay。
- 用户仍只提供一个 `FANBOXSESSID`，不配置多 Cookie、Cookie pool 或多个账号组合。
- 普通 CI 只做 fake service/transport 的小型测试；实现完成后以 synthetic native challenge + real
  solver 做一次可复现的本地 protocol 验证，作为 v1.0.0 前一次性 implementation acceptance，不作为
  普通 CI 或每个 RC 的重复条件。genuine challenge evidence 只在真实网络自然触发时 best-effort
  记录，不是额外发布门禁。

完整契约见 [FANBOX challenge 与 FlareSolverr 路由](fanbox-challenge-routing.md) 和
[网络配置与服务路由](network-routing.md)。

### 9.3 错误 API 命名

v1.0.0 API freeze 前把公开 `Code` 命名整体改为：

```go
type Reason string

type Error struct {
	Product   string
	Operation string
	Reason    Reason
}

func ReasonOf(err error) Reason
func IsReason(err error, reason Reason) bool
```

常量不加人为前缀，直接使用 `InvalidArgument`、`Unauthorized`、`ChallengeRequired` 等；稳定字符串值
保持不变。不增加 solver 专用 reason。该改动已写入生产源码、public inventory 与 locale SDK 文档，并由
`sdk` tests 和 inventory golden 验证。

## 10. 2026-08-04 authdb 审查（历史基线）

SQLite 的单库、分产品表、migration ledger、config/default 分离等总体设计可以保留；基线审查当时识别出
以下 RC 前必须修复的问题：

1. legacy 多账号导入不是单事务，失败可部分提交，bootstrap 又忽略 migration error；
2. 当前 `pool_last_selected` 排序使 3 个账号的序列变成 `1,2,1,2...`，第三个账号饥饿；
3. rotation 没有 expected revision CAS，re-import 还可把 revision 从较大值写回 `1`；
4. schema 允许 `creator_id NULL`，repository 却 scan 到 `string`；
5. SQLite `file:` DSN 未转义，合法路径含 `?`/`#`/`%` 时无法可靠打开；
6. default config error、FANBOX auto default 展示、`application_id` test shadow 与 Windows ACL 文档
   边界也需同轮收口。

现有聚焦回归全部通过，但 disposable probes 已复现上述核心缺口，说明基线 tests 覆盖不足。probe
源码已删除，没有修改 production code 或保留临时数据库。完整 evidence、优先级、migration 建议和
测试清单见 [authdb 设计审查](authdb-design-review-2026-08-04.md)；P0-1 至 P0-5 已在 RC-3 至 RC-5
修复并由当前最终验证记录确认。

## 11. 更新后的下一轮入口（已执行）

初始 [实施顺序](implementation-plan.md) 已经执行，下一轮不能把 follow-up 继续追加为其新 phase。
以下已确认/已审查输入统一从 [RC 后续改动索引](rc-follow-up-index.md)进入。用户完成书面复核后，
已经单独写入 [RC follow-up 实施计划](rc-follow-up-implementation-plan.md)：

- [FANBOX challenge 与 FlareSolverr 路由](fanbox-challenge-routing.md)；
- [网络配置与服务路由](network-routing.md)；
- [Pixiv 账号调度](account-pool-scheduling.md)；
- [authdb 设计审查](authdb-design-review-2026-08-04.md)；
- [显式 debug 诊断](debug-diagnostics.md)；
- [严格 unknown-option 解析](strict-cli-argument-parsing.md)。

该轮代码范围包含 FANBOX route/profile/resource、可选 challenge recovery、错误 `Reason` 改名、authdb
P0 修复、数据库账号调度、service-scoped network、strict parser 与 typed debug diagnostics；这些范围已
实施并补齐聚焦 tests。当前只剩本文件所述 RC-11 外部 release evidence，不能由离线回归自动升级为通过。

## 12. 2026-08-04 debug 诊断决策

v1.0.0 增加当前 invocation 显式开启的根 `--debug`。它覆盖普通 CLI、`pixiv mcp` 与
`pixiv fanbox mcp`，只把实时、叙述式英文诊断写入 stderr；默认不创建 `logs/`，不恢复旧 logging
config，也不改变 MCP stdout、公开 SDK API、账号调度、网络路由或 FlareSolverr 状态机。

每行以明确的 `[业务域 + 子系统]` 开头，不能使用孤立的 `[MCP]`、`[Network]` 或 `debug`/level/
`key=value` 前缀。MCP 并发请求使用进程内本地编号关联；诊断允许显示非 secret UID、资源 ID、状态码、
下载目标、去除 userinfo 的 proxy 与实际 UA，但不记录 token、SSID、Cookie、signed query、body、
proxy credential 或 clearance。完整设计与测试/本机验证边界见
[显式 debug 诊断](debug-diagnostics.md)。

## 13. 2026-08-04 strict unknown-option 决策

v1.0.0 不允许未注册 option 被当作位置参数、透传输入或兼容性 ignored flag。root boundary 统一把
unknown long/short option 映射为 `error: unknown option '--name'`、空 stdout 与 usage exit code `2`；
`--name=value` 不回显 value。解析在 config、账号、网络、MCP runtime 与 debug presenter 之前失败，
因此与 `--debug` 同时出现时也只有一条参数错误。

标准 `--` end-of-options marker 保留；其后的 `-`/`--` token 是位置值，由具体 command Args contract
判断。unknown command、位置参数数量错误和 known option 的 value/互斥错误不在本次统一改名范围。
完整契约见 [严格 unknown-option 解析](strict-cli-argument-parsing.md)。
