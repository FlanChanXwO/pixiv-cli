# v1.0.0 Pixiv 账号调度

状态：设计已确认，RC-5 已实施并通过聚焦回归。确认日期：2026-08-04；实施记录：2026-08-08。

## 结论

Pixiv 账号池采用数据库调度，不再把 UID 列表保存在 `config.toml`：

- `config.toml` 只保存账号池是否启用及选择策略；
- 每个本地 Pixiv 账号以 `schedulable` 表达是否参加调度；
- 不建立黑名单、白名单或独立 membership 表；
- 冻结时间和 round-robin 游标与账号保存在同一行；
- FANBOX 账号不进入该账号池，也不新增 FANBOX 多账号轮换；
- 不增加代理池、账号代理绑定、自动代理轮换或 Pixiv UA 配置。

所谓“只允许指定账号”只是把现有账号全部设为 `schedulable=false`，再把指定 UID 设为
`schedulable=true`。所谓“排除指定账号”则只需把这些 UID 设为 `false`；二者不是额外的数据模型。

## 数据模型

`pixiv_account` 的调度字段为：

```text
schedulable             INTEGER NOT NULL DEFAULT 1
pool_frozen_until       INTEGER NULL
pool_last_selected      INTEGER NOT NULL DEFAULT 0
```

- `schedulable` 与 `pool_last_selected` 只接受 `0`/`1`。
- 新导入与 legacy migration 新增的账号默认 `schedulable=1`。
- `pool_frozen_until` 使用 UTC Unix seconds；只有带有效 `Retry-After` 的 `RateLimited` 才能写入。
- 调度 metadata update 不得改变 `credential_revision`、refresh token 或 premium cache。
- 删除账号会自然删除其调度状态，不建立独立账号池表。
- 已存在的 `0001_initial.sql` 不回写或改变 checksum；先由
  `0002_fanbox_creator_id_not_null.sql` 收敛 FANBOX schema，再由
  `0003_pixiv_account_schedulable.sql` 增加 `schedulable INTEGER NOT NULL DEFAULT 1 CHECK
  (schedulable IN (0, 1))`。SQLite 的 column default 使全部既有 `pixiv_account` 初始化为 `1`；
  migration test 必须直接断言旧行和新行的值。

partial unique index 保证最多一个 last-selected marker：

```sql
CREATE UNIQUE INDEX pixiv_account_one_pool_last_selected
ON pixiv_account(pool_last_selected)
WHERE pool_last_selected = 1;
```

marker 不是账号锁或请求租约，只记录最近一次选择位置。多进程同时选择时，在一个短写事务中读取候选、
选择账号、先清除旧 marker、再设置新 marker 并提交。

## 配置

目标配置只包含：

```toml
[account_pool]
enabled = true
strategy = "round_robin"
```

- `enabled` 默认 `false`。
- `strategy` 默认 `round_robin`，另一个合法值为 `random`。
- 不再接受 `account_pool.accounts`。
- `config.toml` 不保存账号 UID、冻结时间、marker、凭据或其他数据库字段。

为避免用户继续手工编辑长配置，`pixiv config` 增加两个已知 alias：

```text
account_pool_enabled
account_pool_strategy
```

它们只读写上述两个 TOML 值，不直接修改账号行。

## 旧 `account_pool.accounts` 迁移

新 binary 首次读到旧 `account_pool.accounts` 时执行一次可重入 data migration：

1. 完整解析旧列表并读取当前 `pixiv_account` 集合；格式错误时不写数据库或配置。
2. 在一个数据库事务内把现有账号全部设为 `schedulable=0`，再把列表中当前确实存在的 UID 设为
   `schedulable=1`。
3. 数据库提交后原子重写 `config.toml`，只移除 `accounts`，保留 `enabled`、`strategy` 与无关配置。
4. config 重写失败时明确报告 migration incomplete，并阻止账号池数据命令和 pool 管理命令继续；
   下次启动可幂等地重做同一数据库映射后再次移除旧键。

