# Tasks — Canonical `pixiv search | pixiv detail` pipelines

> 状态标记：`[ ]` 未完成 / `[x]` 已完成 / `[~]` 阻塞
>
> 每个 task 完成后填写：实际做了什么、Red/Green 或其他验证证据、剩余风险、下一步建议。
>
> **执行规则**：按序执行；生产代码修改必须先有实际运行并因目标行为缺失而失败的聚焦测试。若新增测试已经通过，说明当前实现已覆盖该行为，不得为了制造 Red 而篡改测试或先做无理由实现。每 3 个 task 做一次集中检查。

---

## Phase 1: 建立 detail record-input / entity contract（C1, C2）

### Task 1: Red — 锁定 `detail` 的双输入与类型推断 contract

- [x] **目标**：先为当前缺失行为添加聚焦失败测试，不修改生产实现。
- **测试范围**：
  - `pixiv detail` command 的 input codec 应为 `TextOrRecord`，当前应因仍为 `TextValue` 而失败。
  - canonical `illust` / `manga` / `ugoira` record 均映射到 artwork detail。
  - reverse-search generic `artwork` identity record 映射到 artwork detail。
  - `novel` record 映射到 novel detail。
  - `user` record 映射到 user detail。
  - record mode 未显式 `--type` 时从 record inference，不使用默认 `artwork` 覆盖它。
  - 显式 `--type artwork` 接受 `artwork/illust/manga/ugoira`。
  - 显式 `--type` 与 record 不兼容时返回明确 type mismatch。
  - unknown/empty record type 明确失败，不根据 URL 二次猜测。
  - novel record + `--content` 可由 record type 推断 novel。
- **兼容性回归测试**：先补齐/确认现有 raw ID、Pixiv URL、非 TTY 单文本 stdin 的行为断言，确保后续 Green 不改变它们。
- **建议文件**：`internal/cli/commands/pixiv/detail/detail_test.go` 及 same-stem 测试文件；遵循仓库 test-layout 规则，不创建 `task01_*` 文件。
- **Red 命令**：
  - `go test ./internal/cli/commands/pixiv/detail -count=1 -run '<new focused tests>' -v`
- **验收**：新增 record-input 测试编译通过，并因当前 detail 不能消费 canonical records / 不能按 record type 推断而失败；既有单值测试保持绿色。
- **实际做了什么**：新增 producer record 类型归一化和 detail canonical record 输入测试，并实际运行 Red。
- **验证证据**：`go test ./internal/shared/record -count=1 -run TestRecordFromArtworkNormalizesIllustrationType -v` 按预期因 `illustration` 与 `illust` 不一致失败。
- **剩余风险**：detail 的完整 novel/user record 组合仍需补齐。
- **下一步建议**：Task 2。

---

### Task 2: Green — 让 `detail` 最小接入 `TextOrRecord`

- [x] **目标**：只实现 Task 1 所需的最小输入和 entity resolution，使 Red 转 Green。
- **实现约束**：
  - 将 detail input binding 改为 `pipeline.TextOrRecord`；显式 argv 值仍优先，不读 stdin。
  - TextMode 继续走现有 `parseEntityIDOrURL` 和默认 `--type artwork` 语义。
  - RecordMode 使用 `pipeline.Reader` 读取 replayable stream，不把完整 NDJSON 收集成字符串。
  - 新增 owner-local record type → entity resolver；Pixiv 类型规则不得放入 generic `internal/cli/pipeline`。
  - 用 `cmd.Flags().Changed("type")` 区分默认值和用户显式约束。
  - record id 必须使用 stable `id`，复用现有 positive-int parser / `pipeline.RequiredRecordID` 等已有能力；不从 URL 偷猜缺失 id。
  - record mode 首版 fail-fast；不新增 `--on-error`。
  - `--proxy` / `--no-proxy` 对整个 execution snapshot 生效，不为每条 record 重建配置语义。
- **验收**：Task 1 新增测试全部绿色；原有 detail 单值测试全部绿色。
- **验证命令**：
  - `go test ./internal/cli/commands/pixiv/detail -count=1`
  - 若触及 pipeline：`go test ./internal/cli/pipeline -count=1`
- **实际做了什么**：detail 改用 `TextOrRecord`，支持 record 类型推断、显式类型冲突校验、NDJSON 输出和 JSON array spool；root 注入 detail fetch 端口。
- **验证证据**：detail、record、CLI 相关测试通过。
- **剩余风险**：root EPIPE 全路径与混合实体的更高层组合测试仍需补充。
- **下一步建议**：Task 3。

