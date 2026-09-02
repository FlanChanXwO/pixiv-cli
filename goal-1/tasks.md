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

- [x] **目标**：复查输入契约和架构边界。
- **检查清单**：
  - [x] Red 证据是否真实来自目标行为缺失。
  - [x] TextValue 单值兼容是否有测试保护。
  - [x] `artwork/illust/manga/ugoira/novel/user` 类型矩阵是否完整。
  - [x] 显式 `--type` 是否只做约束、不覆盖 record。
  - [x] malformed/unknown input 是否真实报错。
  - [x] 是否存在无依据限制、fallback 或 silent skip。
  - [x] 是否越过 CLI/public SDK 架构边界。
- **实际做了什么**：
  - 对完整 goal 范围（`eeaea0b^..HEAD`，并与 `origin/main...HEAD` 交叉核对）复查 Task 1–3 的输入契约；用 `git show 8506202^:internal/cli/commands/pixiv/detail/detail.go` 确认实现前 detail 仍是单值 `TextValue`，因此 Task 1 的 Red 是真实行为缺失而非伪造失败。
  - 对照 `detail_test.go` 的 `TestCommandTextValueKeepsHumanOutputWhenStdoutIsNotTTY`、`TestCommandTextValueJSONRemainsSingleDocument`、`TestCommandTextValueNDJSONOutputsCanonicalRecord` 与 raw ID/URL 测试，确认 TextValue 单值兼容仍有保护；`resolveRecordEntity` 覆盖 `artwork/illust/manga/ugoira` → artwork、`novel` → novel、`user` → user，且 `TestCommandRecordMachineOutputUsesDetailTypes` 已补齐四种 artwork 输入别名，normal/reverse composition 测试覆盖实体路径和顺序。
  - 确认 `entityForRecord` 先推断 record 类型，再在 `cmd.Flags().Changed("type")` 为真时把显式 `--type` 作为兼容性约束；不兼容在 fetch 前显式失败，不从 URL 二次猜测或覆盖 record 类型。`ConsumeNDJSONRecords`、`RequiredRecordID` 以及新增的 `TestCommandRejectsUnsupportedRecordTypeBeforeFetching` 共同覆盖 malformed、aggregate JSON、未知/不支持类型、ID 与远端/I/O 失败的非零诊断路径。
  - 复查 diff 和 import graph：没有新增固定超时、长度/记录数/分页硬限制、重试、静默 skip 或 fallback；detail/record 只依赖 `internal/cli/pipeline`、`internal/shared/record` 与 public `sdk/pixiv`，root 仅保留 composition wiring，没有触及 public SDK、配置或 reverse provider 子包。
- **结论**：输入契约、兼容性、类型推断/约束与架构边界通过集中检查。发现并修复一处由本 goal 初始 root 改动引入的输出协议回归后，当前相关测试和 LSP 诊断均通过。
- **发现问题**：`8506202` 在 `commandAutoWritesNDJSON` 中把“存在任意 command annotation”误当作“存在输出协议 annotation”，导致 `pipeline.Bind` 已写入 input annotation 的非 TTY `pixiv search` 跳过旧的 command-path 自动 NDJSON 判断。先新增 `TestCommandAutoWritesNDJSONFallsBackWithInputAnnotation`，Red 实际失败（返回 false），再将逻辑改为仅在 `pixiv-cli.output-ndjson` 键存在时短路，否则继续非 TTY fallback；focused Green 已通过。此前 Task 11 仅审查 `origin/feature/search-detail-pipeline...HEAD`，未包含初始生产提交，本集中检查已改用完整范围，Task 12 需继续按完整范围终审。
- **验证证据**：
  - Red：`go test ./internal/cli -count=1 -run '^TestCommandAutoWritesNDJSONFallsBackWithInputAnnotation$' -v`，断言失败，实际值为 false。
  - Green/回归：`go test ./internal/cli ./internal/cli/commands/pixiv/detail ./internal/cli/commands/pixiv/search ./internal/shared/record -count=1` 通过；`go vet ./internal/cli ./internal/cli/commands/pixiv/detail ./internal/cli/commands/pixiv/search ./internal/shared/record` 通过。
  - 聚焦 detail matrix/兼容性/error/composition 测试（含四种 artwork alias 与 unknown type）全通过；修改后的 `root.go`、`root_test.go`、`detail_test.go` LSP diagnostics 为空；`gofmt -d` 无输出，`git diff --check` 通过。
- **剩余风险**：未运行需要本机凭据的真实 Pixiv/FANBOX E2E，也未做跨平台真实 TTY smoke；二者均是仓库既有显式环境依赖，不构成本 goal 的 deterministic pipeline 阻塞。Task 12/final check 仍需复查完整 goal range，并重新确认文档、构建与全仓库门禁证据。
- **下一步建议**：Task 12。

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

