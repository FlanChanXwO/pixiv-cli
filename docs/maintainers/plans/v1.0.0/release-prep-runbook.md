# v1.0.0 最终验证操作手册

本手册只在 release candidate 代码与公开契约冻结后执行。开工阶段只确认凭据入口、Docker 与测试
基础设施存在，不提前验证账号联网有效性。实现阶段应把下列命令固化为不接收 secret 参数的聚焦
test/script；若最终名称调整，必须同步本手册与 development guide。

## 1. 冻结输入

1. 在独立 release-prep worktree 记录 commit、Go version、三个 native runner image、Firefox 版本、
   FlareSolverr tag 与 image digest。
2. 确认工作树没有 token、Cookie、`pixiv-cli.db`、浏览器 profile、下载内容或未脱敏 response fixture。
3. 运行 public API inventory 与 `apidiff`，确认候选代码相对冻结 baseline 没有未审查的不兼容变化。

## 2. Offline 与 native browser 门禁

1. 在 macOS、Windows、Linux 分别运行 `internal/platform/browsercookies` provider contracts；Safari
   只在 macOS 运行。
2. Chrome、Edge、Safari 可使用 runner 已安装版本或脱敏 fixture。所有 provider 都必须覆盖 profile
   discovery、目标域筛选、snapshot/lock/WAL、解密失败、权限失败、schema drift 与清理。
3. Firefox 不要求安装到宿主机：下载经过 checksum 验证的固定官方发行包到 runner 临时目录，使用
   隔离 `HOME` 和 profile 目录启动一次以生成/升级 schema，退出后运行 Firefox provider contract。
4. 删除临时 Firefox 发行包、profile、snapshot 与测试 key；再次扫描 runner workspace，确认没有
   Cookie value、绝对 profile 内容或浏览器数据库进入 artifact。
5. evidence 只记录 OS/arch、Firefox version/checksum、fixture schema version、测试 commit、通过/
   失败与脱敏错误 code。任何平台缺失都阻塞发布，不能由另一平台或 Chromium 结果替代。

## 3. 真实 Pixiv SDK E2E

1. 测试进程通过 pixiv-cli auth repository 读取当前选中账号；不得通过 shell substitution、argv、
   环境变量、临时明文文件或测试 flag 传递 refresh token。
2. 调用 `sdk/pixiv.Open`，验证 identity，并完成一个稳定 detail/list 与一个 `Resource` 的 GET 或 HEAD。
3. 若上游返回 rotated credentials，先按正常 repository transaction 持久化，再继续内容请求；保存
   失败必须中止，不能复用旧 RFT。
4. 扫描 stdout、stderr、test log、失败 diff 与 evidence，确认没有 access token、refresh token、
   signed query、原始 response body 或账号私有内容。
5. 失效凭据记为 `credentials_expired` 并重新授权后重跑；challenge、网络或上游错误保留真实分类，
   不切换 Web API 或 FlareSolverr。

## 4. 真实 FANBOX SDK E2E

1. 测试进程直接通过 Keychain API 读取已授权的 `FANBOXSESSID` item；不得把 value 放入 shell argv、
   环境变量、日志、临时文件或完整 Cookie header。
2. 使用 `sdk/fanbox.Client.ValidateSession` 验证身份，再读取一个稳定 detail/list 与至少一个第一方
   `Resource`；不执行支持关系 mutation 或默认批量下载。
3. 验证资源 redirect、credential stripping、Range/conditional request 与 signed-query redaction。
4. session 失效时重新导入并重跑；`challenge_required` 与普通 403 必须保持可区分，禁止自动绕过。
5. 扫描所有输出和 evidence，确认不含 Cookie、signed URL、私密 post body 或下载内容。

## 5. 隔离的 FlareSolverr 辅助验证

1. 复核 Docker daemon 可用；从可信来源拉取经人工核对且固定 digest 的 image，不使用 `latest`。
2. 容器只发布到 loopback 随机 host port；不使用 host network，不挂载仓库、Docker socket、浏览器
   profile、Keychain、`pixiv-cli.db` 或下载目录。
3. 先完成 health check，再由受控测试进程提交一条 FANBOX challenge 诊断请求。secret 只存在于测试
   进程与容器临时内存，Docker command、inspectable environment、label 和日志均不得包含它。
4. evidence 单独记录 image digest、health result、诊断 error/result class 与清理结果，不记录请求
   URL query、Cookie、页面 body 或截图；不得用该结果替代第 4 节 SDK 直连成功。
5. 停止并删除容器、network 与临时 volume；是否删除固定 image 可由本机缓存策略决定，但它不能成为
   项目依赖。复核运行中的容器与 repository diff。

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

随后确认 workflow policy、public API compatibility、三个 native browser job、真实 Pixiv SDK、真实
FANBOX SDK 与 FlareSolverr 辅助验证均有同一候选 commit 的 evidence。报告只允许包含版本、commit、
平台、命令、通过/失败、稳定 error code 与脱敏摘要；不得附 response body、数据库、profile、下载
文件、token、Cookie 或 signed URL。

全部成功后再次检查 Git 状态与 release artifact 内容，再创建不可变 `v1.0.0` tag。任一真实 E2E、
native platform 或兼容性门禁失败都停止发布；修复或重新授权后必须对新的候选 commit 重跑受影响
步骤及最终全量门禁。