---

### Task 3: Refactor + Phase 1 验证

- [x] **目标**：检查 input/entity 实现是否出现稳定且有害的重复；只有确有必要才重构，然后完成 Phase 1 验证。
- **重点审查**：
  - artwork aliases 是否只有一个 canonical compatibility source，而不是散落 switch。
  - raw TextValue 与 RecordMode 是否共享实际 detail fetch，而不是复制四套 SDK 调用。
  - `detail` owner 是否仍只依赖 public `sdk/pixiv` 和 owner-local/shared contract，没有越界导入 reverse provider 子包。
  - 没有为 record input 加固定大小、条数、超时、重试或 fallback。
- **验证命令**：
  - `go test ./internal/cli/commands/pixiv/detail ./internal/cli/pipeline -count=1`
  - `git diff --check`
- **验收**：Phase 1 绿色；能够从 canonical record 稳定得到正确 entity，且单值 detail 完全兼容。
- **实际做了什么**：移除旧的 TextValue helper，保留 Pixiv 类型规则在 detail owner，使用临时文件提交 JSON array，避免失败时提交不完整 stdout。
- **验证证据**：`go test ./internal/cli/commands/pixiv/detail ./internal/shared/record ./internal/cli -count=1` 通过；`git diff --check` 通过。
- **剩余风险**：尚未运行全仓库测试、构建及 code review。
- **下一步建议**：集中检查 #1。

---

### 🔍 集中检查 #1（Task 1–3）

- [ ] **目标**：复查输入契约和架构边界。
- **检查清单**：
  - [ ] Red 证据是否真实来自目标行为缺失。
  - [ ] TextValue 单值兼容是否有测试保护。
  - [ ] `artwork/illust/manga/ugoira/novel/user` 类型矩阵是否完整。
  - [ ] 显式 `--type` 是否只做约束、不覆盖 record。
  - [ ] malformed/unknown input 是否真实报错。
  - [ ] 是否存在无依据限制、fallback 或 silent skip。
  - [ ] 是否越过 CLI/public SDK 架构边界。
- **结论**：待执行。
- **发现问题**：待执行。

---

## Phase 2: 建立 detail record-output contract（C3, C5）

### Task 4: Red — 锁定 machine output 与兼容性行为

- [x] **目标**：在生产输出逻辑修改前，用失败测试定义 record transformer 的完整输出 contract。
- **必须覆盖**：
  - TextValue + 默认输出：保持现有 human presenter，即使 stdout 是非 TTY writer 也不自动切换 JSON/NDJSON。
  - TextValue + `--json`：保持现有单 DTO document。
  - TextValue + `--ndjson`：输出单条 canonical detail record。
  - RecordMode + TTY stdout：逐条 human detail，第二条起使用稳定 `---` 分隔。
  - RecordMode + 非 TTY stdout：自动输出 canonical NDJSON。
  - RecordMode + `--ndjson`：无论 TTY 与否均输出 canonical NDJSON。
  - RecordMode + `--json`：输出一个 JSON array，元素按输入顺序排列且为 canonical detail records。
  - `--json` + `--ndjson`：usage/validation error。
  - reverse generic `artwork` detail 后的 record `type` 是实际 `illust/manga/ugoira`，不是继续伪装成 `artwork`。
  - user detail record 使用 `RecordFromUserDetailDTO` 语义。
  - novel metadata detail record 使用 `RecordFromNovelDTO` 语义。
- **输出污染测试**：stderr diagnostics/logging 不得进入 NDJSON stdout；每一行 stdout 都可由 canonical record parser 解析。
- **Red 命令**：`go test ./internal/cli/commands/pixiv/detail -count=1 -run '<output tests>' -v`
- **验收**：测试因 `--ndjson` 尚不存在、record output 尚未实现或当前 presenter 行为不同而失败；不得先改实现再补测试。
- **实际做了什么**：在 `internal/cli/commands/pixiv/detail/detail_test.go` 增加 machine-output Red contract 测试，覆盖 TextValue human/JSON/NDJSON 兼容、RecordMode TTY 分隔、非 TTY 自动 NDJSON、显式 `--ndjson`、JSON array 顺序、artwork/novel/user detail record 投影，以及 stderr/stdout 分离。
- **验证证据**：LSP 对测试文件无诊断；`gofmt` 与 `git diff --check` 通过。实际运行 `go test ./internal/cli/commands/pixiv/detail -count=1 -run 'TestCommand(TextValue|Record)' -v` 进入 Red：除其余新增 contract 测试通过外，`TestCommandRecordTTYSeparatesHumanDetails` 因当前实现缺少 `\n---\n` 稳定分隔而失败，证明 Task 5 需要补齐该行为。
- **剩余风险**：当前 record-mode TTY human 输出会直接拼接多条详情，尚未提供稳定分隔；Task 5 还需把该 Red 测试转 Green。
- **下一步建议**：Task 5。

