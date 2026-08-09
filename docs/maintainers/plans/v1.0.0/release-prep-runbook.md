# v1.0.0 最终验证操作手册

本手册只在 release candidate 代码与公开契约冻结后执行。开工阶段只确认凭据入口、Docker 与测试
基础设施存在，不提前验证账号联网有效性。实现阶段应把下列命令固化为不接收 secret 参数的聚焦
test/script；若最终名称调整，必须同步本手册与 development guide。

## 1. 冻结输入

1. 在独立 release-prep worktree 记录 commit、Go version、三个 native runner image 与 Firefox 版本。
   首次取得 v1.0.0 solver implementation acceptance 时，额外记录 FlareSolverr tag 与 image digest；
   已有合格 evidence 的后续 RC 不重复要求。
2. 确认工作树没有 token、Cookie、`pixiv-cli.db`、浏览器 profile、下载内容或未脱敏 response fixture。
3. 运行 public API inventory 与 `apidiff`，确认候选代码相对冻结 baseline 没有未审查的不兼容变化。

## 2. Offline 与 native browser 门禁

1. 在 macOS、Windows、Linux 分别运行 `internal/browsercookies` provider contracts；Safari
   只在 macOS 运行。
2. Chrome、Edge、Safari 可使用 runner 已安装版本或脱敏 fixture。所有 provider 都必须覆盖 profile
   discovery、目标域筛选、snapshot/lock/WAL、解密失败、权限失败、schema drift 与清理。
3. Firefox 不要求安装到宿主机：下载经过 checksum 验证的固定官方发行包到 runner 临时目录，使用
   隔离 `HOME` 和 profile 目录启动一次以生成/升级 schema，退出后运行 Firefox provider contract。
4. 删除临时 Firefox 发行包、profile、snapshot 与测试 key；再次扫描 runner workspace，确认没有
   Cookie value、绝对 profile 内容或浏览器数据库进入 artifact。
5. evidence 只记录 OS/arch、Firefox version/checksum、fixture schema version、测试 commit、通过/
   失败与脱敏错误 reason。任何平台缺失都阻塞发布，不能由另一平台或 Chromium 结果替代。

仓库 `.github/workflows/browser-evidence.yml` 已固化六目标 credential-free contract matrix；其中
`firefox_native` 使用 Mozilla Firefox 153.0.3 的固定官方 URL/SHA-256，在 runner 临时目录完成解包、
隔离 profile/schema、synthetic cookie provider contract 和清理。`actionlint` 与
`go run ./scripts/browsernativeevidence policy --workflow .github/workflows/browser-evidence.yml`
只校验提交内容，不能代替一次实际的六目标 workflow run；该 job 也不代替真实用户 browser
profile/Keychain/DPAPI/Secret Service evidence。

公开 SDK worktree 的受控 host probe 可用下列显式、非 secret 入口验证本机已安装 provider；它只输出
安全 profile ID、cookie 数量和稳定 error reason，不把该 probe 当作三平台 runner evidence：

```bash
BROWSER_NATIVE_E2E=1 \
BROWSER_NATIVE_BROWSERS=chrome,edge,safari \
GOPROXY=off go test ./e2e -run '^TestRealNativeBrowserProvider$' -count=1 -v
```

若同一浏览器发现多个 profile，额外设置对应的 `BROWSER_NATIVE_PROFILE_CHROME`、
`BROWSER_NATIVE_PROFILE_EDGE`、`BROWSER_NATIVE_PROFILE_FIREFOX` 或 `BROWSER_NATIVE_PROFILE_SAFARI`；
这些值只能是 provider 输出的安全 profile ID。Keychain/TCC/DPAPI/Secret Service 交互失败必须记录为
失败原因，不得用 fixture、另一个浏览器或自动换 profile 伪造成功。

## 3. 真实 Pixiv SDK E2E

1. 测试进程通过 pixiv-cli auth repository 读取当前选中账号；不得通过 shell substitution、argv、
   环境变量、临时明文文件或测试 flag 传递 refresh token。
