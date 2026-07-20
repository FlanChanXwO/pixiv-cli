# Goal 3 任务清单

规则：一轮只做一个 task；完成后填写“实际/证据/风险/下一步”；每三个普通 task 后必须有一次集中检查 task。

## T01 — 同步 main 并建立隔离 worktree

- 状态：已完成
- 范围：`git fetch` 后对本地干净 `main` 执行 `merge --ff-only origin/main`；在 `.worktrees/app-api-search-cli-ux` 创建 worktree 与分支 `codex/app-api-search-cli-ux`；记录基线 SHA；运行基线 `go test ./... -count=1` 与 `go vet ./...`。
- 验收：worktree 干净且等于 `origin/main`；基线测试/vet 通过或已记录既有失败；主仓 main 已快进。
- 实际：本地 `main` 从 `2dc16c4` 快进到 `074586f`（`origin/main`）；新建 worktree `.worktrees/app-api-search-cli-ux` 与分支 `codex/app-api-search-cli-ux`；复制 `goal-3/` 到 worktree；基线 `go test ./... -count=1` 与 `go vet ./...` 在 worktree 全绿。
- 证据：`git merge --ff-only origin/main` 成功；`git rev-parse HEAD origin/main` 均为 `074586fce4d3aeef43934a0f4737823ff0f7074d`；`SHA_MATCH=yes`；worktree `go test ./... -count=1` exit 0（含 `test/e2e`）；`go vet ./...` exit 0。日志：`/tmp/goal3-t01-worktree-gotest.log`、`/tmp/goal3-t01-govet.log`。
- 风险/下一步：后续业务改动只在 worktree 分支进行；主仓 `goal-3/` 仍为 untracked 规划副本，以 worktree 内 `goal-3/tasks.md` 为执行权威。下一轮 T02：App DTO `illust_ai_type` 兼容。

## T02 — App DTO 读取 `illust_ai_type` 并兼容旧字段

- 状态：已完成
- 范围：修复 App API DTO/mapper，优先 `illust_ai_type`，回退 `ai_type`；本地 AI 判断固定 `== 2`；补契约测试（仅新字段、仅旧字段、双字段、非 2 值）。
- 验收：相关 appapi/public SDK 测试 RED→GREEN；映射不依赖未验证假设。
- 实际：`illustDTO` 自定义 `UnmarshalJSON`：优先 `illust_ai_type`（含显式 0），否则回退 `ai_type`，都缺为 0；`mapIllust` 透传 `AIType`；`searchAIModeAccepts` 注释明确 `==2`；CHANGELOG `[Unreleased]` 记录用户可见修复。
- 证据：先写 `dto_ai_type_test.go` 实际 RED（only `illust_ai_type` 读到 0）；实现后 GREEN。`TestIllustListDTOUnmarshalIllustAITypeThroughList`、`pixiv.TestSearchIllustMapsIllustAITypeFromAppWire`（only/all + 双字段优先级）通过；`go test -race ./internal/pixiv/appapi ./pixiv -count=1`、`go vet` 通过。
- 风险/下一步：Web API 仍用 `aiType` camelCase，本 task 不改。下一轮 T03：后端参数与本地筛选语义（rating/only AI/exclude 双保险、search-options）。

## T03 — 搜索后端参数与本地筛选语义

- 状态：已完成
- 范围：tool/aspect-ratio/type/resolution 只编码为 App query，不做本地重复过滤；rating 按 `x_restrict` 本地筛；only AI 本地筛；exclude AI 发送 `search_ai_type=1` 并在 canary 证明前保留本地后筛；`search-options` 动态工具列表。
- 验收：appapi client 与 CLI/MCP 聚焦测试覆盖 query 编码与筛选矩阵。
- 实际：审计确认现有实现已符合分层：`setSearchIllustFilters` 编码 tool/ratio/type/resolution/`search_ai_type`；`filterSearchIllustBatch` 仅 rating+AI；`SearchIllustOptions` 动态读上游 tools。补强分层注释，并新增 public 契约测试锁定 exclude 双保险、后端参数不本地复筛、rating 仅 x_restrict、search-options 动态列表。无用户可见行为变化，未改 changelog。
- 证据：`TestSearchIllustExcludeAISendsBackendParamAndLocalPostFilters`、`TestSearchIllustBackendOnlyFiltersDoNotLocallyRefilterBatch`、`TestSearchIllustRatingFiltersLocallyByXRestrictOnly`、`TestSearchIllustOptionsReturnsDynamicToolListFromApp` 通过；既有 `TestSearchIllustMapsNormalizedFiltersToAppQuery` / `TestSearchIllustTranslatesStableFiltersToAppParameters` / options 测试仍绿；`go test ./internal/pixiv/appapi ./pixiv ./internal/cli ./internal/mcpserver -count=1`、race、vet 通过。
- 风险/下一步：exclude AI 本地后筛是否可移除依赖 T14 canary。下一轮 C01 集中检查 T01–T03。