旧配置没有 `accounts` 时无需 data migration；现有账号通过 schema default 保持可调度。迁移完成后，
新导入账号仍默认可调度，不继承历史列表语义。新 binary 不再把 `accounts` 当运行时过滤条件。

## CLI 管理

账号调度通过数据库命令管理，不要求用户直接执行 SQL：

```text
pixiv auth pool status [--json]
pixiv auth pool enable UID... [--all]
pixiv auth pool disable UID... [--all]
```

- `enable`/`disable` 只修改 `schedulable`；必须且只能提供 UID 列表或 `--all` 之一，二者不能同时使用。
- UID 必须是当前数据库中的 Pixiv 账号；任一 UID 不存在时整批操作不提交。
- 批量修改在一个事务内提交，不允许部分成功。
- `status` 同时显示全局 `enabled`/`strategy` 与每个账号的 `schedulable`、`frozen_until`、
  当前 `eligible`；`eligible` 表示该账号此刻同时可调度且未冻结，不考虑某次 operation 的 attempted
  set。不得输出 refresh token。
- `pixiv auth list` 增加相同的非 secret 调度摘要，避免用户必须读取数据库才能理解账号状态。

## 候选与选择

候选账号必须同时满足：

1. 存在于 `pixiv_account`；
2. `schedulable=1`；
3. `pool_frozen_until IS NULL` 或冻结时间不晚于当前时间；
4. 尚未在本次安全重放周期中尝试。

过期冻结在选择事务中视为无冻结，并可顺带清空。repository 不能把所有“没有候选”都折叠成同一
`not found`：它必须让 application 区分数据库无账号、无可调度账号和全部可调度账号仍冻结，并在
最后一种结果中返回最早的未来 `pool_frozen_until`。application 可以继续让
`ErrAccountPoolExhausted` 作为内部 error-chain marker，但对 CLI/MCP 暴露前必须完成下文的稳定错误
映射；不 fallback 到 `PIXIV_REFRESH_TOKEN`、任意未调度账号、匿名 Web API 或 FANBOX 凭据。

### `round_robin`

repository 先取得 marker 账号的 `sort_order`，再选择第一个 `sort_order` 更大的 eligible 账号；尾部
没有候选时 wrap 到最小 eligible `sort_order`。marker 账号即使暂时冻结或不可调度，仍可作为游标位置；
marker 账号已删除时从最小 eligible 账号重新开始。

禁止使用以下已知会饿死第三个及后续账号的排序：

```sql
ORDER BY pool_last_selected ASC, sort_order ASC LIMIT 1
```

### `random`

只在当前 eligible 集合中随机选择。选择完成后仍更新 last-selected marker，使后续切回
`round_robin` 时从真实最近选择位置继续；随机策略不改变 `schedulable` 或冻结状态。

## 429 冻结与安全重放

账号池只用于非写入的数据读取、推荐、时间线和下载。认证、配置与 mutation 不使用账号池。
`[account_pool].enabled=false` 时，数据命令继续使用 `pixiv auth use` 选择的默认账号；该直接选择不读取
`schedulable`，因为此字段只控制账号池调度。账号池启用但没有 eligible 账号时明确 exhausted，不
fallback 到默认账号。

一次 operation 可以在当前 eligible 集合内依次尝试不同账号，但只有同时满足以下条件时才允许切换：

- 错误是 `RateLimited`；
- 上游提供了可解析的有效 `Retry-After`；
- operation 尚未向 stdout/NDJSON 暴露记录，也尚未提交下载文件或其他用户可见结果；
- 调用方 context 尚未取消。

满足条件时，把当前账号冻结到上游给出的时间，加入本次 attempted set，再选择另一个 eligible 账号。
尝试次数由本次 eligible 账号集合自然界定，不添加固定重试次数。以下情况直接返回原错误，不切换：

- 普通网络错误、认证失败、服务器错误或无有效 `Retry-After` 的 429；
- 已经输出记录、提交文件或进入 mutation；
- context canceled/deadline exceeded；
- 写入冻结状态失败。