2. 调用 `sdk/pixiv.Open`，验证 identity，并完成一个稳定 detail/list 与一个 `Resource` 的 GET 或 HEAD。
3. 若上游返回 rotated credentials，先按正常 repository transaction 持久化，再继续内容请求；保存
   失败必须中止，不能复用旧 RFT。
4. 由 release-prep 操作者扫描 stdout、stderr、test log、失败 diff 与 evidence，确认没有 access token、refresh token、
   signed query、原始 response body 或账号私有内容。
5. 失效凭据记为 `credentials_expired` 并重新授权后重跑；challenge、网络或上游错误保留真实分类，
   不切换 Web API 或 FANBOX 专用 recovery。

## 4. 真实 FANBOX SDK E2E

当前入口使用 `FANBOX_E2E_CREATOR_ID`、`FANBOX_E2E_TAG`、`FANBOX_E2E_POST_ID` 与
`FANBOX_E2E_POST_URL` 提供显式、非 secret 的 creator/tag/post/page targets；这些值不能包含
session、完整 Cookie、signed query 或私密正文。测试启用后缺少任一 target 或 Keychain item 必须失败，
不能自动发现其他帖子或以 skip 代替 evidence。

若验收对象只需要验证单个 `post.info` 详情，可运行
`scripts/test-e2e.sh --fanbox-post-only`。该模式只要求 `FANBOX_E2E_POST_ID` 与
`FANBOX_E2E_POST_URL`，通过公共 SDK 验证帖子 ID、非空 body、URL reference 和安全的资源清单；
合法的零 file-asset 帖子（例如 `aak/11870583`）不会被错误判为失败。它不替代下面的严格
`TestRealFanboxSDKRead`：后者仍必须从同一目标的 `post.info` 发现每一个 file attachment，并对每项
完成 HEAD、完整 GET/保存与字节数校验（例如 `nakkemos/3625356`）。

1. 测试进程直接通过 Keychain API 读取已授权的 `FANBOXSESSID` item；不得把 value 放入 shell argv、
   环境变量、日志、临时文件或完整 Cookie header。
2. 使用 `sdk/fanbox.Client.ValidateSession` 验证身份，再逐项覆盖公开 creator/tag/post/home/
   supporting/following、pagination 与 resource operation；不执行支持关系 mutation 或默认批量下载。
3. 从合法 `post.info` 详情发现真实 file attachment，验证响应并读取非零字节；详情提供 size 时核对
   完整字节数。cover、preview 或任意预置 CDN URL 不替代该路径。
4. 验证资源 host/redirect revalidation、credential stripping、Range/conditional request 与
   signed-query redaction。只有允许的 FANBOX API/第一方资源 host 可以收到同一个 session。
5. 默认关闭 FlareSolverr，以当前 native Chrome 146 TLS baseline（内置 Firefox 148 HTTP UA）完成 E2E。结果只证明该次目标、账号、
   网络出口和时间下的行为，不声明 profile 永久有效。若当时真实网络触发 challenge，可另以显式
   配置验证 recovery；报告必须区分 native 首次结果、solver 求解与 native replay，不能把 solver
   页面当作 SDK operation 成功。

   真实 SDK E2E 的 recovery 配置使用非 secret 的 `FANBOX_E2E_SOLVER_URL` 与
   `FANBOX_E2E_SOLVER_PROXY`；前者只指向 loopback 或受控 FlareSolverr service，后者只描述
   solver 所在 network namespace 的独立 upstream proxy。两者均未设置时 solver 保持关闭。
6. session 失效时重新导入并重跑；`challenge_required` 与普通 403 必须保持可区分，不 fallback 到
   其他账号或多 Cookie。
7. 由 release-prep 操作者扫描所有输出和 evidence，确认不含 Cookie、signed URL、私密 post body 或下载内容。

## 5. v1.0.0 一次性本机 FlareSolverr 路径验证

该步骤在 recovery 实现完成后、v1.0.0 发布前至少执行一次，作为 implementation acceptance evidence；
它不是普通 CI，也不要求每个 RC 重复。之后只有维护者需要重新诊断上游行为时才按需重跑。

