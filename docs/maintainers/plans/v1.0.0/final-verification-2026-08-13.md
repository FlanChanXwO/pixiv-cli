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
- 真实 SDK E2E 收尾（Pixiv PASS、FANBOX post-only PASS，见下）。

## 最终真实 SDK evidence（2026-08-13，显式授权）

### Pixiv

命令：

```bash
PIXIV_SDK_E2E=1 PIXIV_E2E_PROXY=http://127.0.0.1:7890 \
  go test ./e2e -run 'TestPixivE2ERotationUsesStoredRevision|TestRealPixivSDKRead' -count=1 -v
```

结果：PASS。

- `OpenWith` refresh 交换成功，credential identity 与 authdb 选中账号一致。
- 本机选中账号对 `/v1/user/me` 返回上游 404（`pixiv:CurrentUser: not_found`，
  "Specified end-point doesn't exist"），已按授权将断言改为仅在该 Reason 下跳过
  身份校验并继续；身份仍由 `OpenWith` 的 refresh 交换与 account identity 校验确认。
  非 404 的 `CurrentUser` 错误仍失败。代码位置：
  `e2e/sdk_pixiv_e2e_test.go`（`TestRealPixivSDKRead`）。
- SearchArtworks（"初音ミク"）、Artwork detail、page Resource HEAD 均通过。

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

结果：PASS（`post.info body verified; assets=1 file_assets=0`）。

- 代理出口 IP 被 Cloudflare `block_ip` 拦截时，配置显式 FlareSolverr
  （loopback `:8191`，经 `host.docker.internal:7890` 出网）完成 challenge
  recovery 后，API 请求携带原有 FANBOXSESSID 重放成功。
- 过程中发现并修复本机 Keychain 中仍为 2026-08-10 过期 session 的问题；
  更新后 `find-generic-password -w` 返回值与期望值一致（注意该命令输出带
  尾随换行，对比时需去除）。

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
已推送至 `origin/codex/v1-sdk-rewrite`；随后新增本记录（无代码改动）。