全部 eligible 账号均冻结或已尝试时，返回 `ErrAccountPoolExhausted`，并在 error chain 中保留最后一个
`RateLimited`；不新增 public SDK `Reason`，也不伪装为空结果。

## Exhaustion 错误契约

application 把选择结果映射为现有 `sdk.Error`；下列 `Detail` 是受控、稳定、非 secret 的 detail kind：

| 选择结果 | `Reason` | `Detail` | `RetryAdvice` |
|---|---|---|---|
| 数据库没有任何 Pixiv 账号 | `Unauthorized` | `account_pool_no_local_account` | 零值；需要先导入账号 |
| 有账号，但全部 `schedulable=0` | `LocalStateError` | `account_pool_no_schedulable_account` | 零值；需要 enable 账号或关闭 pool |
| 有可调度账号，但此刻全部仍冻结 | `RateLimited` | `account_pool_all_frozen` | `Safe=true`，`HasAfter=true`，`After` 为最早未来冻结时间 |
| 本次 operation 因有效 429 轮换后耗尽 | `RateLimited` | `account_pool_exhausted` | `Safe=true`，`HasAfter=true`，`After` 为当前账号集合最早的已验证冻结时间 |

两个 `After` 都只能来自先前已验证并写入数据库的上游 `Retry-After`；读状态失败时返回
`LocalStateError`，不得猜测时间。operation 耗尽错误在 chain 中保留最后一个上游 `RateLimited`，但
外层的 stable reason/detail/retry 不得依赖 `errors.As` 恰好命中哪一层。零值 retry advice 表示没有
可供自动调度的时间信息，不代表伪造“不可重试”。

CLI 对上述错误均以非零状态退出，只向 stderr 输出脱敏后的 `sdk.Error` 摘要；不得输出部分 JSON/
NDJSON 或把失败描述成零条结果。MCP 保留对应 tool 的既有失败 structured shape、设置
`isError=true`，并在 text content 中使用同一脱敏摘要。两种 adapter 都不得改写 `Reason`、`Detail`
或 `RetryAdvice`；测试直接断言 application error 以及 CLI/MCP 摘要中的稳定 reason/detail，而不匹配
带 UID、时间或凭据的动态全文。

## 测试门禁

- 3–5 个账号经过多轮 `round_robin` 无饥饿，并覆盖 wrap、marker 删除、marker 不 eligible。
- `random` 只从 eligible 集合选择，切回 round-robin 时 marker 语义正确。
- 新导入账号默认可调度；批量 enable/disable 原子提交。
- 旧 `accounts` 显式列表与省略列表两条迁移路径可重入，config 写失败不允许继续运行旧过滤语义。
- 冻结、过期清理、多进程选择与 marker 移动保持事务一致。
- 只有有效 `Retry-After` 且未 commit 时切换账号；输出或文件 commit 后绝不重放。
- 分别覆盖无账号、无可调度账号、全部冻结和轮换后耗尽的 reason/detail/retry，并验证 CLI 非零退出、
  MCP `isError=true` 与原 structured failure shape。
- status、JSON、错误、日志与 migration 不泄露 refresh token。
- 显式 debug 使用 `[Pixiv account pool]` 说明候选数量、选择、冻结、恢复与 exhaustion；允许显示非
  secret UID，但不能改变选择结果或输出 refresh token。格式与 writer 边界见
  [显式 debug 诊断](debug-diagnostics.md)。

## 非目标

- 不增加真实的 blacklist/whitelist、规则优先级、标签、分组或独立 membership 表。
- 不增加 FANBOX 账号池、跨 Pixiv/FANBOX 账号关联或自动多 Cookie 调度。
- 不增加代理库存、代理文件导入、账号代理绑定、代理健康检查或自动换 IP。
- 不增加 Pixiv UA 配置；FANBOX UA 与固定代理配置见
  [FANBOX challenge 与 FlareSolverr 路由](fanbox-challenge-routing.md)。
