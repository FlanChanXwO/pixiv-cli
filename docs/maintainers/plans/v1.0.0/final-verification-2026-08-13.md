# v1.0.0 最终验证记录（2026-08-13）

对应隔离 worktree `codex/v1-sdk-rewrite`。只记录脱敏的自动门禁与最终真实 SDK
evidence 状态，不包含 credential、Cookie、私有 URL、下载内容或上游响应。

## 范围

在 2026-08-08 记录基础上，本轮完成架构收敛后的最终验证与收尾：

- `internal/application`、`internal/bootstrap` 移除，MCP stdio runtime 经
  `internal/mcpserver`（product aggregator + one tool per package）组装。
- `internal/media/ugoira/rust` 承载 native 库，13 个 Go scripts 收敛为
  thin main + `scripts/internal/<tool>`。
- legacy `internal/services` 协议适配与 downloader 包移除。
- 真实 SDK E2E 收尾（当时记 Pixiv PASS、FANBOX post-only PASS；Pixiv 判定
  已撤销为 BLOCKED，FANBOX post-only 改记 PARTIAL-PASS 且完整 read 为
  NOT-RUN，见下文「判定撤销」与「覆盖分层」）。

## 最终真实 SDK evidence（2026-08-13，显式授权）

### Pixiv

命令：

```bash
PIXIV_SDK_E2E=1 PIXIV_E2E_PROXY=http://127.0.0.1:7890 \
  go test ./e2e -run 'TestPixivE2ERotationUsesStoredRevision|TestRealPixivSDKRead' -count=1 -v
```

结果：**当时记为 PASS；该判定已于 2026-08-13 后续审查中撤销，现状为 BLOCKED。**

- `OpenWith` refresh 交换成功，credential identity 与 authdb 选中账号一致。
- 本机选中账号对 `/v1/user/me` 返回上游 404（`pixiv:CurrentUser: not_found`，
  "Specified end-point doesn't exist"）。当时按授权将断言改为仅在该 Reason 下
  跳过身份校验并继续，并据此把整体结果记为 PASS。
- SearchArtworks（"初音ミク"）、Artwork detail、page Resource HEAD 均通过。

#### 判定撤销（CurrentUser 恢复 strict fail）

上述「按 Reason 跳过」把 mandatory 的身份断言降级为条件式，等于在测试内部
单方面放宽验收契约：`OpenWith` 的 refresh 交换只证明 refresh token 可被兑换，
不构成服务端身份读取，因此不能替代 `/v1/user/me`。同一个 `not_found` 因此在
测试（skip）、evidence（fail/blocked）与本文件（PASS）中出现了三种互相矛盾的
表述。

处置：`e2e/sdk_pixiv_e2e_test.go` 的 `TestRealPixivSDKRead` 已移除该 skip 分支，
恢复对任何 Reason 都失败的 strict 断言，并在注释中写明 CurrentUser 是 mandatory
身份证据、refresh 交换不可替代。

因此本节的**当前**状态是：

| 项 | 状态 |
| --- | --- |
| `TestPixivE2ERotationUsesStoredRevision` | PASS（rotation CAS 持久化契约，与账号 404 无关） |
| `TestRealPixivSDKRead` | **BLOCKED** —— 需要 `/v1/user/me` 可用的本机账号重跑 |
| SearchArtworks / Artwork detail / Resource HEAD 只读路径 | 已在同会话真实验证通过（作为迁移无回归的证据保留，不等同于整体 PASS） |

不追认原 PASS 判定，也不删除上述历史记录。

### FANBOX（post-only）

命令（Keychain item `pixiv-cli-e2e-fanbox/fanbox-e2e` 提供授权 session；
session 不进入 argv/env/log/test name）：

```bash
FANBOX_SDK_E2E=1 FANBOX_E2E_POST_ONLY=1 \
FANBOX_E2E_POST_ID=12398226 FANBOX_E2E_POST_URL='https://www.fanbox.cc/@ro7274/posts/12398226' \
FANBOX_E2E_SOLVER_URL='http://127.0.0.1:8191' \
FANBOX_E2E_SOLVER_PROXY='http://host.docker.internal:7890' \
PIXIV_E2E_PROXY='http://127.0.0.1:7890' \
  go test ./e2e -run '^TestRealFanboxSDKPostInfo$' -count=1 -v
```

结果：**PARTIAL-PASS**（`post.info body verified; assets=1 file_assets=0`）。

