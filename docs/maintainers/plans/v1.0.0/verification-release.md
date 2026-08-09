# v1.0.0 测试、迁移与发布门禁

截至 2026-08-09，RC-1 至 RC-10 的实现与自动门禁已完成；自动验证结果见
[最终验证记录](final-verification-2026-08-08.md)。真实 Pixiv public SDK 与一次性 real-solver
evidence 已在授权环境取得；用户指定的 `nakkemos/3625356` 与 `aak/11870583` 已在 SDK 默认
Chrome_146 transport 下完成真实 `post.info`/资源复核，前者两个 file resource 完整 GET 通过，后者
没有第一方 file asset。旧 `ro7274/12373249`、release-prep Keychain 新鲜凭据与 native browser
evidence 仍按 RC-11 保留为独立发布门禁，不能用离线 fixture 代替。

## Public SDK

- 为 `sdk.Page[T]`、Cursor Text/JSON codec、query/product/operation/identity binding 编写契约测试。
- 为 `sdk.Error` 的 `Reason`、`ReasonOf`/`IsReason`、`errors.Is/As`、context chain、retry advice 与全部
  脱敏字段编写测试；inventory 必须拒绝旧 `Code`/`CodeOf`/`IsCode`/`Code*` 名称。
- 为 Pixiv `Open/OpenWith/New/NewWith`、rotation、access-token expiry、LoginSession one-shot 与
  App-only routing 编写离线 HTTP contract tests。
- 为 `Artwork` variant、Novel、User、`CommentPage`、`NovelSeriesResult`、结构化 `NovelContent`、
  `Options.AcceptLanguage`、列表/详情完整度、未知 enum 和每个 request struct 编写测试。
- 以 [PixivPy App API 能力兼容矩阵](pixivpy-parity.md) 生成逐项 golden，确保每个基线产品方法都有
  直接、等价或明确排除结论；重点覆盖 artwork/novel comments、bookmark detail/tags、novel series/
  content、related/follower/blocked users、AI visibility 和 bookmark restrict/tags。
- 为 `ParseURL` 的全部 canonical/locale 形态、host 混淆、userinfo/port、重复或非法 ID、裸 ID 拒绝、
  query/fragment 清理，以及 `Reference`/canonical output 的兼容性编写 table-driven tests；测试不得
  发起网络请求。
- 验证 Pixiv/FANBOX 每类第一方图片、封面、文件与 archive 都能从公开模型取得同时含 URL 与
  `ResourceRef` 的 `Resource`；public inventory 不含散落的 `DownloadURL`、`OriginalURL`、
  `SignedURL` 或同义字段，第三方 embed canonical link 保持明确例外。
- 验证 addressable entity 的稳定 ID，以及 `Artwork`、`Novel`、`Post` 的 UTC `PublishedAt`、可选
  `UpdatedAt`、summary/detail 一致性、非法必需时间报错和未知可选时间为 `nil`。
- 为 ugoira archive quality/ref、frame 顺序、毫秒 delay、filename 安全/唯一性、缺失 original 不
  fallback、未知 quality 保留和非 ugoira 请求拒绝编写 fixture tests。
- 为 FANBOX creator/tag/post/home/supporting/following、两类 pagination、受限帖、article、cover、
  image/file、未知 block/embed、旧 redirect 与 session expiry 编写 fixture tests。
- 为 `ResourceRef` Text/JSON codec、`Resource.URL` host/path/userinfo validation、`RequestHeaders`
  allowlist 与 defensive copy、`ExpiresAt`/`RequiresCredentials` 语义、redirect revalidation、Cookie
  stripping、signed query redaction、GET/HEAD、Range、conditional read、response header allowlist
  与 atomic save 编写测试。
- 删除检查必须证明生产代码不再存在 `internal/services/pixiv/webapi`、Web route、`BackendWebAPI`、
  `WebAPIBaseURL`、Web fallback 环境变量和真实 Web fallback E2E；只有配置迁移墓碑、迁移指南与
  历史 ADR 可以提及旧键。
