# v1.0.0 RC follow-up 实施计划

状态：专题规格与实施顺序已确认；RC-1 至 RC-10 已实施并完成对应聚焦回归，RC-11 自动门禁已
完成；截至 2026-08-08，真实 Pixiv SDK 与一次性 solver release evidence 已取得，真实 FANBOX SDK
与 native browser evidence 仍待完成。计划创建日期：2026-08-04；当前实施记录：2026-08-08。

## 1. 边界

本文只编排初始 v1.0.0 整体重写完成后发现的 RC delta。历史
[初始实施顺序](implementation-plan.md)继续保留为已执行记录；不得重新建立旧 worktree、重做旧
Phase 0–9，或把已经完成的 SDK package migration、CLI/MCP 重写描述成待办。

本计划的 canonical 输入为：

- [authdb 设计审查](authdb-design-review-2026-08-04.md)；
- [Pixiv 账号调度](account-pool-scheduling.md)；
- [网络配置与服务路由](network-routing.md)；
- [FANBOX challenge 与 FlareSolverr 路由](fanbox-challenge-routing.md)；
- [显式 debug 诊断](debug-diagnostics.md)；
- [严格 unknown-option 解析](strict-cli-argument-parsing.md)；
- [测试、迁移与发布门禁](verification-release.md)。

专题文档决定行为，本文只决定实现依赖、代码落点、聚焦测试和检查点。实施中发现专题文档互相冲突时
先停在当前 phase，修正规格并重新确认；不能在代码里增加未批准的 timeout、retry、截断、fallback、
代理轮换、Cookie pool 或隐藏兼容路径。

## 2. 不变量

- CLI/MCP 只经 `internal/application` 调用 public `sdk`；业务调度不留在
  `internal/bootstrap`。
- 公开 SDK 不读取本地 config、环境变量、authdb 或浏览器，也不增加 logger/debug writer。
- FANBOX 始终先走 native Chrome 146 TLS transport（内置 Firefox 148 HTTP User-Agent baseline）；配置 FlareSolverr 不等于全量转发。
- FlareSolverr 不接收 `FANBOXSESSID`、API/帖子/文件 URL、请求 body 或下载内容。
- `config.toml` 的账号池只保留 `enabled` 与 `strategy`；账号调度字段全部以 authdb 为
  authority。
- Pixiv/FANBOX proxy、FANBOX UA、solver service URL 和 solver upstream proxy 保持各自的显式边界；
  不增加代理池。
- 参数解析先于 config、数据库、启动清理、桌面 handler、network、文件和 diagnostic scope。
- debug 默认关闭，只实时写 stderr；MCP stdout 始终为纯 JSON-RPC，`auth export` 与隐藏 callback
  保持既有静默契约。
- 错误、诊断、测试失败和 release evidence 都不得包含 token、session、Cookie、signed query、
  proxy userinfo、solver response body 或 clearance value。
- 本文不授权自动 stage、commit、push 或清理用户现有改动；Git 操作由实施会话另行确认。

## 3. 执行顺序

| Phase | 目标 | 依赖 |
|---|---|---|
| RC-0 | 固定 follow-up 基线与失败面 | 无 |
| RC-1 | public error `Reason` 命名收口 | RC-0 |
| RC-2 | 严格 unknown-option 与无副作用解析 | RC-1 |
| RC-3 | authdb schema、路径与 credential CAS | RC-1 |
| RC-4 | legacy migration 与 default 跨文件语义 | RC-3 |
| RC-5 | 数据库账号调度与安全重放 | RC-3、RC-4 |
| RC-6 | service-scoped network 与 FANBOX UA | RC-1 |
| RC-7 | FANBOX 正确 route/profile/resource 主链路 | RC-6 |
| RC-8 | challenge-only FlareSolverr recovery | RC-7 |
| RC-9 | typed debug diagnostics | RC-2、RC-5、RC-8 |
| RC-10 | CLI/MCP、文档与 public inventory 收口 | RC-1～RC-9 |
| RC-11 | 全量回归与本机 release evidence | RC-10 |

`RC-2`、`RC-3` 与 `RC-6` 在代码依赖上可以分别开发，但共享 worktree 中仍按表中顺序落地和
回归，避免同时修改 bootstrap/config/CLI 造成难以审查的混合 diff。每个 phase 完成后先看 Go LSP
diagnostics，再运行聚焦测试；不把测试集中拖到最后。

## 4. RC-0：固定 follow-up 基线

### 修改范围

本 phase 不改产品行为。只核对当前分支、dirty worktree、LSP、相关 package 和既有测试基线，并记录
与本轮无关的现存失败。

### 步骤

1. 确认当前工作目录是既有 `codex/v1-sdk-rewrite` worktree，不创建或切换到新的历史重写分支。
2. 使用 Go LSP 核对 `sdk.Error`、authdb repository、account-pool 调用方、CLI root command、
   FANBOX Session/Client、两个 MCP server 的定义、引用与 diagnostics。
3. 用源码 inventory 明确当前 delta：
   - `sdk.Code`、`Error.Code`、`CodeOf`、`IsCode` 和 `Code*` 常量仍存在；
   - bootstrap 仍忽略 legacy migration error，pool use case 仍位于 bootstrap；
   - `account_pool.accounts` 仍进入 runtime；
   - FANBOX 仍使用 `/api/v1/` 与 Chrome 146；
   - `sdk/fanbox.Options` 尚无 UA/FlareSolverr；
   - CLI 仍输出 Cobra 的 `unknown flag`，且启动副作用发生在解析前；
   - `internal/diagnostics` 尚不存在。