1. 复核 Docker daemon 可用；从可信来源拉取经人工核对且固定 digest 的 image，不使用 `latest`。
2. 容器只发布到 loopback host port；不使用 host network，不挂载仓库、Docker socket、浏览器
   profile、Keychain、`pixiv-cli.db` 或下载目录。
3. 显式配置 FlareSolverr service URL；若容器需要 upstream proxy，单独配置 solver proxy，不从
   native FANBOX proxy 猜测或继承。Go → service 使用独立 direct control client，不复用 native
   `HTTPClient`/proxy、solver proxy 或 ambient `HTTP_PROXY`/`HTTPS_PROXY`。Docker/外部 service 的
   地址必须分别从 pixiv-cli 与 FlareSolverr 所在 network namespace 验证；完整示例见
   [网络配置与服务路由](network-routing.md)。
4. 使用注入的 native test transport 首次返回 synthetic challenge；public SDK 随后调用真实 solver，
   确认它只匿名访问 `https://www.fanbox.cc/`、只提取 user agent/`cf_clearance`，最后由 synthetic
   native replay 验证传播。Client 使用非 secret dummy `FANBOXSESSID` 满足构造契约；该值只进入
   injected native transport，绝不发给 solver。不得把真实 session、API/帖子/文件 URL、request body
   或下载内容传给 solver。
5. evidence 只记录 image digest、native/solve/replay 分类、`user_agent_source`、proxy 拓扑是否按配置
   生效与清理结果；不记录完整 UA、URL query、Cookie、页面 body、文件名或截图。
6. 若候选版本已实现 `--debug`，显式开启并将 stderr 重定向到本机临时文件，确认模块名能够区分
   FANBOX native、FlareSolverr 与 replay，且输出不含 dummy session、clearance、proxy userinfo 或
   response body。该文件不作为 release artifact，检查后删除。
7. 停止并删除容器、network 与临时 volume，检查容器日志和 repository diff 不含 secret。是否删除
   固定 image 由本机缓存策略决定，但它不能成为项目依赖。

本 worktree 的一次性 acceptance test 命令为（`FANBOX_SOLVER_PROXY` 仅在 solver 所在 network
namespace 需要独立 upstream proxy 时设置）：

```bash
FANBOX_SOLVER_E2E=1 \
FANBOX_SOLVER_URL=http://127.0.0.1:8191 \
FANBOX_SOLVER_PROXY=http://host.docker.internal:7890 \
GOPROXY=off go test ./e2e -run '^TestRealFanboxSolverProtocolAcceptance$' -count=1 -v
```

上述步骤验证真实 FlareSolverr protocol integration，不算 genuine FANBOX challenge E2E。只有真实
网络自然出现 challenge 时才额外记录 genuine recovery；native Firefox 直接成功、没有可复现的真实
challenge 不构成失败。

## 6. 全量门禁与报告

按顺序执行：

```bash
go test ./...
go test -race ./...
go vet ./...
sh scripts/build.sh
pre-commit run --all-files
go test ./scripts/documentation
git diff --check
```

随后确认 workflow policy、public API compatibility、browser provider native contract matrix、真实 Pixiv SDK 与
真实 FANBOX SDK 均有同一候选 commit 的 evidence。若尚未取得一次性 FlareSolverr implementation
acceptance，v1.0.0 继续阻塞；取得并关联实现 commit 后，后续 RC 不要求重复运行，也不能用它替代
真实 FANBOX SDK evidence。
报告只允许包含版本、commit、平台、命令、通过/失败、稳定 error reason 与脱敏摘要；不得附
response body、数据库、profile、下载文件、token、Cookie 或 signed URL。

全部成功后再次检查 Git 状态与 release artifact 内容，再创建不可变 `v1.0.0` tag。任一真实 E2E、
native platform 或兼容性门禁失败都停止发布；修复或重新授权后必须对新的候选 commit 重跑受影响
步骤及最终全量门禁。