- App-only 离线 fixture 覆盖无 token 返回 `unauthorized` 且不发起 Web 请求。真实匿名探测只作为
  可选 canary 记录上游变化，不把联网结果或某个 endpoint 的暂时行为变成 release guarantee。
- 使用 public API inventory golden 证明旧 SDK symbol 已删除且新导出面完整；v1.0 发布后以
  `apidiff` 对 tag baseline 阻止不兼容变化。
- package layout 检查必须证明公开 SDK 只从 `sdk`、`sdk/pixiv`、`sdk/fanbox` 导出，旧顶层
  `pixiv/` 已删除，不创建顶层 `fanbox/`，也不把 internal adapter 变成可导入 package。
- 使用 `go/ast`/`go/doc` 聚焦检查 `sdk`、`sdk/pixiv` 与 `sdk/fanbox`：package comment 及每个导出
  declaration 都有非空英文 GoDoc，function/method 注释以 identifier 开头，公开 declaration doc
  不包含中文、日文或韩文说明文字。语言检查只作用于 declaration doc comment，不扫描测试数据、
  string literal、上游原值或 internal 实现注释。
- review 每个导出 function/method 的 GoDoc 是否覆盖认证、ownership、取消和非显然错误语义；自动
  检查只能保证存在性与语言边界，不能把空洞模板当作合格说明。

## SQLite 与 migration

- 从空数据库应用完整 embedded migration chain。
- 从每个仍受支持的历史 schema 升级到当前版本。
- 验证 version gap、重复版本、checksum drift、未知更新 schema 和 downgrade 全部 fail closed。
- 验证 `application_id`、`user_version`、foreign key、partial unique index 与所有 CHECK constraint。
- 验证 `sort_order` 保留 JSON 顺序、default config 迁移、rotation revision 与 premium cache。
- 验证至少 3 个账号多轮 pool selection 不饥饿，并覆盖 `schedulable`、freeze、过期清理、删除 marker
  账号和多进程竞争的事务语义；不引入独立 lease/status/membership 表。
- 从只含 `0001` 的数据库依次应用 `0002_fanbox_creator_id_not_null.sql` 与
  `0003_pixiv_account_schedulable.sql`，直接断言既有及新建 Pixiv 账号均为 `schedulable=1`，非法布尔
  值被 CHECK 拒绝，既有 migration checksum 不变。
- 验证旧 `account_pool.accounts` 到 `pixiv_account.schedulable` 的可重入迁移、config rewrite failure、
  新账号默认可调度及批量 enable/disable 原子性；迁移后配置只保留 `enabled`/`strategy`。
- 分别验证 pool 无本地账号、无可调度账号、全部冻结和本次 429 轮换耗尽的稳定
  reason/detail/retry；CLI 非零退出且无部分 JSON/NDJSON，MCP 保留 tool failure shape 并设置
  `isError=true`。
- 验证 credential initial revision、re-import 单调递增、expected revision compare-and-swap 与 stale
  writer conflict；existing-account upsert 不得接受调用方把 revision 写小。
- 验证 JSON migration 的 crash/re-entry、DB/JSON 一致与冲突、config write failure、legacy 删除
  failure 和 commit outcome；任一账号导入失败时不得部分提交，bootstrap 必须传播错误。
- 覆盖 nullable `creator_id` 的 schema migration、特殊字符/Unicode/Windows-like DB path 与真实
  `application_id` 断言，避免 scan、SQLite URI 和测试 shadow 回归。
- 在 Unix-like 验证目录、DB、journal/temp 权限；Windows 验证文件位于用户 profile 并继承/保留
  既有 ACL，不声称主动收紧 DACL 或提供 POSIX mode。
- auth export/restore 只经逻辑 repository，禁止复制数据库文件或输出额外 secret。

2026-08-04 的基线现状证据与 P0 清单见
[authdb 设计审查](authdb-design-review-2026-08-04.md)。P0-1 至 P0-5 已在 RC-3 至 RC-5 修复并由
本地自动回归验证；本节保留原契约清单，不能把历史审查文字理解为当前实现仍未通过。Windows inherited
ACL 与三平台 native provider evidence 仍按本文件的独立发布门禁执行。