4. 运行下列基线，不因 unrelated dirty change 自动修复其他模块：

~~~bash
go test ./sdk/... ./internal/persistence/authdb ./internal/application/... +  ./internal/services/fanbox ./internal/bootstrap ./internal/cli +  ./internal/mcpserver/... -count=1
go test ./scripts/documentation -count=1
git diff --check
~~~

### 完成检查点

- 基线结果和已有失败可区分；没有把旧计划重新标为未完成。
- 后续每个 failing test 都能归到一个明确 RC phase。

## 5. RC-1：public error `Reason` 命名收口

### 修改范围

- `sdk/error.go`、`sdk/error_test.go`；
- `sdk`、`sdk/pixiv`、`sdk/fanbox` 中全部构造、判断与 GoDoc；
- `internal`、`e2e`、documentation public inventory 中的调用方和测试。

### 步骤

1. 在一个可编译批次中完成以下破坏性改名：

~~~go
type Reason string

type Error struct {
	Product   string
	Operation string
	Reason    Reason
	// 其余稳定字段不变。
}

func ReasonOf(err error) Reason
func IsReason(err error, reason Reason) bool
~~~

2. 常量改为 `InvalidArgument`、`InvalidCursor`、`Unauthorized`、
   `CredentialsExpired`、`Forbidden`、`NotFound`、`ContentUnavailable`、
   `ChallengeRequired`、`RateLimited`、`UpstreamError`、`UpstreamUnavailable`、
   `MalformedUpstreamResponse`、`ResourceForbidden`、`LocalStateError` 与
   `RemovedSetting`。字符串值保持原样。
3. `NewError` 接收 `Reason`；`Error.Error`、`Error.Is`、`errors.Is/As` 与空值语义只改
   字段名，不改变分类行为。
4. 删除 `Code`、`Error.Code`、`CodeOf`、`IsCode` 和全部 `Code*` symbol；v1.0.0 尚未
   发布，不增加 deprecated alias 或 compatibility shim。
5. 更新所有公开英文 GoDoc。solver、账号池和 CAS 不新增专用 Reason；受控子分类继续放在
   `Detail`。
6. 更新 public API inventory，使它既要求新 symbol，也显式拒绝旧 symbol。

### 聚焦测试

- `ReasonOf`/`IsReason`、Reason-only sentinel、wrapped/context cause、retry advice、稳定字符串与
  safe formatting。
- `sdk/pixiv`、`sdk/fanbox` 全部旧分类测试迁移后语义不变。
- go/ast inventory 证明生产 Go 源码不再导出或引用旧命名。

~~~bash
go test ./sdk/... -count=1
go test ./scripts/documentation -run 'PublicAPI|GoDoc' -count=1
go test ./internal/application/... ./internal/services/... ./internal/mcpserver/... -count=1
~~~

### 完成检查点

- wire value 与 CLI/MCP stable reason 没有变化。
- 生产源码无旧 public error symbol；历史文档中的迁移说明可以保留旧名称。
- Go LSP diagnostics 为空。

## 6. RC-2：严格 unknown-option 与无副作用解析

### 修改范围

- `internal/cli/cli.go`；
- `internal/cli/cli_test.go`、`cli_startup_test.go`、`api_test.go`、FANBOX/MCP 聚焦测试；
- 仅在需要统一测试注入点时增加小型 CLI parser helper 文件。

### 步骤

1. 在 root command 上安装统一 flag error normalizer。使用 pflag v1.0.9 的
   `*pflag.NotExistError`、`GetSpecifiedName` 与 `GetSpecifiedShortnames` 做 typed
   classification；不能解析完整上游英文错误字符串。
2. long option 规范化为 `unknown option '--name'`；`--name=value` 只显示 name。short/组合
   shorthand 只显示解析到的第一个未知项，如 `unknown option '-x'`。
3. 只把 unknown option 包装为现有 `usageError`：stdout 为空、stderr 恰为一行
   `error: unknown option '...'`、exit code 为 `2`。known option 缺值/非法值、互斥错误、unknown
   command 与 Args usage 保持各自既有分类和文本。
4. 把 pending update cleanup、persistent URL handler 初始化和 config initialization 移到 Cobra
   成功完成 command/flag/Args 解析之后。按解析出的 `CommandPath` 排除 `auth export` 与隐藏
   callback/installer，不再依赖解析前扫描 raw argv。
5. 保留 pflag interspersed option；`--` 后的 token 不经过 unknown-option normalizer，由目标
   command 的 Args contract 处理。
6. 删除或收缩只为解析前 startup 猜测存在的 `scanRootBooleanFlags` 路径；help/version 和重复 bool
   flag 继续由 Cobra/pflag 决定。
7. 保证此时尚未创建 debug presenter；后续 RC-9 接入 `--debug` 后复用同一门禁。

### 聚焦测试

- root、Pixiv command、FANBOX command、`pixiv mcp` 与 `pixiv fanbox mcp`。
- unknown long、`--name=value`、single short、combined shorthand、位置参数前后和 `--` literal。
- exact stderr、empty stdout、exit `2`；所有旧 `unknown flag:` 断言一次性更新。
- 用注入计数器证明 parse failure 未触发 update cleanup、handler、config、services、network、文件或
  MCP runtime。