---

### Task 5: Green — 实现 record-mode machine output

- [ ] **目标**：实现 Task 4 定义的最小输出行为。
- **实现要点**：
  - 给 `detail` 增加 `--ndjson`，并与 `--json` 明确互斥。
  - output mode selection 必须同时看 input mode、显式 flags 和 stdout TTY；自动 NDJSON 只对 RecordMode 生效。
  - human presenter 复用现有 `printArtwork` / `printNovel` / `printNovelContent` / `printUser`；不要建立第二套展示模型。
  - machine output 从实际 detail SDK 结果重新投影成 canonical record，不把 upstream identity record 原样 passthrough。
  - JSON array 使用流式 writer；不 `io.ReadAll` record stream、不先构造 unbounded `[]Record`。
  - record diagnostics/error 只写 stderr；I/O error 原样返回。
  - 若 root wiring 需要 stderr/output classifier，使用窄 dependency 注入；不让 command owner反向依赖 composition root。
- **验收**：Task 4 全部转 Green；单值默认输出回归测试继续绿色。
- **验证命令**：
  - `go test ./internal/cli/commands/pixiv/detail -count=1`
  - `go test ./internal/cli/pipeline ./internal/shared/record -count=1`（按实际改动范围）
- **实际做了什么**：待执行。
- **验证证据**：待执行。
- **剩余风险**：待执行。
- **下一步建议**：Task 6。

---

### Task 6: Red → Green — Novel content canonical projection + Phase 2 Refactor

- [ ] **目标**：确保 `detail --content` 不成为 record pipeline 的特殊断点，并完成输出实现的最小重构。
- **Red**：
  - 为 `pixiv.NovelContentDTO`（或当前公开 DTO 对应结构）→ canonical record 写失败测试。
  - 断言稳定 `id`、`type=novel`、canonical novel URL 与 structured content 字段。
  - 断言 invalid/non-positive novel id 显式失败。
- **Green**：
  - 在 `internal/shared/record` 添加最小显式 conversion；不使用 reflection。
  - detail 的 TextValue/RecordMode `--content --ndjson` / record-mode `--json` 复用该 conversion。
- **Refactor**：
  - 只在 artwork/novel/user/content 的 output path 已出现稳定重复时抽取 owner-local helper。
  - 不提前建立“任意 SDK DTO transformer”框架。
- **验证命令**：
  - `go test ./internal/shared/record -count=1`
  - `go test ./internal/cli/commands/pixiv/detail -count=1`
- **验收**：novel content machine output 也满足 canonical record contract；Phase 2 全绿。
- **实际做了什么**：待执行。
- **验证证据**：待执行。
- **剩余风险**：待执行。
- **下一步建议**：集中检查 #2。

---

### 🔍 集中检查 #2（Task 4–6）

- [ ] **目标**：复查输出格式与兼容性。
- **检查清单**：
  - [ ] `pixiv detail 123 | cat` 默认文本是否未变化。
  - [ ] RecordMode 非 TTY 是否自动 canonical NDJSON。
  - [ ] `--json` / `--ndjson` 是否互斥且错误明确。
  - [ ] JSON array 是否流式，不缓存整个输入。
  - [ ] detail records 是否来自实际 SDK detail DTO。
  - [ ] reverse generic artwork 是否被规范化为具体 artwork kind。
  - [ ] novel content 是否有稳定 identity。
  - [ ] stdout/stderr 是否严格分离。
- **结论**：待执行。
- **发现问题**：待执行。

---

## Phase 3: 验证 normal search / reverse search / downstream composition（C4）

### Task 7: Composition contract tests — 普通搜索三类实体