- [x] **目标**：实现 Task 4 定义的最小输出行为。
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
- **实际做了什么**：在 `detail.runRecords` 中为 TTY human record output 增加单条详情缓冲：每条详情成功后再写入 stdout，后续条目之间输出稳定的 `\n---\n` 分隔；machine output 路径保持逐条写入。另将 canonical record writer 的 stdout 写入错误标记为 fatal pipeline error，避免被误报成 action diagnostic 或吞掉原始 I/O 原因。未缓存完整 NDJSON 输入。
- **验证证据**：先运行 `go test ./internal/cli/commands/pixiv/detail -count=1 -run 'TestCommandRecord(OutputWriteErrorRemainsOriginal|TTYSeparatesHumanDetails)' -v`，确认 separator 已通过而 stdout writer error 先以 `pipeline records failed` 暴露 Red；实现后运行 `go test ./internal/cli/commands/pixiv/detail -count=1 -run 'TestCommand(TextValue|Record)' -v` 全部通过，并运行 `go test ./internal/cli/commands/pixiv/detail -count=1`、`go test ./internal/cli/pipeline ./internal/shared/record -count=1` 均通过。LSP 对 detail 生产/测试文件无诊断，`gofmt` 与 `git diff --check` 通过。
- **剩余风险**：Task 6 仍需补齐 novel content 的 canonical projection Red/Green 与 Phase 2 复查；全仓库门禁和 finding-first review 尚未执行。
- **下一步建议**：Task 6。

---

### Task 6: Red → Green — Novel content canonical projection + Phase 2 Refactor

- [x] **目标**：确保 `detail --content` 不成为 record pipeline 的特殊断点，并完成输出实现的最小重构。
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
- **实际做了什么**：新增 `RecordFromNovelContentDTO` 的 canonical identity/structured block 投影测试，并将 novel content 的非正 ID 纳入显式失败矩阵；新增 detail 命令测试覆盖 TextValue `--content --ndjson`、RecordMode `--content --ndjson` 与 RecordMode `--content --json`。复查发现现有显式 conversion 已由前序实现提供，且 detail 三条 machine-output 路径已复用它，因此本 task 不新增生产代码、不抽取未经证实的通用 transformer，也不使用 reflection。
- **验证证据**：按执行约束先实际运行新增 focused tests；它们在当前实现上全部直接通过，因此没有为了制造 Red 而篡改测试或强行改实现。`go test ./internal/shared/record ./internal/cli/commands/pixiv/detail -count=1 -run 'TestRecordFromNovelContent|TestRecordMappersRejectNonPositiveSDKID|TestCommandNovelContentMachineOutputUsesCanonicalRecordProjection' -v` 通过；随后 `go test ./internal/shared/record -count=1`、`go test ./internal/cli/commands/pixiv/detail -count=1`、两包联合回归、`gofmt`、`git diff --check` 均通过；两个受影响测试文件 LSP diagnostics 为空。
- **剩余风险**：Task 4–6 集中检查、normal/reverse search 组合测试、双语文档同步、全仓库门禁和 finding-first review 尚未执行。
- **下一步建议**：集中检查 #2。

---

### 🔍 集中检查 #2（Task 4–6）

- [x] **目标**：复查输出格式与兼容性。
- **检查清单**：
  - [x] `pixiv detail 123 | cat` 默认文本是否未变化：TextValue 默认路径仍走原 human presenter，已有回归测试覆盖。
  - [x] RecordMode 非 TTY 是否自动 canonical NDJSON：`runRecords` 仅在 record mode、无显式输出 flag 且 stdout 非 TTY 时自动打开。
  - [x] `--json` / `--ndjson` 是否互斥且错误明确：入口在读取输入前检查 `Changed` 状态。
  - [x] JSON array 是否流式，不缓存整个输入：`jsonArraySpool` 逐条写临时文件并在完成后复制，不构造无界内存数组。
  - [x] detail records 是否来自实际 SDK detail DTO：artwork/novel/content/user 均先调用 fetcher，再显式 DTO conversion。
  - [x] reverse generic artwork 是否被规范化为具体 artwork kind：record entity resolver 接受 generic `artwork`，输出使用实际 artwork DTO kind。
  - [x] novel content 是否有稳定 identity：`RecordFromNovelContentDTO` 固定 `id/type/url` 并保留 structured content。
  - [x] stdout/stderr 是否严格分离：record diagnostics 走 `ErrorOutput`，machine output 只写 `Output`，I/O error 不被吞掉。
  - [x] RecordMode 的 `--content` 是否对非 novel 输入在 fetch 前显式失败：已通过新增 Red/Green 回归测试并修复。