- known flag value error 与 protected auth export/callback 的既有静默契约回归。

~~~bash
go test ./internal/cli -run 'Unknown|Option|Startup|AuthExport|Callback|MCP' -count=1
go test ./internal/cli -count=1
~~~

### 完成检查点

- 解析错误只出现一次且没有业务副作用。
- `--` 和 unknown command 没有被误分类。
- CLI 无 per-command allowlist、fuzzy suggestion 或 hidden passthrough。

## 7. RC-3：authdb schema、路径与 credential CAS

### 修改范围

- `internal/persistence/authdb/migrations/0002_fanbox_creator_id_not_null.sql`；
- `internal/persistence/authdb/migrations/0003_pixiv_account_schedulable.sql`；
- `database.go`、`migrate.go`、`repository.go` 及聚焦 tests；
- Pixiv/FANBOX application credential 调用方。

### 步骤

1. 不改已经嵌入 ledger 的 `0001_initial.sql`。新增 `0002`，以显式 table rebuild 把
   `fanbox_account.creator_id` 的既有 NULL 规范化为 `TEXT NOT NULL DEFAULT ''`，保留账号、
   `sort_order`、revision、时间和 migration ledger。
2. 新增 `0003`，只给 `pixiv_account` 增加
   `schedulable INTEGER NOT NULL DEFAULT 1 CHECK (schedulable IN (0, 1))`。保留已有 marker partial
   unique index，不建立 membership/lease/status table。
3. 修复 SQLite DSN 构造，正确编码空格、Unicode、`?`、`#`、`%` 与 Windows-like path；pragma
   仍由受控参数构造，不把用户路径直接拼成 URI query。
4. 把通用 credential upsert 拆成意图明确的 repository operation：
   - initial insert 固定 revision `1`；
   - 已验证 re-import/replacement 在 SQL 内按当前 revision `+1`；
   - Pixiv rotation 和 FANBOX session rotation 接收 expected revision，以 affected rows 判断 CAS；
   - premium cache、display name、pool freeze/marker/schedulable 等 metadata update 不改变 revision。
5. 增加内部 conflict sentinel。stale rotation 由 application 重新读取并关闭刚创建的新 Client，不把
   conflict 伪装成功，也不增加 public Reason。
6. repository 入口继续拒绝空 credential；FANBOX rotation 同时验证合法 `validated_at`。不增加
   credential 长度、TTL 或上游格式 SQL CHECK。
7. 修复 migration test 的 `application_id` variable shadow。
8. Unix-like 权限测试继续检查私有目录/文件；Windows 只验证位于 user profile 并继承既有 ACL，不
   声称 runtime 主动收紧 DACL。

### 聚焦测试

- 空 DB 与只含 `0001` 的 DB 升到 `0003`；旧 checksum 不变。
- NULL creator migration、空字符串 round-trip、旧/新 Pixiv 行默认 `schedulable=1`、非法 bool
  被 CHECK 拒绝。
- 特殊字符、Unicode 与 Windows-like DB path。
- initial/re-import revision 单调、stale expected revision、并发 rotation、metadata 不改 revision；
  stale 后不继续内容请求。
- 真实 `application_id` 断言、wrong ID/version/checksum fail closed。

~~~bash
go test ./internal/persistence/authdb -count=1
go test ./internal/application/pixiv ./internal/application/fanbox -count=1
go test -race ./internal/persistence/authdb ./internal/application/pixiv ./internal/application/fanbox -count=1
~~~

### 完成检查点

- `CurrentVersion()` 与 migration ledger 为 3，且旧开发 DB 可 forward-only 升级。
- credential revision 不回退、不丢更新；metadata 与 credential mutation 已分离。
- 合法本地路径不会被 SQLite URI 误解析。

## 8. RC-4：legacy migration 与 default 跨文件语义

### 修改范围

- `internal/persistence/authdb/legacy.go` 及 tests；
- `internal/bootstrap/bootstrap.go`、`fanbox.go` 及 tests；
- `internal/application/pixiv/service.go`、`fanbox/service.go`；
- config store 的显式 default 写入/删除调用点。

### 步骤

1. 把 migration 拆成“先读完整 legacy JSON 并验证”与“在已打开 final DB 上导入/比较”两个阶段。
   bootstrap 的唯一顺序为：

~~~text
validate JSON
→ open final DB and apply schema migration
→ one credential transaction or full logical compare
→ write default config
→ remove legacy JSON
~~~

2. 全部 legacy Pixiv 账号在同一个 SQLite 写事务中导入并完成逻辑比较；任一账号失败全部 rollback。
   不用 private DB replacement 覆盖同库中可能已有的 FANBOX 行。
3. 扩展 migration result，使成功和失败都能准确报告
   `credentials_committed`、`default_written`、`legacy_removed`。DB 与 config/JSON 不伪称跨文件
   原子；重入时完整比较 identity、顺序、credential 与 premium cache，一致才继续 config/清理。
4. bootstrap/service factory 返回初始化错误，不再以 `_, _ = ...`、nil service 或空结果隐藏
   malformed JSON、DB、config 或 remove failure。CLI/MCP 在业务调用前得到真实 local-state error。
