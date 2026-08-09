# v1.0.0 RC follow-up 验证记录（2026-08-08）

本记录对应隔离 worktree `codex/v1-sdk-rewrite`，只记录脱敏的自动门禁与当前
release evidence 状态，不包含 credential、Cookie、私有 URL、下载内容或上游响应。

## 已完成的实现范围

RC-1 至 RC-10 与内部架构重组已实施：public SDK `Reason` 命名、严格 unknown-option 解析、authdb
schema/CAS 与手工 bundle migration、数据库账号调度、Pixiv/FANBOX service-scoped network、
FANBOX native route/profile/resource、challenge-only FlareSolverr、typed diagnostics、
CLI/MCP wiring、三语 public contract、产品 skill，以及 application/bootstrap/filesystem/
persistence/downloader/ugoira/CLI/MCP/update 的最终目录边界。对应专题文档的状态已同步更新。

实现没有新增无证据的整请求 timeout、数据截断、重试上限、静默 fallback、代理池或 Cookie pool。
FANBOX solver 仅用于 challenge recovery；普通 API/resource 请求仍走 native transport。E2E 入口已收敛为
本地 authdb/Keychain credential boundary，release workflow 只执行不读取 credential 的 SDK contract gate；
真实服务 evidence 仍由获得授权后的 release-prep 手工运行取得。真实 FANBOX 入口现在要求显式非 secret
creator/tag/post/page URL targets，禁止自动发现或在资源缺失时 skip。

本轮 follow-up 又补齐了 FANBOX `Creators` 的公开 continuation、`OpenResource` 的 `GET`/`HEAD`、
Range 与 conditional header 传递，以及 Pixiv E2E 的当前 credential revision/账号 identity 校验。
相应的 SDK、service、E2E 聚焦回归与三语 SDK 契约已同步；全量 gate 在这些修改后重新执行并保持通过。
在获得明确授权后，本轮还取得了真实 Pixiv public SDK evidence 与一次性 real-solver protocol acceptance；
两者与仍待取得的真实 FANBOX 内容/资源和 native browser evidence 分开记录。

## 自动验证

以下命令均在 worktree 根目录运行，测试使用 `GOPROXY=off` 保持离线可复现：

| 命令 | 结果 |
| --- | --- |
| Go LSP diagnostics（最后修改后） | 通过；所有最后修改的 Go 文件无 compiler/type/unused diagnostics |
| `GOPROXY=off go test ./... -count=1` | 通过 |
| `GOPROXY=off go test -race ./... -count=1` | 通过 |
| `GOPROXY=off go vet ./...` | 通过 |
| `sh scripts/build.sh` | 通过；生成 `build/pixiv` |
| `GOPROXY=off go test ./scripts/documentation -count=1` | 通过 |
| `GOPROXY=off go test ./e2e -count=1 -v` | 通过；真实 SDK 场景按无凭据契约跳过，脚本/metadata 测试通过 |
| `PIXIV_E2E_BINARY="$PWD/build/pixiv" PIXIV_E2E_EXPECTED_VERSION=dev GOPROXY=off go test ./e2e -run '^TestPixivBinaryPackagedSmoke$' -count=1 -v` | 通过；packaged binary、root version/config path 与 Pixiv/FANBOX MCP help smoke 通过 |
| `bash scripts/test-e2e.sh --help` | 通过；入口只接受当前 SDK E2E selector，FANBOX target 仅为显式非 secret 环境配置 |
| `GOPROXY=off go run ./scripts/publicapi -check -golden docs/maintainers/public-api-inventory.md` | 通过 |
| `GOPROXY=off go test ./scripts/platformsmokeworkflow -count=1` | 通过；workflow 仅执行 packaged smoke |
| `GOPROXY=off go test ./scripts/browsernativeevidence -count=1` | 通过；YAML matrix/checksum swap policy、隔离 Firefox path/env、可执行文件与 synthetic schema seed tests |
| `GOPROXY=off go run ./scripts/browsernativeevidence policy --workflow .github/workflows/browser-evidence.yml` | 通过；六目标 provider matrix、固定 Firefox 153.0.3 package checksum、清理与 credential boundary |
| `actionlint .github/workflows/browser-evidence.yml` | 通过；browser provider 与 temporary Firefox job 的 workflow syntax |
| `GOPROXY=off pre-commit run --all-files` | 通过；gofmt 与 `go test ./...` 均通过 |
| `git diff --check` | 通过 |
| `GOPROXY=off go test ./internal/browsercookies/... -count=1` | 通过；合成 provider、Chromium GCM/CBC、Safari 多页与错误分类 |
| `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 GOPROXY=off go test -c`（browsercookies chromium/secret） | 通过；编译 DPAPI 路径 |
| `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 GOPROXY=off go test -c`（browsercookies chromium/secret） | 通过；编译 Secret Service 路径 |
| `GOPROXY=off go test ./internal/ugoira -run '^TestRustUgoiraEncoderNativeGIFAndAPNG$' -count=1 -v` | 通过；更新后的唯一 native FFI 入口 |
| downloader source digest/staticlib manifest tests | 通过 |
| `go run ./scripts/nativeevidence policy --workflow .github/workflows/native-evidence.yml` | 通过 |
| `sh scripts/test-release-workflow.sh` | 通过 |
| `go run ./scripts/licensebundle --check` | 通过；六个 release target 的离线 license closure 一致 |

