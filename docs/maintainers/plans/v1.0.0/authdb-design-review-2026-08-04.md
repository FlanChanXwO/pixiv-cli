# v1.0.0 authdb 设计审查（2026-08-04）

状态：审查完成，RC-3 至 RC-5 修复已实施并通过聚焦回归。审查日期：2026-08-04；实施记录：2026-08-08。

## 结论

数据库方向可以保留，不需要改成其他存储或拆成多个数据库。以下基础选择是合适的：

- Pixiv 与 FANBOX 鉴权状态共用一个 SQLite authority，但使用独立 product table；
- 普通配置与默认账号继续留在 `config.toml`，不混入 secret database；
- `schema_migration`、embedded forward-only migration、checksum、`application_id` 与 `user_version`；
- 单数表名、UID primary key、稳定 `sort_order`、短事务和不启用 WAL；
- credential 不加应用层加密，并明确依赖本机账号目录权限与受控 export。

基线审查时实现还不能视为满足 RC 数据一致性门禁；审查发现的 5 个 P0 问题及同轮契约偏差已在
RC-3 至 RC-5 中修复，并通过 authdb/application/bootstrap 聚焦回归、全量 race 与启动错误传播验证。
本文件保留当时的现状、复现和修复要求，作为历史审查证据；其中“当前实现”“下一轮必须”等措辞均
指 2026-08-04 基线，不能理解为当前代码仍有同一缺口。真实跨平台与授权环境 evidence 仍按 RC-11
单独验收。

## 审查范围与证据

审查覆盖：

- `internal/persistence/authdb/database.go`、`migrate.go`、`legacy.go`、`repository.go`；
- `migrations/0001_initial.sql` 与现有 authdb tests；
- bootstrap、Pixiv/FANBOX application service 的调用方；
- [鉴权、浏览器导入与 SQLite](auth-browser-storage.md)、architecture 与 verification 声明。

Go LSP diagnostics 为空，以下现有聚焦回归通过：

```bash
go test ./internal/persistence/authdb ./internal/application/pixiv \
  ./internal/application/fanbox ./internal/bootstrap -count=1
```

另外运行了不保留源码的 disposable probes，专门覆盖现有 tests 未触达的失败路径。probe 文件已删除，
没有修改数据库、凭据或生产源码。

## 必须在 RC 前修复

### P0-1：legacy migration 会部分提交，且启动层吞掉错误

`MigrateLegacyAuthJSON` 对每个账号分别调用一次带独立事务的 `UpsertPixiv`。当第二个账号导入失败时，
第一个账号已经提交；下次运行看到非空数据库后进入一致性比较，并持续失败，无法自行完成迁移。
disposable probe 通过让第二次 insert 失败，稳定复现了“返回错误但首个账号已存在、重跑仍失败”。

同时 `internal/bootstrap/bootstrap.go` 使用：

```go
_, _ = authdb.MigrateLegacyAuthJSON(...)
```

这会把 malformed JSON、部分提交、权限和一致性冲突全部隐藏。调用方也没有在该路径写入旧
`default_user_id` 或删除已验证的 legacy secret。当前实现不符合文档中的原子、可重入和显露 commit
outcome 契约。

下一轮必须：

- 预先完整解析并验证 legacy JSON，再打开/创建最终 `pixiv-cli.db` 并应用 schema migration；
- 在最终数据库的一个写事务中导入全部 Pixiv 账号并执行 integrity/逻辑对比，任一账号失败则全部
  rollback；不得用 private DB replacement 覆盖同库中可能已经存在的 FANBOX 账号；
- credential transaction 提交后写 default config，最后删除 legacy JSON；跨文件步骤失败时返回
  `credentials_committed`、`default_written`、`legacy_removed` 中已经完成的真实阶段；
- 重入时完整比较 identity、顺序、credential 与 premium cache，一致则继续未完成的 config/清理；
- bootstrap 不得忽略 migration、config 或 legacy 删除错误。

