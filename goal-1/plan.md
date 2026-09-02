# Plan — Canonical `pixiv search | pixiv detail` pipelines

## 1. 需求概述

让 `pixiv detail` 从当前“单 ID/URL 查询命令”扩展为兼容现有 canonical record pipeline 的 read transformer，同时完整保留既有单值调用方式。

目标不是重新设计 Unix pipeline，而是补齐已经存在的 producer/consumer 体系中唯一明显缺口：`search` 已能自动产出 canonical NDJSON，`download` / `bookmark` / `follow` 已能消费 records，但 `detail` 仍把 stdin 整体当成一个 TextValue。

最终核心链路：

```text
normal search ─┐
novel search  ─┤
user search   ─┼─ canonical NDJSON ─> detail ─> human / JSON / canonical NDJSON
reverse search ┘                                  │
                                                  └─> compatible downstream consumer
```

## 2. 仓库现状与边界

### 2.1 已有 pipeline contract

`internal/cli/pipeline` 已拥有：

- `TextValue`：完整 stdin → 一个位置参数；
- `TextOrRecord`：首个非 JSON whitespace 为 `{` 时进入严格 record mode，否则为 raw text value；
- replayable record reader；
- canonical NDJSON 的逐行消费与稳定 diagnostics；
- 不使用 `bufio.Scanner`，因此不会引入隐藏 token/line size 限制。

本 goal 必须复用这些能力，不另建 `detail` 私有 stdin 协议。

### 2.2 已有 record contract

`internal/shared/record` 已将 Pixiv DTO 映射为稳定 record：顶层固定 `id`、`type`、`url`，其余 DTO 字段保留。

现有类型来源不完全一致：

| Producer | 可能的 record type |
| --- | --- |
| artwork search/list | `illust`, `manga`, `ugoira` |
| novel search/list | `novel` |
| user search/list | `user` |
| reverse image search | `artwork`, `user` |

因此 detail 的 entity resolver 不能只做 `record.Type() == opts.type`；必须有明确的 artwork alias compatibility。

### 2.3 `detail` 当前 contract

`internal/cli/commands/pixiv/detail/detail.go` 当前：

- 使用 `pipeline.TextValue`；
- 精确要求一个 `ID_OR_URL`；
- 默认 `--type artwork`；
- `--content` 只允许 novel；
- `--json` 输出单个 DTO document；
- human presenter 已分别存在 artwork / novel / novel content / user 路径。

这些单值行为是兼容性基线，不因新增 record mode 而改变。

### 2.4 Search / reverse-search boundary

`pixiv search` 已在 stdout 非 TTY 时自动选择 canonical NDJSON（除非显式选择完整 `--json` document）；反向搜图同样能输出 Pixiv identity records。

因此本 goal **原则上不修改 search producer**。只有组合测试暴露真实 producer contract 缺口时，才允许做最小修复。

## 3. 头脑风暴与方案取舍

### 3.1 输入方案

| 方案 | 优点 | 问题 | 决策 |
| --- | --- | --- | --- |
| A. 文档要求 `jq -r .id` 再喂 detail | 无代码改动 | 丢失 type，reverse mixed records 不自然，不能称为原生管道 | 拒绝 |
| B. `detail` 改为 `TextOrRecord` consumer | 复用现有协议；保留 raw ID/URL；支持流式 records | 需要定义 type/output/error contract | **采用** |
| C. 新建“任意 JSON 自动探测”通用 codec | 看似万能 | aggregate JSON/NDJSON/业务 JSON 语义混杂，扩大影响面 | 拒绝 |

### 3.2 类型决策

采用“**record 自描述 + 显式 flag 约束**”：

1. TextValue mode：保持现有默认 `artwork`，继续由 `--type` 选择解析目标。
2. RecordMode 且用户**未显式**传 `--type`：从每条 record 推断 entity。
3. RecordMode 且用户显式传 `--type`：验证 record 与该 entity 兼容，而不是覆盖 record。
4. artwork compatibility set：`artwork`, `illust`, `manga`, `ugoira`。
5. unknown/empty type 必须失败，不能用 URL 或默认 artwork 偷偷补猜。

