---
name: pixiv-cli-release-notes
description: Audit pixiv-cli release sources, write and validate bilingual Markdown release notes, publish immutable tags, synchronize historical GitHub Release bodies, and hand off SkillHub/ClawHub verification. Use for release preparation, version selection, tag publication, release monitoring, or historical note synchronization.
---

# pixiv-cli Release Notes 与发布

普通 PR 使用 `pixiv-cli-pr`。本技能从合并后的来源审计开始，负责双语版本说明、tag 发布和下游 handoff。先读 `AGENTS.md`、`docs/zh-CN/maintainers/development.md` 和现有 locale 的 `CONTRIBUTING`。

## 授权与不可变边界

- 检查、审计、preview、validate 和 review 默认只读。
- 创建或更新 PR、push、merge、写入 `changelog`、创建或推送 tag、创建或编辑 Release、执行 `sync-history --apply` 前，要取得用户对该动作的明确授权。
- release-prep、tag 和发布授权必须明确给出版本、commit/tag 范围和预期影响。版本建议不等于授权。
- 不把 `GH_TOKEN`、refresh token、Cookie、私有 URL、下载内容或 API 响应写入 argv、日志、PR、changelog、artifact 或输出。
- `.github/workflows/release.yml` 只由 `push.tags: v[0-9]*` 触发。不要手工 dispatch release workflow，不要移动旧 tag，也不要用默认分支内容覆盖不可变 tag。

## 普通 PR

- 使用 `.github/PULL_REQUEST_TEMPLATE.md`，只保留改动、验证和检查清单。
- 用户可见的 CLI、MCP、SDK、config、workflow 或安全语义变化在同一 PR 同步文档。
- 不在 PR 正文添加 release category、breaking、summary 或其他隐藏字段，也不编辑 `changelog/unreleased/`。

## 审计与编写

1. 确认候选范围并把审计报告写到本地临时目录或 CI 的 `$RUNNER_TEMP`：

   ```bash
   audit_report="$(mktemp -t pixiv-cli-release-audit.XXXXXX.json)"
   go run ./scripts/cmd/releasenotes audit \
     --repo FlanChanXwO/pixiv-cli \
     --from vPREVIOUS \
     --to COMMIT_OR_TAG \
     --output "$audit_report"
   ```

   审计只收集真实 PR/direct-commit 来源、标题、作者和首次贡献者，不读取 PR body，也不推断分类或 SemVer。报告是临时输入，不提交仓库。
2. 逐项检查审计来源，结合实际兼容性和最终章节决定版本。获得精确版本和写入授权后，直接创建或编辑：

   ```text
   changelog/vX.Y.Z/en.md
   changelog/vX.Y.Z/zh-CN.md
   ```

   使用适用的 `Breaking changes`、`Added`、`Changed`、`Fixed`、`Security`、`Documentation`、`Maintenance` 章节。每个条目描述结果并附真实 PR 或 commit 链接；没有用户可见影响的来源放进 `Maintenance`，多个内部改动可以合并成一条。
3. 两种语言必须覆盖相同的来源集合。审计范围内的每个来源都要出现，范围外来源不得混入；同一来源可以与相关来源合并到一个条目，但不能遗漏。首次贡献者只使用审计结果，不猜测作者身份。
4. 更新 `changelog/README.md` 和 `changelog/README.zh-CN.md` 的版本入口，然后校验：

   ```bash
   go run ./scripts/cmd/releasenotes validate \
     --version X.Y.Z \
     --dir changelog/vX.Y.Z \
     --previous vPREVIOUS \
     --audit "$audit_report"
   go test ./scripts/internal/releasenotes ./scripts/tests/documentation -count=1
   git diff --check
   ```

   `validate` 检查双语格式、章节顺序、compare footer 和完整来源集合。release-prep PR 仍使用普通三段式模板，不附带中间 plan 或机器可读 metadata。

## Immutable tag 发布与验收

1. release-prep PR 合并且 required checks 通过后，确认默认分支包含 `changelog/vX.Y.Z/`。只创建用户已授权的精确 `vX.Y.Z` tag；不要重写已有 tag。
2. push tag 后由 release workflow 自动发布。用 `pixiv-cli-ci` 和 GitHub CLI 查看对应 run/jobs；正式 tag 会重新执行 `audit` 和 `validate`。
3. 验收至少包括：

   - Release body 来自该 tag 的英文与简体中文 notes；
   - 六平台 archive、安装脚本、checksums 和签名 metadata 属于同一 tag；
   - release notes audit、source ancestry、production rebuild 和 Homebrew policy 通过；
   - SkillHub/ClawHub 使用匹配的 product skill 版本，或记录明确的 unchanged-skill skip。

失败时保留原始 run/step 错误并暂停，不用手工资产或默认分支内容绕过 gate。

## 历史 GitHub Release 同步

先 dry-run：

```bash
go run ./scripts/cmd/releasenotes sync-history \
  --repo FlanChanXwO/pixiv-cli \
  --version X.Y.Z \
  --dir changelog/vX.Y.Z
```

只有用户明确授权列出的版本后才加 `--apply`。该命令只创建或更新 Release body，保留现有 assets；完成后确认远端 body 与本地双语渲染一致。

## 结果报告

报告版本、commit/tag 范围、临时审计报告的位置、修改的 changelog 文件、PR/Run/Release 链接、验证结果、下游 handoff 和未运行的真实环境测试。分别陈述候选版本、CI、公开 Release 与 SkillHub/ClawHub 状态。