5. Pixiv/FANBOX 删除账号前先读显式 default；目标账号仍是 explicit default 时拒绝删除，提示先
   `auth use UID` 或 `auth use --auto`。不能先删 DB 再 best-effort 清 config。
6. config write/unset error 全部返回。FANBOX auto mode 的 list/status 以最小 `sort_order` 账号标记
   为真实 default，与 Pixiv 语义一致。

### 聚焦测试

- 第二账号失败时零账号提交；同一输入重跑成功。
- DB 已提交后的 config write failure、legacy remove failure 与下一次重入 outcome。
- DB 与 JSON 不一致 fail closed；bootstrap 将错误传到 CLI/MCP 且不泄露 token。
- explicit default 删除被拒绝且 DB/config 均不变；auto default 删除与 list/status 标记正确。
- config failure 不被吞掉。

~~~bash
go test ./internal/persistence/authdb -run 'Legacy|Migration' -count=1
go test ./internal/application/pixiv ./internal/application/fanbox ./internal/bootstrap -count=1
~~~

### 完成检查点

- legacy migration 任意单步失败都有真实、可重入 outcome。
- 不存在启动层忽略 authdb/config 错误的路径。
- 默认账号的 DB/config 跨文件边界与专题规格一致。

## 9. RC-5：数据库账号调度与安全重放

### 修改范围

- `internal/application/config/settings.go`、account-pool/config tests；
- `internal/persistence/authdb/repository.go` 与 tests；
- `internal/application/account_pool.go`、account service/types 与 tests；
- `internal/bootstrap/bootstrap.go` 仅保留 production wiring；
- `internal/cli/account_cmd.go` 与 tests；
- 删除不再有生产引用的旧 `storage/accountpool` JSON state 实现及 tests；当前实现已迁移到
  `internal/persistence/authdb` 的账号行状态，旧 JSON 不再作为运行时 fallback。

### 步骤

1. `AccountPoolConfig` 只保留 `Enabled` 与 `Strategy`；增加
   `account_pool_enabled`、`account_pool_strategy` alias，移除运行时
   `account_pool_accounts` alias/allowlist。
2. 首次发现 raw `account_pool.accounts` 时执行可重入 data migration：
   - 完整解析旧 UID 列表；
   - 一个 DB 事务中先把现有 Pixiv 账号全部设为不可调度，再启用列表中仍存在的账号；
   - DB commit 后原子重写 config，只移除 `accounts`；
   - config write 失败时阻止 pool 数据命令和 pool 管理命令，下一次重做映射并再次写 config。
   没有旧 key 时不运行该迁移，既有/新账号都由 schema/default 保持可调度。
3. repository 提供事务化的 status、批量 enable/disable、freeze 和 select；批量操作先验证所有 UID，
   任一不存在则零提交。
4. 选择结果明确区分：无本地账号、无 schedulable 账号、全部冻结、存在 eligible。全部冻结同时返回
   最早的未来 `pool_frozen_until`；数据库读取失败不能猜 RetryAdvice。
5. `round_robin` 以 marker 账号的 `sort_order` 为游标，从下一 eligible 账号开始并 wrap；marker
   不 eligible 仍保留位置，marker 被删时从最小 eligible 开始。禁止旧
   `ORDER BY pool_last_selected ASC, sort_order ASC`。
6. `random` 只从 eligible 集合选择并更新同一 marker；随机源可测试注入，不改变 schedulable/freeze。
7. 把账号池 use case 从 bootstrap 移入 `internal/application`。一次 operation 维护 attempted set；
   只有 `RateLimited`、有效 `Retry-After`、尚未输出/落盘、context 未取消时才冻结并选择下一账号。
   次数由当前 eligible 集合自然结束，不增加固定 retry count。
8. 映射专题文档定义的四组 Reason/Detail/RetryAdvice；耗尽时保留最后一个上游
   `RateLimited` chain。不得 fallback 到 default、未调度账号、环境 token、Web API 或 FANBOX。
9. 增加：

~~~text
pixiv auth pool status [--json]
pixiv auth pool enable UID... [--all]
pixiv auth pool disable UID... [--all]
~~~

   `auth list` 同步显示非 secret 调度摘要。status 的 `eligible` 只表示当前 schedulable 且未冻结。
10. 删除未使用的独立 account-pool JSON scheduler；不自动读取、迁移或删除用户磁盘上的历史
    `data/account-pool.json`，migration guide 只说明它不再是 authority 和可选手工清理方式。

### 聚焦测试

- 3–5 账号多轮 round-robin、wrap、marker frozen/disabled/deleted 与多进程事务竞争。
- random 候选与切回 round-robin marker。
- 旧 accounts 迁移、缺失账号、config rewrite failure/re-entry、新账号默认可调度。
- enable/disable UID 或 `--all` 的互斥、未知 UID 和批量原子性。
- 仅有效 429/Retry-After 且未 commit 时切换；普通错误、取消、已输出/已落盘不重放。
- 四种 exhaustion 的 application、CLI、MCP stable reason/detail/retry 和无 partial output。
- status/list/JSON/错误不含 refresh token。

