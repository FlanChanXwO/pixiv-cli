# 任务清单

每个任务完成后填写“实际/证据/风险”。每三个实现任务后执行集中检查。

## T01 — 建立隔离 worktree 与干净基线

- 状态：完成
- 实际：创建 `codex/goal-setup` 元数据分支，提交 goal 文件与 `.worktrees/` ignore；在 `.worktrees/v030` 创建 `codex/v030` 隔离工作区。
- 证据：Go 1.26.3；`go test ./...` 退出成功。
- 风险/下一步：基线来自 main 的 v0.2.0 后续提交；下一任务只修改 recovery test overlay，保持 v0.2.0 tag 不变。

## T02 — 修复 v0.2.0 Windows recovery 测试门并扩展白名单

- 状态：完成
- 实际：Windows 下把 config 私有文件 mode 断言改为既有 ACL `0666` 表示，Unix 保持 `0600`；将精确测试文件加入 workflow 与 canonical policy 的 archive/diff allowlist；新增遗漏/额外路径拒绝测试，并修正文档计数。
- 证据：提交 `9a41124`、`8c5739f`；`go test ./pkg/pixiv ./scripts/releaseworkflow`、`sh scripts/test-release-workflow.sh`、`git diff --check 3d59741..HEAD` 均通过；规格与质量复审均批准。
- 风险/下一步：本机非 Windows，实际 Windows gate 留待恢复 workflow 的原生 runner 证实；下一任务只审查并触发 v0.2.0 recovery，不移动 tag。

## T03 — 审查 recovery 修改并 dispatch/验收 v0.2.0 Release

- 状态：完成
- 实际：recovery PR #4、overlay 修复 PR #5 与 update compatibility PR #6 已合并至 main。经过 protected Environment 的两次记录审核后，run `29307406643` 成功发布 v0.2.0、四平台验证并部署 Homebrew formula；User-Agent 修复经 PR #6 的 quality/六平台 CI 合并，为 v0.3.0 分发准备。
- 证据：run `29307406643` 的 validate、六个 build、六个 build_production、trust、publish、formula render、四个 Homebrew verify、tap deploy 全部 success；Release `v0.2.0` 为非 draft/non-prerelease，含六平台 archive 与 `checksums.{txt,json}`，tag commit 仍为 `329711121588d9f054fb3d15540bb0fd6c134e42`。`homebrew-tap` formula version/URL/SHA 与 Release 对齐（commit `710f997`）。PR #6（merge `73a4d5d`）的 quality 和六平台 smoke 全绿，且 `3b46b9f` 是 origin/main 祖先；隔离 HOME/cache 的正式 v0.2.0 buildinfo 连续两次 update check 返回 latest v0.2.0/no update。
- 风险/下一步：v0.2.0 binary/tag 不可重写，故其既有下载资产不会包含之后的 User-Agent 源码修复；该修复已在 main，必须由 v0.3.0 签名 Release 对外分发。下一任务 C01 对恢复链路、tag 不变性、文档与外部发布证据作集中审计。

## T03a — 收敛 recovery overlay 至实际最小审计差异并复验 policy

- 状态：完成
- 实际：将 test-only recovery overlay 的 archive、工作树 diff 断言和 Go canonical verifier 同步收敛为 tag 与当前默认分支实际不同的四条路径；移除遗留的两条 `git add -N`，并把聚焦策略测试改为精确四路径命令、逐条缺失和额外路径拒绝。
- 证据：提交 `59105ed`；TDD RED `go test ./scripts/releaseworkflow -run '^TestCheckRecoveryPolicyRequiresExactFourPathOverlay$' -count=1` 在旧 17 条命令下失败，GREEN 后通过；`go test ./scripts/releaseworkflow -count=1`、`sh scripts/test-release-workflow.sh`、`go test ./...`、`git diff --check origin/main..HEAD` 和 pre-commit 均通过。临时 detached `v0.2.0` worktree 从 `origin/main` archive 四条路径后，`git diff --name-only` 精确等于该集合且 cached diff 为空。规格审查与质量审查均批准。
- 风险/下一步：仍需把修复推送、通过六平台 PR CI，并从默认分支重新 dispatch `release_tag=v0.2.0`；tag 保持不可变，生产构建隔离仍待远端实际 run 验证。

## T03b — 对齐 protected release Environment 与受审计 main recovery dispatch

