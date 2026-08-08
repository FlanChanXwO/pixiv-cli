# v1.0.0 RC follow-up 本地代码审查（2026-08-08）

## 范围与方法

审查对象是隔离 worktree `codex/v1-sdk-rewrite` 中 RC-1 至 RC-10 与内部架构重组的实现、聚焦测试、
public API inventory、三语契约文档和 RC-11 验证记录。审查重点为：

- CLI/MCP 是否仍只通过 `internal/application` 调用 public SDK；
- authdb migration、credential CAS、账号池事务和跨文件 legacy migration 是否会伪造成功；
- FANBOX native route、Cookie 边界、challenge-only solver、代理/UA 优先级；
- diagnostics 的 stdout 隔离、request scope、URL/header/token 脱敏；
- 新增行为是否有聚焦测试和对应文档。

方法包括 Go LSP 的 symbol/blast-radius 查询与修改后 diagnostics、源码边界检索、完整 diff
审阅、聚焦测试、全量测试、race、vet、构建、pre-commit 和文档 whitespace 检查。本审查主体未读取
credential；随后按授权取得的真实 Pixiv/solver evidence 独立记录在最终验证报告，不扩大本地 code review
的 secret 读取范围。

## 发现与处理

发现 `NewMCPRuntime` 在 SDK client 构造失败时已经打开的 authdb 没有关闭；已在
`internal/bootstrap/bootstrap.go` 的失败分支补上关闭动作，并通过 bootstrap、CLI、application
聚焦回归。

后续聚焦复核又发现并修复了三个契约缺口：FANBOX `Creators` 忽略了服务端 continuation，
`OpenResource` 未把 `HEAD`/Range/conditional request 传到受控 resource transport，以及 Pixiv
OAuth rotation 未在 application 层核对返回 UID。三项均先以失败测试固定，再由最小实现修复；另补充
了 `Open` 对缺失 OAuth account identity 的 `malformed_upstream_response` 回归。

其余审查未发现新的阻断性代码问题。修改后 LSP 对本轮新增/修改的核心路径没有 compiler、type
或 Go 语义错误；全 workspace 仍有非阻断的 `strings.Title`、`strings.Cut`、`errors.As` 等
Go 1.26.3 modernize 建议，不改变当前行为，也未为清理它们扩大产品改动范围。

## 代理审查状态

本轮尝试了只读审查代理；一个完成了 RC-1 检查，另一个在服务端 stream 断开，后续独立审查因
服务端 `429 Too Many Requests` 达到重试上限。没有把未返回的代理结果当作通过依据；上述本地
审查与自动门禁是当前可复核证据。代理可用后仍可独立复核，但不应替代真实 SDK release evidence。

## 结论

自动门禁和离线/合成契约测试支持 RC-1 至 RC-10 与内部架构重组的实现结论；2026-08-08 已补充真实
public Pixiv SDK 与一次性 real-solver protocol acceptance。真实 public FANBOX SDK/资源读取与 native
browser provider evidence 仍按 `final-verification-2026-08-08.md` 保持 release blocker；没有明确
凭据、session 或 solver 授权时不应擅自运行，也不能用离线 fixture 冒充真实 evidence。