~~~bash
go test ./internal/application/config ./internal/persistence/authdb ./internal/application -run 'AccountPool|Pool|Schedul' -count=1
go test ./internal/cli ./internal/mcpserver/... -run 'Pool|Account|RateLimit' -count=1
go test -race ./internal/persistence/authdb ./internal/application -run 'Pool|Schedul' -count=1
~~~

### 完成检查点

- config 中不再有运行时 UID allowlist，数据库是唯一调度 authority。
- bootstrap 只组装 repository/application，不包含选择循环。
- 三个以上账号无饥饿，且 debug 尚未接入时行为已稳定。

## 10. RC-6：service-scoped network 与 FANBOX UA

### 修改范围

- `internal/application/config/settings.go` 与 config tests；
- `internal/application/runtime.go` 的 request/config value；
- `internal/bootstrap` 的 Pixiv、FANBOX、MCP 组装；
- `sdk/fanbox/client.go` 与 `internal/services/fanbox/session.go` 的 UA/proxy option；
- CLI proxy tests。

### 步骤

1. runtime config 以显式 presence + value 类型保留 service proxy 的 absent/empty/value 三态；不能在
   装配前压成普通 string 零值。
2. 分别实现 Pixiv/FANBOX native 优先级：

~~~text
当前命令 --no-proxy / --proxy
> 对应 service proxy（含显式空值）
> https_proxy / HTTPS_PROXY
> [network].https_proxy
> direct
~~~

3. Pixiv 继续接受 `http`、`https`、`socks5`、`socks5h`；FANBOX native 只接受无 userinfo
   的 `http`/`https` CONNECT。无效显式值直接失败，不继承下一层。
4. `pixiv mcp` 只消费 Pixiv 路由，`pixiv fanbox mcp` 只消费 FANBOX 路由；update 等非产品请求
   只用通用 fallback。FANBOX command 的 `--proxy`/`--no-proxy` 绝不覆盖 solver service/upstream。
5. 给 `sdk/fanbox.Options` 增加 `UserAgent`。空值使用内置 baseline；非空值只改 native header，
   拒绝 CR/LF/NUL，不加长度限制，不改变 TLS profile。
6. 增加 `FlareSolverrOptions{URL, ProxyURL}` 的配置和 public option shape，但本 phase 只完成严格
   parse/validation/wiring；`nil` 仍表示关闭，不发起 solver 请求。
7. solver service root、solver upstream proxy 与 native proxy 分开验证。control client 预留 direct、
   不读 ambient proxy 的组装点；不增加 `service_proxy_url`。
8. 默认 config generator 不创建可选 `[pixiv.network]`、`[fanbox.network]` 或
   `[fanbox.flaresolverr]` table；用户需要时按文档显式配置。

### 聚焦测试

- 两产品完整 proxy precedence matrix、absent/empty/value、命令 override。
- 同一 global SOCKS 值对 Pixiv 合法、对 FANBOX 明确失败；FANBOX 可用显式空 key 选择 direct。
- Pixiv/FANBOX/MCP/update 不跨域继承。
- config UA 只改变 FANBOX native header；非法 header 在请求前失败。
- native proxy、service URL、solver proxy 三值互不继承；错误不回显 userinfo/query。

~~~bash
go test ./internal/application/config ./internal/bootstrap ./sdk/fanbox ./internal/services/fanbox -run 'Proxy|Network|UserAgent|FlareSolverr' -count=1
go test ./internal/cli -run 'Proxy|MCP|Fanbox' -count=1
~~~

### 完成检查点

- runtime 能区分 service key 缺失与显式空字符串。
- 未配置 solver 时仍是零外部 solver dependency。
- 自定义 UA 没有被描述或实现成 Cloudflare bypass 保证。

## 11. RC-7：FANBOX 正确 route/profile/resource 主链路

### 修改范围

- `internal/services/fanbox/api.go`、`session.go`、DTO 和 tests；
- `sdk/fanbox/client.go`、operations/resource tests；
- `internal/application/fanbox`、CLI/MCP FANBOX fixture tests；
- `e2e` 的 public FANBOX SDK 入口只补代码，不在普通测试中联网。

### 步骤

1. 把 API root 修正为 `https://api.fanbox.cc/`，用已验证 operation 替换旧路由：
   `post.listHome`、`post.listSupporting`、`post.listTagged`、
   `tag.getFeatured`、`post.info` 等；`post.listCreator` 带上游要求的显式 `limit`，
   creator pagination 使用 `post.paginateCreator` 返回的 `pageUrls`。
2. 在 fixture 中先固定每个 public operation 的 path、query、response envelope 和 pagination binding。
   `limit` 必须来自已捕获的上游 contract/evidence并写代码注释，不凭空增加本地页数限制。
3. 更新 DTO/map 以匹配真实 body shape；未知 block/embed 继续保留既有安全语义，不以空结果掩盖
   malformed response。
4. 将 production tls-client profile 从 Firefox 148 改为 Chrome 146；同步 transport/type/comment 与
   内置 baseline UA，删除会误导为 Chrome 的命名。调用方 context 继续负责取消，不加内部总 deadline。
5. challenge 检测改为专题文档的严格信号。对 body marker 使用流式匹配并丢弃内容，不保留/记录 raw
   body，也不沿用无依据的固定字节截断。
6. 修正资源 credential 传播：同一 `FANBOXSESSID` 只可发给经严格校验、确实需要认证的 FANBOX API/
   `downloads.fanbox.cc` host；Pixiv image/第三方 host 不带 session。每次 redirect 都重新校验
   scheme、userinfo、host 和 credential policy，不能把首跳 Cookie 盲传。
