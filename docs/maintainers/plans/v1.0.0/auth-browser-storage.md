# v1.0.0 鉴权、浏览器导入与 SQLite

状态：目标契约已实施；authdb RC-3 至 RC-5、browser provider 代码与离线/交叉编译门禁已于
2026-08-08 通过。本文件继续作为支持矩阵与安全契约；三平台 native provider evidence、Firefox
临时发行包/清理以及真实凭据边界仍属于 RC-11 发布门禁，不能用本机合成 fixture 或交叉编译替代。
历史基线问题与修复记录见 [authdb 设计审查](authdb-design-review-2026-08-04.md)。

## 浏览器导入分层

浏览器读取位于 `internal/browsercookies`。它保持协议无关，方便其他项目参考源码，
但不作为 v1 公共 Go API 冻结。

```text
internal/browsercookies/
├── core
├── chromium
├── firefox
├── safari
└── secret backend
    ├── darwin Keychain
    ├── windows DPAPI
    └── linux secret service
```

core 只定义 browser/profile discovery、受约束的 cookie query、只读数据库 snapshot、脱敏
`Secret` 与 provider interface。它不知道 FANBOX、Pixiv、CLI、MCP 或账号 store；不接受任意
SQL、任意数据库路径或完整 Cookie header。

FANBOX adapter 只请求精确允许的 FANBOX domain 与 `FANBOXSESSID`，随后调用公开
`sdk/fanbox.Client` 在线验证身份并保存结果。参考实现只用于确认入口与内容形态：
[gallery-dl FANBOX extractor](https://github.com/mikf/gallery-dl/blob/master/gallery_dl/extractor/fanbox.py)
和 [fanbox-dl](https://github.com/hareku/fanbox-dl/tree/develop)；不得复制 GPL 实现。

## 支持矩阵

- macOS：Chrome、Edge、Firefox、Safari。
- Windows：Chrome、Edge、Firefox。
- Linux：Chrome、Edge、Firefox。

不模糊识别其他 Chromium 衍生浏览器。新增 browser 必须实现独立 provider 和 native fixture。

当前实现按平台选择真实系统凭据边界：macOS 使用 Keychain，Windows 使用当前用户 DPAPI，Linux
使用固定属性的 Secret Service `secret-tool` 查询；缺少系统客户端或权限不足时返回明确的
不可用/访问错误，不安装项目依赖。Chromium provider 同时处理 Local State key、v10/v11
AES-GCM 与 legacy CBC，profile 根目录按 macOS、Windows、Linux 的用户目录规则解析。上述代码
契约仍需在对应 native host 运行 provider contract，才能形成发布 evidence；本机合成 fixture
不能替代跨平台 evidence。

`--profile` 只接受 provider 返回的稳定、安全 identifier，不接受路径。多个 profile 且调用方未
指定时明确失败，只输出安全 identifier。适配器不修改或启动浏览器；数据库锁、schema 变化、
Keychain/DPAPI/secret-service 权限与解密失败均显露安全分类，不安装依赖、不静默换 provider。

临时 snapshot 使用进程私有目录；Unix-like 目录 `0700`、文件 `0600`。只查询和解密目标
cookie。Cookie、加密值、绝对路径、profile 内容与系统凭据不得进入日志、错误、JSON 或 MCP。

## 单一鉴权数据库

路径固定为：

```text
~/.pixiv-cli/pixiv-cli.db
~/.pixiv-cli/config.toml
```

Windows 使用 `%USERPROFILE%\.pixiv-cli` 等价路径。数据库只保存鉴权和 Pixiv account-pool
租约状态，不保存普通配置、下载 archive、缓存、请求日志、usage、MCP config、browser profile、
签名 URL 或自动 backup。

SQLite 不提供 secret encryption。安全边界仍是当前用户目录、文件权限/ACL、受控查询、脱敏和显式
auth export。Unix-like 主动收紧目录为 `0700`、数据库与 sidecar 为 `0600`；Windows 首次创建继承
用户 profile 父目录 ACL，替换时保留既有 ACL，不声称 authdb runtime 主动收紧 DACL。

## Schema

表名使用单数：

```text
pixiv_account
fanbox_account
schema_migration
```

不用 SQL 关键字 `order`；稳定入库顺序字段名为 `sort_order`。

### `pixiv_account`

```text
user_id                 INTEGER PRIMARY KEY
sort_order              INTEGER NOT NULL UNIQUE
username                TEXT NOT NULL
refresh_token           BLOB NOT NULL
credential_revision     INTEGER NOT NULL
premium_status          INTEGER NULL
premium_checked_at      INTEGER NULL
schedulable             INTEGER NOT NULL DEFAULT 1
pool_frozen_until       INTEGER NULL
pool_last_selected      INTEGER NOT NULL DEFAULT 0
created_at              INTEGER NOT NULL
updated_at              INTEGER NOT NULL
```

`user_id`、`sort_order` 与 `credential_revision` 必须为正数；`sort_order` 分配后不因删除其他
账号而重新编号。`premium_status`、`schedulable` 与 `pool_last_selected` 只接受布尔值；所有时间使用
UTC Unix seconds 并由 repository 统一转换。`credential_revision` 从 1 开始，在每次 RFT rotation
后递增。access token 不落库。

目标 repository 使用 revision compare-and-swap：rotation 以调用方读取的 expected revision 更新，
stale revision 明确冲突且不得继续内容请求；经验证的 credential replacement 递增 revision。
premium cache、pool 状态等 metadata-only update 使用独立 operation，不改变 credential revision。
RC-3 至 RC-5 已补齐上述 revision/CAS、迁移、调度、schema 与路径门禁；聚焦、全量和 race 证据见
[最终验证记录](final-verification-2026-08-08.md)。

使用 partial unique index 保证最多一个 last-selected account：

```sql
CREATE UNIQUE INDEX pixiv_account_one_pool_last_selected
ON pixiv_account(pool_last_selected)
WHERE pool_last_selected = 1;
```

账号池在一个短事务内选择 `schedulable=1` 且未冻结的候选账号、移动 `pool_last_selected` 标记并
提交。有效 `Retry-After` 只更新该账号的 `pool_frozen_until`；过期值在选择时视为未冻结并可顺带
清理。删除账号会自然删除其 pool 状态，不建立独立 pool 表。配置迁移、CLI 管理、公平选择与安全
重放的唯一契约见 [Pixiv 账号调度](account-pool-scheduling.md)。

### `fanbox_account`

```text
user_id                 INTEGER PRIMARY KEY
sort_order              INTEGER NOT NULL UNIQUE
display_name            TEXT NOT NULL
creator_id              TEXT NULL
session_id              BLOB NOT NULL
credential_revision     INTEGER NOT NULL
validated_at            INTEGER NOT NULL
created_at              INTEGER NOT NULL
updated_at              INTEGER NOT NULL
```

`user_id`、`sort_order` 与 `credential_revision` 必须为正数，且 `sort_order` 不重排。`session_id`
只保存非空 `FANBOXSESSID` value，不保存 `name=value` header、其他 Cookie、browser、profile
或路径。重新导入同一身份时在线验证后更新 session，并递增 revision。

当前 draft schema 允许 `creator_id` 为 NULL，但 Go repository 按 `string` 读取。审查建议下一 migration
收敛为 `TEXT NOT NULL DEFAULT ''`；该调整在 schema freeze 前完成，不能保留互相矛盾的 NULL/string
契约。

### `schema_migration`

```text
version                 INTEGER PRIMARY KEY
name                    TEXT NOT NULL UNIQUE
checksum                TEXT NOT NULL
applied_at              INTEGER NOT NULL
```

同时设置固定 SQLite `application_id` 与 `user_version`，防止误开其他数据库，并拒绝 schema
比当前 binary 更新的 downgrade 运行。

## 默认账号配置

默认账号不进入数据库：

```toml
[pixiv.auth]
default_user_id = 123456

[fanbox.auth]
default_user_id = 654321
```

选择规则：

1. config 指定 UID 时精确选择；不存在则明确失败，不 fallback。
2. 未配置时选择 `sort_order` 最小的账号。
3. 没有账号时返回未认证。

`pixiv auth use UID` 与 `pixiv fanbox auth use UID` 只修改对应 config key；`auth use --auto`
删除显式 key，恢复首个入库账号。import 只写数据库。删除显式 default 必须先切换或改为 auto，
避免数据库与 `config.toml` 无法跨文件原子提交。

`[account_pool]` 只保存 `enabled` 与 `strategy`。每个账号是否参加调度由数据库的 `schedulable`
字段表达；不保存 UID 列表，也不存在独立 blacklist/whitelist。

## SQL migration 目录

```text
internal/persistence/authdb/
├── database.go
├── repository.go
├── migrate.go
└── migrations/
    ├── 0001_initial.sql
    ├── 0002_fanbox_creator_id_not_null.sql
    ├── 0003_pixiv_account_schedulable.sql
    └── ...
```

通过 `//go:embed migrations/*.sql` 嵌入 binary。migration 只向前执行，每份脚本在事务中应用；
版本、名称、checksum 和时间写入 `schema_migration`。已应用脚本 checksum 漂移、版本缺口、
重复版本或未知更新 schema 都 fail closed。需要重建表的 SQLite migration 仍由脚本显式完成，
不提供破坏性的自动 down migration。已经进入 ledger 的 `0001` 不回写；`0002` 收敛 FANBOX
`creator_id`，`0003` 增加默认开启且带布尔 CHECK 的 Pixiv `schedulable`。

推荐连接设置：rollback journal、`synchronous=FULL`、`secure_delete=ON`、`foreign_keys=ON`、
`trusted_schema=OFF`。锁等待只受调用方 context 控制，不设置任意 busy timeout。数据库很小且
写事务短，不使用 WAL，也不自动复制 `.db` 作为 backup。

## Legacy `auth.json` migration

本节的早期设计曾考虑由启动流程把 JSON 自动导入 SQLite；该方案已被 RC 后续计划明确废止。
新 CLI 不读取、迁移或删除旧 `~/.pixiv-cli/auth.json`，因此启动不会产生跨文件迁移副作用，也不会
把旧 secret 带入错误、日志或 stdout。

跨版本迁移由用户显式完成：在旧 CLI 执行
`pixiv auth export --all --output <private bundle>`，再把该文件通过
`pixiv auth import --file <bundle>` 交给新 CLI。bundle restore 在 SQLite 内以事务写入并保留
现有默认账号规则；任何输入、数据库或写回错误都直接返回，旧文件由用户自行保留或删除。
