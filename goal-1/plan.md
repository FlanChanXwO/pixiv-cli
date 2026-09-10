# Goal-1：Pixiv vNext 重规划与收敛计划

## 背景

本 Goal 基于 `refactor/goal-1` 分支启动，该分支从 `codex/goal-3-vnext-progress` 当前 HEAD `e40443495981cdaf01215d6711cb24fa618b087a` 分出，保留既有实现与历史证据，不重做已经被真实测试和审计证明正确的工作。

旧 `goal-3/` 目录继续作为历史计划、验证证据和兼容资料来源，但不再作为本 Goal 的执行状态机。旧计划存在以下结构问题：

- 长期 vNext 总目标、单轮执行授权、capability 状态、task 日志和 release gate 混在一起。
- `tasks.md` 存在未分解的 meta-task，例如 T38 仍要求后续再拆 owner 卡。
- T44 被标注为“本轮不执行”，却又是 T45 的必需依赖，导致终态不可达。
- R01 名义上是发布 gate，却未接入 T45 依赖。
- capability authority 与 task 状态发生漂移：大量 task 已 verified，但 capability 仍停留在 `scope_admitted`。
- 41 个 capability 被当作同权重 all-or-nothing gate，真实生产 bug、核心 API 缺口与产品增强混在一起。

本 Goal 的任务不是重新发明 Pixiv vNext，而是：**盘点当前真实状态，保留已验证成果，补齐必要剩余实现，并把执行图重构成有限、可验证、可终止的 Goal Mode 任务集。**

## 权威输入与证据来源

以下旧资料允许作为历史事实与验证证据引用，但不得直接继承其中的执行状态：

- `goal-3/plan.md`
- `goal-3/tasks.md`
- `goal-3/capability-admission.md`
- `goal-3/current-surface-risk-audit.md`
- `goal-3/api-migration-verification.md`
- `goal-3/upstream-contract-matrix.md`
- `goal-3/cli-migration-matrix.md`
- `goal-3/mcp-compatibility-matrix.md`
- `goal-3/pagination-validation-report.md`
- `goal-3/mutation-validation-report.md`
- `goal-3/feasibility-report.md`
- `goal-3/wire-adapter-sdk-diff.md`
- `goal-3/shaft-protocol-diff.md`
- `goal-3/evidence/`

任何“已完成”结论必须由当前分支代码、测试、diff、历史提交或上述证据支持；不能仅因为旧 `tasks.md` 标记为 `verified` 就自动视为完成。

## 目标

1. 建立当前 HEAD 的真实 capability/layer 状态，而不是延续旧的单字段状态。
2. 将剩余工作拆成有限的 executable leaf tasks；禁止留下“以后再拆卡”的 meta-task。
3. 优先完成真实生产缺陷与核心 API 迁移，再处理非阻塞 enhancement。
4. 保持既有架构边界：upstream contract → endpoint adapter → public SDK → shared semantics → CLI/MCP。
5. 保持兼容策略显式，不由执行 Agent 临时决定 breaking change。
6. 保留 cursor、pagination、mutation uncertainty、error propagation、no-fallback 等已经验证的重要约束。
7. 定义明确终点：所有本 Goal 的 required executable task 完成或被显式 blocked，且不存在未分解任务；如果 live validation 需要外部账号/环境，可作为 release gate 单独记录，不能制造不可达 DAG。

## 执行模型

本 Goal 不再把 41 个 capability 直接作为单一线性 DAG。采用“状态盘点 → 风险分层 → vertical slices → 集成验证 → release 收敛”的执行方式。

每个 capability 记录以下 layer 状态：

- Contract
- Adapter
- SDK
- Shared semantics
- CLI
- MCP
- Offline/fixture regression
- Live validation
- Release readiness

`public_ready` 如需使用，只能作为这些 layer 的派生结论，而不是另一个需要人工维护的独立真相。

## 优先级

### P0/P1：Correctness 与已知生产故障

优先处理会导致错误结果、分页遗漏、接口不可用、错误 DTO、假过滤或错误 continuation 的问题，包括但不限于：

