# Goal 1 执行计划：SauceNAO / ascii2d 以图搜图集成

## 目标与成功标准

在当前 `codex/image-search-integration` worktree 中完成已确认的反向搜图能力，同时保持 v1 Pixiv/FANBOX public SDK 边界不变。

完成标准：

- `pixiv search SOURCE` 能可靠区分关键词、HTTP(S) 图片源和本地常规文件。
- SauceNAO、ascii2d color、ascii2d bovw 与 `all` 按已确认协议工作。
- CLI、MCP、配置、Record 管道、partial/error 和敏感信息契约全部由聚焦测试固定。
- 不增加 public SDK API，不引入新依赖，不出现 source/API key/上游私密载荷泄漏。
- 英/中文用户文档、维护者文档、产品 skill 与未发布说明同步。
- `go test ./...` 与 `sh scripts/build.sh` 通过；真实上游 e2e 保持显式 opt-in。

## 当前上下文

- 目标 worktree：`/Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli/.worktrees/image-search-integration`
- 分支：`codex/image-search-integration`
- 初始化时 worktree 干净。
- 目标分支是 v1 重构后的布局：CLI owner 位于 `internal/cli/commands`，Pixiv MCP tool 位于 `internal/mcpserver/pixiv/tools/<tool>`，配置位于 `internal/config/settings`。
- 维护者文档仅有英/中文，路径为 `docs/{en,zh-CN}/maintainers/`；发布说明直接维护 `changelog/unreleased/{en,zh-CN}.md`。
- `golang.org/x/net/html` 已是直接依赖，可用于 ascii2d HTML 解析。
- 当前进度：Tasks 1–10 与 Checkpoints 1–3 已完成；Task 11 的双语文档、产品 skill 和文档路由门禁已完成；Task 12 进行中，下一步是更新双语 unreleased changelog 并执行最终验证。

## 固定设计决策

1. 不新增 `sdk/reversesearch`，不修改 `sdk/pixiv`。
2. 新能力位于 `internal/services/reversesearch`；CLI/MCP 仅允许依赖顶层 Facade/契约，不得导入 provider 协议子包。该精确例外必须写入 `AGENTS.md` 和双语架构文档。
3. CLI 使用自动识别，不新增独立命令或 `--image`；URL 失败不得回退关键词。
4. MCP 默认允许任意可读常规文件以及私网、环回、链路本地 URL。不得擅自收紧；必须显著记录其仅适合可信本机客户端。
5. SauceNAO key 必填，缺失时 provider 显式失败；禁止匿名 SauceNAO 查询和隐藏 fallback。
6. 结果过滤仅由 `reverse_search_pixiv_only` 配置控制；CLI/MCP 不提供 `all_results`。
7. Provider 默认值来自配置，但 CLI `--provider` 与 MCP `provider` 可以逐次覆盖。
8. 仅知道作品 ID 时输出通用 `type:"artwork"` Record；不得冒充 `illust`，也不得为补 subtype 自动调用 Pixiv detail。
9. Service 返回领域结果，不依赖 CLI/MCP Record；Record 投影在 adapter 边界完成。
10. context 取消是整体取消，不得伪装成 partial；不添加固定总超时、重试或统一载荷上限。

## 实现方案

### 配置与构造

- 扩展 `SettingSpec`、`RuntimeConfig` 与默认 TOML，加入 provider、pixiv-only 和 Sensitive SauceNAO key。
- `SAUCENAO_API_KEY` 覆盖 TOML。Sensitive 环境覆盖提示只能报告来源，不能报告值。
- `config set saucenao_api_key` 使用专门的非 TTY stdin 路径；argv value 必须在任何写入前拒绝。
- reverse-search Facade 构造时接收分离的 payload/SauceNAO/ascii2d HTTP client、key 与 provider ports；Request 不携带凭据或 client。
- CLI 每次图片调用按已有 proxy override 构造；MCP 在启动时注入稳定服务。

### 载荷与 provider

- source loader 将 URL 或本地文件流式复制到私有临时快照，同时计算 SHA-256。所有 provider 重开同一快照，避免重复抓取和 TOCTOU。
- URL 只允许 HTTP(S)，禁止 userinfo，每次重定向和最终响应均重验协议；明确允许所有地址范围。
- SauceNAO adapter 使用 fixture 固定 multipart、`output_type=2`、`db=999`、安全 quota 和错误映射。
- ascii2d adapter 使用独立 cookie jar，完成首页 CSRF、一次上传、严格同源 Location/hash，再读取 color/bovw。JPEG/PNG/WEBP 与 10 MB 限制只属于 ascii2d。
- Facade 负责 `all` 并发、固定 provider 顺序、partial/all-failed、canonical Pixiv ref 识别、去重和 evidence 合并。