这使 reverse image search 的 generic `artwork` identity 能被 detail 接受，同时普通 search 的具体 artwork kind 也保持精确信息。

### 3.3 输出方案

| 输入模式 | 默认 TTY stdout | 默认非 TTY stdout | `--json` | `--ndjson` |
| --- | --- | --- | --- | --- |
| TextValue | **保持现有 human 输出** | **保持现有 human 输出** | 保持现有单 DTO document | 单条 canonical detail record |
| RecordMode | 多条 human detail，条目间 `---` 分隔 | canonical detail NDJSON | canonical detail record JSON array | canonical detail NDJSON |

关键理由：

- 自动 NDJSON **只绑定 RecordMode**，避免 `pixiv detail 123 | cat` 这种既有用法被悄悄改格式。
- `detail --ndjson` 为显式可组合输出，方便单 ID detail 也继续进入 record consumer。
- RecordMode `--json` 必须是一个完整 JSON document，因此使用 array；逐条流式写入，不先收集全部结果到 `[]any`。
- `--json` / `--ndjson` 互斥。

### 3.4 Detail record projection

复用或补齐显式 DTO → record 转换：

- Artwork：`RecordFromArtworkDTO`，输出真实 `illust/manga/ugoira`。
- Novel metadata：`RecordFromNovelDTO`。
- User detail：`RecordFromUserDetailDTO`。
- Novel content：新增最小 `RecordFromNovelContentDTO`（若现有 DTO 能稳定提供 novel ID），固定 `type=novel` 和 canonical novel URL，并保留 structured content DTO 字段。

不通过 reflection、map 猜字段或把 upstream record 原样冒充成“已 detail”。详情输出必须来自实际 detail SDK 结果。

### 3.5 错误策略

本 goal 首版采用**fail-fast** record transform：

- 每条 record 顺序处理；
- malformed record / unsupported type / type mismatch / detail SDK error 立即返回非零；
- 使用现有 pipeline diagnostics 能力时，错误写 stderr，stdout 只保留成功输出；
- 已输出的前序 records 不伪装成整体成功；
- 不新增 `--on-error` 公共 flag，避免为单一 read transformer提前扩展错误策略 surface。

如果未来有真实需求要“跳过失效详情继续处理”，可单独设计与现有 action `--on-error` 对齐的功能；本 goal 不预置。

### 3.6 不支持的“看似管道”输入

以下不属于 canonical pipeline contract：

```bash
pixiv search "miku" --json | pixiv detail
pixiv search --trending-tags | pixiv detail
cat image.png | pixiv search --provider saucenao
```

原因分别是 aggregate JSON document、非实体数据、二进制 image stdin。不能为了让命令“看起来都能接”而引入格式猜测或隐藏转换。

## 4. 预计代码影响面

### 4.1 主要修改

- `internal/cli/commands/pixiv/detail/detail.go`
  - `TextValue` → `TextOrRecord`
  - 增加 `--ndjson`
  - record type → entity resolver
  - raw/record 两条执行路径
  - output mode selection
  - multi-record presenter / canonical record encoder
- `internal/cli/commands/pixiv/detail/*_test.go`
  - 输入、类型、输出、错误和兼容性 Red/Green 测试
- `internal/shared/record/pixiv.go`
  - 仅在 novel content 确实需要时补显式 canonical conversion
- `internal/shared/record/*_test.go`
  - 新 record projection 的字段 contract

### 4.2 可能的最小共享修改

- `internal/cli/pipeline/*`
  - 原则上不改 codec；只有测试证明 detail 缺少可复用的“读取一条 record 并执行 read transformer”接口时，才添加最小 helper。
  - 不把 Pixiv entity/type 规则放入 generic pipeline 包。