- 状态：完成
- 实际：在 GitHub `release` Environment 的 custom deployment branch policies 中新增精确 `main` branch 规则；保留既有 `v*` tag 规则、required reviewer 及 `prevent_self_review=false`，未更改 secrets、环境名或 workflow。
- 证据：GitHub API 创建 policy `54575656`（`main` / `branch`）；复查显示 policy 集合仅为 `main` branch 与 `v*` tag，Environment 仍含 required_reviewers 和 branch_policy protection rules；`main` 的 GitHub branch 元数据为 `protected=true`。
- 风险/下一步：需重新从 main dispatch immutable `v0.2.0`；publish 应进入 required reviewer gate，而非 runner 前 branch policy 拒绝。随后验收签名 Release、Homebrew 与更新检查；不得删除这两条 policy 或绕过 reviewer。

## T03c — 修复 GitHub Releases update check 的 User-Agent 兼容性

- 状态：完成
- 实际：给 GitHub Releases 每个分页请求加入固定、非敏感 `User-Agent: pixiv-cli`，保持 ETag、缓存、分页、proxy、timeout、错误和 fallback 语义不变；新增经公开 `GitHubReleaseClient.Check` 的首页/next-page 回归，并更新 Unreleased changelog。
- 证据：提交 `3b46b9f`；TDD RED 显示默认 Go User-Agent/403，GREEN `go test ./internal/update -run '^TestGitHubReleaseClientUsesStableUserAgentForEveryReleasePage$' -count=1`、`go test ./internal/update -count=1`、`go test ./internal/cli -count=1`、`go test ./...`、pre-commit 和 diff check 全通过；规格与质量审查批准。将正式 v0.2.0 buildinfo 编入临时 binary，在隔离 HOME/cache 下连续两次 `pixiv update --check --json` 均返回 source release、latest `v0.2.0`、`update_available=false`，第二次覆盖 ETag cache 重验证。
- 风险/下一步：不可变 v0.2.0 资产不包含该源码修复；必须把修复经 PR CI 合并，使后续 v0.3.0 包含。T03 的 release/Homebrew/tag 已完成，更新修复的公开分发留待 v0.3.0 Release。

## C01 — 集中检查：恢复链路、tag 不变性、文档与外部发布证据

- 状态：完成
- 实际：独立审计 recovery overlay、受保护 Environment、不可变 tag、Release、Homebrew 和 update 修复合并状态；审计发现 ADR 0008 仍把发布写成未部署，已按 v0.2.0 外部事实最小更正并由原审计者复核批准。
- 证据：`go test ./...`、`sh scripts/test-release-workflow.sh`、`go test ./scripts/releaseworkflow -count=1` 和 `git diff --check` 均通过；run `29307406643` 的 21 个 job 全部 success，Release `v0.2.0` 为公开 stable 且含 8 个资产，annotated tag 最终指向 `329711121588d9f054fb3d15540bb0fd6c134e42`，tap formula 已对齐 v0.2.0 URL/SHA。`release` Environment 仍保留 required reviewer 与精确 `main`/`v*` policy；PR #6 的 `3b46b9f` 已为 main 祖先，隔离 HOME/cache 的正式 buildinfo update check 连续两次返回 latest v0.2.0/no update。ADR 更正经独立复核 APPROVE。
- 风险/下一步：v0.2.0 tag/资产不可变，故不包含随后合并的 User-Agent 修复；该修复将随 v0.3.0 发布。下一任务 T04 执行公开 SDK 顶层路径迁移。

## T04 — 将公开 SDK 迁移到顶层 pixiv 并更新全仓 import

