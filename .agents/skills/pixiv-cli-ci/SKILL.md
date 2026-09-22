---
name: pixiv-cli-ci
description: Diagnose, verify, monitor, and safely operate pixiv-cli GitHub Actions runs and local CI gates, including PR Quality gate, workflow policy, platform smoke, native/browser evidence, release handoffs, and approved reruns. Use when checks fail or hang, a PR needs readiness verification, a workflow run needs root-cause analysis, or an authorized downstream workflow must be dispatched.
---

# pixiv-cli CI 与 workflow

按仓库维护流程诊断和验证 CI。先读取 `AGENTS.md`、`docs/zh-CN/maintainers/development.md`、目标
workflow、对应 `scripts/cmd/*` policy 和 `scripts/tests` 归属；默认只读，先获得证据再决定是否需要改代码或重跑。

## 安全边界

- 查看 PR、run、job、step、日志和本地 policy 是只读操作；重跑、取消、dispatch、修改 workflow、push 或修复代码前取得用户对具体动作的明确授权。
- 不把 token、Cookie、签名私钥、代理凭据、私有 URL、下载作品、本地状态或 API response 写入日志、issue、PR 或最终报告；日志中发现 secret 时只报告脱敏位置。
- 不用 rerun 把代码/策略失败伪装成通过。重跑成功只能证明该次执行通过，必须保留原失败原因和 run/attempt 链接。
- `.github/workflows/release.yml` 只接受 `push.tags: v[0-9]*`；不要对它调用 `gh workflow run`，不要移动旧 tag、向 tag 注入默认分支新内容或手工绕过 release gate。
- 不为解决“运行较久”凭空增加 timeout、retry、跳过条件或 fallback；区分真实无响应、基础设施故障、代码失败和正常长任务。

## 识别准确的 run 和提交

1. 先确定仓库、PR/branch、workflow、run ID、attempt、head SHA 和默认分支；不要根据标题猜 run：

   ```bash
   gh pr checks <number> --required --json name,state,bucket,workflow,link
   gh run list --workflow <workflow-file> --limit 20
   gh run view <run-id> --json name,event,headBranch,headSha,status,conclusion,jobs,url
   ```

2. PR 检查未完成时使用：

   ```bash
   gh pr checks <number> --required --watch --fail-fast
   ```

   对单个 run 使用：

   ```bash
   gh run watch <run-id> --compact --exit-status
   gh run view <run-id> --log-failed
   ```

   记录失败的 job、step、命令、head SHA、workflow ref 和首次错误；不要只摘最后一行。
3. 失败日志不足时查询具体 job ID：

   ```bash
   gh run view <run-id> --json jobs \
     --jq '.jobs[] | {name, databaseId, status, conclusion}'
   gh run view --job <job-database-id> --log-failed
   ```

   将 GitHub API 的网络/权限失败与 workflow 本身失败分开报告。

## 本地门禁选择

先根据 diff 分类，不要无依据地把所有门禁都扩展到每个小改动：

| 范围 | 最小验证 |
| --- | --- |
| README/docs/agent-only | `go test ./scripts/tests/documentation -count=1`、`git diff --check` |
| Go 或行为代码 | 聚焦测试；随后 `go test ./... -count=1`、`go vet ./...`，构建相关时运行 `sh scripts/build.sh` |
| 共享、认证、下载、CLI、MCP、SDK | 另跑 `go test -race ./... -count=1` |
| scripts、workflow、release policy | `go test ./scripts/... ./tools/release ./tools/platformmatrix -count=1`、`go vet ./scripts/...`，以及受影响的行为契约测试 |
| shell 脚本 | 对保留脚本运行 `sh -n`；不要只依赖 YAML 解析 |
| 真实 Pixiv/FANBOX API、native host、Keychain/DPAPI 或受保护 release evidence | 只有用户显式授权并具备对应环境才运行；否则记录未运行和风险 |

当前 workflow/policy 对应关系：

```bash
go test ./tools/release ./tools/platformmatrix -count=1
sh scripts/test-homebrew-formula.sh
go test ./scripts/internal/nativeevidence -run '^TestNativeEvidenceWorkflowKeepsSecurityAndOwnershipBoundaries$' -count=1
go test ./scripts/internal/browsernativeevidence -run '^TestBrowserEvidenceWorkflowKeepsSecurityAndFixtureBoundaries$' -count=1
```