## C01 — 集中检查 1（T01–T03）

- 状态：已完成（无新增修复项）
- 检查：需求是否偏离；AI/评级分层是否正确；是否出现无据限制/静默 fallback；全量 test/vet/diff-check；脱敏与架构边界。
- 实际：对照 input 前三段搜索契约：T01 基线隔离、T02 `illust_ai_type` 优先映射、T03 后端/本地筛选分层与 search-options 动态工具均符合。未发现无据 timeout/截断/静默 fallback；CLI/MCP/application 无协议子包直连；diff 无 secret 字面量；工作树干净。
- 证据：`go test ./... -count=1` exit 0；`go vet ./...` exit 0；`git diff --check 074586f..HEAD` 通过；`git diff --name-status` 仅 goal-3 规划、appapi AI DTO、lists 注释、契约测试与 changelog；日志 `/tmp/goal3-c01-gotest.log`。
- 风险/下一步：无 C01 插入修复 task。下一轮 T04：CLI/MCP 逻辑批次补拉与逻辑分页。

## T04 — CLI/MCP 逻辑批次补拉与逻辑分页

- 状态：已完成
- 范围：本地筛选启用时跳过连续空批次；默认补拉到首个非空逻辑批次或结束；`--limit N` 填满 N；`--limit 0` 全量；`--page N --limit M` 按过滤后结果分页；SDK 仍一次上游批次。
- 验收：CLI 与 MCP 公共测试覆盖空批次、limit、page、结束游标。
- 实际：`application.TraversePages` 的 `OneBatch` 改为跳过前导空批直至非空或结束；limit 路径本就会跨批填满。MCP `search_illust` 新增 `page`/`limit`，与 legacy `offset` 互斥，经 `searchIllustListPlan` + `nextNonEmptySearchBatch` + `CollectPages` 统一逻辑分页。CLI 默认搜索自动受益。CHANGELOG 已记。
- 证据：application 三测 RED→GREEN；`TestSearchDefaultOneBatchSkipsLeadingEmptyUpstreamBatches`；MCP `PageLimitFills`/`PageTwo`/`RejectsOffset` 与既有 `ContinuesAfterFilteredEmptyBatch` 通过；`go test -race ./internal/application ./internal/cli ./internal/mcpserver`、vet 通过。
- 风险/下一步：SDK 仍一次上游批次未改。下一轮 T05 移除冗余 CLI 兼容入口。

## T05 — 移除冗余 CLI 兼容入口

- 状态：已完成
- 范围：删除 CLI `--ai-type`、`--r18`、`--profile`、`--offset`、`comics`；能力保留在规范字段（`--ai-mode`、`--rating`、`--uid`、`--page/--limit`、规范 type）；更新测试与帮助。
- 验收：旧 flag 被拒绝或不再注册；规范路径行为不变；文档/changelog 同步。
- 实际：从 `api_cmd`/`cli`/`sdk_cmd` 删除上述兼容入口与映射逻辑；`TestSearchRejectsRemovedCompatibilityFlags` 断言 unknown flag / comics 类型错误且不打开 SDK；三语 CLI reference、architecture、CHANGELOG Breaking 已同步。
- 证据：`go test ./internal/cli -count=1`、`go test ./scripts/documentation -count=1`、vet 通过；新测试五子场景全 PASS。
- 风险/下一步：产品 skill 无这些旧 flag 残留。下一轮 T06 移除 MCP 旧 wire 字段。

## T06 — 移除冗余 MCP 旧 wire 字段

- 状态：已完成
- 范围：删除 MCP `search_r18`、`user_id_to_check`、`max_bookmark_id`、`offset`、`include_thumbnail` 等已确认旧字段；改用唯一规范字段；更新 schema/测试/文档。
- 验收：schema 不再暴露旧字段；旧输入测试改为拒绝或不识别；规范路径通过。
- 实际：legacy/search/related/ranking/recommended/follow/user tools 统一为 `page`/`limit` 与 `user_id`/`rating`；`formatIllusts` 去掉缩略图开关；schema 拒绝旧字段；MCP docs 与 CHANGELOG Breaking 同步。
- 证据：`go test ./internal/mcpserver -count=1`、`go test ./scripts/documentation -count=1`、vet 通过；schema rejection 与规范路径测试覆盖。
- 风险/下一步：下一轮 C02 集中检查 T04–T06。

## C02 — 集中检查 2（T04–T06）

- 状态：未开始
- 检查：分页/空批次契约、兼容入口删除完整性、CLI/MCP 文档与测试、全量门禁。
- 实际：
- 证据：
- 风险/下一步：

## T07 — public Illust 与输出加入首字段 `url`

- 状态：未开始
- 范围：所有 public `Illust`、CLI JSON、MCP 结构化输出及嵌套推荐作品加入 `url=https://www.pixiv.net/artworks/${pid}` 为首字段；CLI/MCP 文本输出把 URL 放在每件作品第一行。
- 验收：JSON 字段序与文本输出契约测试通过；不新增 like 字段。
- 实际：
- 证据：
- 风险/下一步：