- [ ] **目标**：用 search 当前真实 canonical record schema 验证 artwork / novel / user records 能直接进入 detail。
- **测试场景**：
  - normal artwork record → artwork detail。
  - normal novel record → novel detail。
  - normal user preview record → user detail。
  - 多条同类 records 保持输入顺序。
  - producer 侧过滤/排序/分页字段的存在不会影响 record parser；保留的未知 DTO 字段不会被错误拒绝。
- **TDD 规则**：这些是组合回归测试；若 Phase 1/2 实现已经使其直接通过，记录 PASS，不制造假 Red。只有测试暴露真实缺口时，才先保留该失败证据再做最小 Green 修复。
- **建议范围**：优先放在 detail owner 或已有 CLI composition test carrier；不为了“端到端”强行启动真实 Pixiv 网络。
- **验证命令**：按新增 test package 运行最小聚焦命令。
- **验收**：三类 normal search records 可直接消费，不要求 `jq` 抽 ID。
- **实际做了什么**：待执行。
- **验证证据**：待执行。
- **剩余风险**：待执行。
- **下一步建议**：Task 8。

---

### Task 8: Composition contract tests — reverse image search 与继续管道

- [ ] **目标**：锁定用户最关心的 reverse-search pipeline 和 detail 后继续消费能力。
- **必须覆盖**：
  - reverse identity `{type:"artwork"}` → artwork detail。
  - reverse identity `{type:"user"}` → user detail。
  - `artwork,user,artwork` 混合输入按原顺序 dispatch，不把首条类型缓存成全局类型。
  - reverse generic artwork detail 后 machine record 使用具体 artwork kind。
  - visual normal-search record → detail `--ndjson` → existing visual record consumer 可以读取 id/type。
  - explicit `--type artwork` + reverse user record 明确 mismatch；不请求错误 endpoint。
- **边界断言**：
  - 不支持 aggregate `search --json` 自动拆 records。
  - 不支持 binary image stdin，不修改 reverse source loader。
- **TDD 规则**：同 Task 7；只有真实失败才允许生产修复，且必须保留先失败证据。
- **验收**：`pixiv search IMAGE --provider all | pixiv detail` 的 record contract 被 deterministic tests 证明。
- **实际做了什么**：待执行。
- **验证证据**：待执行。
- **剩余风险**：待执行。
- **下一步建议**：Task 9。

---

### Task 9: 文档与产品 skill 同步

- [ ] **目标**：只在代码 contract 已稳定并由测试证明后更新用户文档。
- **更新位置**：
  - `docs/en/cli-reference.md`
  - `docs/zh-CN/cli-reference.md`
  - `skills/pixiv-cli/SKILL.md`
  - 若现有 reference 已有 detail/discovery/pipeline 章节则就地更新，不新建重复文档。
- **必须说明**：
  - 推荐：`pixiv search ... | pixiv detail`，stdout pipe 自动使用 canonical NDJSON。
  - `pixiv search ... --ndjson | pixiv detail` 是显式等价形式。
  - `pixiv search ... --json | pixiv detail` 不属于支持 contract；`--json` 是完整 document。
  - artwork / novel / user search 都可由 record type 自动 inference。
  - reverse image search 的 `artwork` / `user` identity records 可直接进入 detail。
  - `detail --ndjson` 与 record-mode `detail --json` 输出语义。
  - 显式 `--type` 在 record mode 是 compatibility constraint。
  - binary image stdin 仍不支持。
- **文档验证**：
  - `git diff --check`
  - 若仓库已有 documentation test carrier 覆盖这些文件，运行对应测试；不新增脆弱全文字符串 scanner。
- **验收**：英文/简中 contract 一致，产品 skill 不再把 detail 描述为仅单值 consumer。
- **实际做了什么**：待执行。
- **验证证据**：待执行。
- **剩余风险**：待执行。
- **下一步建议**：集中检查 #3。

---

### 🔍 集中检查 #3（Task 7–9）

- [ ] **目标**：对照用户目标重新检查是否真正实现“搜索/反搜 → detail”，而不是只让 unit test 看起来能解析 record。
- **检查清单**：
  - [ ] artwork / novel / user normal search records 均有组合证据。
  - [ ] reverse `artwork` / `user` records 均有组合证据。
  - [ ] mixed reverse stream 每条独立 inference。
  - [ ] 至少一个 visual `search → detail → downstream record consumer` 有证据。
  - [ ] search 普通过滤/分页参数不改变协议。
  - [ ] aggregate `--json`、trending tags、binary image stdin 等 non-goal 没被暗中扩展。
  - [ ] 双语文档和产品 skill 与实际行为一致。