- 状态：完成
- 实际：将全部 24 个公开 SDK 文件从 `pkg/pixiv` 物理迁移至顶层 `pixiv`，全仓 Go consumer、公开文档、ADR、PRD、架构说明和 changelog 均改用唯一 import `github.com/FlanChanXwO/pixiv-cli/pixiv`，未保留兼容 package；新增外部 tracer 通过该 import 构造 `Client`。移除与源码目录冲突的 `/pixiv` ignore，并把 CGO 负向构建测试改为临时显式输出，避免同名目录掩盖真实 staticlib 门禁。未来 tag 的 recovery overlay/canonical verifier/tests 同步使用 `pixiv/account_external_test.go`；文档保留 v0.2.0 tag 中旧 `pkg/pixiv/account_external_test.go` 仅为已完成恢复的历史事实。
- 证据：TDD RED 从 `HEAD` archive 执行 `go test ./pixiv -count=1` 因目录不存在退出 1，GREEN 后新增 `pixiv/path_external_test.go` 经顶层公开 import 构造 Client；`go test ./pixiv -count=1`、`go test ./internal/application ./internal/bootstrap ./internal/cli ./internal/mcpserver -count=1`、`go test ./scripts/releaseworkflow -count=1`、`sh scripts/test-release-workflow.sh`、`go test ./... -count=1`、`gofmt -l pixiv` 和 `git diff --check` 均通过。旧公开 Go import 检索为空；规格审查与质量审查均 APPROVE。
- 风险/下一步：本地未跑真实六平台 Release CI 与 opt-in Pixiv E2E；release 验收任务会覆盖前者。知识图谱按显式 T13 统一重建。下一任务 T05 集中协议 profile、endpoint catalog 与脱敏 adapter failure。

## T05 — 集中协议 profile、endpoint catalog 与脱敏 adapter failure

- 状态：完成
- 实际：新增纯内部 `internal/pixiv/protocol`，成为 App/Web/OAuth base、profile headers、OAuth 常量和 App/Web endpoint 的唯一维护点；appapi、webapi、oauth、resource、legacy aliases 与顶层登录 URL 均改为消费该 catalog。adapter 以不保存 body、URL、header、token、cookie 或 envelope message 的 `protocol.Failure` 交给 facade，公开 SDK 经单一 `mapAdapterFailure` 保持既有 typed error 字段和 context 语义。CLI 登录的实际打开 URL、callback state 例外、post-redirect return_to 与 Chromium history 扫描均经顶层 SDK 的 catalog-derived helpers，生产 CLI 不再硬编码 App OAuth host/path；legacy Source 的 Web resource error body 行为明确暂留 T12。
- 证据：公开 SDK profile/failure tracer 覆盖 App HTTP body、Web envelope、OAuth body、普通 transport 脱敏、App/Web headers 与显式 base；Web `io.ReadAll` 失败已被归一为安全 transport failure。首次 tracer 因接手时 protocol 接线已存在而直接 GREEN，未伪造 RED；随后登录 URL helper 的 RED `go test ./pixiv -run TestBuildLoginAuthorizationURLUsesOfficialLoginRoute -count=1`（helper 未定义）及 OAuth helpers 的 RED `go test ./pixiv -run TestOfficialOAuthURLHelpersAcceptOnlyCatalogRoutes -count=1` 均在实现后 GREEN。`go test ./pixiv -count=1`、`go test ./internal/pixiv/... -count=1`、`go test ./internal/application ./internal/bootstrap ./internal/cli ./internal/mcpserver -count=1`、`go test ./... -count=1`、`git diff --check` 均通过；规格审查、P1 窄复审、质量审查和 P2 窄复审均 APPROVE。
- 风险/下一步：未联网验证真实 Pixiv，真实六平台 Release CI 留待发布门禁；legacy `internal/pixiv.Source` 的 Web resource body 兼容语义仍待 T12 删除。下一任务 T06 扩展 User Detail 正规模型与 SDK contract。

## T06 — 扩展 User Detail 正规模型与 SDK contract

- 状态：完成
- 实际：将 App `/v1/user/detail` 扩展为严格四 envelope 的 DTO→internal model→公开 SDK 映射；`UserDetailResult` 现稳定包含 `User`、`Profile`、`ProfilePublicity` 与 `Workspace`，`User` 新增可选 `ProfileImageURLs.Medium`。`profile` 的统计/身份字段、公开性六个布尔字段和 workspace 文本均规范化为公开模型；可选 URL 的缺失/null/空串统一为 nil，隐藏文本/未知计数/布尔值保持零值。四个 envelope 的缺失、null、非 object、解码失败或无效 user ID 均返回脱敏 typed malformed error。legacy `Source.UserDetail` 继续对旧 consumer 返回摘要 User，并把 adapter `(nil,nil)` 显式归类 malformed，避免 panic。
- 证据：TDD RED `go test ./pixiv -run '^TestUserDetailReturnsCompleteStableProfileFromOneAppRequest$' -count=1` 因尚无完整模型字段编译失败，GREEN 后通过；公开测试覆盖完整映射、四 envelope malformed/错误 metadata 脱敏、optional URL nil 与零值，internal mapper/legacy Source 测试覆盖三段隔离与 nil adapter。`go test ./pixiv -count=1`、`go test ./internal/pixiv/appapi ./internal/pixiv -count=1`、`go test ./internal/application ./internal/bootstrap ./internal/cli ./internal/mcpserver -count=1`、`go test ./... -count=1`、`git diff --check` 均通过；规格审查、质量审查与 P2 nil-safe 窄复审均 APPROVE。
- 风险/下一步：默认测试未访问真实 Pixiv，字段兼容性将在 T14 opt-in App API canary 复核；CLI/MCP 暂未暴露完整详情。按三个实现任务后的门禁，下一任务 C02 集中检查 SDK 路径、协议边界与公开模型稳定性。

