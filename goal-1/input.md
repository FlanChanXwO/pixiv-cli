# Goal Input — Canonical `pixiv search | pixiv detail` pipelines

## 原始需求

把现有 canonical CLI pipeline 能力完整接入 `pixiv detail`，使搜索结果能够直接进入详情查询，并覆盖普通作品/小说/用户搜索、带过滤/分页参数的搜索，以及反向搜图返回的 Pixiv identity records。

核心体验应成立：

```bash
pixiv search "miku" --type artwork --limit 20 | pixiv detail
pixiv search "novel" --type novel --limit 10 | pixiv detail
pixiv search "artist" --type user --limit 10 | pixiv detail
pixiv search ./image.png --provider all | pixiv detail
```

并保留后续继续组合的能力，例如视觉作品流：

```bash
pixiv search "landscape" --type artwork --limit 20 \
  | pixiv detail --ndjson \
  | pixiv download --on-error fail-fast
```

## 当前事实

- `pixiv search` 的实体列表在 stdout 为非 TTY 且未显式选择完整 JSON document 时，已经会输出 canonical NDJSON records。
- canonical record 固定顶层 `id`、`type`、`url`，并保留对应 DTO 字段；普通 artwork 搜索的 `type` 可能是 `illust`、`manga`、`ugoira`，novel 为 `novel`，user 为 `user`。
- reverse image search 的 NDJSON 只输出可确定 Pixiv identity 的结果，类型为通用 `artwork` 或 `user`。
- `download`、`bookmark`、`follow` 已使用 `pipeline.TextOrRecord` / record consumer；`detail` 仍使用 `pipeline.TextValue`，把整个 stdin 当成一个 ID/URL，因此无法直接消费 search 的 NDJSON。
- `detail` 当前单值输入支持 ID 或 Pixiv URL，支持 `--type artwork|novel|user`、`--content`（novel）、`--json`、`--proxy` / `--no-proxy`；这些既有单值语义不得回归。

## 核心要求

1. **Canonical record input**：`pixiv detail` 接入现有 `TextOrRecord` 协议，能够逐条消费 canonical NDJSON；不得引入第二套 record schema。
2. **类型推断**：record mode 未显式传 `--type` 时，根据 record 的 `type` 自动选择 detail 路径：
   - `artwork` / `illust` / `manga` / `ugoira` → artwork detail
   - `novel` → novel detail
   - `user` → user detail
3. **显式类型约束**：显式 `--type` 不得覆盖或伪造 record 类型；它作为输入约束验证兼容性。`--type artwork` 允许 `artwork` / `illust` / `manga` / `ugoira`，其他类型必须显式报错。
4. **普通单值兼容**：显式 ID/URL，以及 `printf 'ID\n' | pixiv detail` 这类 TextValue 路径保持现有行为和输出语义。
5. **多 record 顺序处理**：record mode 按输入顺序处理，不缓存整个 NDJSON 输入，不添加长度、记录数或分页硬上限。
6. **输出可组合**：
   - TextValue mode 默认输出保持不变；`--json` 仍输出单个现有 DTO document。
   - RecordMode + TTY stdout 默认输出可读的人类详情，多条之间有稳定且不含业务歧义的分隔。
   - RecordMode + 非 TTY stdout 默认输出 canonical NDJSON detail records，使 `search | detail | compatible-consumer` 成立。
   - 新增 `--ndjson` 作为显式 record 输出开关；`--json` 与 `--ndjson` 互斥。
   - RecordMode + `--json` 输出一个完整 JSON array，元素为 canonical detail records；不得把整条输入流收集到内存后再一次性编码。
7. **详情后的 record 仍规范化**：artwork detail 输出真实 artwork kind（`illust` / `manga` / `ugoira`），reverse search 的通用 `artwork` identity 在 detail 后被规范化为具体类型；novel/user 保持 `novel` / `user`。
8. **Novel content 可组合**：record mode 的 novel + `--content` 必须可处理，并为 detail content 定义可验证的 canonical record 投影，而不是退回不带 `id/type/url` 的任意 JSON。
9. **错误显式可诊断**：畸形 record、不支持的 type、显式 `--type` 冲突、远端 detail 失败都必须返回非零并保留真实原因；不得静默跳过、伪造成功或自动降级为 TextValue。
10. **Search 参数兼容边界**：所有仍产生实体 canonical records 的普通 search 参数（过滤、排序、分页、类型等）不应影响管道协议；reverse search 的 provider/proxy/output contract 保持原样。

## 头脑风暴后的设计决策

### 选择：让 `detail` 成为 canonical record transformer

不采用外部 `jq` 抽 ID 作为正式功能，也不在 `search` 中增加 detail 特例。`detail` 自己拥有输入解析、类型解析、详情调用和 presenter，因此应直接消费现有 record contract。

### 选择：只支持 canonical NDJSON 作为命令间流协议