7. 保持 public `Resource`/`ResourceRef`、signed-query redaction、GET/HEAD/Range/conditional read、
   atomic save 与 response-header allowlist 契约。
8. public SDK 构造器仍不联网；native 成功、普通 403/401/404 和 malformed JSON 先在 offline tests
   稳定分类。

### 聚焦测试

- 每个 public FANBOX operation 的 canonical path/query/envelope/pagination fixture。
- Chrome 146 TLS profile 与 config/built-in HTTP UA header。
- `post.info` article/image/file 映射；从详情得到 file attachment ResourceRef。
- session 只传播到允许 host；`downloads.fanbox.cc` 有 session 成功、无 session 对照失败 fixture；
  redirect 到不允许 host 时 Cookie 被移除或请求被拒绝。
- 普通 403 不误判 challenge，401/session envelope 为 `CredentialsExpired`。

~~~bash
go test ./internal/services/fanbox ./sdk/fanbox -count=1
go test ./internal/application/fanbox ./internal/cli ./internal/mcpserver/fanbox -run 'Fanbox|FANBOX|Resource' -count=1
~~~

### 完成检查点

- 生产源码和 tests 不再请求旧 `/api/v1/`/旧 operation。
- native post/resource 闭环完全不依赖 solver。
- 未运行真实 E2E 前只声明 offline implementation 通过，不升级 release evidence。

## 12. RC-8：challenge-only FlareSolverr recovery

### 修改范围

- `sdk/fanbox.Options` 与 error mapping；
- `internal/services/fanbox` 新增聚焦的 solver control/state/coordinator 文件及 tests；
- bootstrap FANBOX option wiring；
- fake service/native transport contract tests。

### 步骤

1. `FlareSolverr == nil` 保持零调用。只有 native response 满足严格 challenge 识别才进入 recovery；
   普通成功、普通 403、401、网络错误、JSON error 均不调用 solver。
2. control client 使用独立 direct transport，不读取 ambient/native/solver-upstream proxy；规范化
   absolute root 后只调用 `/v1`，命令固定为 `request.get`，目标固定为
   `https://www.fanbox.cc/`。
3. 请求只可携带显式 solver upstream proxy；不得携带 session、原 API/文件 URL、body 或其他 Cookie。
4. 解析 solver success 时要求非空合法 UA，以及恰好一个非空、合法的 `cf_clearance`；重复 clearance
   或非法 byte 返回 `MalformedUpstreamResponse`。其余 Cookie 全部丢弃。
5. state 只存在单个 Client 内存中。solver UA 优先于 config/built-in UA；有 expiry 时使用上游 expiry，
   无 expiry 时不臆造 TTL，下一 challenge 或 Client 回收时失效。
6. 原请求只允许一次 fresh solve 和一次 native replay；replay 再 challenge 返回
   `ChallengeRequired`。该上限直接来自防止同一请求无限 recovery 的已批准协议边界，代码注释必须
   写明。
7. 同 Client 并发 challenge 合并为一次 solve。coordinator 使用独立 context 和 waiter ownership：
   单个 waiter 取消不影响其他 waiter；最后一个 waiter 取消时取消 solver；全员取消竞态结果不缓存。
8. 保留错误 chain 中的 caller cancel/deadline；service connection 为 `UpstreamUnavailable`，
   solver fail/未配置/replay challenge 为 `ChallengeRequired`，非法 response 为
   `MalformedUpstreamResponse`。不增加 solver Reason。
9. 不创建 FlareSolverr session、Cookie jar/pool、后台 goroutine、自动 retry、固定 timeout、默认 URL、
   Docker 管理或 full forwarding。

### 聚焦测试

- native success 时 solver 零调用。
- synthetic challenge → anonymous homepage solve → clearance/UA-only state → native replay success。
- 普通 403、未配置、service unavailable、solver failure、malformed response、replay challenge。
- root URL path/query/userinfo/fragment matrix、`/v1` 构造与错误脱敏。
- config/native/solver UA 优先级，三类 proxy 不继承。
- duplicate/invalid clearance、invalid UA、其他 Cookie 被丢弃；失败不 cache、不 replay。
- concurrent leader/follower cancel、all canceled、single solve、无 state/goroutine 残留。

~~~bash
go test ./internal/services/fanbox ./sdk/fanbox -run 'Challenge|FlareSolverr|Solver|Replay|UserAgent' -count=1
go test -race ./internal/services/fanbox ./sdk/fanbox -run 'Challenge|FlareSolverr|Solver|Replay' -count=1
go test ./internal/bootstrap ./internal/application/fanbox -run 'FlareSolverr|Fanbox' -count=1
~~~

### 完成检查点

- 配置 solver 不会让正常 API/文件请求经过 solver。
- solver 从未接收用户 session 或业务 URL。
- ordinary CI 只依赖 fake service/transport，不新增 Docker/service topology。

## 13. RC-9：typed debug diagnostics

### 修改范围

- 新增 `internal/diagnostics` 的 event/sink/scope/presenter 及 tests；
- `internal/cli` root flag、operation scope 和 writer failure 汇总；
- application/account pool、authdb/config adapter、Pixiv/FANBOX network、solver、download 的 typed
  event 发射点；