- **结论**：输出格式、machine projection、流式 array、option validation 与 stdout/stderr 边界通过检查；未发现需要调整回滚方案或新增限制的问题。
- **发现问题**：已修复 `internal/cli/commands/pixiv/detail/detail.go` 中 RecordMode 未校验 `--content` 的缺口；现由统一 `validateContentEntity` 同时保护 TextValue 与 RecordMode。

---

### Task 6.1: Red → Green — Reject `--content` for non-novel records

- [x] **目标**：修复集中检查发现的 RecordMode option-validation 缺口，保持 TextValue 与 RecordMode 的 `--content` 语义一致。
- **Red**：
  - 增加 artwork/user canonical record 配合 `--content` 的 command test。
  - 断言返回 `--content is only supported when --type novel`，且在 fetch 前失败、不请求错误 endpoint。
- **Green**：
  - 在 RecordMode 完成 entity inference/compatibility 后、读取 ID 和 fetch 前复用同一条显式校验。
  - 不改变 novel record + `--content` 的正常路径，不新增 fallback、限制或通用抽象。
- **Refactor**：仅检查是否能复用已有 option-validation helper；不为一个 guard 建立框架。
- **验证命令**：
  - `go test ./internal/cli/commands/pixiv/detail -count=1 -run 'TestCommand.*Content' -v`
  - `go test ./internal/cli/commands/pixiv/detail -count=1`
- **验收**：非 novel record + `--content` 明确失败且未调用对应 fetcher；TextValue 与 novel content 回归继续通过。
- **实际做了什么**：在 `internal/cli/commands/pixiv/detail/detail.go` 新增 `validateContentEntity`，让 TextValue 与 RecordMode 共用同一校验；RecordMode 在 entity inference/compatibility 完成后、读取 record ID 和调用 fetcher 前拒绝 artwork/user + `--content`。测试覆盖 artwork 与 user record，并断言错误诊断写入 stderr、fetcher 未被调用。
- **验证证据**：Red 阶段实际运行 `go test ./internal/cli/commands/pixiv/detail -count=1 -run 'TestCommandRejectsContentForNonNovelRecordBeforeFetching' -v`，在修复前两项均以 `got <nil>` 失败；Green 阶段该 focused test 与 TextValue 回归通过。随后 `go test ./internal/cli/commands/pixiv/detail -count=1`、`go test ./internal/cli/pipeline ./internal/shared/record -count=1`、`gofmt`、`git diff --check` 通过，detail 生产/测试文件 LSP diagnostics 为空。
- **剩余风险**：normal/reverse search composition 测试、双语文档同步、全仓库门禁和 finding-first review 尚未执行。
- **下一步建议**：Task 7。

---

## Phase 3: 验证 normal search / reverse search / downstream composition（C4）

### Task 7: Composition contract tests — 普通搜索三类实体

- [x] **目标**：用 search 当前真实 canonical record schema 验证 artwork / novel / user records 能直接进入 detail。
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
- **实际做了什么**：在 detail owner 组合测试中直接调用 search listing 使用的 `RecordFromArtworkDTO`、`RecordFromNovelDTO`、`RecordFromUserPreviewDTO`，生成两条 artwork、novel、user preview canonical records；为每条输入补充 filter/sort/page/limit/next_cursor 元数据和未知 DTO 字段，再通过 `detail --ndjson` 消费。
- **验证证据**：
  - 按任务书规则执行 TDD：新增组合测试已直接 PASS，说明 Phase 1/2 实现已经覆盖该路径，因此没有制造无意义的 Red。
  - `go test ./internal/cli/commands/pixiv/detail -count=1 -run '^TestCommandConsumesNormalSearchCanonicalRecordsInOrder$' -v`：artwork search、novel search、user search preview 三个子测试均 PASS。
  - `go test ./internal/cli/commands/pixiv/detail -count=1`：PASS。
  - `gofmt -d internal/cli/commands/pixiv/detail/detail_test.go` 无输出；`git diff --check` PASS；修改后 LSP diagnostics 为空。
- **剩余风险**：本任务使用 deterministic search-record fixture，未启动真实 Pixiv 网络；reverse-search、下游 consumer 和真实命令组合留待 Task 8。
- **下一步建议**：Task 8。

---

### Task 8: Composition contract tests — reverse image search 与继续管道