只运行与受影响 workflow 对应的命令；优先验证外部行为、artifact 与安全边界，不为 YAML 排版、job 名称或具体实现形状新增脆弱测试。

## 按故障类型诊断

- **测试/构建失败**：读取完整失败 step，使用同一 ref/SHA 在本地重现；检查依赖、平台、静态库 manifest、Rust toolchain 和源码边界。修复代码需要用户明确请求，CI skill 默认只诊断。
- **policy/契约失败**：定位对应行为契约与 workflow diff，确认是 artifact、权限、secret 边界、平台矩阵或命令语义变化；保持 fail-closed，不用脆弱的 YAML 形状断言掩盖真实问题。
- **docs-only 分类异常**：核对 changescope 输出、空 diff/初始 push/手动触发行为以及 Quality gate 的 docs-only 分支；不能把无法读取 diff 当作 docs-only。
- **缺失、skipped 或 pending required check**：区分有意的 docs-only skip、Windows ARM64 race 特例、job 未创建、权限不足和真正卡住；不得把 pending 当成功。
- **基础设施/瞬态失败**：只有日志证据支持 runner、网络、GitHub API 或服务异常时才建议 rerun；若同一失败重复出现，停止重跑并报告共同根因。
- **release/evidence 失败**：保持 immutable tag、同一 source commit、同一 artifact identity；不要从默认分支补文件或混用不同 run 的 evidence。

## 受控重跑与 workflow dispatch

1. 诊断完成且用户明确授权后，优先只重跑失败 job：

   ```bash
   gh run rerun <run-id> --failed
   gh run watch <run-id> --compact --exit-status
   ```

   保存原 run、rerun attempt、授权理由和最终结果。代码或 policy 失败先修复并提交新的受审计 commit，不要重复 rerun。
2. 仅对确实声明 `workflow_dispatch` 的 workflow 使用 `gh workflow run`，并显式指定受审计的 ref。PR/main 常规验证、native/browser evidence 和 platform smoke 的手动运行必须遵守各自 workflow 的 branch/path 条件和无凭据边界。
3. Homebrew prepublish 只读验证已公开、非 draft、非 prerelease 的 stable Release；它不读取 deploy key，也不写 tap。需要恢复 Homebrew 发布时，使用 `publish-homebrew.yml` 并传入原始受信 Release workflow 的 `release_run_id`，不能把 prepublish 当作发布入口：

   ```bash
   gh workflow run homebrew-prepublish-verify.yml \
     --ref <default-branch> \
     -f release_tag=vX.Y.Z

   gh workflow run publish-homebrew.yml \
     --ref <default-branch> \
     -f release_run_id=<trusted-release-run-id>
   ```
4. Release 成功并公开同一 immutable tag 后，SkillHub/ClawHub 才可做下游恢复；输入必须是已存在的精确 tag，不能使用后续 main 内容：

   ```bash
   gh workflow run publish-skillhub.yml \
     --ref <default-branch> \
     -f release_tag=vX.Y.Z

   gh workflow run publish-clawhub.yml \
     --ref <default-branch> \
     -f release_tag=vX.Y.Z

   gh workflow run publish-clawhub.yml \
     --ref <default-branch> \
     -f release_tag=vX.Y.Z \
     -f verify_only=true
   ```

   `verify_only=true` 只用于已有 ClawHub 版本的最终审核，不得借此重发。每次 dispatch 前确认 Release 已公开、tag 属于默认分支、对应 workflow 的输入契约仍未变化；记录 returned run URL 并按本技能监控。

## 结果报告

按以下顺序报告：

- PR/run/workflow、event、attempt、head SHA、job/step 和 URL；
- 原始失败命令与第一处错误；
- 本地复现/policy/test 结果；
- 根因分类：代码、契约、配置、权限、基础设施或未知；
- 是否重跑/dispatch、授权依据、run 结果；
- 未运行的真实环境测试、剩余风险和下一步最小动作。

不要只写“CI 通过/失败”；说明哪些 required checks 已完成、哪些被有意跳过、哪些仍 pending。