显式 `pixiv search --json` 产生的是完整终端 JSON document，不是 canonical record stream。本 goal 不把 aggregate JSON 自动猜测/拆解成 records，也不扩展 `TextOrRecord` 为“任意 JSON 猜格式”解析器。

因此：

```bash
pixiv search "miku" | pixiv detail                 # 支持，search 自动 NDJSON
pixiv search "miku" --ndjson | pixiv detail        # 支持，显式 NDJSON
pixiv search "miku" --json | pixiv detail          # 不属于支持的 pipeline contract
```

### 选择：record mode 才启用自动 NDJSON 输出

不能因为 `pixiv detail 123 | cat` 的 stdout 是 pipe 就改变既有单值 detail 的默认文本输出。自动 NDJSON 只在输入已经被分类为 RecordMode 时启用；TextValue mode 保持兼容。

## Scope

- `internal/cli/commands/pixiv/detail` 的 input/output contract、类型解析和 record-mode 执行。
- 必要时扩展 `internal/shared/record` 的显式 DTO → canonical record 转换，例如 novel content。
- 复用 `internal/cli/pipeline` 已有 record 解析/诊断能力；只有存在真实缺口时才做最小共享扩展。
- `detail` 聚焦测试、pipeline contract 测试和必要的 root/组合测试。
- 双语 CLI reference 及 `skills/pixiv-cli/` 的命令/管道语义同步。

## Non-goals

- 不改变 Pixiv public SDK API 或 endpoint 实现。
- 不改变 reverse-search provider、排序、匹配策略、SauceNAO/ascii2d transport 或配置。
- 不支持 `cat image.png | pixiv search` 这种二进制图片 stdin；reverse search source 仍是本地 regular file path 或 HTTP(S) URL。
- 不让 `detail` 消费 `search --json` 的完整 aggregate document。
- 不让 `--trending-tags` 等非实体输出进入 detail；只有 canonical entity records 才属于该管道。
- 不设计通用 shell expression/filter DSL，不新增 `jq` 依赖，也不新增第三方 Go 依赖。
- 不把所有 CLI 命令统一改造成 transformer；本 goal 只补齐 detail 的实际缺口。
- 不为长管道添加固定 timeout、记录数上限、长度截断、重试次数或静默 fallback。
- 不预写版本化 changelog；按仓库正常 PR/release policy 处理用户可见变更来源。

## Completion Criteria

- **C1 — Input contract**：`detail` 能同时正确处理单个 raw ID/URL 和 canonical NDJSON，且 TextValue 既有行为无回归。
- **C2 — Entity semantics**：artwork aliases、novel、user、reverse-search `artwork` identity 均能正确推断；显式 `--type` 冲突可诊断；`--content` 只作用于 novel。
- **C3 — Output contract**：record input 在 TTY 下可读、非 TTY 下自动 canonical NDJSON；`--ndjson` 可显式选择 record 输出；`--json` 在 record mode 输出完整 JSON array；single-value `--json` 保持原格式。
- **C4 — Composition**：普通 artwork/novel/user search 和 reverse image search 的 canonical records 都能直接进入 detail；visual detail records 可继续进入现有 compatible consumers。
- **C5 — Error contract**：malformed/unsupported/mismatched record 与远端 detail 错误均显式失败，stderr/stdout 边界不被日志或诊断污染。
- **C6 — Documentation & regression**：CLI reference、产品 skill 与聚焦测试同步；相关包测试、全量 Go 测试、构建和 diff 检查通过。

## Evidence Required

| Criterion | Verification / evidence |
| --- | --- |
| C1 | `go test ./internal/cli/commands/pixiv/detail ./internal/cli/pipeline -count=1`，包含先失败的 Red 证据 |
| C2 | detail 聚焦类型矩阵测试：`artwork/illust/manga/ugoira/novel/user`、显式 `--type` match/mismatch、novel content |
| C3 | detail output 聚焦测试：TextValue compatibility、RecordMode TTY、auto-NDJSON、`--ndjson`、record-mode `--json`、互斥 flag |
| C4 | 组合测试使用正常 search record 与 reverse identity record fixture，验证 `search-output → detail`；至少一个 visual `detail → record consumer` 组合测试 |
| C5 | malformed record、unknown type、远端 SDK error、stdout/stderr I/O error 测试；不得以空结果冒充成功 |
| C6 | `go test ./...`、`sh scripts/build.sh`、`git diff --check`，以及仓库 review checklist |

## 执行约束

- 所有生产代码变更必须先写并实际运行会失败的聚焦测试，再进入 Green；不能定义 Red 时必须停止该实现路径并说明阻塞。
- 每个阶段只做达到当前 contract 所需的最小实现；Refactor 只处理已证实的重复/复杂度。
- 不预设 Goal Mode 迭代次数或墙钟时间上限；遵循当前会话/host 的客观限制，不把执行控制转成产品运行时限制。
