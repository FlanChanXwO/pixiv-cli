Goal-3 保持单一完整 Goal。2026-09-07 用户批准在 codex/goal-3-vnext-plan 分支修订固定基线 2167445 的全部 9 项审查意见，并按 TDD 修复生产搜索分页续读，验证、自审后提交推送原分支。

本轮实现仅限 T23A：共享 checkpoint、可持久化 SDK cursor、CLI/MCP 两搜索调用方与回归；不执行真实账号 API，不新增依赖，不启动其余 vNext 功能。bookmark list/tags --type all 均列为未来必交付，同名标签按内容类型分别保留计数。

当前 required_scope/状态/发布授权唯一来源为 capability-admission.md；执行顺序见 tasks.md。用户原先只修订计划的边界已由本次明确授权扩大；历史 evidence 保持真实。无需创建新 Goal 或另行生成通用计划文件。