## Browser providers

- 使用 synthetic profile/database/key fixtures 覆盖 Chrome、Edge、Firefox 与 Safari schema；Firefox
  fixture 应来自受支持真实版本并完成脱敏，不依赖开发机预装 Firefox。
- 在 macOS、Windows、Linux 原生 CI 运行各自 provider contract。
- 最终 Firefox native job 在 runner 工作目录临时解包固定版本官方发行包，以隔离 `HOME`/profile
  生成或升级 schema，运行 discovery、snapshot、读取与清理契约；job 结束删除发行包和 profile，
  不写入系统应用目录。发行包版本与 checksum 进入 evidence，不把下载产物提交仓库。
- 覆盖浏览器运行中的 lock/WAL、profile ambiguity、schema change、permission denied、Keychain、
  DPAPI、Linux secret service unavailable、malformed/decryption failure。
- 验证查询在解密前已收敛到 FANBOX domain 与 `FANBOXSESSID`。
- 验证临时 snapshot 清理及所有错误、JSON、stderr、MCP 与格式化路径不泄露 secret/path。
- browser provider 与普通 contract test 精确区分 challenge 与普通 403。FlareSolverr recovery 只在
  FANBOX SDK 的 fake service/transport contract 中测试；普通 CI 不启动真实 solver。
- 真实浏览器导入只在用户授权的本机或受保护 release environment 运行，不把 session 带入 CI
  artifact、日志或 shell history。

## CLI 与 MCP

- 覆盖 Pixiv 无认证行为以及 FANBOX auth import/list/use/use-auto/remove/status。
- 覆盖 `--stdin`、TTY 隐藏输入、browser/profile matrix 和所有互斥 flag。
- 覆盖文本、JSON、NDJSON、progress、错误与 secret-output isolation。
- 覆盖两个独立 MCP server 的 tool inventory、structured output、`isError=true` 和 stdout isolation。
- 验证 MCP 无认证/config tool，两个 registry 不交叉，FANBOX 不读取 Pixiv RFT。
- 覆盖 creator/tag/post/URL 输入矩阵以及无法唯一解析时的明确失败。
- 覆盖高级 Pixiv/FANBOX downloader 的完整成功边界、archive、sidecar、原子落盘和提交后不重放。
- 覆盖根 `--debug` 默认关闭、只写 stderr、明确产品/子系统模块名、MCP 本地请求关联、secret isolation
  与 failing writer；debug on/off 不得改变路由、调度、solver 调用次数或最终业务错误。完整矩阵见
  [显式 debug 诊断](debug-diagnostics.md)。
- 覆盖 `auth export --debug` 仍不创建 diagnostic scope，成功 stderr 为空且 stdout byte-for-byte 保留
  raw-token/bundle 契约。
- 覆盖 unknown long/short/`--name=value` option 在 root、Pixiv、FANBOX 与两个 MCP command 上统一返回
  `error: unknown option '--name'`、空 stdout 与 usage exit code `2`；验证 interspersed flag、`--`
  literal，以及解析失败不触发 config、SDK、network、MCP runtime 或 debug presenter。canonical 行为见
  [严格 unknown-option 解析](strict-cli-argument-parsing.md)。

## 真实 SDK E2E 与本机凭据

v1.0.0 release evidence 必须至少包含一次真实 Pixiv SDK 读取，并通过 public `sdk/fanbox` 逐一调用
每个公开 FANBOX operation。offline fixture、CLI 间接成功、SDK 外部手工 HTTP 探针或 FlareSolverr
页面不能替代 SDK Client 的真实结果。E2E 默认关闭，只在用户显式授权且本机凭据存在时运行。开工
前不要求联网探测；入口存在性与联网成功是两项独立 evidence，后者只在最终 release-prep 判定。