这是唯一权威顺序：`validate JSON → migrate schema → credential transaction/compare → write config →
remove JSON`。SQLite transaction 提供账号集合的原子提交；SQLite 与 `config.toml`/legacy JSON 不可能
跨文件原子，因此依赖可重入阶段和明确 commit outcome，而不声称 private DB install/目录 sync 能把
三者合成一个原子操作。

### P0-2：三账号以上的 pool round-robin 会饿死后续账号

当前选择按：

```sql
ORDER BY pool_last_selected ASC, sort_order ASC LIMIT 1
```

排序。选中账号的 marker 被清零后，所有未标记账号仍按最小 `sort_order` 优先。三个账号的实际序列为
`1, 2, 1, 2, 1, 2`，第三个账号永远不会被选中。

schema 中“最多一个 marker”的 partial unique index可以保留；repository 应根据 last selected 的
`sort_order` 选择下一个 eligible account，并在末尾 wrap。候选范围改由
`pixiv_account.schedulable` 表达，不再读取 config allowlist。测试至少覆盖 3 个账号、多轮、freeze、
不可调度账号、删除 marker account 与并发事务。完整目标见
[Pixiv 账号调度](account-pool-scheduling.md)。

### P0-3：`credential_revision` 既非单调，也没有 compare-and-swap

当前 rotate SQL 无 expected revision 条件；并发调用都可成功覆盖 credential。现有 architecture 却
声明 Pixiv rotation 与 FANBOX replacement 使用 compare-and-swap。

同时 existing-account upsert 直接写调用方传入的 revision。FANBOX re-import 总是传 `1`，因此一个
revision 已为 `5` 的账号重新导入后会退回 `1`，与“replacement 后递增”契约相反。

建议在下次计划锁定真实 CAS，而不是删除 revision，并把通用 upsert 拆成不同意图的 repository
operation：

- initial credential insert 固定 revision `1`；
- 经在线验证的 re-import/credential replacement 在 SQL 内以当前值 `+1`，不接受调用方覆盖现有
  revision；
- rotation 接收 expected revision，并以 `WHERE user_id=? AND credential_revision=?` 更新；stale
  rotation 失败后 application 必须关闭新 Client，不能继续发起内容请求；
- premium cache、pool freeze/marker、display name 等 metadata-only update 使用独立 SQL，不改变
  credential revision；
- affected rows 为 0 时返回内部 conflict sentinel，由 application 重新读取，不伪装成功；
- 不为该内部竞争额外创造 public `Reason`，除非未来有调用方证据需要。

### P0-4：nullable `creator_id` 与 Go scan 类型不一致

schema 允许 `fanbox_account.creator_id NULL`，repository 却直接 scan 到 `string`。插入合法 NULL 后，
`GetFanbox`/`ListFanbox` 会返回 scan error。

公开模型和 application 都把缺失 creator ID 表达为空字符串。建议下一 schema migration 把列收敛为
`TEXT NOT NULL DEFAULT ''`，并迁移既有 NULL；如果不采用该表示，就必须统一改为 `sql.NullString`。
不能继续同时宣称 NULL 合法又按 non-null string 读取。

### P0-5：SQLite file URI 未转义合法路径

`database.go` 直接拼接 `"file:" + path + "?..."`。app data path 含 `?`、`#` 或 `%` 时会被解释成 URI
query/fragment/escape，disposable probe 已复现合法目录无法打开。

下一轮应使用正确的 SQLite URI/path 构造方式，并覆盖空格、Unicode、`?`、`#`、`%` 与 Windows path
形态；错误不得回显包含 secret 的完整配置值。

## 同轮应收口的问题

### 默认账号跨 DB/config 语义

目标设计要求删除显式 default 前先切换账号或改为 auto。当前 Pixiv/FANBOX service 先提交 DB row
删除，再根据 config 判断 default 并尝试清 key；config write error 被 `_ =` 忽略。清理失败时会留下
指向已删除账号的 dangling default，调用方却收到成功。