- `internal/cli/root.go`
  - 仅补齐 detail 所需的 stderr/output capability 注入，不把业务逻辑搬到 composition root。

### 4.3 文档

- `docs/en/cli-reference.md`
- `docs/zh-CN/cli-reference.md`
- `skills/pixiv-cli/SKILL.md`
- 若现有 discover/detail reference 有对应章节，则就地更新；不新建重复文档。

README 只有在其命令示例或高层 feature claim 因此需要变化时才更新，避免无意义扩散。

## 5. 分阶段执行

### Phase 1 — 输入和 entity contract

先用聚焦测试锁定：

- raw ID / raw URL / stdin TextValue 兼容；
- `TextOrRecord` 分类；
- artwork aliases、novel、user、reverse `artwork` identity；
- omitted `--type` inference；
- explicit `--type` compatibility/mismatch；
- `--content` + inferred novel；
- malformed/unknown record fail-fast。

Red 必须实际运行并因当前 detail 不能消费 record 而失败；随后只做达到 Green 的最小 input/entity 实现。

### Phase 2 — detail record output contract

再写 Red 测试锁定：

- TextValue 默认 human 输出不变；
- TextValue `--json` 不变；
- `--ndjson` 输出 canonical single record；
- RecordMode → TTY human multi-detail；
- RecordMode → non-TTY auto NDJSON；
- RecordMode `--json` → JSON array；
- `--json` + `--ndjson` 拒绝；
- novel `--content` canonical record；
- stdout/stderr I/O failure 原样返回。

Green 后再判断 presenter/output helper 是否存在稳定重复；没有则不抽象。

### Phase 3 — producer/consumer composition

用真实 canonical record 结构做 command-level 组合测试：

1. artwork search record → detail；
2. novel search record → detail；
3. user search record → detail；
4. reverse search `artwork` identity → detail → 具体 artwork kind record；
5. reverse search `user` identity → detail；
6. mixed reverse records 保持输入顺序；
7. artwork detail record → existing visual record consumer contract。

这里优先使用已有 command dependency stubs / in-memory reader/writer，不要求真实 Pixiv 网络凭据。只有现有测试载体无法证明 producer contract 时才增加更上层 root test。

### Phase 4 — 文档与产品 skill

同步说明：

- `search | detail` 的推荐形式；
- stdout pipe 自动 canonical NDJSON；
- `--ndjson` / `--json` 的区别；
- `--type` 在 record mode 的 inference/constraint 语义；
- reverse image search pipeline；
- aggregate `search --json` 不属于 record pipeline；
- binary image stdin 仍不支持。

文档只描述已由测试证明的行为。

### Phase 5 — 集成验证与审查

按范围从小到大：

1. detail 聚焦测试；
2. pipeline / record 相关测试；
3. `internal/cli` 相关回归；
4. `go test ./...`；
5. `sh scripts/build.sh`；
6. `git diff --check`；
7. 使用 `.agents/skills/pixiv-cli-review/SKILL.md` 的 finding-first checklist 审查 CLI contract、错误语义、文档和测试。

真实 Pixiv E2E 不是该功能的默认完成条件；若本地恰有授权且确需验证，可额外运行，但不能用真实网络测试替代 deterministic unit/command tests。

## 6. TDD 关键测试矩阵

| Case | Input mode | Flags | Expected entity/output |
| --- | --- | --- | --- |
| `123` argv | text | none | artwork human，完全兼容旧行为 |
| artwork URL stdin | text | `--json` | 单 artwork DTO JSON，兼容旧行为 |
| artwork ID | text | `--ndjson` | 1 canonical artwork record |
| `illust` record | record | none | infer artwork |
| `manga` record | record | none | infer artwork |
| `ugoira` record | record | none | infer artwork |
| generic `artwork` record | record | none | infer artwork；detail 后输出具体 kind |
| `novel` record | record | none | infer novel metadata |
| `novel` record | record | `--content` | infer novel content + canonical novel record |
| `user` record | record | none | infer user |
| `user` record | record | `--type artwork` | type mismatch error |
| `illust` record | record | `--type artwork` | compatible |
| unknown type | record | none | explicit unsupported type error |
| malformed JSON line | record | none | pipeline diagnostic + nonzero |
| multi records | record | stdout TTY | human entries with stable separator |
| multi records | record | stdout pipe | canonical NDJSON in same order |
| multi records | record | `--json` | one JSON array in same order |
| any | any | `--json --ndjson` | usage/validation error |
| remote detail failure | record | any | fail-fast, real SDK error retained |

