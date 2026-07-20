# Goal 3 实施计划：App API 搜索完善与 CLI 体验修复

## 权威输入与完成定义

- 原始需求以 `input.md` 为唯一权威，不得缩减或改写范围。
- 完成定义：搜索/AI/评级契约、作品 URL、下载 pages/quality、MCP 本地文件交付、登录最终页、文件日志、仓库清理与文档同步全部落地；相关测试与全量门禁通过；合并推送后远端树不含 `test/`、`goal-2/`、`docs/adr/`、`docs/superpowers/`。

## 当前基线（T01 完成后）

- 主仓根：`/Users/flanchan/Development/SourceCode/GithubProjects/pixiv-cli`
- 隔离 worktree：`/Users/flanchan/Development/SourceCode/GithubProjects/pixiv-cli/.worktrees/app-api-search-cli-ux`
- 分支：`codex/app-api-search-cli-ux`
- 基线提交：`074586fce4d3aeef43934a0f4737823ff0f7074d`（与 `origin/main` 一致）
- 基线门禁：`go test ./... -count=1` 通过；`go vet ./...` 通过（2026-07-20 worktree 复验）
- 执行权威：worktree 内 `goal-3/{input,plan,tasks}.md`
- 现状要点（T01 时仍成立）：
  - App DTO 仍读 `ai_type`，未见 `illust_ai_type` 兼容。
  - CLI 仍有 `--ai-type`、`--r18`、`--profile`、`--offset`、`comics` 兼容入口。
  - MCP 仍有 `search_r18`、`user_id_to_check`、`max_bookmark_id`、`offset`、`include_thumbnail`、`image_content` delivery。
  - public `Illust` JSON 字段序尚无首字段 `url`。
  - 下载未见 `--pages` / `--quality` 规范入口。
  - 仍存在 tracked `goal-2/`、`docs/adr/`、`docs/superpowers/`、`test/e2e`。
  - `docs/maintainers/adr/` 必须保留。

## 默认假设（禁止再问用户）

1. **基线同步**：本地 `main` 工作树干净且仅落后，可 `git merge --ff-only origin/main`；若非快进，停止并在 tasks 标阻塞，不得 reset/force。
2. **隔离工作区**：在 `.worktrees/app-api-search-cli-ux` 建 worktree，分支 `codex/app-api-search-cli-ux`，基于已同步的 `main`/`origin/main`。
3. **AI 字段**：App 响应可能同时或仅出现 `illust_ai_type` / `ai_type`；映射优先 `illust_ai_type`，缺失时回退 `ai_type`；本地 AI 判断固定 `AIType == 2`。
4. **筛选分层**：
   - 后端参数：tool / aspect-ratio / type / resolution / `search_ai_type=1`（exclude AI）。
   - 本地后筛选：rating（`x_restrict`）、only AI；exclude AI 在 canary 证明前仍保留本地后筛选。
5. **SDK 契约不变**：public SDK 一次调用 = 一次上游批次；连续空批次补拉、逻辑分页、limit 填满在 CLI/MCP（及 application 共用逻辑若已有）层实现。
6. **删除兼容入口**：移除而非 soft-deprecate 所列旧 CLI flags / MCP wire 字段；测试与文档同步删除；能力保留在规范字段上。
7. **不引入 like count**：不得新增 likes 字段，也不得把 `total_bookmarks` 文案为点赞。
8. **URL 字段**：`url = https://www.pixiv.net/artworks/${id}`，JSON 序列化时为 public Illust 的**首字段**；嵌套推荐作品同样处理。
9. **下载质量映射**（静态图）：
   - `original` → 原图 URL
   - `regular` → 最长边 1200
   - `small` → 最长边 540
   - `thumb` → 居中裁剪 250×250
   - `mini` → 居中裁剪 48×48
   - 保留 JPEG/PNG 原格式与透明通道；页不存在 → 显式错误；Ugoira + pages/非 original quality → unsupported。
10. **MCP 图像交付**：只返回本地 path / file_uri / mime_type / page / size；删除 `image_content` 与 base64 缩略图工具；skill 指导宿主本地附件，否则只分享作品 URL。
11. **日志**：`os.UserStateDir()/pixiv/logs`，按日 JSONL，默认保留 7 天；仅清理可识别的历史日志文件；失败静默；终端无日志痕迹；不记录 token/query/绝对路径/上游 body/原始错误。
12. **登录页**：OAuth 真正完成后才返回最终成功/失败 HTML；标题与正文居中；失败页不泄露敏感原因；CLI 成功提示前空一行。
13. **仓库清理**：`test/e2e` → 顶层 `e2e/`；删除空 `test/`；删除 tracked `goal-2/`、`docs/adr/`、`docs/superpowers/`；保留 `docs/maintainers/adr/`。同步脚本/文档/AGENTS。
14. **文档**：双语 README、三语 CLI reference、MCP docs、产品 skill、development、AGENTS、CHANGELOG `[Unreleased]` 全量同步；明确官方 Pixiv 能力面，不宣称 Lolicon 式聚合/随机 API。
15. **Agent skill 安装**：README 要求安装与稳定二进制同 tag 的完整 `skills/pixiv-cli/` 到用户确认目录；不跟踪 main、不猜路径。
16. **Canary**：opt-in 真实 App canary；AI exclude 仅在基线含 AI 样本时判定，否则 inconclusive。
17. **提交策略**：每个 task 独立 commit；不 force push、不 rewrite history、不移动 tag。
18. **代理**：直连失败时可用 `http://127.0.0.1:7890` 作 HTTP(S) 代理，限必要联网操作。