应先检查 explicit default 并拒绝删除，或由一个明确 operation 先完成 config 状态切换；所有 config
错误必须返回。FANBOX auto 模式下 `isDefault` 目前对所有账号都返回 false，也应和真实“最小
`sort_order`”选择一致。

### migration test 的 `application_id` 断言失效

`migrate_test.go` 的局部变量与 package constant 同名，当前正向比较实际是变量与自身比较，无法确认
新数据库写入了预期 `application_id`。应消除 shadow；现有 `TestOpenRejectsWrongApplicationID` 已覆盖
错误 ID 拒绝，可视修复后的断言结果加强，但无需重复新增同类测试。

### Windows 权限声明过强

当前 authdb 代码在 Unix-like 主动设置目录/DB mode；Windows 路径依赖用户 profile 的 inherited ACL，
没有主动创建并验证“仅当前用户”DACL。计划文档应与 architecture/development 的真实边界一致：验证
文件位于当前用户目录并继承既有 ACL，不声称 authdb runtime 主动收紧 Windows ACL。

### `validated_at` 与 credential 空值

现有 schema 只约束 UID/order/revision。下一轮应在 repository 入口继续拒绝空 credential，并补齐
`RotateFanboxSession` 的合法 `validated_at` 检查。这里不建议新增无证据的长度、TTL 或条数限制，也
不要求把上游 session 格式写成脆弱的 SQL CHECK。

## Schema migration 建议

不要修改已嵌入且可能已写入开发数据库 ledger 的 `0001_initial.sql`；否则 checksum drift 会让现有
数据库按设计 fail closed。建议：

1. 新增 `0002_fanbox_creator_id_not_null.sql`，通过显式 table rebuild 把 NULL 规范为 `''`；
2. 新增 `0003_pixiv_account_schedulable.sql`，只增加
   `schedulable INTEGER NOT NULL DEFAULT 1 CHECK (schedulable IN (0, 1))`；default 把既有账号初始化为可
   调度，并由 migration test 直接断言旧行与新行；
3. round-robin、revision CAS、legacy migration、DSN 与 default semantics 使用 repository/application
   修复，不为纯逻辑问题新增表或列；
4. 迁移后保留现有 `application_id`、`sort_order`、revision 与 partial unique index；
5. 不新增 lease/status/membership table、自动 backup、credential encryption 或无依据的清理 TTL。

如果团队决定 v1.0.0 前完全不支持任何已生成的开发数据库，也可以重写 `0001` 并要求维护者手工删除
本地开发 DB；该路线更简单，但会主动破坏本地状态，因此不能由程序静默执行。默认建议仍是按顺序
追加 `0002` 与 `0003`。

## 下一轮聚焦测试

- legacy 第二账号失败时零账号提交；重跑、config write failure、legacy delete failure 都报告准确
  commit outcome；bootstrap 传播错误；
- 3–5 个账号多轮选择不饥饿，并覆盖 freeze、`schedulable`、删除与并发；
- re-import revision 单调递增、stale expected revision CAS 失败、并发 rotation 不丢更新；premium/
  pool metadata update 不改变 credential revision，stale rotation 后不继续内容请求；
- `creator_id` NULL migration 与空字符串 round-trip；
- 特殊字符/Unicode/Windows-like app data path；
- 真实 `application_id` 断言和错误 ID 拒绝；
- explicit/auto default 删除与 FANBOX list 的 default 标识；
- Unix-like DB/sidecar 权限及 Windows inherited ACL 文档边界。

这些 tests 是本次修复采用的聚焦契约；已先以回归测试固定失败路径，再运行现有 authdb、application 与
bootstrap 回归。SQLite migration、并发与 crash recovery 的本地自动 evidence 已通过；三平台 native
凭据和授权环境 evidence 不属于本历史审查的本地结论，继续由 RC-11 追踪。
