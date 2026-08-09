# v1.0.0 RC 后续改动索引

状态：专题设计与历史/后续文档拆分已通过独立规格审查；新的
[RC follow-up 实施计划](rc-follow-up-implementation-plan.md)已获用户批准并写入。RC-1 至 RC-10
以及独立的内部架构重组已在隔离 worktree 实施并完成对应自动/聚焦回归；截至 2026-08-09，RC-11
已取得真实 Pixiv SDK、一次性 solver acceptance，以及两个用户指定 FANBOX target 的 production
SDK/resource evidence；Keychain、旧 ro7274 target 与 native browser provider release evidence
仍需按发布条件逐项记录。

## 文档边界

[初始重写设计](design.md) 与 [初始实施顺序](implementation-plan.md) 已经执行，继续保留为 v1.0.0
整体重写的历史记录。2026-08-04 以后发现的 RC 问题与新增行为不再追加为旧实施计划的新 phase；每个
独立主题写入新专题文档，最终由新的
[RC follow-up 实施计划](rc-follow-up-implementation-plan.md)统一编排。

该新实施计划是在专题规格通过用户书面复核后创建的。它没有把旧 Phase 0–9 描述成尚未完成，也
不要求重新执行已经交付的整体 SDK 重写；只安排 follow-up delta、聚焦测试、文档同步与 RC evidence。

## 已确认或已审查的输入

- [FANBOX challenge 与 FlareSolverr 路由](fanbox-challenge-routing.md)
- [网络配置与服务路由](network-routing.md)
- [Pixiv 账号调度](account-pool-scheduling.md)
- [authdb 设计审查（2026-08-04）](authdb-design-review-2026-08-04.md)
- [显式 debug 诊断](debug-diagnostics.md)
- [严格 unknown-option 解析](strict-cli-argument-parsing.md)
- [会话状态与验证证据](session-status-2026-08-03.md)

## 仍然适用的门禁文档

- [测试、迁移与发布门禁](verification-release.md)
- [最终验证操作手册](release-prep-runbook.md)
- [环境就绪审计](environment-readiness.md)

这些门禁文档可以根据新 evidence 修正当前 RC 判定，但不是新的实施步骤清单。后续计划只链接其
canonical rule，不复制整段验证或 secret 边界。

## 实施计划范围

[RC follow-up 实施计划](rc-follow-up-implementation-plan.md)编排并记录以下 follow-up delta：

1. authdb 审查列出的阻塞修复与数据库账号调度（RC-3 至 RC-5 已完成）；
2. FANBOX native route/profile/resource 与 challenge-only FlareSolverr recovery（RC-6 至 RC-8 已完成）；
3. Pixiv/FANBOX service-scoped network 与 FANBOX UA 配置（RC-6 至 RC-7 已完成）；
4. error `Reason` public naming 收口（RC-1 已完成）；
5. 单一显式 `--debug` 与严格 unknown-option parsing（RC-2、RC-9 已完成）；
6. 聚焦测试、三语 public contract、产品 Skill、一次性 solver implementation acceptance 与最终 RC
   回归（RC-10 已完成；RC-11 自动门禁已完成，Pixiv/solver evidence 已通过，两个指定 FANBOX
   target 的 production SDK/resource evidence 已通过，Keychain/旧 ro7274/native browser release
   evidence 仍待完成）。

不在新计划中重做已经完成的 SDK package migration、初始 CLI/MCP 重写、历史 worktree 建立或旧
Phase 0–9 的全量步骤。

## 下一轮正式版架构计划

[v1.0.0 下一轮内部架构重组计划](architecture-reorganization-plan.md)记录正式版前的包边界、CLI
命令树、application/persistence/bootstrap 重组、TOML 配置约束、downloader/ugoira/native 路径
收敛，以及对应的迁移、测试和文档门禁。该计划已完成本地验收；其结果独立记录，不把架构重组
误写成 RC-1 至 RC-10 的历史步骤。