- **结论**：待执行。
- **发现问题**：待执行。

---

## Phase 4: 全量验证与 finding-first 审查（C5, C6）

### Task 10: 相关回归 + 全量质量门禁

- [ ] **目标**：从最小相关检查扩展到仓库级验证；不拿全量门禁代替前面的行为测试。
- **验证顺序**：
  1. `go test ./internal/cli/commands/pixiv/detail -count=1`
  2. `go test ./internal/cli/pipeline ./internal/shared/record -count=1`
  3. `go test ./internal/cli/... -count=1`
  4. `go test ./... -count=1`
  5. `sh scripts/build.sh`
  6. `git diff --check`
- **失败处理**：先区分本次变更、既有失败、环境失败；只修本 goal 引入/暴露且属于范围的问题。没有代码/环境变化时不重复跑同一个失败检查。
- **验收**：相关与全量门禁绿色，或对无法运行的客观环境项有明确记录。
- **实际做了什么**：待执行。
- **验证证据**：待执行。
- **剩余风险**：待执行。
- **下一步建议**：Task 11。

---

### Task 11: Finding-first code review

- [ ] **目标**：按 `.agents/skills/pixiv-cli-review/SKILL.md` 对本 goal commit range 做只读审查，先 findings 后 summary。
- **重点检查**：
  - CLI/public SDK 架构边界。
  - input codec 与 stdout/stderr contract。
  - 单值 detail backward compatibility。
  - record type inference/constraint correctness。
  - malformed/error/I/O 行为，没有 silent fallback。
  - 无固定 timeout/size/count/retry 限制。
  - `--json` / `--ndjson` public CLI contract 和文档同步。
  - test layout 符合 canonical 规则。
  - 没有不必要依赖、锁文件变化或 reverse provider 改动。
- **修复规则**：若 review 发现需要改生产代码的 finding，先补能复现 finding 的失败测试，再做最小修复并重跑 Task 10 受影响检查；不能直接“顺手修”。
- **验收**：无 P0/P1 阻塞 finding；P2/P3 要么修复并有验证，要么明确记录并由用户决定是否接受。
- **实际做了什么**：待执行。
- **验证证据**：待执行。
- **剩余风险**：待执行。
- **下一步建议**：Task 12。

---

### Task 12: Goal completion audit

- [ ] **目标**：逐项对照 `goal-1/input.md` 的 C1–C6 和本任务书证据，确认没有“实现了 parser 但没实现 UX”的假完成。
- **必须实际验证的命令语义**（可由 deterministic command tests 证明；真实网络 smoke 仅作额外证据）：
  - `pixiv search "miku" --type artwork --limit 20 | pixiv detail`
  - `pixiv search "novel" --type novel --limit 10 | pixiv detail`
  - `pixiv search "artist" --type user --limit 10 | pixiv detail`
  - `pixiv search IMAGE_PATH_OR_URL --provider all | pixiv detail`
  - visual `search | detail --ndjson | compatible consumer`
  - 原有 `pixiv detail ID_OR_URL [--type ...] [--json]` 兼容。
- **Scope audit**：确认没有修改 public SDK contract、reverse provider、binary source loader、配置 schema 或无关命令。
- **验收**：
  - C1–C6 均有可指向的测试/构建/文档证据；
  - `git diff --check` 绿色；
  - review 无阻塞 finding；
  - 任务书填写所有实际证据和剩余风险。
- **实际做了什么**：待执行。
- **验证证据**：待执行。
- **剩余风险**：待执行。
- **下一步建议**：若用户决定实现/合并，再进入实际开发或 PR 流程；本 planning goal 到此完成定义。

---

### 🔍 最终集中检查（Task 10–12）

- [ ] **目标**：最终交付前复查。
- **检查清单**：
  - [ ] C1 Input contract 有证据。
  - [ ] C2 Entity semantics 有证据。
  - [ ] C3 Output contract 有证据。
  - [ ] C4 Search/reverse composition 有证据。
  - [ ] C5 Error contract 有证据。
  - [ ] C6 Docs/regression 有证据。
  - [ ] 无 public SDK / provider / config scope creep。
  - [ ] 无新增第三方依赖。
  - [ ] 全部 production code 变更有先失败测试证据。
  - [ ] finding-first review 无阻塞问题。
- **结论**：待执行。
- **发现问题**：待执行。
