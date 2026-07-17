# Goal 2 实施计划：修复全项目代码评审问题

## 权威输入与完成定义

- 原始需求以 `input.md` 和附件 `/Users/flanchan/.codex/attachments/23daed5d-a86c-4691-aa50-fbb2edbcefce/pasted-text-1.txt` 为准。
- 范围包含报告中全部 P1、P2 和 P3 具体 finding；不能只修阻断项。若当前代码证据证明某项误报、已修或删除会破坏既定契约，必须在 `tasks.md` 记录证据，并以更符合原始风险的修复替代，不能静默跳过。
- 完成要求：每条 finding 都有实现、删除、文档化依据或证据充分的“不适用”结论；相关测试、全量测试、race、vet、release/Rust policy、pre-commit、知识图谱、远端 PR/CI 和最终 `main` 同步全部通过。

## 当前基线

- 隔离 worktree：`/Users/flanchan/Development/SourceCode/GithubProjects/pixiv-cli/.worktrees/review-fixes`
- 分支：`codex/review-fixes`
- 基线提交：`fc4d3847a4a845da9a34129fd4d05ba67870f78a`
- `go test ./... -count=1` 与 `go vet ./...` 已通过；基线工作树干净。
- v0.3.0 tag/Release 不可变。本 goal 只在后续 `main` 修复，不移动或改写既有 tag。

## 默认假设

1. P1 必须修复；P2/P3 的建议也视为待处理，除非当前证据明确否定。
2. 错误透明性必须兼顾脱敏：公开错误可暴露稳定分类和本地安全真因，但不得包含 token、Cookie、完整 URL、header 或上游 body。
3. 无依据限制优先删除或改为显式参数错误；不得用新 timeout、截断、重试上限或静默 fallback 掩盖问题。
4. P2-8 当前已有 release workflow SemVer 门禁且单测锁定 strict reject；先审计门禁是否覆盖全部发布入口。若证据完整，保留 fail-closed 并补依据注释/交叉测试；只有证明外部无效 tag 仍可进入受信发布面时才改为 skip。
5. P2-12 当前由 ADR 0006/0009 和外部测试明确锁定为 enrichment 失败则整体失败。先用 fixture 与可用的真实 R-18/登录墙证据重新评估；只有 ADR 取舍和兼容设计完整时才改变公开行为，否则以“已采纳设计、非当前 bug”的证据关闭并增强文档，不盲目 fallback。
6. 大文件拆分必须保持行为与 package API；拆分本身不借机改语义。
7. 纯死代码只有在 `rg`、调用图和测试证明无生产调用者后删除；不凭覆盖率单独判断。

## 初始化核查对报告措辞的修正

- P1-2 的 application 聚焦覆盖 29.6% 和列举方法 0% 成立，但 CLI `-coverpkg` 黑盒已覆盖相同用户流；这是聚焦测试债，不是“token 路径完全未测”的行为漏洞。
- P2-8 的 strict SemVer reject 已由单测锁定，release workflow 也强制 SemVer；策略改变必须先过 ADR，不能直接照建议 skip。
- P2-12 是 ADR 0006/0009 已采纳的完整 enrichment 契约，不是违反现有规格；必须证据驱动重新决策。
- `RuntimeConfig.RefreshToken` 当前未进入 TOML 写回，属于未来序列化诱导风险；删除死字段，但不得声称已有泄漏。
- `isAuthError` 函数仍有生产调用者，只删除在 adapter 已统一交付 `protocol.Failure` 后不可达的字符串 fallback，不删除 typed 401/403 判断。

## TDD 与子代理执行规则

- 每个普通 task 使用一个 fresh implementer；不得并行派发多个实现代理。
- 行为 bug 的 implementer 先写一个公开接口测试并展示 RED，再做最小 GREEN；后续行为继续逐个 RED→GREEN，最后才重构。纯覆盖补充或机械拆分先写/确认 characterization test，若它对既有正确行为直接 GREEN，必须如实记录，不能人为制造失败。
- 每个 task 实现后必须依次通过独立 spec reviewer、code-quality reviewer；有 finding 时由原 implementer/fix worker 修复，并做窄复审直到 APPROVE。
- 默认 implementer 不提交；主线程在两阶段复核通过、验证完成后提交，并回写 `tasks.md`。
- 所有代理必须保护共享 dirty worktree，不 reset/checkout/revert/覆盖无关改动。

## 架构与实现策略

1. 先修用户可观察的错误语义和 P1 测试缺口，建立安全错误分类与 application fake seam。
2. 再修 composition root、无据限制、死代码、更新选择和文件持久性，降低后续重构风险。
3. 然后统一 timeout/分页/输出小项，并拆分 MCP、登录、Web API 与 release verifier 大文件。
4. 最后审计 enrichment 既定设计、补 bootstrap coverage、更新文档和两份 knowledge graph，做全量终审与远端 CI。

## 验证层级

- 聚焦：每个 task 的相关 package `go test ... -count=1`，必要时 `-race`。
- 每三 task 集中检查：`go test ./... -count=1`、`go vet ./...`、`git diff --check`，并检查 secret/error/fallback/文档边界。
- 发布与 Rust：`sh scripts/test-release-workflow.sh`、`sh scripts/test-rust-vendor.sh`。
- 最终：`go test -race ./... -count=1`、`pre-commit run --all-files`、六平台 CI、真实认证 canary（不输出 token）。

## 回滚与安全

- 每个 task 独立提交，可按提交回滚；不使用 `reset --hard`、force push 或移动 tag。
- 任何涉及 auth/config 文件写入的修复先用临时目录测试权限、原子替换和 Sync 失败路径。
- 任何公开 model/错误/CLI/MCP 语义变化同步 README、`docs/` 与 `[Unreleased]`。