- [x] **目标**：锁定用户最关心的 reverse-search pipeline 和 detail 后继续消费能力。
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
- **实际做了什么**：在 detail owner 组合测试中调用真实 `search.New` 生成 reverse `artwork,user,artwork` NDJSON，再调用真实 `detail.New` 按每条 record 独立 inference，验证输出顺序为 `illust,user,illust`；另测显式 `--type artwork` 对 reverse user record 的 mismatch 在打开 client 前失败。通过真实 search `--json` aggregate 输出喂给 detail，确认不会自动拆 `records`；将 detail NDJSON 喂给真实 `download.New`，确认现有 visual record consumer 能读取 `type`/`id` 并收到 artwork ID。search owner 另有 binary stdin 边界测试，确认不会进入 reverse-search seam。未修改生产代码、reverse source loader 或 provider。
- **验证证据**：
  - 按任务书规则执行 TDD：新增组合测试在现有实现上直接 PASS；第一次下游测试暴露的是测试 fake 传入 nil SDK client，而 downloader 的既有 seam 明确拒绝 nil client，随后仅修正测试 fixture 为非 nil `&pixiv.Client{}`，无生产修复。
  - `go test ./internal/cli/commands/pixiv/detail -count=1 -run '^(TestCommandConsumesReverseSearchIdentityRecordsInOrder|TestCommandRejectsReverseUserRecordForArtworkConstraintBeforeFetching|TestCommandDoesNotSplitReverseSearchAggregateJSONIntoRecords|TestDetailNDJSONFeedsExistingVisualDownloadConsumer)$' -v`：4 个测试 PASS（其中 mismatch/aggregate 测试输出预期的 structured pipeline error）。
  - `go test ./internal/cli/commands/pixiv/search -count=1 -run '^TestCommandDoesNotTreatBinaryStdinAsReverseSearchSource$' -v`：PASS。
  - `go test ./internal/cli/commands/pixiv/detail ./internal/cli/commands/pixiv/search ./internal/cli/commands/pixiv/download -count=1`：三个相关包均 PASS。
  - `gofmt -d internal/cli/commands/pixiv/detail/detail_test.go internal/cli/commands/pixiv/search/search_test.go` 无输出；`git diff --check` PASS；两份修改文件的 LSP diagnostics 均为空；变更影响面仅为测试符号。
- **剩余风险**：仍未启动真实 Pixiv/reverse provider 网络；下游证明使用既有 download manager 注入 seam 与一个 illust fixture，不覆盖真实文件写入。binary image stdin 仍是明确 non-goal。
- **下一步建议**：Task 9。

---

### Task 9: 文档与产品 skill 同步

- [x] **目标**：只在代码 contract 已稳定并由测试证明后更新用户文档。
- **更新位置**：
  - `docs/en/cli-reference.md`
  - `docs/zh-CN/cli-reference.md`
  - `skills/pixiv-cli/SKILL.md`
  - `skills/pixiv-cli/references/discover.md`（沿用既有 discovery 章节，修正旧的 ID 提取式 guidance）
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
  - `git diff --check`：通过。
  - `go test ./scripts/tests/documentation -count=1`：通过；未新增脆弱全文字符串 scanner。
  - 另做了目标 contract 片段的双语/产品 skill presence check：通过。
- **验收**：英文/简中 contract 一致，产品 skill 及其 discovery reference 不再把 detail 描述为仅单值 consumer。
- **实际做了什么**：以英文 CLI reference 为 canonical baseline，同步简体中文 CLI reference、产品 `SKILL.md` 与其 `references/discover.md`；补充自动/显式 NDJSON search → detail 示例、三种 normal search record inference、显式 `--type` compatibility constraint、reverse identity record 直连 detail、record-mode `--ndjson`/`--json` 输出语义，并明确 `search --json` 聚合文档和 binary image stdin 都不属于支持 contract。
- **验证证据**：文档测试通过；`git diff --check` 通过；Task 8 已提供 search/detail/downstream 与 reverse/error 行为测试证据，本 Task 9 未改生产代码。
- **剩余风险**：未执行真实网络反搜或真实账号命令；本次风险限于文档措辞与示例，实际 runtime contract 仍以 Task 8 已验证实现为准。
- **下一步建议**：集中检查 #3。

---

### 🔍 集中检查 #3（Task 7–9）

- [x] **目标**：对照用户目标重新检查是否真正实现“搜索/反搜 → detail”，而不是只让 unit test 看起来能解析 record。
- **检查清单**：
  - [x] artwork / novel / user normal search records 均有组合证据。
  - [x] reverse `artwork` / `user` records 均有组合证据。
  - [x] mixed reverse stream 每条独立 inference。
  - [x] 至少一个 visual `search → detail → downstream record consumer` 有证据。
  - [x] search 普通过滤/分页参数不改变协议。
  - [x] aggregate `--json`、trending tags、binary image stdin 等 non-goal 没被暗中扩展。
  - [x] 双语文档和产品 skill 与实际行为一致。