## C02 — 集中检查：SDK 路径、协议边界、公开模型稳定性

- 状态：完成
- 实际：独立审计 T04–T06 的 SDK 路径、protocol adapter 边界、完整 User Detail 和跨切片文档事实。审计发现 README/architecture 仍把当前正式发布误写为 v0.1.1，已最小更正为已验收的 v0.2.0，并保留“wiring 不替代每个版本独立发布验收”的安全边界；原审计者窄复核批准。
- 证据：`go test ./... -count=1`、`go vet ./...`、`git diff --check origin/main..HEAD` 均通过；`go list ./...` 只列出顶层公开 `github.com/FlanChanXwO/pixiv-cli/pixiv`，根 SDK 有 26 个已跟踪文件。旧 `pkg/pixiv` 仅出现在 v0.2 immutable recovery 历史说明与 releaseworkflow mutation fixture；CLI/MCP 未直连 appapi/webapi/oauth/resource。审计确认 protocol 是生产 base/profile/endpoint/failure 单一来源，UserDetail 四 envelope/可选 URL/零值/脱敏/App-only 与 legacy nil-safe 均有回归覆盖；文档 P2 经独立窄复审 APPROVE。
- 风险/下一步：完整 `UserDetailResult` 的用户可见 SDK 新字段尚待 T13 在 CHANGELOG 汇总；知识图谱亦按 T13 重建。真实 Pixiv API canary 和六平台 Release CI 留待 T14/T15。下一任务 T07 新增 CLI `pixiv user detail USER_ID` 并保持 SDK 单链路。

## T07 — 新增 CLI user detail 与 SDK 单链路调用

- 状态：完成
- 实际：新增必填 `pixiv user detail USER_ID`，仅经一次 `SDK.OpenOperation` 调用公开 `SDKClient.UserDetail`；`--json` 直出完整 `UserDetailResult`，文本仅输出必要非空资料、四项公开计数和非空 workspace。文本网页仅保留无 userinfo/query/fragment 的 HTTP(S) 地址。补齐 SDK 接口演进所需的 MCP 测试 fake，但未新增 MCP tool；README 与 Unreleased 已同步。
- 证据：TDD 覆盖公开 `Run` 边界的完整 JSON、SDK 请求/单次 factory、refresh-token/uid/proxy/no-proxy 透传、文本空字段省略与 URL 脱敏、无效 ID 不打开 SDK、typed SDK error stdout 为空；早期基线缺少 `UserDetail` SDK 接口和 CLI 子命令，新增 tracer 无法编译，后续 GREEN。`go test ./internal/cli -count=1`、`go test ./internal/application ./internal/bootstrap -count=1`、`go test ./pixiv -count=1`、`go test ./internal/mcpserver -count=1`、`go test ./... -count=1`、`gofmt -d`、`git diff --check` 均通过；规格审查批准，质量审查发现的 gofmt P2 已修复并窄复审批准。
- 风险/下一步：默认测试不访问真实 Pixiv；完整详情的真实上游兼容性留待 T14 opt-in canary。下一任务 T08 仅新增 MCP `user_detail` 并使用同一 SDK 调用链。

## T08 — 新增 MCP user_detail、更新 tool skill 与文档