- Pixiv E2E 通过本机 `pixiv-cli` 的显式 auth export/repository 边界取得当前选中账号的 refresh
  token，只在测试进程内存中传给 `pixiv.Open`；不得进入 shell argv、环境 dump、日志、test name、
  artifact 或失败 diff。rotation 后的新 credentials 必须按正常契约持久化，不能继续复用旧 RFT。
- FANBOX E2E 当前授权 session 保存于 macOS Keychain，service 为 `pixiv-cli-e2e-fanbox`、account 为
  `fanbox-e2e`。测试只读取 `FANBOXSESSID`，不保存或恢复 GA、广告标识、行为数据及短期 Cloudflare
  Cookie。启用 FlareSolverr 时取得的 `cf_clearance` 只在该 Client 内存中存在。session 失效时明确报
  `credentials_expired` 并要求重新导入，不 fallback 到其他账号或 Cookie。
- FANBOX E2E 首先验证当前身份，再覆盖 creator/tag/post/home/supporting/following、两类 pagination
  及 public resource operation。账号相关列表合法为空时，HTTP 状态、error envelope 和 schema 正确
  即可计为该 operation 通过；不得用空结果替代详情或资源语义覆盖。
- creator/tag/post/resource 的非空目标通过显式、非 secret 的 E2E 环境配置提供；不得硬编码账号或
  内容 ID，也不得在运行时自动发现目标。mutation、支持关系变更和批量下载不作为默认真实 E2E。
- 当前 FANBOX 入口的 target key 固定为 `FANBOX_E2E_CREATOR_ID`、`FANBOX_E2E_TAG`、
  `FANBOX_E2E_POST_ID` 与 `FANBOX_E2E_POST_URL`；启用真实 E2E 后缺少任一 key 或 Keychain item
  必须显式失败，未启用时才允许默认 skip。
- 资源门禁必须从合法 `post.info` 详情发现真实帖子 file attachment URL，并读取非零字节且验证响应；
  cover、thumbnail、preview 或预先提供的任意 CDN URL 都不能替代此 evidence。
- resource client 只向严格校验后的 FANBOX API/下载 host 传播同一个 `FANBOXSESSID`，redirect 后重新
  校验 host；不要求额外 Cookie，也不得向任意详情返回 URL 泄露 session。
- release-prep 操作者对所有返回和失败路径执行 secret/signed-query 扫描。Keychain item、refresh token、
  测试响应和下载内容都属于本机私有状态，不提交 Git，也不进入 CI secret-less job。跨平台 CI 继续只
  使用 synthetic fixtures。

## FlareSolverr 可选 recovery 边界

FlareSolverr 是 v1.0.0 显式配置、默认关闭的可选 challenge recovery，不是默认 runtime、全量代理或
普通测试依赖。实现完成后必须在 v1.0.0 发布前取得一次本机 protocol acceptance evidence，但不进入
普通 CI，也不要求每个 RC 重复。native Chrome 146 TLS transport（内置 Firefox 148 HTTP UA）是当前 baseline 而非永久兼容性保证；
它始终先请求。只有严格识别的 Cloudflare challenge 才允许匿名求解 `https://www.fanbox.cc/`，再由
native transport 使用 solver user agent、单个 `cf_clearance` 与用户原有 `FANBOXSESSID` 重放一次。
API JSON、帖子 URL、文件 URL、session 和下载字节不得交给 solver。

普通 CI 使用 fake FlareSolverr service 与 fake native transport 覆盖零调用成功路径、config/native/
solver UA 优先级、challenge solve/replay、错误映射、一次重放、并发合并及三类 proxy 不继承；还要
覆盖 service root URL 的结构/脱敏/`/v1` 构造、重复或非法 clearance、非法 solver UA，以及 leader
取消而 follower 继续、全员取消会终止 shared solve。FANBOX command `--proxy`/`--no-proxy` 只改变
native transport，绝不覆盖 solver service/upstream。普通 CI 不运行 Docker、真实网络或真实
session。实现完成后维护者只在本机用固定 tag/digest 做一次 real-solver protocol 验证：synthetic
native challenge + 真实匿名首页求解 + synthetic native replay。Client 使用非 secret dummy session，
该值只进入 injected native transport，绝不发给 solver；全程不使用真实 session。真实网络自然出现
challenge 时可另留 genuine recovery evidence，但没有自然 challenge 不算失败。两种结果都不是每次
CI/RC 的前置条件，也不能替代 public SDK 的真实 operation/file E2E。