聚焦覆盖包括 authdb migration/CAS 与 pool selection、CLI parser/startup side effects、
FANBOX route/UA/proxy/Cookie/challenge、solver waiter cancellation、diagnostic redaction、
MCP request scope、public API inventory、Runtime 初始化错误传播、架构目录/导入边界和三语文档契约。

`CGO_ENABLED=0 GOPROXY=off go test ./internal/ugoira -count=1` 按预期拒绝编译，错误明确指出
Go 1.26.3、CGO、Rust staticlib 与 target C linker 要求；这是保留的 deliberate compile-gate，
不是回归失败。

## RC-11 外部 release evidence（2026-08-08）

以下证据在本 worktree、Go 1.26.3 与已授权的本机网络/Docker 环境取得；只记录脱敏结果：

| 场景 | 结果 | 脱敏 evidence |
| --- | --- | --- |
| `PIXIV_SDK_E2E=1` public SDK read/resource | PASS | `PIXIV_E2E_PROXY` 显式指向本机 loopback proxy；测试从 selected local authdb 读取账号，完成 identity、动态 search、artwork 与 resource HEAD，并按 credential revision 规则持久化 rotation；stdout/stderr 未记录 credential。 |
| 一次性 real-solver protocol acceptance | PASS | `ghcr.io/flaresolverr/flaresolverr:v3.5.0@sha256:139dfee1c6f89249c8d665d1333a42e8ec74ec0a86bc6bb1c8461e10d3a66a47`；loopback-only 临时容器，Chromium 148 启动成功；synthetic native challenge → 真实匿名 FANBOX homepage solve → synthetic native replay，native 共两次请求，容器已删除。solver upstream proxy 通过独立配置显式提供。 |
| 固定 Firefox 153.0.3 temporary profile replay | PASS（本机 macOS arm64） | 官方 DMG 分片完整下载并 SHA-256 校验；真实 Firefox 生成隔离 profile/schema，`TestRealNativeBrowserProvider/firefox` discovery/read 通过，恰好读取一个 synthetic allowlisted cookie；临时包、profile 与数据库已清理。该结果不替代六目标 native runner 或真实用户 profile evidence。 |
| `FANBOX_SDK_E2E=1` target `ro7274/12373249` public SDK operations | PARTIAL / PENDING | 用户已提供 creator 与 post target。修复 `post.listHome`/`post.listSupporting` 的上游 `limit=10` 后，`ValidateSession`、`CurrentUser`、`Creator`、`CreatorTags`、`Creators`、`Home`、`Supporting`、`CreatorPosts` 与显式 tag query 均到达真实 API；该 creator 的 featured/post tag 列表为空，显式 `all` query 返回合法空页。默认 native `Post` 返回 `challenge_required`；显式 FlareSolverr SDK recovery 可继续取得该 post 详情，但详情没有 file attachment，因而按契约在资源阶段失败。没有将此记为资源成功，也没有输出 credential、正文或 signed URL。 |