- **结论**：通过。Task 7–9 的组合、边界和文档证据一致，没有发现需要回退或修复的问题。
- **发现问题**：无。Task 7 的 normal-record fixture 保留 filter/sort/page/limit/next_cursor 与未知 DTO 字段；Task 8 的 mixed reverse、显式类型 mismatch、aggregate JSON、下游 download 和 binary stdin 测试均通过。现有 search 测试继续拒绝 image source 搭配 `--trending-tags`，本 goal diff 没有修改 search provider、source loader、SDK、配置 schema 或 trending-tags 生产路径；Task 9 的双语 CLI reference、产品 skill 和 discovery reference 与 runtime contract 对齐。
- **验证证据**：
  - `go test ./internal/cli/commands/pixiv/detail -count=1 -run '^(TestCommandConsumesNormalSearchCanonicalRecordsInOrder|TestCommandConsumesReverseSearchIdentityRecordsInOrder|TestCommandRejectsReverseUserRecordForArtworkConstraintBeforeFetching|TestCommandDoesNotSplitReverseSearchAggregateJSONIntoRecords|TestDetailNDJSONFeedsExistingVisualDownloadConsumer)$' -v`：PASS。
  - `go test ./internal/cli/commands/pixiv/search -count=1 -run '^TestCommandDoesNotTreatBinaryStdinAsReverseSearchSource$' -v`：PASS。
  - `go test ./internal/cli/commands/pixiv/detail ./internal/cli/commands/pixiv/search ./internal/cli/commands/pixiv/download -count=1`：PASS。
  - `go test ./scripts/tests/documentation -count=1`：PASS；`git diff --check`：PASS；detail/search 受影响 Go 文件的 LSP diagnostics 为空。
- **剩余风险**：尚未执行 Task 10 的仓库级全量门禁、构建和 Task 11 finding-first review；真实 Pixiv/reverse provider 网络仍未运行。上述风险不影响本轮对 Task 7–9 deterministic contract 的结论。

---

## Phase 4: 全量验证与 finding-first 审查（C5, C6）

### Task 10: 相关回归 + 全量质量门禁

- [x] **目标**：从最小相关检查扩展到仓库级验证；不拿全量门禁代替前面的行为测试。
- **验证顺序**：
  1. `go test ./internal/cli/commands/pixiv/detail -count=1`
  2. `go test ./internal/cli/pipeline ./internal/shared/record -count=1`
  3. `go test ./internal/cli/... -count=1`
  4. `go test ./... -count=1`
  5. `sh scripts/build.sh`
  6. `git diff --check`
- **失败处理**：先区分本次变更、既有失败、环境失败；只修本 goal 引入/暴露且属于范围的问题。没有代码/环境变化时不重复跑同一个失败检查。
- **验收**：相关与全量门禁绿色，或对无法运行的客观环境项有明确记录。
- **实际做了什么**：按任务书顺序执行 detail、pipeline/record、`internal/cli/...`、仓库全量 Go 测试、构建脚本和 diff 门禁；所有检查均通过，未修改生产代码或测试以迎合门禁。
- **验证证据**：
  - `go test ./internal/cli/commands/pixiv/detail -count=1`：PASS。
  - `go test ./internal/cli/pipeline ./internal/shared/record -count=1`：PASS。
  - `go test ./internal/cli/... -count=1`：PASS。
  - `go test ./... -count=1`：PASS，包含 e2e（真实 SDK 默认跳过）。
  - `sh scripts/build.sh`：PASS，生成 Darwin arm64 `build/pixiv`；构建产物未进入 Git 变更。
  - `git diff --check`：PASS；提交前后工作区均保持 clean。
- **剩余风险**：未运行需要本机真实凭据的 Pixiv/FANBOX SDK E2E；该项是仓库既有的显式 opt-in 测试，不属于本 goal 的 deterministic pipeline 门禁。未执行 Task 11 finding-first review。
- **下一步建议**：Task 11。

---

### Task 11: Finding-first code review