2026-08-04 的验证补充了关键边界：FlareSolverr 自己转发正确 API 未稳定返回目标 JSON；匿名首页
求解却能返回 user agent 与 `cf_clearance`，二者交给 Chrome 146 `tls-client` 后可由 native 请求取得
合法详情。只使用 clearance 成功，混入额外 Cloudflare Cookie 曾失败；把 session 交给 solver 还会
进入其 INFO 日志。因此 production route 必须采用匿名、clearance-only、native replay，而不是全量
转发。完整设计与脱敏 evidence 见
[FANBOX challenge 与 FlareSolverr 路由](fanbox-challenge-routing.md)。

## 文档与迁移材料

实现时同步更新：

- 三语 README；
- `docs/en|zh-CN|ja/cli-reference.md`；
- `docs/en|zh-CN/sdk.md`；
- `docs/en|zh-CN/mcp-tools.md`；
- architecture ADR、development guide 与 `CONTEXT.md`；
- `skills/pixiv-cli/`；
- v1.0 migration guide；
- bilingual release notes。

迁移指南必须提供旧 SDK symbol 到新 symbol/removed 的完整矩阵、`auth.json` 的显式 bundle 迁移行为、默认
账号 config、removed Web fallback setting、两个 MCP 启动命令和 FANBOX 浏览器导入权限说明。

## 发布阶段

1. 完成 shared SDK/error/cursor 与 SQLite foundation。
2. 重写 Pixiv App-only public SDK，并把本地状态移到 application/storage。
3. 实现 FANBOX public SDK 与资源安全边界。
4. 实现浏览器 provider、FANBOX auth、CLI/MCP 与产品 downloader。
5. 发布 beta，允许基于真实使用修订公开契约。
6. 进入按需 release candidate；每次 public change 都重新生成 API inventory。
7. 公开契约无阻塞问题且完整门禁通过后发布 v1.0.0。

不设置固定 beta/RC 次数；是否继续由未解决契约、真实 evidence 和回归结果决定。

## 最终命令门禁

```bash
go test ./...
go test -race ./...
go vet ./...
sh scripts/build.sh
git diff --check
```

同时运行 documentation tests、workflow policy、public API compatibility 与 browser provider native contract
provider jobs。普通 CI 不要求真实凭据或真实 FlareSolverr；但 v1.0.0 最终发布前必须在授权的本机
release-prep 中取得上述 Pixiv/FANBOX 真实 SDK evidence。凭据失效或网络 challenge 导致无法完成时，
必须记录为 release blocker/risk 并更新凭据或环境，不能把 offline fixture 当作真实 E2E 成功。

完整执行顺序、凭据读取边界、Firefox 临时验证、FlareSolverr 隔离方式、evidence 字段与清理步骤见
[最终验证操作手册](release-prep-runbook.md)。

### 历史 release blocker 与当前状态（基线记录：2026-08-03）

当时的 FANBOX 实现使用错误的 `/api/v1/` base path、旧 operation 名称与 Chrome profile，自动化 E2E
的 HTTP 404 因而不能作为 CDN 指纹证据。使用有效 `FANBOXSESSID` 请求正确 `post.listHome` 已取得
HTTP 200 和非空
帖子摘要，证明 session 与部分直连 API 可用。普通非浏览器 UA、Python requests 的 Firefox
header/cipher 近似和通用 headless Edge 对 `post.info` 仍返回 403；但项目现有 `tls-client` 的完整
Firefox 135/148 profile 对正确详情取得 HTTP 200，Firefox 148 + 同一 session 对首页 10 条详情达到
**10/10 HTTP 200 合法 JSON**。