- 状态：完成
- 实际：注册 MCP `user_detail`，只接受必填正整数 `user_id`；handler 经一次 `openSDKOperation` 后只调用公开 `SDKClient.UserDetail`，成功 structured output 直接使用完整未裁剪的 `UserDetailResult`。认证/配置、typed SDK 和 nil result 失败均返回安全文本与 `isError=true`，不走 legacy Source 或匿名 Web fallback。同步 README、MCP tools 文档和 Unreleased。
- 证据：TDD RED `go test ./internal/mcpserver -run '^TestSDKUserDetailReturnsStructuredSDKResult$' -count=1` 在基线报告 `unknown tool \"user_detail\"`，GREEN 后测试覆盖完整四区段、SDK request、一次 operation；增量覆盖零/负/缺失/类型错误均不打开 SDK、typed SDK 失败和无 SDK 配置。`go test ./internal/mcpserver -count=1`、`go test ./internal/application ./internal/bootstrap ./internal/cli -count=1`、`go test ./pixiv -count=1`、`go test ./... -count=1`、`gofmt -d` 与 `git diff --check` 均通过；规格审查和质量审查均 APPROVE。
- 风险/下一步：默认测试不访问真实 Pixiv；真实 App API 兼容性留待 T14 opt-in canary。下一任务 T09 扩展小说、作者、漫画三类推荐 SDK 与稳定模型。

## T09 — 新增小说/作者/漫画推荐 SDK 与稳定模型

- 状态：完成
- 实际：保留插画推荐并新增原子 `MangaRecommended`、`NovelRecommended`、`UserRecommended`。漫画固定复用 `/v1/illust/recommended?content_type=manga`；小说和作者 endpoint 统一收敛进 protocol catalog。小说仅公开必要稳定字段；作者推荐仅公开作者与插画/小说预览。四类各用不同 Operation、query digest 与 opaque cursor，不公开 raw `next_url`、ranking、privacy policy 或 UI 字段；均为认证 App-only 路径，无 Web fallback。
- 证据：TDD RED 为公开 `MangaRecommended` 未定义，GREEN 验证 catalog URL 与 `content_type=manga`；随后 `NovelRecommended`/request 未定义 RED，GREEN 验证稳定小说映射与 continuation 隔离。公开 SDK 回归覆盖作者预览、四类 cursor 互斥、原始 next_url 不泄漏、小说/作者 malformed envelope/continuation，以及未认证不触网。`go test ./pixiv -count=1`、`go test ./internal/pixiv/appapi -count=1`、`go test ./internal/application ./internal/bootstrap ./internal/cli ./internal/mcpserver -count=1`、`go test ./... -count=1`、`gofmt -d`、`git diff --check` 均通过；规格与质量审查均 APPROVE。
- 风险/下一步：默认测试使用 httptest，不访问真实 Pixiv；endpoint/response 与真实上游兼容性留待 T14 opt-in App API canary。CLI/MCP 尚未暴露三类新推荐，下一轮按门禁执行 C03，随后 T10/T11 接入。

## C03 — 集中检查：用户详情和推荐模型、认证、错误与分页

- 状态：完成
- 实际：独立审计 T06–T09 的 User Detail、四类推荐、认证/错误和分页边界，未发现阻塞问题。确认 UserDetail 的四个 required envelope、nil-safe legacy 适配与公开 typed error；插画/漫画/小说/作者推荐均为 App-only，漫画复用正确 catalog，四 Operation/digest/cursor 互不兼容，`next_url` 只解析数值 continuation 且不进入公开模型。CLI/MCP 未直连底层 adapter；三类新推荐尚未提前暴露，按计划留给 T10/T11。
- 证据：`go vet ./...`、`go test ./pixiv ./internal/pixiv/appapi ./internal/application ./internal/bootstrap ./internal/cli ./internal/mcpserver -count=1`、`go test ./... -count=1`、`git diff --check origin/main..HEAD` 均通过。导入检索确认 CLI/MCP 不导入 appapi/webapi/oauth/resource；独立审计复查 DTO→model→public、malformed envelope/ID/continuation、未认证不触网、cursor 交叉拒绝和无 raw `next_url` 后 APPROVE。
- 风险/下一步：默认测试仍未访问真实 Pixiv；小说/作者推荐的实际 endpoint/wire shape 需在 T14 显式、脱敏 opt-in App API canary 复核。下一任务 T10 实现 `pixiv recommended all|illust|manga|novel|user`，并扩 SDKClient 接口及测试 fake。

## T10 — 改造 CLI recommended 子命令与原子输出