- `internal/mcpserver` 与 FANBOX 子 server 的 tool wrapper/request counter；
- bootstrap 只负责 presenter/sink 生产组装。

### 步骤

1. 建立封闭 typed event contract，不接受任意 map、raw header/body、预格式化 transport dump 或
   package-level logger。关闭时使用 no-op sink。
2. Scope 通过 context 绑定 sink、业务域、adapter 和可选 MCP request number。公开 SDK 签名不变，
   外部 caller 的 context 没有内部 scope 时保持安静。
3. presenter 以 mutex 保护整行写入，注入 clock，并渲染：

~~~text
[Module name] HH:MM:SS Complete English sentence.
~~~

   module 只能来自专题文档锁定的明确产品+子系统枚举；正文不用 `debug` prefix、level 或
   `key=value`。
4. root 增加唯一 persistent `--debug`。严格解析成功后，普通 CLI command 才创建 scope；
   `auth export`（含 `--output`）和隐藏 callback/installer 不创建 scope。
5. 两个 MCP server 各自维护进程内单调 request number。统一 tool registration wrapper 在 handler
   context 上创建 scope并报告 start/end；不暴露 JSON-RPC ID，不改变 tool schema/result。
6. 业务层只发送安全字段：config source、候选/选择/freeze、route、sanitized proxy、实际 UA、HTTP
   status、challenge/solve/replay、资源数量、目标路径和受控错误原因。
7. 事件构造阶段即禁止 token、session、Cookie、Authorization、raw header/body、signed query、
   proxy userinfo、solver solution、raw argv 和 browser profile content；transport error 先安全分类。
8. presenter 保留第一次 stderr write error。已开始业务 operation 继续完成：
   - CLI 成功时最终因诊断通道失败返回非零；
   - CLI 业务也失败时业务 error 仍为 primary，并附带 channel failure；
   - MCP 当前 tool result/stdout 不被改写，response boundary 后把 writer failure 交给 runtime 终止路径。
   不重试、不切文件、不静默吞掉。
9. instrumentation 只观察已稳定的 RC-5/RC-7/RC-8 路由；debug on/off 共享同一 use case 和 transport，
   不增加分支式 fallback。

### 聚焦测试

- no flag 时零事件、不创建文件；flag 时只写 stderr。
- fixed clock 的少量完整英文 sentence 与明确 module；不维护整段大 golden。
- Pixiv/FANBOX CLI 和两个 MCP server 不混名；并发 MCP request number 可关联且行不交错。
- MCP stdout byte stream 仍是合法纯 JSON-RPC；tool structured failure/`isError` 不变。
- secret/signed query/proxy userinfo/clearance 不能出现在 buffer 或 failure diff。
- `auth export --debug` byte-for-byte stdout 和 empty stderr；unknown option + `--debug` 只有 usage
  error。
- failing writer 不取消业务，CLI/MCP 按既定 runtime contract 暴露。
- debug on/off 的账号选择、proxy/UA、solver call count、replay、下载 commit 和 primary error 相同。

~~~bash
go test ./internal/diagnostics -count=1
go test ./internal/cli -run 'Debug|Unknown|AuthExport|Writer' -count=1
go test ./internal/mcpserver/... -run 'Debug|Diagnostics|Stdout|Writer' -count=1
go test ./internal/application/... ./internal/services/fanbox -run 'Debug|Diagnostics|Pool|Challenge' -count=1
go test -race ./internal/diagnostics ./internal/mcpserver/... -run 'Debug|Diagnostics' -count=1
~~~

### 完成检查点

- 默认没有日志文件、logs directory、旧 logging config 或 SDK logger API。
- 诊断能说明成功和失败路径，但不改变任何业务选择。
- stderr writer failure 不被吞掉，也不污染 MCP stdout。

## 14. RC-10：CLI/MCP、文档与 public inventory 收口

### 修改范围

- CLI/MCP cross-product integration tests；
- 三语 README 与 `docs/en|zh-CN|ja/cli-reference.md`；
- `docs/en|zh-CN/sdk.md`、`docs/en|zh-CN/mcp-tools.md`；
- `docs/maintainers/architecture.md`、`development.md`、`CONTEXT.md`、v1 migration guide；
- `skills/pixiv-cli/`；
- PR release-note 声明与 release-prep 入口。

### 步骤

1. 统一检查 root/子命令 flag inheritance、两个 MCP 启动命令、pool CLI、FANBOX config 和下载路径；
   Pixiv/FANBOX registry、auth、proxy 和 diagnostic module 不交叉。
2. 更新英文 canonical public docs，再同步简中与日文 CLI reference。README 只保留必要入口，不复制
   完整配置矩阵。
3. 文档明确：
   - public error 新命名与旧 symbol migration；
   - config 只存 pool enabled/strategy，调度由 `auth pool` 管理；
   - service proxy 三态与 FANBOX UA；
   - FlareSolverr 显式、默认关闭、challenge-only，service/upstream 两地址和 Docker/外部部署；
   - `--debug` 只写 stderr、无 logs directory，`auth export` 例外；
   - unknown option exact error 与 exit `2`。
4. default config/sample 不生成外部 solver/network table；只展示用户主动配置示例，不暗示必须安装
   FlareSolverr。