- [x] **目标**：按 `.agents/skills/pixiv-cli-review/SKILL.md` 对本 goal commit range 做只读审查，先 findings 后 summary。
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
- **实际做了什么**：对 `origin/feature/search-detail-pipeline...HEAD`（10 个 goal commits）执行 finding-first 只读审查；按边界、行为/错误、CLI contract、文档、测试布局与依赖范围逐项检查。确认 `detail` 仅经 `internal/cli/pipeline`、`internal/shared/record` 与 public `sdk/pixiv` 工作，单值 detail 兼容路径保留，record type 推断与显式 `--type` 约束一致，aggregate `search --json` 不误当 record stream，I/O 失败不被静默吞掉，且没有新增 timeout/截断/重试/隐式 fallback。审查结果采用 finding-first 结论：未发现 P0/P1/P2/P3 问题；没有生产修复需要追加。
- **验证证据**：
  - `go test ./internal/cli/commands/pixiv/detail ./internal/cli/commands/pixiv/search ./internal/shared/record -count=1`：PASS。
  - `go vet ./internal/cli/commands/pixiv/detail ./internal/cli/commands/pixiv/search ./internal/shared/record`：PASS。
  - LSP 诊断：`detail.go`、`detail_test.go` 无诊断；`search_test.go:365` 的 `forvar` warning 已由 base commit 引入，非本 goal 变更。
  - Task 10 已验证 `go test ./... -count=1`、`sh scripts/build.sh` 与 `git diff --check` 均 PASS；提交前后工作区保持 clean。
  - `docs/zh-CN/maintainers/{architecture,development}.md` 的边界与 canonical test-layout 规则、双语 CLI reference、`skills/pixiv-cli` 文档均已对照；改动未触及 `go.mod`/`go.sum`、public SDK、config、reverse provider 或 search 生产实现。
- **剩余风险**：未运行需要本机真实凭据的 Pixiv/FANBOX SDK E2E，也未在真实跨平台 TTY/非 TTY 环境做 smoke；两者均为显式环境依赖的补充风险，不构成本次 review finding。`search_test.go:365` 的 `forvar` 是既有 warning，留待独立清理，未在本 task 扩大范围。
- **下一步建议**：Task 12。

---

### Task 12: Goal completion audit

- [x] **目标**：逐项对照 `goal-1/input.md` 的 C1–C6 和本任务书证据，确认没有“实现了 parser 但没实现 UX”的假完成。
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
- **实际做了什么**：对当前完整 goal 范围 `origin/main...HEAD` 建立 C1–C6 与命令语义验收矩阵，并复核 Task 11 的 finding-first review 与集中检查 #1 对初始生产提交的补充审查。确认输入、实体、输出、组合、错误和文档回归均有对应实现与 deterministic test，不存在只接 parser 而未接 UX 的半成品。
  - **C1 Input contract**：`internal/cli/commands/pixiv/detail/detail.go:119-128` 绑定 `pipeline.TextOrRecord`，`:130-205` 区分 TextValue/RecordMode 并按顺序消费 `pipeline.ConsumeNDJSONRecords`；`detail_test.go:294-424` 覆盖 canonical artwork、普通 search 的 artwork/novel/user records 及保留 search metadata；`:464-514` 覆盖单值 ID、`--json` 单 document、`--ndjson` record 输出。
  - **C2 Entity semantics**：`detail.go:418-446` 集中维护 `artwork/illust/manga/ugoira -> artwork`、`novel -> novel`、`user -> user` 与显式 `--type` compatibility constraint；`detail_test.go:452-462`、`:578-675` 覆盖冲突、四种 artwork alias 和 unsupported type；`:677-752` 与 `internal/shared/record/pixiv_test.go:13-122` 覆盖 novel content、novel/user canonical identity 和 artwork kind normalization。
  - **C3 Output contract**：`detail.go:130-137` 实施 `--json`/`--ndjson` 互斥，`:176-267` 用逐条写入和 `jsonArraySpool` 生成 record-mode JSON array，不收集完整输入流；`detail_test.go:426-450`、`:516-576` 覆盖 array、TTY separator、非 TTY 自动 NDJSON 和显式 `--ndjson` precedence。
  - **C4 Composition**：`detail_test.go:20-48` 覆盖 reverse-search artwork/user identity 顺序消费与 detail 后的具体 artwork type，`:107-152` 覆盖 detail NDJSON 进入既有 download consumer，`:322-391` 覆盖普通 artwork/novel/user search records；`internal/cli/commands/pixiv/search/search_test.go:293-328,503-533` 保留 reverse NDJSON 与非终端自动 NDJSON 证据；`docs/en/cli-reference.md:367-387` 和 `skills/pixiv-cli/references/discover.md:138-173` 对应记录了六类命令语义。
  - **C5 Error contract**：`detail.go:75-90,308-390` 对 pooled/fetch/record projection 错误直接返回；`detail_test.go:50-105,192-289,442-462,645-675,754-794` 覆盖显式类型冲突、aggregate JSON 拒绝、unsupported URL/type、非 novel content、stderr structured diagnostic 与原始 output write error；错误路径没有静默跳过、伪造成功或回退到 TextValue。
  - **C6 Documentation & regression**：双语 CLI reference、产品 skill 与 discover reference 已同步 pipeline、类型推断、输出和 reverse contract；本轮实际执行的相关测试、全量测试、构建、vet、LSP diagnostics 和 diff 检查均通过。