### CLI、MCP 与 Record

- search owner 在任何 Pixiv SDK pooled read 前完成 source 分类和图片 flags 校验；图片模式不得因 JSON 输出配置而打开 Pixiv 账号数据库。
- CLI JSON/MCP 共享同一 wire 语义：`input/providers/results/records/provider_errors/partial`。human 输出只做安全摘要；NDJSON 只输出 records。
- `internal/shared/record` 新增受校验的 identity constructor；reverse-search artwork record 使用 `type:"artwork"`，user 使用 `type:"user"`。
- download 与 artwork bookmark action 接受通用 `artwork`，仍只消费正数 ID；其他类型兼容规则保持不变。
- MCP 新 tool 使用封闭 input schema 和显式 output schema；执行期失败始终保留 structured envelope。

### 文档与交付

- 更新英/中文 README、CLI reference、MCP tools、维护者 architecture/development、产品 skill 和双语 unreleased changelog。
- 文档必须覆盖自动识别、配置、provider、第三方上传、MCP 文件外传/SSRF 权限、SauceNAO 保存策略、partial 与 NDJSON。
- 不创建或恢复日文文档、旧 maintainers 路径或旧 PR release-note 流程。

## 风险与控制

- **上游 HTML/API 漂移**：默认门禁使用版本化 fixtures；关键结构缺失返回 `malformed_upstream_response`，不吞错。
- **敏感信息泄漏**：key、source、临时路径、CSRF、Location、原始 body 全部进入负向泄漏测试；错误只暴露稳定 code 和安全 message。
- **MCP 文件外传/SSRF**：这是用户明确接受的信任模型，不做隐藏限制；通过文档、tool 描述和不回显 source 降低误用风险。
- **并发非确定性**：provider 并发执行但聚合按固定 provider/rank 顺序发布。
- **Record subtype 不确定**：使用通用 `artwork`，不伪造精确 subtype；仅扩展真正只依赖 ID 的 action。
- **错误触发 Pixiv 认证**：图片模式不得调用 Pixiv SDK 或仅为输出配置打开账号 DB；用构造调用计数测试固定。
- **上游限额/反爬**：不自动重试、不隐藏 fallback；按 provider error/partial 公开结果。

## 验证策略

每个实现 task 都执行 Red → Green → Refactor：先新增聚焦测试并实际确认因缺失行为失败，再实现最小代码并运行相关回归。每三个实现 task 后进行一次集中检查-debug 循环。

最终验证顺序：

1. `go test ./internal/services/reversesearch/...`
2. `go test ./internal/shared/record/...`
3. `go test ./internal/config/settings/... ./internal/cli/commands/config/...`
4. `go test ./internal/cli/commands/pixiv/search/... ./internal/cli/commands/pixiv/download/... ./internal/cli/commands/pixiv/bookmark/...`
5. `go test ./internal/mcpserver/pixiv/...`
6. 相关 architecture/secret/e2e fixture tests
7. `go test ./...`
8. `sh scripts/build.sh`

真实网络验证仅在 `PIXIV_REVERSE_SEARCH_E2E=1` 时运行；SauceNAO 路径还要求 `SAUCENAO_API_KEY`。真实 e2e 失败不得通过修改默认 fixture 契约来掩盖。

## 回滚方案

- 所有功能按 task 独立提交；发现阻塞回归时优先 revert 对应 task commit，不使用 destructive reset。
- 若上游 provider 暂时不可兼容，可回滚对应 adapter 和注册，不保留静默 fallback。
- 若 generic `artwork` Record 影响既有 action，回滚 action 类型扩展与 reverse-search NDJSON 输出，不能把类型改成虚假的 `illust`。
- 配置回滚时保留已写入 key 的安全清理路径；删除 setting 前需添加 removed-setting 墓碑或明确迁移，不能让旧配置静默改变行为。

## 默认假设

- 用户接受 SauceNAO/ascii2d 为不稳定第三方服务，fixture contract 是默认发布依据。
- 用户接受 MCP 对本地文件和私网 URL 的完全权限，并只在可信本机客户端中运行。
- ascii2d 的 10 MB 按实现命名常量固定并由边界测试证明；不得把该限制套给 SauceNAO。
- 不需要公开 SDK、数据库迁移、新依赖或 UI。
- 无可用 key 时只阻塞需要 SauceNAO 的 task/真实 e2e；ascii2d fixture 实现和其他非网络 task 可继续。
- 用户在执行期间补充授权：可使用本地 Docker 的 FlareSolverr 进行 Cloudflare 兼容性测试。该授权仅用于后续 ascii2d/真实网络验证，不把 FlareSolverr 变成默认依赖、生产配置或静默 fallback；使用前检查本地容器状态并在对应 task 记录证据与影响。