5. 同步 product Skill 的命令、flag、错误、账号池、代理和 debug troubleshooting。
6. public inventory/golden 加入 `Reason`、FANBOX Options，拒绝旧 Code API；MCP inventory 不因 debug
   增加 tool/schema。
7. 按 PR template 记录用户可感知的 Added/Changed/Fixed；正式 changelog 仍由 release-prep 流程生成，
   不在功能 phase 手工伪造已发布版本。
8. 更新本文与各专题状态为“已实施/待 evidence”时只依据真实测试结果；旧
   `implementation-plan.md` 保持历史状态。

### 聚焦测试

~~~bash
go test ./scripts/documentation -count=1
go test ./internal/cli ./internal/mcpserver/... -count=1
git diff --check
~~~

人工检查三语示例的 command/flag、TOML key、stdout/stderr 与当前 binary help 一致；文档不得包含
credential、用户帖子正文、密码、signed URL 或本机绝对配置路径。

### 完成检查点

- public contract、product Skill 与实现一致，无提前宣传或历史计划重复。
- MCP tool inventory/schema 未因 diagnostics 或 solver 改变。
- 文档中的 FlareSolverr 仍是可选恢复路线，不是成功保证。

## 15. RC-11：全量回归与本机 release evidence

### 自动门禁

先看全部目标 Go package 的 LSP diagnostics，再执行：

~~~bash
go test ./...
go test -race ./...
go vet ./...
sh scripts/build.sh
go test ./scripts/documentation -count=1
pre-commit run --all-files
git diff --check
~~~

若本机缺少 `pre-commit` 或其他门禁依赖，报告准确缺口并先征求安装授权；不能静默跳过或擅自安装。
普通 CI 不增加真实 FANBOX credential、FlareSolverr container 或外部 proxy。

### 真实 Pixiv/FANBOX SDK evidence

1. 按 [最终验证操作手册](release-prep-runbook.md) 使用本机受控 credential，逐项运行 public Pixiv
   read 与每个 public FANBOX operation。
2. FANBOX 必须通过修复后的 public `sdk/fanbox` 取得合法 `post.info`，从详情发现真实 file
   attachment，并读取非零字节；手工 HTTP probe、cover 或 solver 页面不能替代。
3. session/token 只在进程内存/受控 Keychain/repository 边界出现，不进入 argv、env dump、日志、
   test name、artifact 或失败 diff。
4. 真实 E2E 失败保留为 release blocker；不能用 offline fixture 伪装成功。

### 一次性 real-solver protocol acceptance

FlareSolverr production recovery 实现完成后，在 v1.0.0 发布前只执行一次：

1. 使用已锁定的 `ghcr.io/flaresolverr/flaresolverr:v3.5.0` 与固定 digest，本机 loopback/隔离网络
   启动；验证拓扑需要代理时显式传入 solver upstream proxy，direct 时也记录该选择，不让 SDK
   猜测宿主机/容器地址。
2. Client 使用非 secret dummy session和 injected native transport：首次返回 synthetic challenge，
   真实 solver 匿名访问 FANBOX 首页，随后 injected native replay 验证只收到 solver UA 与一个
   clearance。
3. 检查 debug 仅显示 route/status/solve/replay，MCP/CLI stdout 不受影响，solver 请求和日志没有
   dummy session、业务 URL、Cookie value 或 response body。
4. 记录 image tag/digest、配置拓扑、测试命令、结果和清理状态；删除容器、临时重定向日志和 probe。
5. 该项不是 ordinary CI，也不要求每个 RC 重复。只有真实网络自然产生 challenge 时才 best-effort
   增补 genuine recovery evidence；没有自然 challenge 不算失败。

### 最终检查点

- authdb、账号调度、native FANBOX、solver recovery、strict parsing 与 debug 的自动门禁全部通过。
- 一次性 synthetic real-solver protocol acceptance 已有脱敏 evidence（固定 image digest，2026-08-08）。
- 真实 public Pixiv SDK E2E 与一次性 real-solver acceptance 已满足对应 evidence gate；用户已提供
  `ro7274/12373249` 及作者索引，真实 FANBOX SDK 已覆盖到详情阶段，但当前授权 session 下没有可验证
  的 file attachment，因此 file-resource evidence 仍待新的可读 target 或受控网络/权限状态。
- 仍未完成的 browser native profile evidence 或 API freeze 项继续按
  [测试、迁移与发布门禁](verification-release.md)列为 blocker，不因本计划结束而自动通过。

## 16. 停止条件

出现以下任一情况时停止当前 phase 并显露真因：

- schema migration checksum、已有数据或跨文件 outcome 无法按已批准顺序安全处理；
- 实现需要新增未批准的 public Reason、config key、环境变量、timeout、retry、Cookie/代理池或
  FlareSolverr full forwarding；
- FANBOX route/profile evidence 与已记录 contract 冲突；
- strict parser 只能靠 raw argv value 或完整上游英文错误字符串泄露输入；
- debug 无法保持 MCP stdout、`auth export` 或 secret boundary；
- 聚焦测试出现与本 phase 无关且无法隔离的 dirty-worktree 冲突；
- 真实凭据、联网或本机容器验证尚未获得明确授权。

停止不等于把错误吞掉或标记成功。应记录已完成 checkpoint、失败命令、脱敏原因、未运行门禁与下一步
所需授权。