- 状态：完成
- 实际：将 CLI 改为必填 `pixiv recommended all|illust|manga|novel|user`。`all` 在一个 SDK/account snapshot 内按插画、漫画、小说、作者顺序读取，`--limit`/`--page` 对每条流独立生效。`all` 的文本与 JSON 都先落入私有临时文件，四类全部成功后才提交 stdout；`--limit 0` 逐项落盘而不累积结果。JSON 为稳定单对象 `{illusts,manga,novels,user_previews}`，单类 key 与对应 all 分段一致。SDKClient/fake 同步扩口；README/Unreleased 已更新。
- 证据：TDD RED 为旧 `recommended all --json` 因无 kind 被 usage 拒绝，GREEN 验证单次 factory 与固定四类调用顺序；补充 Run 边界测试覆盖 all 出错 stdout 为空、无/未知 kind 不开 SDK、每流 `page=2 limit=1` 使用独立 cursor、单类漫画与 all 的 `manga` JSON schema 一致。规格审查发现的 P1 schema 不一致已修复并窄复审；质量审查发现的 JSON spool header 写失败临时文件泄漏 P2 已修复、测试并窄复审关闭。`go test ./internal/cli -count=1`、`go test ./internal/application ./internal/bootstrap ./internal/mcpserver -count=1`、`go test ./pixiv -count=1`、`go test ./... -count=1`、`gofmt -d`、`git diff --check` 均通过。
- 风险/下一步：真实认证推荐流留待 T14 opt-in canary；T11 将以相同 SDK 接口新增 MCP `recommended(kind)`，并保留旧 `illust_recommended` 兼容行为。

## T11 — 新增 MCP recommended(kind) 并保留旧 tool 兼容

- 状态：完成
- 实际：新增 MCP `recommended`，强制 `kind=all|illust|manga|novel|user`。新 tool 只经一次 SDK operation 调用公开 SDK；`all` 固定依次读取插画、漫画、小说、作者四流，`page`/`limit` 分别作用于每流，任一流失败时不返回局部 structured data。输出只含必要推荐、作者 preview、按流 pagination，SDK cursor 不离开 MCP 边界。为满足 MCP schema，适配层把可选预览数组和小说 tags 归一为空数组。旧 `illust_recommended` 与 `download_random_from_recommendation` 仍保留 legacy Source 行为；README、MCP 工具文档与 Unreleased 已同步。
- 证据：基线未注册 `recommended`；新增 MCP tracer 覆盖四流顺序、单次 SDK factory、单类 kind、缺失/类型错误/未知 kind、无 SDK 配置、每流第二页、all 失败无局部输出及 structured schema。新增随机下载兼容回归经真实 MCP 调用断言 legacy `IllustRecommended` 一次、SDK factory 零次和下载 ID `77`。`go test ./internal/mcpserver -count=1`、`go test ./internal/application ./internal/bootstrap ./internal/cli -count=1`、`go test ./pixiv -count=1`、`go test ./... -count=1`、`gofmt -l`、`git diff --check` 均通过；规格复审、兼容窄复审和质量复审均 APPROVE。
- 风险/下一步：默认回归不访问真实 Pixiv；四类认证推荐的 wire 兼容性留待 T14 显式 opt-in canary。下一任务 T12 迁移其余 CLI/MCP/download 至 SDK 并删除 legacy Source 双栈，但不得移除本任务明确保留的兼容语义，除非后续计划另行审定。

## T12 — 迁移剩余 CLI/MCP/download 到 SDK，删除 legacy Source 双栈

- 状态：未完成
- 实际：
- 证据：
- 风险/下一步：

## C04 — 集中检查：能力矩阵、架构导入门禁、完整回归

- 状态：未完成
- 实际：
- 证据：
- 风险/下一步：

## T13 — 同步 README、MCP 文档、ADR、CHANGELOG 和知识图谱

- 状态：未完成
- 实际：
- 证据：
- 风险/下一步：

## T14 — v0.3.0 Release 候选门禁与 opt-in API canary

- 状态：未完成
- 实际：
- 证据：
- 风险/下一步：

## T15 — 创建并发布不可变 v0.3.0，验收 Release/Homebrew/更新

- 状态：未完成
- 实际：
- 证据：
- 风险/下一步：

## C05 — 终审

- 状态：未完成
- 实际：
- 证据：
- 风险/下一步：
