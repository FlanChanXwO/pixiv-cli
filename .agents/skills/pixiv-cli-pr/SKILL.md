---
name: pixiv-cli-pr
description: Prepare, create, update, and monitor pixiv-cli pull requests using the repository template, verification checklist, review handoff, and authorization boundaries. Use when drafting a PR body, opening or editing a PR, requesting review, checking readiness, or handing a merged PR to release preparation.
---

# pixiv-cli Pull Request

按仓库流程准备和维护 GitHub PR。先读 `AGENTS.md`、现有 locale 的 `CONTRIBUTING` 和
`.github/PULL_REQUEST_TEMPLATE.md`。代码审查使用 `pixiv-cli-review`，CI 诊断使用
`pixiv-cli-ci`，合并后的版本说明交给 `pixiv-cli-release-notes`。

## 授权与范围

- 默认只读。创建或更新 PR、push、请求 reviewer、评论、merge 或关闭前，要取得用户对该动作的明确授权。
- 先确认 branch、base、remote 和 diff 范围，不要带入无关的既有改动。
- PR、commit、日志和检查输出不得包含 token、Cookie、授权码、代理凭据、私有 URL、下载内容、本地数据库或机器配置。
- 普通 PR 不编辑 `changelog/unreleased/` 或预写最终版本说明。用户可见的 CLI、MCP、SDK、config、workflow 或安全语义变化要同步对应文档。

## 准备 PR

1. 收集范围：

   ```bash
   git status --short
   git branch --show-current
   git diff --stat
   git diff --name-status
   ```

   确认当前不是 detached HEAD，并解析仓库的 default branch，不凭空假定为 `main`。
2. 检查公开契约、架构边界、相关文档和产品 `skills/pixiv-cli/`。需要代码审查时运行 `pixiv-cli-review`；发现问题先报告，不把修复混进单纯的 PR 准备。
3. PR 正文保留模板的三个部分：`变更`、`验证`、`自查`。不要添加版本分类、breaking 标记或隐藏 metadata。
4. Verification 只记录实际运行过的完整命令和结果。按范围选择：

   - 文档或 agent-only：`go test ./scripts/tests/documentation -count=1`、`git diff --check`；
   - Go 或行为改动：聚焦测试、`go test ./... -count=1`、`go vet ./...`，必要时 `sh scripts/build.sh`；
   - 共享、认证、下载、CLI、MCP 或 SDK：补跑 `go test -race ./... -count=1`；
   - workflow 或 release 脚本：运行对应 `scripts/cmd/*` policy 和受影响的 `scripts/tests`。

   未运行真实 Pixiv/FANBOX API、native host 或受保护 release evidence 时，说明原因和剩余风险。

## 创建或更新 PR

1. 获得授权后先 push 当前分支，不让 `gh pr create` 隐式 fork 或 push：

   ```bash
   git push --set-upstream origin <branch>
   ```

2. 从仓库模板创建 PR：

   ```bash
   gh pr create \
     --repo FlanChanXwO/pixiv-cli \
     --base <default-branch> \
     --head <owner>:<branch> \
     --title "<focused title>" \
     --template .github/PULL_REQUEST_TEMPLATE.md
   ```

   非交互创建时，把填好的模板写入临时文件，再传给 `--body-file`。只有用户明确指定时才添加 reviewer、draft、label、assignee 或 project。
3. 更新后复查模板和脱敏情况：

   ```bash
   gh pr edit <number> --body-file <prepared-body>
   gh pr view <number> --json number,title,body,baseRefName,headRefName,url
   ```

4. 用 `pixiv-cli-ci` 查看 required checks，用 `pixiv-cli-review` 处理审查。不要自动 merge；merge 需要单独授权。

## 合并后交接

向 `pixiv-cli-release-notes` 提供 PR URL、merge commit、候选 tag 范围、实际验证结果，以及用户可见文档或 workflow 变化。发布准备阶段会重新审计范围内的全部 PR 和 direct commit，无需在 PR 正文携带版本元数据。

## 结果报告

报告 PR URL/编号、head/base/ref、模板是否完整、验证命令和结果、required checks/review 状态及剩余风险。分别陈述“已创建”“CI 已通过”“已获 review”和“已合并”，不要互相推断。