## 7. 风险分析

| 风险 | 级别 | 影响 | 缓解 |
| --- | --- | --- | --- |
| 自动 NDJSON 意外改变既有 `detail ID | cat` 输出 | 高 | shell script breaking change | 自动 NDJSON 仅在 RecordMode；TextValue 默认输出锁回归测试 |
| `artwork` 与 `illust/manga/ugoira` 类型不一致 | 高 | reverse 或普通 search 其中一边失败 | 显式 compatibility set + table tests |
| `--type` 覆盖 record 造成错误实体请求 | 高 | 用 artwork ID 调 user endpoint 等 | explicit flag 仅约束，不覆盖 record type |
| record-mode `--json` 为大输入占用大量内存 | 中 | OOM | 流式 array writer，不先收集全部 records |
| detail 后丢失 canonical identity | 中 | 无法继续 pipeline | 所有 machine record 输出重新走 explicit DTO → record mapping |
| novel content 缺 stable record projection | 中 | `--content` 成为管道断点 | 仅基于稳定 NovelID/DTO 定义显式 projection，并单测 |
| 为兼容 `search --json` 扩大 parser | 中 | pipeline 协议模糊、维护成本增加 | 明确 aggregate JSON 是 non-goal |
| 错误/日志写入 stdout | 中 | 下游 NDJSON 被污染 | diagnostics/logger 只走 stderr；组合测试断言 stdout 可逐行 parse |

## 8. 验证命令

实现阶段按最小相关到全量运行，实际命令以当时测试文件名为准：

```bash
go test ./internal/cli/commands/pixiv/detail -count=1
go test ./internal/cli/pipeline ./internal/shared/record -count=1
go test ./internal/cli/... -count=1
go test ./... -count=1
sh scripts/build.sh
git diff --check
```

如果文档测试 carrier 当前存在并覆盖 CLI reference，再额外运行对应 `go test ./scripts/tests/documentation -count=1`；不为本 goal 新造一个仅做字符串扫描的文档测试框架。

## 9. 回滚方案

- 所有工作只在 `feature/search-detail-pipeline` 分支进行，未合并前不影响 `main`。
- 生产变更集中在 detail owner、必要的 shared record conversion 和最小 composition wiring；可按阶段 commit 独立 revert。
- 不修改 public SDK / reverse provider，因此回滚 detail integration 不需要数据迁移或配置迁移。
- 新增 `--ndjson` 若最终 contract 审查不通过，可在合并前连同文档和测试整体移除，不保留半支持状态。

## 10. 默认假设

1. canonical NDJSON 是唯一正式的命令间实体流协议。
2. `search --json` 继续是完整终端 document，不自动转成 record stream。
3. `search` 的过滤、分页、sort、content-type、AI、resolution 等参数只改变 records 集合，不改变 record protocol。
4. reverse image search 的 `artwork` / `user` identity records 是本 goal 的一级验收输入。
5. detail record mode 默认 fail-fast，本 goal 不新增 `--on-error`。
6. `--type` 在 record mode 中是兼容性约束；未显式指定时从 record inference。
7. record mode stdout 为非 TTY 时自动 NDJSON；TextValue mode 不自动切换，确保兼容。
8. `--json` 与新增 `--ndjson` 互斥。
9. 不新增第三方依赖，不修改锁文件。
10. 不支持二进制图片 stdin，不扩展 reverse-search source contract。