- logical pagination / checkpoint / replay correctness
- novel latest continuation
- novel detail / series 旧 endpoint 迁移
- artwork recommended continuation
- artwork / novel comments contract 与 DTO
- restrict / rating filter 校验与语义

### P1/P2：核心 API surface

在 correctness 稳定后完成必要 read/mutation surface，例如：

- novel ranking
- stamps
- bookmark read/mutation
- comment mutation
- user / MyPixiv / relationship MCP parity

### P2/P3：Enhancement

不得阻塞核心 correctness 和 API migration 的完成：

- bookmark `--type all`
- tags `--type all`
- recommended all
- bare-ID probe
- optional subtype expansion

## 验证策略

按层级执行，避免每个 leaf task 都重复全仓 release 验证。

单个 leaf task：

1. 先运行目标 Red 测试并确认行为性失败。
2. 实现最小修复。
3. 运行目标测试与相关 package tests。
4. 必要时运行相关 integration / compatibility test。
5. 记录实际证据与剩余风险。

每三个 task 后执行 Goal Mode 集中检查-debug task。

每个 milestone 结束时运行对应子系统 regression。

最终终审才运行最大范围验证，包括：

- `go test ./...`
- `go vet ./...`
- `sh scripts/build.sh`
- 必要 race tests
- public API compatibility
- CLI/MCP compatibility replay
- docs/Skill consistency
- 禁止 endpoint / fallback 负向检查
- 敏感信息与 evidence redaction

## Live validation 策略

Live validation 不再用“本轮不执行但又是硬依赖”的方式表达。

若当前环境具备经过授权的隔离账号和真实目标，则把 live tasks 作为 executable leaf tasks 执行。

若缺少账号、权限、真实目标或外部条件，则对应 task 标记为 `blocked_external`，记录：

- 缺失条件
- 已完成的 offline/fixture coverage
- 风险
- release 是否因此 blocked

`blocked_external` 可以结束本 Goal 的无人值守执行，但不能被误写成 `verified` 或 `public_ready`。

## 兼容原则

默认保持当前既有策略：

- public Go SDK 尽量源码兼容；已有 exported method/type 不因新 endpoint 迁移被随意删除。
- CLI 保留必要 alias/deprecation 路径。
- MCP 保留旧 tool/input/output wire compatibility，并用旧 JSON replay 验证。
- rejected endpoint 不 fallback，不伪装成功。
- `NovelContent` 等无可用 App API replacement 的旧 symbol 可保留兼容错误，但不得继续请求已拒绝 endpoint。

若执行中发现必须 breaking change，当前 task 只能记录 blocker，不得自行扩大范围实施。

## 回滚策略

- 本分支从既有 vNext WIP HEAD 分出，因此不通过大规模 revert 来“恢复干净历史”。
- 每个新 task 仅修改其 vertical slice 必需文件。
- 发现既有实现有问题时，优先用最小修正提交，不重写历史。
- 公共接口变更必须记录 blast radius 和兼容影响。
- mutation/live task 不得留下无法识别来源的远端副作用；只清理本轮明确创建的资源。

## 明确终止条件

Goal-1 只有在以下条件全部成立时结束：

1. `tasks.md` 中不存在 `pending` 的 executable leaf task。
2. 不存在未分解 meta-task。
3. 所有检查-debug task完成。
4. 当前分支真实状态表与代码/测试证据一致。
5. 所有 P0/P1 correctness 项均为 `verified` 或明确 `blocked_external`；代码内部可解决的问题不能标 external blocker。
6. 最终 regression 与 build gate 通过。
7. 所有尚未完成的事项必须显式列为 `blocked_external` 或 `deferred_nonblocking`，并说明其是否阻塞 release。
8. 不允许在终审时临时发现缺口后无限追加 scope；只允许追加用于修复本 Goal 自己引入的回归或证明既有 required 项未真实完成的 correction task。

达到以上条件后停止 Goal Mode 自动推进。