- **验证证据**：
  - `go test ./internal/cli/commands/pixiv/detail -count=1 -v`：PASS，包含全部 detail pipeline、TTY/非 TTY、array、alias、novel content、error/I/O cases。
  - `go test ./internal/cli ./internal/cli/commands/pixiv/search ./internal/shared/record -count=1`：PASS。
  - `go vet ./internal/cli ./internal/cli/commands/pixiv/detail ./internal/cli/commands/pixiv/search ./internal/shared/record`：PASS。
  - `go test ./... -count=1`：PASS；真实 Pixiv/FANBOX SDK E2E 仍按仓库约定默认不启用。
  - `sh scripts/build.sh`：PASS，生成 Darwin arm64 `build/pixiv`；构建产物未进入 Git 变更。
  - LSP diagnostics：`detail.go`、`root.go`、`internal/shared/record/pixiv.go`、`detail_test.go` 均为空；`git diff --check origin/main...HEAD`：PASS；构建后 `git status --short` 保持 clean。
  - 范围复核：`git diff --name-only origin/main...HEAD` 的 production Go 改动仅为 `detail.go`、`internal/cli/root.go`、`internal/shared/record/pixiv.go`；其余为聚焦测试、双语文档/skill 和 goal ledger。没有 `sdk/`、`internal/services/reversesearch/`、binary source loader、`internal/config/`、`go.mod` 或 `go.sum` 的 production 变更，也没有新增第三方依赖。
  - review 证据：Task 11 finding-first review 未发现 P0/P1/P2/P3；集中检查 #1 已把审查范围扩展到初始 `8506202` 生产提交，并修复 `commandAutoWritesNDJSON` 对 input annotation 的真实回归，随后相关测试、vet、LSP 和 diff 检查均为 PASS。
- **剩余风险**：未运行需要本机真实凭据的 Pixiv/FANBOX SDK E2E，也未在真实跨平台 TTY/非 TTY 与真实 reverse provider 网络上执行 smoke；这些是环境依赖的补充验证，不影响 deterministic contract 结论。`internal/cli/commands/pixiv/search/search_test.go:365` 的 `forvar` warning 为 base commit 既有问题，未归属于本 goal，也未扩大范围修复。
- **下一步建议**：进入“最终集中检查（Task 10–12）”，再次核对 C1–C6、先失败测试证据、范围和 review 结论；完成后才可把 goal 标记为 complete。

---

### 🔍 最终集中检查（Task 10–12）

- [x] **目标**：最终交付前复查。
- [x] **检查清单**：
  - [x] C1 Input contract 有证据。
  - [x] C2 Entity semantics 有证据。
  - [x] C3 Output contract 有证据。
  - [x] C4 Search/reverse composition 有证据。
  - [x] C5 Error contract 有证据。
  - [x] C6 Docs/regression 有证据。
  - [x] 无 public SDK / provider / config scope creep。
  - [x] 无新增第三方依赖。
  - [x] 全部 production code 变更有先失败测试证据，或属于已有 contract 覆盖的 composition-only 变更。
  - [x] finding-first review 无阻塞问题。