## 架构与实现策略

### 阶段 A — 基线与隔离
1. 快进本地 `main` 到 `origin/main`。
2. 创建隔离 worktree + 功能分支。
3. 记录基线 SHA、`go test`/`go vet` 基线结果。

### 阶段 B — 搜索/AI/评级协议
1. 修 App DTO/mapper：`illust_ai_type` + 旧字段兼容。
2. 确认/修正 query 编码：tool、ratio、content_type、resolution、`search_ai_type`。
3. 明确本地筛选：rating、only AI；exclude AI 双保险直至 canary。
4. 强化 CLI/MCP 连续空批次与逻辑分页（limit/page）。
5. 删除冗余兼容入口（CLI flags + MCP wire）。
6. `search-options` 动态工具列表。

### 阶段 C — 作品 URL
1. public model 增加 `URL` 字段（JSON `url`，首字段）。
2. CLI JSON/文本、MCP structured/text、嵌套推荐统一输出。

### 阶段 D — 下载 pages/quality 与 MCP 交付
1. 设计 pages 解析（1-based、闭区间、去重、自然序）。
2. quality 选择与 URL 解析。
3. 暴露 public SDK 下载选项与结果类型。
4. 重构下载核心，避免 `pixiv` ↔ `internal/download` 循环依赖。
5. CLI flags + MCP 参数对齐。
6. 移除 MCP `image_content`/base64 缩略图；只返回本地文件元数据。

### 阶段 E — 登录与文件日志
1. OAuth callback 最终页时序与居中/脱敏。
2. CLI 成功提示空行。
3. 文件日志子系统（路径、轮转、保留、脱敏、静默失败、错误提示策略）。

### 阶段 F — 仓库清理与文档
1. 迁移 e2e、删除目标路径、同步引用。
2. README/CLI/MCP/skill/AGENTS/CHANGELOG/development。
3. Agent skill 安装说明。

### 阶段 G — 验证与合并
1. 单元/契约/opt-in canary。
2. 全量 test/race/vet/build/pre-commit/diff-check。
3. 隔离二进制 CLI/MCP 黑盒。
4. 合并推送，远端树验证。

## TDD 与执行规则

- 行为变更：先写公开接口/契约测试，展示 RED，再最小 GREEN，后重构。
- 删除兼容入口：先改测试期望为“未知 flag/字段被拒绝或不存在”，再删生产代码。
- 每 task 聚焦 `go test`；每三 task 一次集中检查（全量 test/vet/diff-check + 契约/脱敏/文档边界）。
- 不新增无依据 timeout/截断/重试/静默 fallback。
- 用户可见变更必须更新 `[Unreleased]` 与对应 locale 文档。

## 验证层级

| 层级 | 命令/动作 |
| --- | --- |
| 聚焦 | 相关 package `go test ... -count=1`，必要时 `-race` |
| 集中检查 | `go test ./... -count=1`、`go vet ./...`、`git diff --check` |
| 终审 | `go test -race ./... -count=1`、`sh scripts/build.sh`、`pre-commit run --all-files`、黑盒 CLI/MCP |
| Canary | opt-in 真实 App；AI exclude 可 inconclusive |
| 远端 | merge/push 后 `git ls-tree -r --name-only origin/main` 无禁路径 |

## 回滚与安全

- 每 task 独立 commit，可按提交回滚。
- 禁止 `reset --hard`、force push、改写 tag/历史。
- 日志与错误输出严格脱敏；测试用 canary 字符串验证不泄漏。
- 删除仓库路径前确认 `docs/maintainers/adr/` 不受影响，且引用已更新。

## 风险与缓解

| 风险 | 缓解 |
| --- | --- |
| `illust_ai_type` 与 `ai_type` 语义不一致 | 兼容读取 + 单测双字段；canary 观察 |
| exclude AI 后端无效 | 保留本地后筛选，canary inconclusive 不宣称成功 |
| 下载重构循环依赖 | 先抽 protocol-free port/DTO，再移动实现 |
| MCP 删除 image_content 破坏调用方 | 文档/skill 明确迁移；结构化输出保留 path/file_uri |
| 快进 main 失败 | 阻塞并记录，不强制合并 |
| 误删 maintainers ADR | 清理前 `git ls-files` 白名单校验 |

## 非目标

- 不复刻 Lolicon 聚合/随机 API。
- 不新增 like count。
- 不把 App API 无法验证的能力伪装为后端筛选。
- 不重写 git 历史。
- 不在 goal 初始化阶段改业务代码。