随后对同一 creator 公开索引中的另外三篇免费帖子进行了显式 SDK recovery 探测：`12246608` 与 `12225489`
能够取得详情但没有 file attachment，`12246623` 仍返回 `challenge_required`。对索引中五篇 500 日元
限制帖的探测结果为：`12378168`、`12366301`、`12220263` 返回无 post body，`12310314`、`12220589`
返回 `challenge_required`；当前授权 session 没有任何一篇形成可验证的 file attachment。上述探测没有
自动改变 `FANBOX_E2E_*` 的固定 target，也没有输出 credential、正文或 signed URL，因此当前没有证据
表明该 creator 的公开可读帖子能完成 file-resource 闭环。

## RC-11 follow-up runner / session evidence（2026-08-09）

本节补记最后一轮 runner 修复、外部门禁和用户提供 session 的脱敏结果；不记录 cookie、响应正文、
signed URL 或下载内容。

| 场景 | 结果 | 脱敏 evidence |
| --- | --- | --- |
| 最终 Quality workflow（`7426549`） | PASS | [run 31269321296](https://github.com/FlanChanXwO/pixiv-cli/actions/runs/31269321296)；Classify、Windows login handler contracts 与 Linux quality 均成功。 |
| Platform smoke workflow | PASS | [run 31269025337](https://github.com/FlanChanXwO/pixiv-cli/actions/runs/31269025337)；macOS/Linux/Windows 的 amd64/arm64 六个 packaged smoke 与 gate 均成功。该 run 使用的 `0d29c05` 已包含平台修复，后续 `7426549` 仅收紧 Windows 测试隔离。 |
| 当前提交 `e310732` 的 Quality workflow | PASS | [run 31289026912](https://github.com/FlanChanXwO/pixiv-cli/actions/runs/31289026912)；在 feature ref 手动 dispatch，Classify、Windows login handler contracts 与 quality gate 均成功。 |
| 当前提交 `e310732` 的 Platform smoke workflow | PASS | [run 31289027915](https://github.com/FlanChanXwO/pixiv-cli/actions/runs/31289027915)；在 feature ref 手动 dispatch，六个 macOS/Linux/Windows amd64/arm64 packaged smoke 与总 gate 均成功。 |
| 受保护的 native evidence workflow | BLOCKED BY POLICY | feature branch 的六目标 job 被 `Require audited main ref` 保护条件拒绝；没有通过推送 main、tag 或 release 绕过。 |
| browser evidence workflow dispatch | UNAVAILABLE | 默认分支尚未注册该 feature-branch workflow，直接 dispatch 返回 HTTP 404；没有通过 main/release 绕过。 |
| 用户提供的 FANBOX session（`ro7274`） | PARTIAL / PENDING | session validation 通过；当前 session 对作者索引中的十个显式 post target 逐一运行 native SDK，均为 `challenge_required`。修正后的三-cookie web probe 返回 HTTP 403 非 JSON；没有取得新的 post body 或 file attachment。session 只在进程内使用，未写入仓库、日志或 artifact。 |
| 隔离 headed Chromium 的 FANBOX 页面验证（2026-08-09） | PARTIAL / PENDING | 通过 Keychain 的 `FANBOXSESSID` 注入隔离浏览器后，作者列表明确显示 `12373249` 为公开帖，并确认当前账号是“正在关注”而非“正在赞助”。公开帖子 `12120175`、`12108370`、`12032687`、`12246608` 与 `12246623` 能渲染文章；页面只发现 `downloads.fanbox.cc/images/post/...` 图片链接，没有 file attachment。目标 `12373249` 在该会话下仍未挂载文章内容；付费帖子显示需要月费高于 500 日元，未在未授权情况下访问。浏览器 session、临时 profile 与输出 artifact 均已清理。 |
| 补充复核历史公开 file target（`aak/12221352`） | NOT REPRODUCED / PENDING | 使用当前 public SDK、同一 Keychain session 与显式 loopback proxy 重跑；身份与前置 operation 未作为 ro7274 目标的替代，单帖 `Post` 在当前出口返回 `challenge_required`，因此没有发起资源请求。历史手工 Firefox 记录仍仅作为历史背景，不升级为本轮 SDK file-resource evidence。 |
| 用户新增目标 `nakkemos/3625356` 的 headed browser evidence | PASS（浏览器路径） | 修正 Chromium modern host-digest 解密后，Edge `Default` profile provider 返回恰好一个非空 allowlisted session；headed Edge 先访问 `api.fanbox.cc` 再打开帖子，`post.info` 返回 HTTP 200 JSON（1820 bytes），`fileMap` 有 2 个 MP4。浏览器 fetch 两个附件均 HTTP 200，实际大小 `3,440,013` 与 `3,376,037` bytes，分别等于 `post.info` 声明大小；完整内容只写入临时 `/tmp` evidence，未进入仓库。 |
| 用户新增目标 `aak/11870583` 的 headed browser evidence | PASS（浏览器路径） | 同一 headed Edge/API-host warm-up 路径取得 HTTP 200 `post.info` JSON（2059 bytes）；文章 body 完整可读（9 blocks），`fileMap` 为空，因此没有可下载附件，未伪造资源成功。完整 JSON 只写入临时 `/tmp` evidence，未进入仓库。 |
| 用户新增目标的 Go SDK native/solver replay | PARTIAL / PENDING | 两个显式 `FANBOX_SDK_E2E=1` 目标的身份、creator、列表与 tag 前置 operation 均通过；native `Post` 与显式固定 digest FlareSolverr replay 仍为 `challenge_required`。这是 SDK direct transport 的独立发布门禁，不否定上面已完成的 headed browser `post.info`/resource evidence；没有把 browser session、正文或 signed URL 写入仓库。 |
| `tls-client` profile matrix re-probe（2026-08-09 resumed） | FAIL / PENDING | 同一 Keychain session 在 `Chrome_146`、`Chrome_144`、`Chrome_133`、`Chrome_131` 与 `Firefox_148` 五种 native profile 下，对两个新增 post 都分类为 `challenge_required`；探针只输出 profile、post id 与 error class，不输出 Cookie、响应正文或 URL 参数。 |
| FlareSolverr anonymous API-host root probe | INSUFFICIENT / PENDING | 固定 digest 临时容器请求匿名 `https://api.fanbox.cc/` 根页返回 solver HTTP 200、页面 HTTP 200，但只返回 `__cf_bm`，没有 `cf_clearance`；没有把 `post.info`、帖子或文件 URL 交给 solver，容器已停止并清理。该 probe 不改变生产 solver 的 homepage-only contract。 |
| headed Edge browser re-probe（当前 session 状态） | NOT REPRODUCED / PENDING | 新的隔离 Edge profile 可加载 FANBOX 公共页面，但访问 API host 与两个目标的 `post.info` 又返回 Cloudflare HTML/403；即使加入与 Playwright automation profile 一致的反检测 flag 仍未恢复。此前 PASS evidence 保留为当时有效的历史结果，当前 session 状态不升级为新的 PASS。 |
| 已连接 Edge extension 的现有用户标签与资源下载（2026-08-09） | PASS（仅 browser 路径） | 不读取 Cookie 值，直接接管精确匹配的现有 `aak/11870583` 用户标签；文章 body 完整可见，实际 DOM 没有 FANBOX 一方视频/下载节点，只有外部嵌入，因此不把外部内容冒充 `fileMap`。同一浏览器会话此前观察到 `nakkemos/3625356` 的两个第一方 MP4，并用页面资源 bundle 下载成功；两项均为合法 MP4，大小为 `3,440,013` 与 `3,376,037` bytes，临时 artifact 已在本轮验证后清理。该路径证明了现有浏览器权限可直接读取页面/资源，但不升级 Go SDK direct transport 的 challenge 门禁。 |
| 页面内浏览器授权 `post.info` re-probe（2026-08-09 resumed） | PASS（仅 browser 路径） | 不导出 Cookie，使用现有 Edge 会话的页面执行上下文发起授权 GET：`aak/11870583` 取得 HTTP 200 JSON（2059 bytes，`fileMap` 计数 0）；同一会话随后取得 `nakkemos/3625356` HTTP 200 JSON（1820 bytes），页面渲染 1 个 article、2 个第一方下载链接和 2 个已加载 video。Nakk 的页面资源 bundle 首次下载 1 个，第二个通过页面原生 media download 完成；前一轮独立 bundle 已对两项校验为合法 MP4，大小分别为 `3,440,013` 与 `3,376,037` bytes。紧接重复 API 请求又返回 403，证明 challenge 状态短时变化；该 evidence 仍只属于 browser 路径。 |
| public SDK topology comparison（2026-08-09 resumed） | FAIL / PENDING | 固定 digest FlareSolverr 容器已启动并只收到匿名 `https://www.fanbox.cc/` 首页请求；native 与 solver 均使用显式 loopback proxy 时，Nakk public SDK `Post` 仍为 `challenge_required`。去掉 native 与 solver proxy 的直连对照在 `ValidateSession` 阶段为 `upstream_error`。两种结果均未进入资源读取，容器和临时日志已清理；没有以 browser `post.info` 结果替代 SDK direct gate。 |
| post-only FANBOX SDK E2E harness（当前 worktree） | PASS（离线契约） | `TestFanboxPostTargetRequiresOnlyPostValues`、`TestE2EScriptSelectsFanboxPostOnly` 与 `go test ./e2e -count=1` 通过；`--fanbox-post-only` 只需要单帖 ID/page URL，允许 `post.info` 返回零 file assets。该结果只证明验收入口语义，不是新的真实 API evidence。 |
| 用户在同一网络出口重新登录后的 Edge extension re-probe（2026-08-09） | PASS（browser） / FAIL（SDK direct） | 不读取 Cookie 值，复用现有登录 Edge 会话。页面上下文对 `aak/11870583` 与 `nakkemos/3625356` 的 `post.info` 分别取得 HTTP 200 JSON（2059、1820 bytes）；页面资源清单显示 Aak 无 FANBOX 一方 video、Nakk 有 2 个 video。Nakk 两项资源通过页面资源 bundle 下载成功（2/2、0 failures），大小为 `3,440,013` 与 `3,376,037` bytes，均为 MP4；临时 artifact 已清理。使用本机当前 Keychain session 运行 `TestRealFanboxSDKPostInfo` 仍在 `Post` 阶段返回 `challenge_required`；浏览器重新登录不等同于更新该 Keychain 条目，也没有把 browser 结果替代 public SDK direct gate。 |

上述 Quality 与 Platform workflow 只用于 CI 质量和 packaged smoke 验证，没有触发任何发版操作。第一轮
runner 失败及其修复原因也已保留在实现提交历史：packaged smoke 的自动 update 提示已在隔离 profile
中关闭，Windows handler contract 已缩小为不初始化 authdb 的 root wiring 测试与完整 `loginhelper` 测试。

## 仍待取得的 release evidence

真实 FANBOX target 已由用户补齐；新增的 `nakkemos/3625356` 与 `aak/11870583` 已通过 headed
browser 路径形成完整的 `post.info` evidence，其中前者的两个 file attachment 也已完整读取并通过
声明大小校验，后者的 `fileMap` 为空。仍待取得的是 `ro7274/12373249` 的可验证 file-resource
闭环，以及两个新增 target 在 Go SDK native/solver transport 上的稳定 `Post` 结果；它们当前仍在
`challenge_required` 阶段。资源门禁仍必须从合法 `post.info` 详情发现 file attachment 并完整读取，
不能用 cover/preview 或 solver 页面替代；本轮 headed browser evidence 不替代 SDK direct transport
门禁。最新 native profile、匿名 solver root、隔离 profile headed browser re-probe 与本轮两种 SDK
topology 对照仍未形成新的 direct SDK 成功证据；页面内 browser-only 路径已经重新取得两个目标的
`post.info` 元数据，并读取/下载 Nakk 的两个已观察资源，但同样不能替代 SDK direct transport，且没有
理由放宽 solver 不接收业务 URL/Cookie 的边界。

此外，三平台 Chrome/Edge/Firefox provider contract（Safari 仅 macOS）的实际六目标 native runner
evidence 仍尚未取得；当前新增的 DPAPI、Secret Service、跨平台 profile path 和 Chromium crypto
只有本地代码审查、合成 fixture 与跨编译证据。固定 Firefox temporary profile 已在本机 macOS arm64
形成 PASS，但不能替代其他 OS/arch。`.github/workflows/browser-evidence.yml` 已提供
macOS/Linux/Windows × amd64/arm64 的 credential-free contract matrix，并包含固定 Firefox 153.0.3
发行包的临时 profile job；本地 actionlint/policy/单测不能代替该 workflow 在六个 runner 上实际成功。
即使 workflow 成功，它也只证明合成 profile 与 provider contract，不等于真实用户浏览器
profile/系统凭据 evidence。按计划，native host/CI 仍须记录 lock/WAL、权限、schema drift、真实
系统凭据边界与清理结果，不能将离线门禁标为 native PASS。

本机 macOS host probe 的当前结果为：Edge `Default` profile provider 已通过真实读取，原因是补上
现代 Chromium Cookies 的 `SHA-256(host_key)` 明文前缀剥离；显式
`BROWSER_NATIVE_E2E=1 BROWSER_NATIVE_BROWSERS=edge` evidence 通过，session value、path 与数据库
内容均未进入输出。Chrome `Default` profile 未返回目标 cookie，Safari 仍返回稳定的 storage
permission denied；这些环境缺口不替代六目标 native runner。

本机曾通过独立 Range 分片取得并完整校验固定 Firefox 153.0.3 DMG，挂载后确实启动了真实 Firefox
profile。第一次 replay 暴露出 seed 依赖已迁移/可选的 `schemeMap` 等列，第二次 replay 暴露出临时
HOME 改变嵌套 `go test` 的默认 GOPATH/module cache；两项均已修正。随后并行 Range 下载、完整 SHA-256
校验和 temporary profile replay 已通过，结果只证明本机 macOS arm64 的 synthetic Firefox contract，
不替代六目标 runner 或真实用户 profile evidence；临时挂载/下载目录已清理。

后续只读 target probe 也未产生可提交的目标：历史记录中的公开 `aak` post 在当前 native 出口返回
`challenge_required`；代码库 reference 中的公开 `pixiv/illust` tag route 返回 HTTP 404。两次 probe
均未输出 credential、正文或 signed query，临时测试文件已删除；因此没有把失效目标或任意 tag 写入
`FANBOX_E2E_*` 环境契约。

离线 e2e 默认跳过这些场景；synthetic challenge、solver protocol、并发取消和 secret-boundary
测试已通过，但不能替代真实服务 evidence。发布前应按
[最终验证操作手册](release-prep-runbook.md) 和
[RC follow-up 实施计划](rc-follow-up-implementation-plan.md) 在获得明确凭据/容器授权后运行，
并只将脱敏结果补回本目录。

## Release-note declaration 草稿

本次若作为 feature PR 提交，建议在 `.github/PULL_REQUEST_TEMPLATE.md` 使用：

```text
category: Changed
breaking: true
summary: Replace the v0 Pixiv facade and legacy auth/pool runtime with the v1 public SDK, database-backed account scheduling, explicit FANBOX support, service-scoped network routing, and opt-in diagnostics.
none_reason:
```

正式 release-prep 仍应由维护者根据真实 evidence 审核后生成双语 `changelog/vX.Y.Z/`；本记录不修改
`changelog/unreleased/`。