随后对用户显式提供的非 secret 目标使用 Firefox 148 profile：`post.info` 返回 HTTP 200 合法 JSON，
详情中发现 1 个真实 file attachment；同一单一 `FANBOXSESSID` 传播到 `downloads.fanbox.cc` 后，附件
返回 HTTP 200，完整读取 28,544,885 字节且与详情声明大小一致。未携带 session 的附件对照请求为
HTTP 403。全过程未使用 FlareSolverr/浏览器进程，未落盘正文、密码信息、signed URL 或下载内容。

该次 native 成功说明 Firefox 路径应作为默认首选，但不能推出所有网络状态永远不需要 recovery。
2026-08-04 又验证了“匿名 FlareSolverr 首页求解 + clearance-only + native Firefox replay”可取得合法
详情，因此已锁定为显式可选路线。

该历史 blocker 中的 route/profile、资源 host session 传播和 challenge-only FlareSolverr 代码项已在
RC-6 至 RC-8 实施，并由离线/合成 focused tests 覆盖；public SDK 完整自动化真实 E2E 仍是当前 RC-11
release blocker。不得把手工探针、cover 下载、offline fixture 或 solver 自己返回的页面记为完整 FANBOX
E2E。完整历史诊断与当前决策分别见 [会话状态记录](session-status-2026-08-03.md)、
[challenge 路由设计](fanbox-challenge-routing.md) 和
[最终验证记录](final-verification-2026-08-08.md)。

### 历史 authdb blocker 与当前状态（基线记录：2026-08-04）

authdb 的总体 SQLite 设计可保留；当时识别的 legacy migration 部分提交/错误吞没、三账号 pool 饥饿、
revision 非 CAS、nullable `creator_id` scan 与 SQLite file URI 路径问题已在 RC-3 至 RC-5 修复，并由
authdb/application/bootstrap focused tests 与全量 race 回归覆盖。真实发布前仍需按 RC-11 补回授权环境
evidence；修复范围和测试契约见 [authdb 设计审查](authdb-design-review-2026-08-04.md)。

## v1.0.0 发布条件

- public API inventory 已冻结且没有未决命名、零值或错误语义。
- v0 `/pixiv` 到 v1 `/sdk/pixiv` 的迁移矩阵、不可变 tag 和 v1 package compatibility gate 已验证。
- Pixiv/FANBOX credential、rotation、cursor、URL reference、identity/time、native resource、ugoira
  与 stdout isolation review 通过。
- 三个平台的浏览器 provider 实现与离线/交叉编译门禁通过；固定 Firefox 153.0.3 temporary
  profile contract 已在本机 macOS arm64 通过，但真实用户 profile 与 Windows/Linux native
  host/runner evidence 仍按 RC-11 单独取得，Safari 仅要求 macOS。
- SQLite migration、权限、并发与 crash recovery evidence 完整。
- 两个 MCP server 的 tool inventory 与文档一致。
- 真实 Pixiv SDK E2E、一次性 real-solver protocol acceptance，以及用户指定的两个 FANBOX target
  production SDK/resource evidence 已完成；`nakkemos/3625356` 的两个 file resource 完整 GET 计数
  `6,816,050` bytes，`aak/11870583` 的 `post.info` 成功且没有第一方 file asset。报告不包含
  credential、Cookie、signed query 或内容。旧 `ro7274/12373249` 仍没有可验证的 file-resource 闭环。
- FANBOX native Chrome 146 主路径与可选 FlareSolverr recovery 的 focused tests 通过；实现后的 real
  solver protocol 只需以 synthetic challenge 在本地验证一次，genuine challenge evidence 是
  best-effort，不是重复 CI/RC gate。
- 三语文档、migration guide、Skill、ADR 和 release notes 已完成。
- `--debug` 的 CLI/MCP stdout isolation、明确模块名、secret 扫描通过，并在上述同一次本机 solver
  protocol acceptance 中确认诊断链；默认运行不创建项目级日志或 `logs/`。
- full test、race、vet、build、docs、workflow、diff 与 API compatibility gates 全部通过。
