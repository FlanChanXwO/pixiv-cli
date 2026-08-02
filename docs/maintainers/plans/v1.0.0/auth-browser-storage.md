# v1.0.0 鉴权、浏览器导入与 SQLite

## 浏览器导入分层

浏览器读取位于 `internal/platform/browsercookies`。它保持协议无关，方便其他项目参考源码，
但不作为 v1 公共 Go API 冻结。

```text
internal/platform/browsercookies/
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

SQLite 不提供 secret encryption。安全边界仍是私有目录、文件权限/ACL、受控查询、脱敏和显式
auth export。Unix-like 主动收紧目录为 `0700`、数据库与 sidecar 为 `0600`；Windows 创建并验证
当前用户私有 ACL。

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
pool_frozen_until       INTEGER NULL
pool_last_selected      INTEGER NOT NULL DEFAULT 0
created_at              INTEGER NOT NULL
updated_at              INTEGER NOT NULL
```

`user_id`、`sort_order` 与 `credential_revision` 必须为正数；`sort_order` 分配后不因删除其他
账号而重新编号。`premium_status` 与 `pool_last_selected` 只接受布尔值；所有时间使用 UTC Unix
seconds 并由 repository 统一转换。`credential_revision` 从 1 开始，在每次 RFT rotation 后递增。
access token 不落库。

使用 partial unique index 保证最多一个 last-selected account：

```sql
CREATE UNIQUE INDEX pixiv_account_one_pool_last_selected
ON pixiv_account(pool_last_selected)
WHERE pool_last_selected = 1;
```

账号池在一个短事务内选择候选账号、移动 `pool_last_selected` 标记并提交。有效
`Retry-After` 只更新该账号的 `pool_frozen_until`；过期值在选择时视为未冻结并可顺带清理。
删除账号会自然删除其 pool 状态，不建立独立 pool 表。

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

现有 `[account_pool]` 继续保存 enabled、account whitelist 与 strategy；它不保存凭据或租约状态。

## SQL migration 目录

```text
internal/storage/authdb/
├── database.go
├── repository.go
├── migrate.go
└── migrations/
    ├── 0001_initial.sql
    ├── 0002_<name>.sql
    └── ...
```

通过 `//go:embed migrations/*.sql` 嵌入 binary。migration 只向前执行，每份脚本在事务中应用；
版本、名称、checksum 和时间写入 `schema_migration`。已应用脚本 checksum 漂移、版本缺口、
重复版本或未知更新 schema 都 fail closed。需要重建表的 SQLite migration 仍由脚本显式完成，
不提供破坏性的自动 down migration。

推荐连接设置：rollback journal、`synchronous=FULL`、`secure_delete=ON`、`foreign_keys=ON`、
`trusted_schema=OFF`。锁等待只受调用方 context 控制，不设置任意 busy timeout。数据库很小且
写事务短，不使用 WAL，也不自动复制 `.db` 作为 backup。

## Legacy `auth.json` migration

JSON 到 SQLite 是一次性 Go data migration，schema 变化仍由 SQL scripts 管理：

1. 锁定 legacy store 与数据库目标。
2. 创建私有数据库并应用全部 SQL migration。
3. 按原 `accounts` 顺序分配 `sort_order`，导入 token、username 与 premium cache。
4. 把旧 `default_user_id` 写入 `[pixiv.auth]`，保持原选择。
5. 做 SQLite integrity check 与逐账号逻辑对比。
6. 原子安装数据库并同步目录。
7. 删除旧 `auth.json`；删除失败时明确报告数据库已提交且 legacy secret 仍存在。

迁移可安全重入：如果数据库已导入但旧 JSON 仍存在，必须先完整逻辑对比；一致则继续完成
config/清理，不一致则 fail closed。迁移过程不输出 token，不创建自动备份。