## T08 — 下载 pages/quality 解析与 public SDK 暴露

- 状态：未开始
- 范围：实现 `--pages 1,3-5`（1-based 闭区间、去重、自然序）与 `--quality original|regular|small|thumb|mini`；静态图质量语义按计划；页不存在报错；Ugoira 派生质量/页选择 unsupported；暴露到 public SDK 与结果类型；重构避免循环依赖。
- 验收：SDK/download 单元测试覆盖解析、质量、Ugoira 拒绝、错误路径。
- 实际：
- 证据：
- 风险/下一步：

## T09 — CLI/MCP 接入下载 pages/quality，MCP 仅本地文件交付

- 状态：未开始
- 范围：CLI/MCP 共用 SDK 下载选项；MCP 只返回 path/file_uri/mime_type/页号/大小；移除 `image_content` 与 base64 缩略图工具；更新 skill 指导宿主本地附件，否则只分享作品 URL。
- 验收：CLI/MCP 黑盒与 in-memory MCP 测试通过；无内嵌图片内容。
- 实际：
- 证据：
- 风险/下一步：

## C03 — 集中检查 3（T07–T09）

- 状态：未开始
- 检查：URL 字段序、下载契约、循环依赖、MCP 交付变更、文档/changelog、全量门禁。
- 实际：
- 证据：
- 风险/下一步：

## T10 — 登录 callback 最终页与 CLI 成功提示空行

- 状态：未开始
- 范围：OAuth 真正完成后再返回成功/失败页；标题与正文居中；失败页不泄露敏感原因；CLI 成功提示前增加一个空行。
- 验收：OAuth 页面与 CLI 输出测试/手工可复现证据。
- 实际：
- 证据：
- 风险/下一步：

## T11 — 文件日志子系统

- 状态：未开始
- 范围：`os.UserStateDir()/pixiv/logs` 按日 JSONL 轮转，默认保留 7 天，仅清理识别出的历史日志；脱敏操作摘要；终端无日志痕迹；目录/轮转/清理失败静默；仅特殊非认证故障可建议查看日志。
- 验收：日志脱敏 canary 测试、保留策略测试、CLI/MCP 终端无痕迹。
- 实际：
- 证据：
- 风险/下一步：

## T12 — 仓库清理：e2e 迁移与删除过时目录

- 状态：未开始
- 范围：`test/e2e` → 顶层 `e2e/`；删除空 `test/`；删除 tracked `goal-2/`、`docs/adr/`、`docs/superpowers/`；保留 `docs/maintainers/adr/`；同步脚本、测试命令、文档路由、AGENTS。
- 验收：本地树与引用一致；相关测试路径可运行；无误删 maintainers ADR。
- 实际：
- 证据：
- 风险/下一步：

## C04 — 集中检查 4（T10–T12）

- 状态：未开始
- 检查：登录页安全、日志脱敏、仓库清理完整性、引用同步、全量门禁。
- 实际：
- 证据：
- 风险/下一步：

## T13 — 文档与产品 skill 全量同步

- 状态：未开始
- 范围：双语 README、三语 CLI reference、MCP tools、产品 skill、development、AGENTS、CHANGELOG `[Unreleased]`；Agent 安装提示同 tag 完整 skill 目录；明确官方 Pixiv 能力面，不宣称 Lolicon 聚合/随机 API；不新增 like count 文案。
- 验收：文档与实现一致；安装说明不猜路径、不跟 main。
- 实际：
- 证据：
- 风险/下一步：

## T14 — Opt-in App canary 与契约回归补强

- 状态：未开始
- 范围：补齐/运行单元与契约测试清单；opt-in 真实 App canary 验证 search-options、tool/ratio/type/resolution；AI exclude 仅在基线含 AI 时判定，否则 inconclusive。
- 验收：测试清单勾选；canary 结果有记录，不把 inconclusive 当成功。
- 实际：
- 证据：
- 风险/下一步：

## T15 — 全量门禁、黑盒验收、合并推送与远端树验证

- 状态：未开始
- 范围：`go test ./... -count=1`、`go test -race ./... -count=1`、`go vet ./...`、构建脚本、pre-commit、`git diff --check`；最终二进制隔离 CLI/MCP 黑盒；合并推送；`git ls-tree -r --name-only origin/main` 验证无 `test/`、`goal-2/`、`docs/adr/`、`docs/superpowers/`；不重写历史。
- 验收：全部门禁通过；远端树干净；PR/merge 记录可追溯。
- 实际：
- 证据：
- 风险/下一步：

## C05 — 终审（全部 task 完成后）

- 状态：未开始
- 检查：对照 `input.md` 逐条验收；C 端体验、安全性、脱敏、架构边界、测试覆盖、文档、回滚、远端树；无已知高风险问题。
- 实际：
- 证据：
- 风险/下一步：