- 代理出口 IP 被 Cloudflare `block_ip` 拦截时，配置显式 FlareSolverr
  （loopback `:8191`，经 `host.docker.internal:7890` 出网）完成 challenge
  recovery 后，API 请求携带原有 FANBOXSESSID 重放成功。
- 过程中发现并修复本机 Keychain 中仍为 2026-08-10 过期 session 的问题；
  更新后 `find-generic-password -w` 返回值与期望值一致（注意该命令输出带
  尾随换行，对比时需去除）。

#### 覆盖分层（不得把 post-only 当作完整 FANBOX read）

| 档位 | 测试 | 状态 | 覆盖 |
| --- | --- | --- | --- |
| post-only | `TestRealFanboxSDKPostInfo` | **partial-pass** | `Post`、post body、`ResolveURL`，以及 challenge recovery 路径 |
| 完整 read | `TestRealFanboxSDKRead` | **not-run** | `ValidateSession`、`CurrentUser` 身份、`Creator`、`CreatorTags`、`Creators`（supporting/following）、`Home`、`Supporting`、`CreatorPosts`、显式 tag query、cursor continuation、post body 的**强制** file attachment、`OpenResource` HEAD 与 `SaveResource` 大小交叉校验 |

post-only 明确**不覆盖**：会话校验、身份、creator/tag/list 家族、分页 continuation、file resource 的打开与保存。

完整 read 未运行的**非 secret** 前提（缺一不可）：

| 前提 | 环境变量 / 条件 | 现状 |
| --- | --- | --- |
| creator 目标 | `FANBOX_E2E_CREATOR_ID` | 未提供 |
| tag 目标 | `FANBOX_E2E_TAG` | 未提供 |
| post 目标 | `FANBOX_E2E_POST_ID`、`FANBOX_E2E_POST_URL`（必须是无 query/fragment/userinfo 的 `https://(www.)fanbox.cc/...`） | 已提供 |
| **post 必须带 file attachment** | 由 `TestRealFanboxSDKRead` 强制要求（`explicit post target has no file attachment`） | **不满足**：现有 post 目标 `file_assets=0`；2026-08-08 记录中的另一目标同样在资源阶段失败 |
| 授权 session | macOS Keychain item `pixiv-cli-e2e-fanbox/fanbox-e2e` | 已具备（session 不进入 argv/env/log/test name） |

即：阻塞完整 read 的唯一实质缺口是**一个带 first-party file attachment 的显式 post 目标**（外加 creator 与 tag 目标）。这不是 secret，也不是代码问题。运行方式：

```bash
FANBOX_SDK_E2E=1 \
FANBOX_E2E_CREATOR_ID=<non-secret-creator-id> FANBOX_E2E_TAG=<non-secret-tag> \
FANBOX_E2E_POST_ID=<post-with-file-attachment> FANBOX_E2E_POST_URL=<its-page-url> \
  go test ./e2e -run '^TestRealFanboxSDKRead$' -count=1 -v
```

## 自动门禁（本轮收尾后）

| 命令 | 结果 |
| --- | --- |
| `GOPROXY=off go test ./... -count=1` | 通过 |
| `GOPROXY=off go test -race ./... -count=1` | 通过 |
| `GOPROXY=off go vet ./...` | 通过 |
| `sh scripts/build.sh` | 通过 |
| `GOPROXY=off go test ./e2e -count=1 -v`（无凭据契约 skip） | 通过 |
| `go vet ./e2e` | 通过 |
| gopls 诊断 / gofmt | 通过 |

## 收尾提交

`8179cb1 refactor: converge v1 architecture, restructure MCP and scripts`
已推送至 `origin/codex/v1-sdk-rewrite`；随后新增本记录（当时无代码改动）。

后续对本记录的两次修订**含代码改动**，不属于「只加文档」：

| commit | 内容 |
| --- | --- |
| `211ab2d test(e2e): restore strict CurrentUser assertion in Pixiv SDK e2e` | 删除 `e2e/sdk_pixiv_e2e_test.go` 中对 `NotFound` 的 skip 分支；撤销本文件与 `index.md` 的 Pixiv PASS 判定 |
| `4feeb79 docs(e2e): separate FANBOX post-only partial-pass from full read` | 为两个 FANBOX 真实测试补 doc comment 固定分层语义；本文件 FANBOX 结果改记 PARTIAL-PASS 并新增「覆盖分层」；`AGENTS.md` 补 post-only 变体说明 |

两次修订均通过 pre-commit（`gofmt` + `go test ./...`）与全量离线门禁。