- **实际做了什么**：对完整 goal 范围 `origin/main...HEAD` 重新执行最终验收矩阵，覆盖初始生产提交 `8506202`、后续实现、测试、文档和 ledger；同时复核 Task 1–12 的 Red/Green 记录、架构边界、依赖变化和 review 结论。确认 detail 已完成 canonical `TextOrRecord` 输入、实体类型推断/显式类型约束、TTY/非 TTY/`--json`/`--ndjson` 输出、novel content 投影、normal/reverse search 组合与既有 downstream consumer 连接。
  - **C1 Input contract**：`internal/cli/commands/pixiv/detail/detail.go` 使用 `pipeline.TextOrRecord`，RecordMode 通过 `pipeline.ConsumeNDJSONRecords` 顺序消费；detail tests 覆盖 canonical artwork、normal search 的 artwork/novel/user、reverse identity、单值 ID、保留 search metadata 与 TextValue 兼容。raw ID/URL 解析和单值 stdin 路径由 `TestCommandTextValueAcceptsRawURLAndStdin` 与既有 TextValue 回归测试保护。
  - **C2 Entity semantics**：`entityForRecord` 集中维护 `artwork/illust/manga/ugoira -> artwork`、`novel -> novel`、`user -> user`，显式 `--type` 作为 compatibility constraint；测试覆盖四种 artwork alias、novel/user、冲突和 unsupported type，并验证 detail 输出重新归一化为实际实体类型。
  - **C3 Output contract**：`--json` 与 `--ndjson` 互斥；RecordMode TTY 使用人类可读详情和稳定 `---` 分隔，非 TTY 自动 NDJSON，显式 `--ndjson` 优先；RecordMode `--json` 通过临时文件 spool 逐条生成完整 JSON array，没有先收集完整输入流。TextValue 默认输出与 `--json` 单 document 语义保持不变。
  - **C4 Composition**：normal search、reverse search 和 detail 的顺序消费、具体 artwork type、novel/user 投影以及 `detail --ndjson` 到既有 download consumer 的测试均通过；reverse provider、proxy、image-source 和 downstream output contract 未被改动。
  - **C5 Error contract**：malformed record、aggregate JSON、unknown/unsupported type、显式类型 mismatch、非 novel `--content`、stdout/stderr 边界、远端 detail error 和原始 output write error 均有非零失败/诊断测试；错误直接保留真实原因，无静默跳过、伪造成功、隐式 fallback 或把诊断写入 stdout。最终审计发现原 C5 证据缺少直接的远端详情失败测试，已新增 `TestCommandPreservesRemoteDetailError`，仅补测试、不改变生产行为。
  - **C6 Documentation & regression**：双语 CLI reference、`skills/pixiv-cli` 及 discover reference 已同步 pipeline、类型推断、输出、reverse contract 和 aggregate JSON 非目标；相关测试、文档测试、全量测试、构建、vet、LSP diagnostics 和 diff 检查均有实际通过记录。
  - **TDD/范围证据**：Task 1 记录真实 Red（record type normalization）；Task 4/5 记录输出 separator 和 I/O error 的真实 Red→Green；Task 6 记录 novel content projection 的 Red→Green；集中检查 #1 记录 `commandAutoWritesNDJSON` annotation 回归的真实 Red→Green。composition-only 检查按任务规则允许在已有实现覆盖时直接 PASS，未为制造 Red 而篡改测试或实现。
- **验证证据**：
  - `go test ./internal/cli/commands/pixiv/detail -count=1 -v`：PASS，包含 detail pipeline、TTY/非 TTY、JSON array、alias、novel content、malformed/remote/I/O error cases。
  - `go test ./internal/cli ./internal/cli/commands/pixiv/search ./internal/shared/record -count=1`：PASS。
  - `go test ./scripts/tests/documentation -count=1`：PASS。
  - `go vet ./internal/cli ./internal/cli/commands/pixiv/detail ./internal/cli/commands/pixiv/search ./internal/shared/record`：PASS。
  - `go test ./... -count=1`：PASS；仓库默认跳过需本机凭据的真实 Pixiv/FANBOX SDK E2E。
  - `sh scripts/build.sh`：PASS，生成 Darwin arm64 `build/pixiv`；构建产物未进入 Git 变更。
  - LSP diagnostics：`detail.go`、`root.go`、`internal/shared/record/pixiv.go`、`detail_test.go` 均为空；当前工作区与提交后 `git diff --check origin/main...HEAD` 均通过。
  - 范围复核：`git diff --name-only origin/main...HEAD` 的 production Go 改动仅为 `internal/cli/commands/pixiv/detail/detail.go`、`internal/cli/root.go`、`internal/shared/record/pixiv.go`；没有 `sdk/`、`internal/services/reversesearch/`、binary source loader、`internal/config/`、`go.mod` 或 `go.sum` 的 production 变更，也没有新增第三方依赖。
  - review 证据：Task 11 finding-first review 未发现 P0/P1/P2/P3；集中检查 #1 已将审查范围扩展到初始 `8506202` 生产提交并修复 annotation 回归，后续相关测试、vet、LSP 和 diff 检查均通过。
- **结论**：通过。goal-1 的 C1–C6、测试先行、范围边界和 review 门禁均满足；最终 ledger 已提交，提交后构建、范围 diff、LSP diagnostics、无未完成 ledger marker 与 clean-worktree 复核均通过，可以将 goal 标记为 complete。
- **发现问题**：C5 的 Evidence Required 原先没有直接覆盖远端详情 SDK 错误；已由 `TestCommandPreservesRemoteDetailError` 补齐，测试与全量回归均通过。未发现生产逻辑阻塞问题。
- **剩余风险**：未运行需要本机真实凭据的 Pixiv/FANBOX SDK E2E，也未在真实跨平台 TTY/非 TTY 与真实 reverse provider 网络上执行 smoke；这些是环境依赖的补充风险，不影响 deterministic contract 结论。`internal/cli/commands/pixiv/search/search_test.go:365` 的 `forvar` warning 为 base commit 既有问题，未归属于本 goal。
- **下一步建议**：goal-1 已完成；无后续 task。
