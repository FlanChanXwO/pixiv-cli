# v1.0.0 测试、迁移与发布门禁

## Public SDK

- 为 `sdk.Page[T]`、Cursor Text/JSON codec、query/product/operation/identity binding 编写契约测试。
- 为 `sdk.Error` 的 sentinel、`errors.Is/As`、context chain、retry advice 与全部脱敏字段编写测试。
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
- 验证 pool selection、freeze、过期清理、删除账号和多进程竞争的事务语义；不引入独立 lease/status 表。
- 验证 JSON migration 的 crash/re-entry、DB/JSON 一致与冲突、config write failure、legacy 删除
  failure 和 commit outcome。
- 在 Unix-like 验证目录、DB、journal/temp 权限；Windows 验证私有 ACL，不声称 POSIX mode。
- auth export/restore 只经逻辑 repository，禁止复制数据库文件或输出额外 secret。

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
- browser provider 与普通 contract test 精确区分 challenge 与普通 403，且不运行挑战绕过；唯一例外是
  下文显式隔离的本机 FlareSolverr release-prep 辅助验证。
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

## 真实 SDK E2E 与本机凭据

v1.0.0 release evidence 必须至少包含一次真实 Pixiv SDK 读取和一次真实 FANBOX SDK 读取；offline
fixture、CLI 间接成功或 FlareSolverr 返回的页面不能替代 SDK Client 的真实结果。E2E 默认关闭，
只在用户显式授权且本机凭据存在时运行。开工前不要求联网探测；入口存在性与联网成功是两项独立
evidence，后者只在最终 release-prep 判定。

- Pixiv E2E 通过本机 `pixiv-cli` 的显式 auth export/repository 边界取得当前选中账号的 refresh
  token，只在测试进程内存中传给 `pixiv.Open`；不得进入 shell argv、环境 dump、日志、test name、
  artifact 或失败 diff。rotation 后的新 credentials 必须按正常契约持久化，不能继续复用旧 RFT。
- FANBOX E2E 当前授权 session 保存于 macOS Keychain，service 为 `pixiv-cli-e2e-fanbox`、account 为
  `fanbox-e2e`。测试只读取 `FANBOXSESSID`，不保存或恢复 GA、广告标识、行为数据及短期 Cloudflare
  Cookie。session 失效时明确报 `credentials_expired` 并要求重新导入，不 fallback。
- E2E 首先验证当前身份，再读取一个稳定、低副作用的 detail/list 与至少一个 `Resource`；mutation、
  支持关系变更和批量下载不作为默认真实 E2E。所有返回和失败路径执行 secret/signed-query 扫描。
- Keychain item、refresh token、测试响应和下载内容都属于本机私有状态，不提交 Git，也不进入 CI
  secret-less job。跨平台 CI 继续只使用 synthetic fixtures。

## FlareSolverr 测试边界

v1.0.0 release-prep 必须在本机 Docker 中安装并启动一次经过核对、固定 tag/digest 的 FlareSolverr，
完成 health check 与一条 FANBOX 测试辅助请求，用于确认 challenge 诊断路径。执行时不得使用浮动
`latest`，容器只绑定 loopback，不挂载仓库、浏览器 profile、Docker socket 或 `pixiv-cli.db`，凭据
只通过测试进程临时传递。该容器运行不是每次 CI 或普通开发测试的前置条件。

FlareSolverr 不进入 `go.mod`、生产 image、CLI/MCP 配置、公开 SDK option 或默认测试依赖。E2E 报告
必须分别记录直连 SDK 结果与 FlareSolverr 辅助结果；辅助成功不得把 SDK 的 `challenge_required`
改写成直连成功，也不得触发生产代码自动 fallback。容器不可用时，普通 SDK/CLI/MCP 与 offline
test 必须完全不受影响。

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

迁移指南必须提供旧 SDK symbol 到新 symbol/removed 的完整矩阵、`auth.json` 自动迁移行为、默认
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

同时运行 documentation tests、workflow policy、public API compatibility 与全部 native browser
provider jobs。普通 CI 不要求真实凭据或 FlareSolverr；但 v1.0.0 最终发布前必须在授权的本机
release-prep 中取得上述 Pixiv/FANBOX 真实 SDK evidence。凭据失效或网络 challenge 导致无法完成时，
必须记录为 release blocker/risk 并更新凭据或环境，不能把 offline fixture 当作真实 E2E 成功。

完整执行顺序、凭据读取边界、Firefox 临时验证、FlareSolverr 隔离方式、evidence 字段与清理步骤见
[最终验证操作手册](release-prep-runbook.md)。

## v1.0.0 发布条件

- public API inventory 已冻结且没有未决命名、零值或错误语义。
- v0 `/pixiv` 到 v1 `/sdk/pixiv` 的迁移矩阵、不可变 tag 和 v1 package compatibility gate 已验证。
- Pixiv/FANBOX credential、rotation、cursor、URL reference、identity/time、native resource、ugoira
  与 stdout isolation review 通过。
- 三个平台的浏览器 provider 都有 native evidence；Safari 仅要求 macOS。
- SQLite migration、权限、并发与 crash recovery evidence 完整。
- 两个 MCP server 的 tool inventory 与文档一致。
- 真实 Pixiv/FANBOX SDK E2E evidence 已完成；报告不包含 credential、Cookie、signed query 或内容。
- 本机 FlareSolverr 测试辅助容器已按固定 digest 运行一次并留下脱敏 evidence；SDK 直连 evidence
  与该结果分开记录。
- 三语文档、migration guide、Skill、ADR 和 release notes 已完成。
- full test、race、vet、build、docs、workflow、diff 与 API compatibility gates 全部通过。
