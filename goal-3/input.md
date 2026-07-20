# App API 搜索完善与 CLI 体验修复整合计划

## 摘要

以 `origin/main` 为基线建立隔离 worktree，先安全快进同步本地 `main`，再并行完成协议适配、下载/登录/日志、文档与仓库清理。所有作品数据继续优先走 App API；不把无法验证的上游能力伪装为后端筛选。

## 搜索、SDK、CLI 与 MCP

- 修复 App DTO 对 `illust_ai_type` 的读取，并兼容旧字段；AI 本地判断固定以 `== 2` 为 AI。
- 工具、比例、类型、分辨率只传 App API 参数，不做本地重复过滤；`search-options` 动态读取工具列表。
- `rating` 继续按响应 `x_restrict` 本地筛选：App API 没有可靠的分级 query。`only AI` 同样本地筛选；`exclude AI` 发送 `search_ai_type=1`，仅在可区分的真实 canary 证明生效后移除其本地后筛选。
- SDK 保持“一次上游批次”的契约；CLI/MCP 在本地筛选启用时跳过连续空批次：
  - 默认搜索补拉到首个非空逻辑批次或真正结束；
  - `--limit N` 填满 N 条逻辑结果或结束；
  - `--limit 0` 遍历全部；
  - `--page N --limit M` 以过滤后的逻辑结果分页。
- 移除全部已确认的冗余兼容入口：CLI `--ai-type`、`--r18`、`--profile`、`--offset`、`comics`；MCP 的 `search_r18`、`user_id_to_check`、`max_bookmark_id`、`offset`、`include_thumbnail` 等旧 wire 字段。保留能力，改用唯一规范字段。
- 不新增“点赞数”字段：App 响应没有可信的直接 like count，`total_bookmarks` 不得冒充点赞数。

## 作品 URL、下载与 Agent 图像交付

- 为所有 public `Illust`、CLI JSON、MCP 结构化输出及嵌套推荐作品加入首字段 `url`，固定为 `https://www.pixiv.net/artworks/${pid}`；CLI/MCP 文本输出把 URL 放在每件作品的第一行。
- 下载默认保留所有原图；新增 `--pages 1,3-5`（1-based、闭区间、去重并按自然页序）和 `--quality original|regular|small|thumb|mini`。
- 静态图质量语义固定为：原图、最长边 1200、最长边 540、居中裁剪 250×250、居中裁剪 48×48；保留 JPEG/PNG 原格式与透明通道。选中的页不存在即报错。
- Ugoira 只支持现有原始 GIF/APNG 流程；派生质量或页选择显式返回 unsupported。
- 将页面选择、质量、目标路径与结果类型暴露到 public SDK，CLI/MCP 共用；重构下载核心以避免 public SDK 与 internal downloader 循环依赖。
- MCP 下载只返回本地文件 `path`、`file_uri`、`mime_type`、页号和大小；移除内嵌 `image_content` 与 base64 缩略图工具。产品 skill 指导 Agent 用宿主的本地附件能力发送文件；宿主不支持时仅分享作品 URL，不宣称已发送图片。

## 登录、日志、仓库与文档

- 登录 callback 在真正完成 OAuth 后再向浏览器返回最终成功/失败页；成功和失败标题、正文均居中，失败页不泄露敏感原因；CLI 成功提示前增加一个空行。
- 新增文件日志：`os.UserStateDir()/pixiv/logs` 下按日 JSONL 轮转，默认保留 7 天，仅清理识别出的历史日志文件；记录脱敏操作摘要，不记录 token、查询串、绝对路径、上游 body 或原始错误。
- CLI 与 MCP 的终端不输出日志痕迹；日志目录创建、轮转或清理失败时静默继续。仅特殊非认证故障可在用户错误中建议查看日志；登录失败与 token 过期不提示。
- 将 `test/e2e` 移为顶层 `e2e/`，删除空 `test/`；删除 tracked `goal-2/`、`docs/adr/`、`docs/superpowers/`，但保留 `docs/maintainers/adr/`。同步脚本、测试命令、文档路由和 AGENTS 引用。
- README 的 Agent 安装提示要求安装与稳定二进制相同 tag 的完整 `skills/pixiv-cli/` 目录到用户确认的 Agent skills 目录，不跟随 `main`、不猜测路径。
- 同步双语 README、CLI/MCP 文档、产品 skill、开发文档、AGENTS、CHANGELOG `[Unreleased]`；明确这是官方 Pixiv 能力面，不宣称复刻 Lolicon 的聚合/随机 API。

## 验证与验收

- 单元与契约测试：App query 编码、`illust_ai_type` 映射、评级/AI 语义、cursor 绑定、连续空批次、逻辑分页、JSON 原子输出、URL 输出顺序、下载范围/质量/Ugoira 拒绝、OAuth 最终页面、日志脱敏与保留。
- Opt-in 真实 App canary：验证 `/v1/search/options`、工具/比例/类型/分辨率后端结果；AI exclude 只在基线含 AI 样本时判定，未满足条件则标记 inconclusive，不作成功结论。
- 运行 `go test ./... -count=1`、`go test -race ./... -count=1`、`go vet ./...`、构建脚本、pre-commit、`git diff --check`，并以最终二进制完成隔离 CLI/MCP 黑盒验收。
- 合并并推送后，用 `git ls-tree -r --name-only origin/main` 验证远端树中不再含 `test/`、`goal-2/`、`docs/adr/`、`docs/superpowers/`；不重写历史。
