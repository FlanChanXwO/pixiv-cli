# pixiv-cli 重构：goal 语句

规格见同目录的 `spec.md` 第 7 节 7.0。每条语句可以独立粘贴。开始一个 goal 前，它依赖的 goal 必须已合并进 main；同一阶段内没有依赖关系的 goal 可以同时运行，每个都在自己的 worktree 中进行。

标题带“（可并行）”的 goal 可以与其他 goal 同时运行，只要下表中的依赖都已合并。同一线路内仍按依赖顺序执行，例如 G08 要等 G07 合并。

依赖于 2026-10-23 调整：G09、G23–G27 不再等待 G22 和整个阶段 1，只等待与其代码冲突的 goal；G24、G26 先于 G22 进行，G22 在它们的结果上改造日志。G19 照常执行，不再等待 ClawHub 等发布 workflow 的状态；G20 扩大为 scripts 与 CI 的全面精简，G21、G33 照常执行（2026-10-23 更正：此前误标为取消）；frontend/ 的文件头注释并入 G32。以下表为准。

2026-10-24 补充：G39 拆为 G39–G42 四个按目录并行的逐包审查；G29–G33 扩大为注释整理（规格 3.5.1）；新增 G43 面向用户的 changelog。

2026-10-25 补充：新增 G44，删除全部测试后按规格 3.4.4 的清单与上限重写；G39–G42 改为在 G44 之后进行。

2026-10-24 起冻结各 goal 语句：执行中发现的新问题记录在 PR 描述和最终汇报中，由 G28（代码）或 G34（文档、skill）统一处理，不再修改尚在执行的 goal。

| Goal | worktree | 依赖 | 可并行 |
|---|---|---|---|
| G01 | `refactor-g01-tidy` | 无 | 否 |
| G02 | `refactor-g02-move-shared` | G01 | 否 |
| G03 | `refactor-g03-move-services` | G02 | 否 |
| G04 | `refactor-g04-move-cli` | G03 | 否 |
| G05 | `refactor-g05-sdk-types` | G04 | 否 |
| G06 | `refactor-g06-sdk-artwork` | G05 | 是 |
| G07 | `refactor-g07-sdk-novel` | G06 | 是 |
| G08 | `refactor-g08-sdk-user` | G07 | 是 |
| G09 | `refactor-g09-sdk-fanbox` | G06 | 是 |
| G10 | `refactor-g10-mcp-tools` | G05 | 是 |
| G11 | `refactor-g11-cli-commands` | G05 | 是 |
| G12 | `refactor-g12-cli-prompt` | G11 | 是 |
| G13 | `refactor-g13-config-tests` | G05 | 是 |
| G14 | `refactor-g14-config-toml` | G13 | 是 |
| G15 | `refactor-g15-transport` | G05 | 是 |
| G16 | `refactor-g16-storage` | G05 | 是 |
| G17 | `refactor-g17-live-tests` | G05 | 是 |
| G18 | `refactor-g18-leaf-packages` | G05 | 是 |
| G19 | `refactor-g19-scripts-move` | 无 | 是 |
| G20 | `refactor-g20-scripts-slim` | G19 | 是 |
| G21 | `refactor-g21-workflow-refs` | G20 | 是 |
| G22 | `refactor-g22-logging` | G07、G08、G09、G10、G11、G14、G15、G24、G26 | 否 |
| G23 | `refactor-g23-shared-mechanisms` | G08、G14、G19 | 是 |
| G24 | `refactor-g24-media` | G05 | 是 |
| G25 | `refactor-g25-mcp-runtime` | G05 | 是 |
| G26 | `refactor-g26-accounts` | G05 | 是 |
| G27 | `refactor-g27-cli-auth` | G12 | 是 |
| G28 | `refactor-g28-sweep` | G22、G23、G24、G25、G26、G27 | 否 |
| G29 | `refactor-g29-headers-sdk` | G39、G40、G41、G42 | 是 |
| G30 | `refactor-g30-headers-cli` | G39、G40、G41、G42 | 是 |
| G31 | `refactor-g31-headers-mcp` | G39、G40、G41、G42 | 是 |
| G32 | `refactor-g32-headers-internal` | G39、G40、G41、G42 | 是 |
| G33 | `refactor-g33-headers-scripts` | G21、G39、G40、G41、G42 | 是 |
| G35 | `refactor-g35-tests-sdk` | G28 | 是 |
| G36 | `refactor-g36-tests-mcp` | G28 | 是 |
| G37 | `refactor-g37-tests-cli` | G28 | 是 |
| G38 | `refactor-g38-tests-internal` | G28 | 是 |
| G39 | `refactor-g39-audit` | G20、G28、G35、G36、G37、G38、G44 | 是 |
| G40 | `refactor-g40-audit-mcp` | G20、G28、G35、G36、G37、G38、G44 | 是 |
| G41 | `refactor-g41-audit-cli` | G20、G28、G35、G36、G37、G38、G39、G40、G43、G44 | 否 |
| G42 | `refactor-g42-audit-internal` | G20、G28、G35、G36、G37、G38、G44、G43 | 是 |
| G43 | `refactor-g43-changelog` | 无 | 是 |
| G44 | `refactor-g44-tests-rewrite` | G35、G36、G37、G38 | 否 |
| G34 | `refactor-g34-finish` | G29、G30、G31、G32、G33、G39、G40、G41、G42、G43 | 否 |

## G01：go mod tidy

```text
/goal 执行 pixiv-cli 重构规格的 G01：go mod tidy。

【本 goal】
工作项：A1。
- 只运行 go mod tidy，修正直接依赖上的 // indirect 标记，删除未使用的条目。

【验收】
- 只改动 go.mod、go.sum。
- go build ./... 和 go test ./... 通过。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g01-tidy
- 路径：/Users/flanchan/.codex/worktrees/refactor-g01-tidy/pixiv-cli
- 分支：refactor/g01-tidy
- 开始前用 gh 确认依赖已满足：无。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g01-tidy /Users/flanchan/.codex/worktrees/refactor-g01-tidy/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G02：撤销 shared、utils、storage/file，改名 selfupdate、toolhost

```text
/goal 执行 pixiv-cli 重构规格的 G02：撤销 shared、utils、storage/file，改名 selfupdate、toolhost。

【本 goal】
工作项：A7①（规格 5.2、5.3）。
- 按 5.3 撤销 internal/shared、internal/utils、storage/file；releaseversion 并入 version；internal/update 改名为 selfupdate；mcpserver/{pixiv,fanbox}/internal/runtime 改名为 toolhost。
- 只移动、改名、改写 import，不改逻辑。utils/text 的 FirstNonEmpty、DefaultString 改用 cmp.Or 属于逻辑改动，单独一次提交。
- 修改 scripts/build-platform.sh 中 ldflags 注入的 buildinfo.Version 路径，并同步 AGENTS.md、中英文 architecture.md 和 development.md、相关 skill 中的路径。

【验收】
- go build ./... 通过；pixiv --help、tools/list 与改动前一致（改动前先在 origin/main 上采集）。
- go list ./internal/... 中目录名与包名一致。
- 用 scripts/build-platform.sh 构建后，pixiv --version 输出注入的版本号。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g02-move-shared
- 路径：/Users/flanchan/.codex/worktrees/refactor-g02-move-shared/pixiv-cli
- 分支：refactor/g02-move-shared
- 开始前用 gh 确认依赖已满足：G01。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g02-move-shared /Users/flanchan/.codex/worktrees/refactor-g02-move-shared/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G03：撤销 services 层

```text
/goal 执行 pixiv-cli 重构规格的 G03：撤销 services 层。

【本 goal】
工作项：A7②（规格 5.2、5.3、D7）。
- 建立 pixivapi（含 appapi、protocol、resource）、pixivapi/oauth、fanboxapi、pixivaccount、pixivaccount/pool、fanboxaccount、session（Facade 改名为 session.Pixiv/Fanbox）、loginrelay、reversesearch。
- services/*/endpoint/** 暂留原处，由 G06–G09 并入 SDK。
- 合并后会产生同名冲突的部分，先保持独立。
- 同步 AGENTS.md 中引用 services/reversesearch/assembly 的边界条款与维护文档路径。

【验收】
- go build ./... 通过；pixiv --help、tools/list 与改动前一致。
- 只有移动、改名和 import 改写。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g03-move-services
- 路径：/Users/flanchan/.codex/worktrees/refactor-g03-move-services/pixiv-cli
- 分支：refactor/g03-move-services
- 开始前用 gh 确认依赖已满足：G02。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g03-move-services /Users/flanchan/.codex/worktrees/refactor-g03-move-services/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G04：CLI 目录按命令路径重排

```text
/goal 执行 pixiv-cli 重构规格的 G04：CLI 目录按命令路径重排。

【本 goal】
工作项：A7③（规格 5.2、5.3）。
- 撤销 cli/commands/，目录与命令树对应，mcp 移到根下；支撑代码移入 cli/internal/{command,input,pixivcmd,fanboxcmd,resolver}。
- deps 与 listing 的合并、fanbox.go 的拆分留给 G11；如果必须暂时把 commands/fanbox/fanbox.go 整体放进 cli/internal/fanboxcmd，在 PR 中说明。
- 修改 platform-smoke.yml 中 loginhelper 的测试路径，同步文档与 skill。

【验收】
- go build ./... 通过；pixiv --help、tools/list 与改动前一致。
- cli/ 下除 internal/ 外的目录路径与命令路径一致。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g04-move-cli
- 路径：/Users/flanchan/.codex/worktrees/refactor-g04-move-cli/pixiv-cli
- 分支：refactor/g04-move-cli
- 开始前用 gh 确认依赖已满足：G03。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g04-move-cli /Users/flanchan/.codex/worktrees/refactor-g04-move-cli/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G05：SDK 公开类型合一与公开 API 定稿

```text
/goal 执行 pixiv-cli 重构规格的 G05：SDK 公开类型合一与公开 API 定稿。

【本 goal】
工作项：A2，以及 T6 中 SDK 公开 API 的全部删除与改名（规格 6.2、3.1.2、3.4.2 的 P-l）。
- 删除 dto.go、resource_dto.go、page_dto.go；为 sdk.Resource、sdk.Page 实现 MarshalJSON；mediaproxy 缓存改为显式字段；映射层保证切片不为 nil；record.RecordFrom* 改名。
- 删除 AddBookmark、RemoveBookmark 等旧入口，按 3.1.2 完成 SDK 公开名的改名。此后 SDK 公开 API 在阶段 1 内不再变动。
- 补做 G03 推迟的 session 改名（PR #162 只移动了路径）：internal/session/pixiv（包 pixiv）与 internal/session/fanbox（包 fanbox）与 SDK 包同名，调用方只能用 sdkpixiv、fanboxsdk 这类别名 import，违反规格 5.3 的命名约束。合并为一个包 internal/session，分为 pixiv.go、fanbox.go 两个文件；Facade/NewFacade 改名为 Pixiv/NewPixiv、Fanbox/NewFanbox（规格 3.1.2），删除因同名而写的 import 别名，并修改全部调用方。只做合包和改名；Dependencies、NewFacadeWithCloseClient 这类注入点的清理仍由 G26 按 3.3 处理。改名单独提交，不与 A2 混在一个 commit 里。G10、G11 会大量调用 session，所以这项必须在阶段 1 开始前合并。

【验收】
- internal/session 是一个包 session，不再有 session/pixiv、session/fanbox 子包；仓库中 import session 和 SDK 的地方都不需要别名。
- CLI 的 JSON/NDJSON 输出和 MCP 结构化输出改动前后逐字节一致（用 fixture 或 httptest 夹具比对）。
- 6.2 要求的安全测试与缓存往返测试都已加上。
- go doc -all 与改动前比对，PR 中把全部公开 API 变化列为 changelog 条目。
- go test -race ./... 通过。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g05-sdk-types
- 路径：/Users/flanchan/.codex/worktrees/refactor-g05-sdk-types/pixiv-cli
- 分支：refactor/g05-sdk-types
- 开始前用 gh 确认依赖已满足：G04。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g05-sdk-types /Users/flanchan/.codex/worktrees/refactor-g05-sdk-types/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
允许用本机 Homebrew 安装的 pixiv-cli（v1.2.1）中已登录的 pikman1（128042145）运行 Pixiv 真实读取测试，用 PIXIV_E2E_READ_USER_ID 指定。禁止使用 Flan（25649510）和默认账号 Remi（127975236）。不直接打开账号数据库，不在任何输出中出现 token。FANBOX、以图搜图和浏览器凭据的真实请求未授权。遇到 429 不重试、不换账号。可选：用 TestRealPixivSDKRead 确认真实响应仍能正确映射。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G06：Pixiv artwork 端点并入 SDK（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G06：Pixiv artwork 端点并入 SDK。

【本 goal】
工作项：A3 的 artwork 端点（规格 6.1、3.3、附录 A 的 F4/F5）。
- 端点实现并入 sdk/pixiv 对应的领域文件，路径和参数名写成 pixivapi/api.go 中的常量（只新增常量，不改 pixivapi 的其他代码，那部分归 G15）。
- 重复的 mapUser、mapArtwork 等映射函数合并为一份，放在 wire.go；continuation 改为“每个端点一条 Spec + 一个解析函数”；删除对应的 Transport 接口和子 client 字段。

【验收】
- 原有 endpoint 测试中的响应校验断言全部保留，改写成按端点族组织的表驱动测试。
- 各种映射写法之间的差异逐项列在 PR 中，并给出取舍。
- go doc -all 与改动前一致（公开 API 不变）。
- 合并前用 code-review-expert 审查；go test -race ./... 通过。
- 用真实读取测试确认覆盖到的端点。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g06-sdk-artwork
- 路径：/Users/flanchan/.codex/worktrees/refactor-g06-sdk-artwork/pixiv-cli
- 分支：refactor/g06-sdk-artwork
- 开始前用 gh 确认依赖已满足：G05。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g06-sdk-artwork /Users/flanchan/.codex/worktrees/refactor-g06-sdk-artwork/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
允许用本机 Homebrew 安装的 pixiv-cli（v1.2.1）中已登录的 pikman1（128042145）运行 Pixiv 真实读取测试，用 PIXIV_E2E_READ_USER_ID 指定。禁止使用 Flan（25649510）和默认账号 Remi（127975236）。不直接打开账号数据库，不在任何输出中出现 token。FANBOX、以图搜图和浏览器凭据的真实请求未授权。遇到 429 不重试、不换账号。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G07：Pixiv novel 端点并入 SDK（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G07：Pixiv novel 端点并入 SDK。

【本 goal】
工作项：A3 的 novel 端点（规格 6.1、3.3、附录 A 的 F4/F5）。
- 端点实现并入 sdk/pixiv 对应的领域文件，路径和参数名写成 pixivapi/api.go 中的常量（只新增常量，不改 pixivapi 的其他代码，那部分归 G15）。
- 重复的 mapUser、mapArtwork 等映射函数合并为一份，放在 wire.go；continuation 改为“每个端点一条 Spec + 一个解析函数”；删除对应的 Transport 接口和子 client 字段。

【验收】
- 原有 endpoint 测试中的响应校验断言全部保留，改写成按端点族组织的表驱动测试。
- 各种映射写法之间的差异逐项列在 PR 中，并给出取舍。
- go doc -all 与改动前一致（公开 API 不变）。
- 合并前用 code-review-expert 审查；go test -race ./... 通过。
- 用真实读取测试确认覆盖到的端点。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g07-sdk-novel
- 路径：/Users/flanchan/.codex/worktrees/refactor-g07-sdk-novel/pixiv-cli
- 分支：refactor/g07-sdk-novel
- 开始前用 gh 确认依赖已满足：G06。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g07-sdk-novel /Users/flanchan/.codex/worktrees/refactor-g07-sdk-novel/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
允许用本机 Homebrew 安装的 pixiv-cli（v1.2.1）中已登录的 pikman1（128042145）运行 Pixiv 真实读取测试，用 PIXIV_E2E_READ_USER_ID 指定。禁止使用 Flan（25649510）和默认账号 Remi（127975236）。不直接打开账号数据库，不在任何输出中出现 token。FANBOX、以图搜图和浏览器凭据的真实请求未授权。遇到 429 不重试、不换账号。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G08：Pixiv user 及其余 Pixiv 端点并入 SDK（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G08：Pixiv user 及其余 Pixiv 端点并入 SDK。

【本 goal】
工作项：A3 的 user 及其余 Pixiv 端点；完成后删除整个 internal/services/（G09 已删除 services/fanbox；规格 6.1、3.3、附录 A 的 F4/F5）。
- 端点实现并入 sdk/pixiv 对应的领域文件，路径和参数名写成 pixivapi/api.go 中的常量（只新增常量，不改 pixivapi 的其他代码，那部分归 G15）。
- 重复的 mapUser、mapArtwork 等映射函数合并为一份；continuation 改为“每个端点一条 Spec + 一个解析函数”；删除对应的 Transport 接口和子 client 字段。
- sdk/pixiv 改为按领域组织文件（取代规格 3.1、5.1、6.1、6.2 示例中的 wire.go 与 ops_*.go 命名）：
  - ops_*.go 改为领域文件名（artwork.go、novel.go、user.go、bookmark.go、comment.go 等，规格 5.1）。
  - 只被一个方法使用的请求类型移到该方法旁边，删除 request.go。
  - 只被一个领域使用的响应结构与映射函数放进该领域文件；多个领域共用的响应结构与映射（user、tag、image_urls 等）放在 response.go；删除 wire.go 与 map_*.go。
  - models.go 保留。改名与移动单独提交，不与端点合并混在一起。
- 删除 G07 留下的临时桥接：mapUserEntity、mapNovelEntity、novelWireFromEntity、artworkWireFromEntity，以及注释中引用 goal 编号的说明。
- 同步中英文 architecture.md 中描述 endpoint 族的段落（第 141 行附近），改为描述 sdk/pixiv 的领域文件；修正 docs/zh-CN/maintainers/architecture-mcp.md 中的 services/pixiv/account/reads.go（现为 internal/pixivaccount/reads.go），并核对英文版。

【验收】
- internal/services/ 已删除；以合并时的 main 重新 grep，全仓库（.agents/refactor/ 除外）不再引用 internal/services。
- sdk/pixiv 中没有 wire.go、request.go、map_*.go、ops_*.go；sdk/ 与 internal/ 的代码注释中不出现 goal 编号。
- 发往上游的 query/form 参数（名称、取值、增删）如有任何变化，在 PR 中逐条列为 changelog 条目并说明原因，不写 None。
- 原有 endpoint 测试中的响应校验断言全部保留，改写成按端点族组织的表驱动测试。
- 各种映射写法之间的差异逐项列在 PR 中，并给出取舍。
- go doc -all 与改动前一致（公开 API 不变）。
- 合并前用 code-review-expert 审查；go test -race ./... 通过。
- 用真实读取测试确认覆盖到的端点。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g08-sdk-user
- 路径：/Users/flanchan/.codex/worktrees/refactor-g08-sdk-user/pixiv-cli
- 分支：refactor/g08-sdk-user
- 开始前用 gh 确认依赖已满足：G07。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g08-sdk-user /Users/flanchan/.codex/worktrees/refactor-g08-sdk-user/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
允许用本机 Homebrew 安装的 pixiv-cli（v1.2.1）中已登录的 pikman1（128042145）运行 Pixiv 真实读取测试，用 PIXIV_E2E_READ_USER_ID 指定。禁止使用 Flan（25649510）和默认账号 Remi（127975236）。不直接打开账号数据库，不在任何输出中出现 token。FANBOX、以图搜图和浏览器凭据的真实请求未授权。遇到 429 不重试、不换账号。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G09：FANBOX 端点并入 SDK（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G09：FANBOX 端点并入 SDK。

【本 goal】
工作项：A4（规格 6.1）。
- services/fanbox/endpoint/** 并入 sdk/fanbox，完成后 services/fanbox/ 消失；services/pixiv/endpoint 由 G08 删除，两者都合并后 services/ 整体消失。
- 只修改 sdk/fanbox 与 services/fanbox，与 G07、G08 的目录不重叠，不需要等待它们。
- Client 字段按 3.3 收敛。

【验收】
- 原有断言全部保留；go doc -all 与改动前一致。
- 合并前用 code-review-expert 审查；go test -race ./... 通过。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g09-sdk-fanbox
- 路径：/Users/flanchan/.codex/worktrees/refactor-g09-sdk-fanbox/pixiv-cli
- 分支：refactor/g09-sdk-fanbox
- 开始前用 gh 确认依赖已满足：G06（依赖已于 2026-10-23 调整，以本条为准）。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g09-sdk-fanbox /Users/flanchan/.codex/worktrees/refactor-g09-sdk-fanbox/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G10：MCP tools 每个产品一个包（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G10：MCP tools 每个产品一个包。

【本 goal】
工作项：A5（规格 6.3）。
- Pixiv、FANBOX 的 tools 各合成一个包，重命名 Register、In、handle 等同名声明，合并重复声明；端口类型沿用 toolhost.SDKPorts。
- 同步 AGENTS.md、pixiv-cli-mcp-tool skill、中英文 architecture.md 与 mcp-tools.md。

【验收】
- tools/list 与改动前完全一致，包括 annotations。
- 原有 tool 测试的断言全部保留；合并前用 code-review-expert 审查；go test -race ./... 通过。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g10-mcp-tools
- 路径：/Users/flanchan/.codex/worktrees/refactor-g10-mcp-tools/pixiv-cli
- 分支：refactor/g10-mcp-tools
- 开始前用 gh 确认依赖已满足：G05。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g10-mcp-tools /Users/flanchan/.codex/worktrees/refactor-g10-mcp-tools/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G11：CLI 命令支撑与列表入口收敛（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G11：CLI 命令支撑与列表入口收敛。

【本 goal】
工作项：A6（规格 6.4、3.1.2、3.4.2 的命令支撑例子）。
- root app、deps.Data、fanbox.Data 合为 cli/internal/command.Env；各命令的 Dependencies、Deps、Data 改为构造参数；cli/fanbox.go 只保留命令组定义。
- deps 与 listing 合并为 pixivcmd，删除 Request 转换；RunPooled* 收敛为 RunArtworks/RunNovels/RunUsers（Format、JSONKey 类型）；合并重复的输出与 flag 绑定函数。
- 可以修改 composition.go 中构造命令依赖的部分；配置读取部分归 G14。

【验收】
- pixiv --help 和各命令的 text/JSON/NDJSON 输出与改动前一致。
- PR 中列出被删除的依赖袋类型。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g11-cli-commands
- 路径：/Users/flanchan/.codex/worktrees/refactor-g11-cli-commands/pixiv-cli
- 分支：refactor/g11-cli-commands
- 开始前用 gh 确认依赖已满足：G05。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g11-cli-commands /Users/flanchan/.codex/worktrees/refactor-g11-cli-commands/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G12：survey 换成 x/term（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G12：survey 换成 x/term。

【本 goal】
工作项：B3（规格 6.7）。
- 四种提示改用 golang.org/x/term 与 bufio，从 go.mod 中移除 survey。

【验收】
- 密码不回显；非 TTY 时的报错、默认值、提示写入 out 的位置都与原来一致。
- Select 改为编号选择，写成 changelog 条目。
- go mod why -m github.com/AlecAivazis/survey/v2 显示该模块已不在依赖图中。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g12-cli-prompt
- 路径：/Users/flanchan/.codex/worktrees/refactor-g12-cli-prompt/pixiv-cli
- 分支：refactor/g12-cli-prompt
- 开始前用 gh 确认依赖已满足：G11。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g12-cli-prompt /Users/flanchan/.codex/worktrees/refactor-g12-cli-prompt/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G13：配置测试去重（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G13：配置测试去重。

【本 goal】
工作项：T1（规格 3.4.1 的第一个例子）。
- config/settings 的测试每条契约只断言一次，归到对应生产文件的测试中；删除 tags_red_test.go 等迁移期文件里的重复断言。

【验收】
- PR 中按 3.4.3 的表格列出删除或合并的测试，以及仍然覆盖该契约的测试。
- 生产代码不改。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g13-config-tests
- 路径：/Users/flanchan/.codex/worktrees/refactor-g13-config-tests/pixiv-cli
- 分支：refactor/g13-config-tests
- 开始前用 gh 确认依赖已满足：G05。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g13-config-tests /Users/flanchan/.codex/worktrees/refactor-g13-config-tests/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G14：Koanf 换成 go-toml，配置键常量（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G14：Koanf 换成 go-toml，配置键常量。

【本 goal】
工作项：B1，以及配置墓碑（T6）（规格 6.5、3.2）。
- 改为直接使用 go-toml/v2；只保留 Store 方法这一种入口。
- 新增 keys.go 的 settings.Key 常量和表驱动一致性测试；LogLevel、LogFormat 改为具名类型。
- 删除 removed_setting 墓碑前，确认旧配置文件中残留的已删除键在加载时被忽略；如果会导致启动失败，保留墓碑并记为假设。
- 负责修改 settings 旧入口在 CLI、MCP 中的调用处。

【验收】
- 来源优先级、显式空值、脱敏、稀疏写回、校验时机都与原来一致。
- pixiv --help 与改动前一致；go mod why -m 确认 koanf 已不在依赖图中。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g14-config-toml
- 路径：/Users/flanchan/.codex/worktrees/refactor-g14-config-toml/pixiv-cli
- 分支：refactor/g14-config-toml
- 开始前用 gh 确认依赖已满足：G13。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g14-config-toml /Users/flanchan/.codex/worktrees/refactor-g14-config-toml/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G15：resty 换成 net/http（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G15：resty 换成 net/http。

【本 goal】
工作项：B2（规格 6.6，以及 6.9 中 do/send 的结构，暂不加日志）。
- pixivapi 的发送与重试收敛为 do（重试循环）和 send（单次发送），构造参数改为 Config 结构体；form 请求显式设置 Content-Type；401 刷新、Retry-After、disableRetryAfterRetry 的语义不变。
- 保持 GetJSON 等被 SDK 调用的方法签名不变（G06–G09 正在使用）。

【验收】
- 用 httptest 比对改动前后的请求方法、路径、请求头和 form 编码。
- go mod why -m 确认 resty 已不在依赖图中；go test -race ./... 通过。
- 用真实读取测试确认请求正常。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g15-transport
- 路径：/Users/flanchan/.codex/worktrees/refactor-g15-transport/pixiv-cli
- 分支：refactor/g15-transport
- 开始前用 gh 确认依赖已满足：G05。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g15-transport /Users/flanchan/.codex/worktrees/refactor-g15-transport/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
允许用本机 Homebrew 安装的 pixiv-cli（v1.2.1）中已登录的 jjkjk（128042194）运行 Pixiv 真实读取测试，用 PIXIV_E2E_READ_USER_ID 指定。禁止使用 Flan（25649510）和默认账号 Remi（127975236）。不直接打开账号数据库，不在任何输出中出现 token。FANBOX、以图搜图和浏览器凭据的真实请求未授权。遇到 429 不重试、不换账号。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G16：存储换成 GORM + gormlite（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G16：存储换成 GORM + gormlite。

【本 goal】
工作项：B4（规格 6.8、B 组中 B4 的要求、第 4 节）。
- 已批准的依赖：gorm.io/gorm v1.31.2、github.com/ncruces/go-sqlite3 v0.35.x 及其 gormlite、julianday、go-sqlite3-wasm。不引入其他依赖。移除 modernc.org/sqlite。
- 使用驱动名 sqlite 的 5 个文件一起改为 sqlite3。禁止使用 AutoMigrate、gorm.Model、软删除、隐式时间戳；逐处检查单连接下的 Rows、QueryRow。
- 重新生成许可证文件，人工确认 go-sqlite3-wasm 的原许可证声明都已收录。

【验收】
- 先用改动前的代码在临时目录生成一个包含典型数据的数据库，再用新代码直接打开并完成迁移。
- 覆盖 revision 冲突、事务回滚，以及 false、0、空串、NULL 的写入。
- CI 中实际打开 Windows 路径，六平台交叉编译通过；记录链接模块数和二进制体积。
- 合并前用 code-review-expert 审查；go test -race ./... 通过。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g16-storage
- 路径：/Users/flanchan/.codex/worktrees/refactor-g16-storage/pixiv-cli
- 分支：refactor/g16-storage
- 开始前用 gh 确认依赖已满足：G05。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g16-storage /Users/flanchan/.codex/worktrees/refactor-g16-storage/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不得用真实账号库运行任何测试，只能使用临时目录中的数据库。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G17：拆分 e2e 目录（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G17：拆分 e2e 目录。

【本 goal】
工作项：A8（规格 A 组中“A8 的要求”）。
- 按去向表搬迁并按 3.4.1 精简测试；修改 release.yml 中只离线运行 e2e 的 job、platform-smoke.yml 的测试路径、中英文 development.md、pixiv-cli-test skill。
- platform-smoke.yml 中的 go test ./e2e -run '^TestPixivBinaryPackagedSmoke$' 改为 ./cmd/pixiv，按规格 7.0 第 10 条分两步：先单独提交并合并一个过渡 PR，让该步骤在检出的代码树中存在 cmd/pixiv/main_test.go 时运行新路径，否则运行 ./e2e（做法同 PR #163）；再开主 PR；主 PR 合并后，用一个小 PR 删除回退分支（同 PR #164）。

【验收】
- 过渡 PR 和回退清理 PR 都已合并，main 上的 platform-smoke.yml 只引用 ./cmd/pixiv。
- 未设置开关变量时，真实环境测试全部 SKIP，go test ./... 仍然离线。
- 搬迁后的 TestRealPixivSDKRead、TestRealPixivSDKLiveManifestRead 用真实账号运行通过；写入测试运行通过并确认清理完成。
- PR 中逐项列出删除的测试及依据。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g17-live-tests
- 路径：/Users/flanchan/.codex/worktrees/refactor-g17-live-tests/pixiv-cli
- 分支：refactor/g17-live-tests
- 开始前用 gh 确认依赖已满足：G05。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g17-live-tests /Users/flanchan/.codex/worktrees/refactor-g17-live-tests/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
允许用本机 Homebrew 安装的 pixiv-cli（v1.2.1）中已登录的 josh（128042265）运行 Pixiv 真实读取测试，用 PIXIV_E2E_READ_USER_ID 指定。禁止使用 Flan（25649510）和默认账号 Remi（127975236）。不直接打开账号数据库，不在任何输出中出现 token。FANBOX、以图搜图和浏览器凭据的真实请求未授权。遇到 429 不重试、不换账号。写入测试（收藏、关注、评论）只用 davi（128042328），以非默认账号身份显式指定，并确认测试结束后已清理。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G18：叶子包清理（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G18：叶子包清理。

【本 goal】
工作项：T5、T2、T6 在 media/ugoira、browsercookies/*、selfupdate/*、reversesearch/*、loginrelay 中的部分（规格 3.1.2、3.4）。
- reversesearch 的 Facade 按职责改名。

【验收】
- PR 中按 3.4.3 的表格列出删除或合并的测试和符号。
- 用户可见行为不变。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g18-leaf-packages
- 路径：/Users/flanchan/.codex/worktrees/refactor-g18-leaf-packages/pixiv-cli
- 分支：refactor/g18-leaf-packages
- 开始前用 gh 确认依赖已满足：G05。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g18-leaf-packages /Users/flanchan/.codex/worktrees/refactor-g18-leaf-packages/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G19：合并 tools、ci 进 scripts（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G19：合并 tools、ci 进 scripts。

【本 goal】
工作项：S1（规格 5.4、S 组、第 10 节中 S1 的风险）。
- 按 5.4 合并 tools/、ci/，每个工具一个 package main；删除 publicapi、browser-evidence 及其工具和 fixture、homebrew-prepublish-verify。
- 更新所有 workflow、pixiv-cli-pr（prmeta 与命令白名单的路径）、pixiv-cli-ci、pixiv-cli-native、pixiv-cli-test、AGENTS.md 以及中英文维护文档。
- platform-smoke.yml 与 container-smoke.yml 由 main 上的定义运行，却检出 PR head 的代码（规格 7.0 第 10 条）。其中引用的 ./tools/platformmatrix、./scripts/cmd/licensebundle、./scripts/cmd/nativeevidence、./scripts/tests/installers、./scripts/tests/containerrelease 会被本 goal 移动，因此分两步：先单独提交并合并一个过渡 PR，让这些步骤在检出的代码树中存在新路径时运行新路径，否则运行旧路径（做法同 PR #163）；再开主 PR；主 PR 合并后，用一个小 PR 删除全部回退分支（同 PR #164）。以合并时的 main 为准重新 grep 一遍这两个 workflow，不只依赖这份清单。

【验收】
- 过渡 PR 和回退清理 PR 都已合并，main 上的两个 smoke workflow 只引用新路径。
- 每个 workflow 引用的路径都存在；go test ./scripts/... 通过。
- 本 PR 自身的 Classify、Validate PR metadata 检查通过。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g19-scripts-move
- 路径：/Users/flanchan/.codex/worktrees/refactor-g19-scripts-move/pixiv-cli
- 分支：refactor/g19-scripts-move
- 开始前用 gh 确认依赖已满足：无。ClawHub v1.2.1 的发布状态不阻塞本 goal（依赖已于 2026-10-23 调整，以本条为准）。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g19-scripts-move /Users/flanchan/.codex/worktrees/refactor-g19-scripts-move/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G20：scripts 与 CI 精简（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G20：scripts 与 CI 精简。

【本 goal】
工作项：S2，范围扩大（2026-10-23）。目标：scripts/ 与 .github/ 只留下发布、安装、必需检查和 PR 治理实际执行的东西，并用最简单的实现。
- 删除：
  - nativeevidence 工具、native-evidence.yml（只能手动触发，最后一次运行在 2026-09-22）、platform-smoke.yml 中的 nativeevidence record 步骤，以及 skill 和文档中的引用。platform-smoke 在各平台的真实运行本身就是证据。
  - linuxabi：先确认 release 产物是静态链接（CGO_ENABLED=0），确认后删除工具及 build-platform.sh、test-build-platform.sh 中的调用；不是静态链接则保留并在 PR 中说明。
  - auto-assign.yml。
  - CI 与发布流程都不调用、只有文档提到的脚本（例如 test-build-platform.sh、test-homebrew-formula.sh）。
  - 其余没有 workflow、发布流程、README 或安装路径调用的脚本和工具：逐个 grep 调用方后判断。
- 改用 sh 只限很小的工具：能缩成约 20 行 jq 或 sh、并且只在 Linux runner 上运行的 Go 工具才改写，目前只有 platformmatrix（改为 build/matrix.sh）。发布链路、PR 治理、licensebundle 保持 Go，只做合并与删减。不引入新依赖或新工具。
- 撤销 scripts/internal/：把共享包的使用方合并为一个带子命令的工具，共享包随之消失。
- 预期目录结构（2026-10-24 与维护者确认；文件名以实际代码为准）：
  - scripts/ 根目录：install.sh、install.cmd（README 中公开的 raw URL，不动）、build.sh（本机构建入口）。
  - scripts/build/：platform.sh（← build-platform.sh）、package.sh（← package-release.sh）、platforms.json（← ci/platforms.json）、matrix.sh（← platformmatrix；保留 --capability、--registry、--github-output 参数，默认 registry 改为 scripts/build/platforms.json；保留对 registry 的校验与报错；无 --github-output 时把 matrix JSON 写到 stdout，提供时追加 matrix=<json>）、licensebundle/（Go）。
  - scripts/release/：一个 Go 工具，由 release、releaseassets、releasenotes、homebrewformula、homebrewrecovery 与 internal/releasenotesrender 合并，按领域分文件。release 现有的 9 个子命令（verify-source、verify-published-release、verify-handoff、write-handoff、verify-handoff-set、verify-handoff-identity、verify-artifact-set、verify-container-set、checksums）名称和输出契约不变；并入的工具各成为新的子命令（例如 assets、notes、formula、recover，以实际功能命名）。
  - scripts/pr/：一个 Go 工具，以及 command-whitelist.txt；由 prmeta、verification 与 internal/verificationpolicy 合并。verification 现有的 init、execute、render 与 prmeta 的各个模式都保留为子命令，语义不变。
  - PR 中列出每个旧调用（workflow、脚本、文档中的 go run ./scripts/... 命令）到新调用的完整对照，并同步修改全部调用处。
  - scripts/ci/：classify-change-scope.sh、race-gate.sh（← test-race-gate.sh）、test-package-release.sh。
  - scripts/citest/：精简后保留的测试。
  - .github/workflows/：删除 native-evidence.yml、auto-assign.yml 后剩 12 个；ci-change-scope.gitignore、labeler.yml 中的路径同步更新。
- licensebundle：删除暂存、备份、回滚和 bundleFileOps，改为删除旧目录后直接写入，--check 生成到临时目录后比对；--check 从 7 处减为 2 处（ci.yml 的 Quality、release.yml 的 build）。
- 测试：citest 中只断言 YAML、Dockerfile 文本的测试删除；保留真实执行代码的（installers、changescope、dispatch_prefilter）、workflowpins，以及 containerrelease 中固定 base digest、非 root 运行、不内嵌密钥或状态、只使用受信 release 产物这 4 个。各工具自身的测试只保留“出错会悄悄产出错误发布物”的部分：校验和、归档命名、formula 版本、release 正文渲染。改写为 sh 的工具，其 Go 单元测试随之删除。
- PR 治理体系（/test 验证、无效 PR 关闭、prmeta）保留功能，只简化实现（规格第 8 节）。
- 移动或删除 platform-smoke.yml、container-smoke.yml 引用的路径时，按规格 7.0 第 10 条：先合并过渡 PR，再合并主 PR，最后用小 PR 删除回退。
- 同步 AGENTS.md、pixiv-cli-ci、pixiv-cli-native、pixiv-cli-test、pixiv-cli-pr、pixiv-cli-release-notes skill，以及中英文 ci.md、release.md、development.md、architecture.md。

【验收】
- matrix.sh 在相同输入上与原 platformmatrix 的输出逐字节一致；合并后的 release、pr 工具对 release notes、formula、校验和文件、归档名、PR 元数据的输出与改动前逐字节一致；比对命令与结果写进 PR。
- licensebundle 的生成结果与改动前逐字节一致。
- test-package-release.sh 与 CI 中的发布前检查通过；过渡 PR 和回退清理 PR 都已合并，smoke workflow 只引用存在的路径。
- scripts/internal/ 已删除；scripts/ 的 Go 包约为 4 个（licensebundle、release、pr、citest）。
- PR 中列出删除的工具、workflow、脚本和测试及依据，以及 scripts/、.github/ 的行数和 Go 包数变化。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g20-scripts-slim
- 路径：/Users/flanchan/.codex/worktrees/refactor-g20-scripts-slim/pixiv-cli
- 分支：refactor/g20-scripts-slim
- 开始前用 gh 确认依赖已满足：G19（依赖已于 2026-10-23 调整，以本条为准）。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g20-scripts-slim /Users/flanchan/.codex/worktrees/refactor-g20-scripts-slim/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G21：workflow 与脚本中的编号引用（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G21：workflow 与脚本中的编号引用。

【本 goal】
工作项：T4 中 .github/workflows 与 scripts 的部分（规格 3.4.2 的 P-h）。
- 把 §14–§24、R10/R11、R14 一类指向仓库外设计文档的编号（2026-10-23 统计约 60 处，集中在 scripts/release、scripts/prmeta、scripts/citest 与 publish-*.yml、pr-verification.yml、ci.yml、release.yml；以 G20 合并后的 main 重新统计）改为直接写出原因或删除。RFC 章节号保留。

【验收】
- 只改注释；git diff --check 通过；相关 workflow 检查通过。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g21-workflow-refs
- 路径：/Users/flanchan/.codex/worktrees/refactor-g21-workflow-refs/pixiv-cli
- 分支：refactor/g21-workflow-refs
- 开始前用 gh 确认依赖已满足：G20（依赖已于 2026-10-23 调整，以本条为准）。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g21-workflow-refs /Users/flanchan/.codex/worktrees/refactor-g21-workflow-refs/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G22：slog 日志

```text
/goal 执行 pixiv-cli 重构规格的 G22：slog 日志。

【本 goal】
工作项：C1（规格 6.9、D4、3.3）。
- 两个 SDK 增加 Options.Logger；pixivapi、fanboxapi 在 do/send 与各决策点记录日志；SDK 方法入口用 WithOperation 登记操作名。
- 新建 internal/logging（Handler、ctx 调用信息、属性键常量），取代 shared/diagnostics 与 cli/diagnostics；CLI、MCP 执行边界接入；LogLevel 新增 warn、error。按 AGENTS.md 执行 Red/Green。
- G24、G26 先于本 goal 合并：media/downloader、pixivaccount、session 中的诊断调用以它们合并后的代码为准替换。
- 触及的代码（composition.go、execution.go、pixivapi、fanboxapi、toolhost）按规格 3.4.2 的 P-m 删除过度防御：重复校验、对构造时一定设置的依赖判 nil、悄悄降级的分支。新写的日志代码不为“以防万一”加判空。
- 本 goal 触及的文件中，超过约 100 行的编排函数，按真实阶段拆出有名字的步骤函数（只拆能用一个名字说清职责的部分，不为缩短而拆）；超过约 600 行、包含多个主题的文件按主题拆分。现状：composition.go 781 行，其中 newPixivSDKPorts、runPixivMCP 各约 130 行。

【验收】
- CLI 的数据 stdout 和 MCP stdio 的 stdout 中没有日志。
- 日志中不出现 token、Cookie、query、响应体或签名 URL，并有对应测试。
- stderr 诊断格式的变化写成 changelog 条目；go test -race ./... 通过。
- 用真实读取测试在 debug 级别确认能看到请求和决策日志。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g22-logging
- 路径：/Users/flanchan/.codex/worktrees/refactor-g22-logging/pixiv-cli
- 分支：refactor/g22-logging
- 开始前用 gh 确认依赖已满足：G07、G08、G09、G10、G11、G14、G15、G24、G26（依赖已于 2026-10-23 调整，以本条为准）。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g22-logging /Users/flanchan/.codex/worktrees/refactor-g22-logging/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
允许用本机 Homebrew 安装的 pixiv-cli（v1.2.1）中已登录的 jjkjk（128042194）运行 Pixiv 真实读取测试，用 PIXIV_E2E_READ_USER_ID 指定。禁止使用 Flan（25649510）和默认账号 Remi（127975236）。不直接打开账号数据库，不在任何输出中出现 token。FANBOX、以图搜图和浏览器凭据的真实请求未授权。遇到 429 不重试、不换账号。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G23：共享机制包清理（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G23：共享机制包清理。

【本 goal】
工作项：T5，以及 T2、T6 在 lifecycle、pagination、record、searchfilter、httpclient、plaintext、version 中的部分（规格 3.1.2、3.4）。
- 按 3.4.2 的全部标准检查生产代码，按 3.4.1 检查测试。
- 被其他包调用的导出 API 需要修改时，一并修改调用处。
- 这些共享包按规格 3.4.2 的 P-m 删除过度防御：同一前提在多层重复校验、对调用方已保证的参数判 nil、悄悄返回零值的分支。

【验收】
- PR 中按 3.4.3 的表格列出删除的防御性判断及依据（P-m）。
- PR 中按 3.4.3 的表格列出删除或合并的测试和符号。
- 用户可见行为不变；触及生命周期、持久化或并发时 go test -race ./... 通过。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g23-shared-mechanisms
- 路径：/Users/flanchan/.codex/worktrees/refactor-g23-shared-mechanisms/pixiv-cli
- 分支：refactor/g23-shared-mechanisms
- 开始前用 gh 确认依赖已满足：G08、G14、G19（依赖已于 2026-10-23 调整，以本条为准）。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g23-shared-mechanisms /Users/flanchan/.codex/worktrees/refactor-g23-shared-mechanisms/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G24：下载与文件名包清理（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G24：下载与文件名包清理。

【本 goal】
工作项：T5，以及 T2、T6 在 media/downloader、media/filename（DownloadManager、Manager 按职责改名） 中的部分（规格 3.1.2、3.4）。
- 按 3.4.2 的全部标准检查生产代码，按 3.4.1 检查测试。
- 被其他包调用的导出 API 需要修改时，一并修改调用处。
- 本 goal 在 G22 之前进行：现有的 internal/logging 诊断调用保持原样（可以随代码移动），不改成 slog；日志改造由 G22 在本 goal 合并后的代码上完成。

【验收】
- PR 中按 3.4.3 的表格列出删除或合并的测试和符号。
- 用户可见行为不变；触及生命周期、持久化或并发时 go test -race ./... 通过。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g24-media
- 路径：/Users/flanchan/.codex/worktrees/refactor-g24-media/pixiv-cli
- 分支：refactor/g24-media
- 开始前用 gh 确认依赖已满足：G05（依赖已于 2026-10-23 调整，以本条为准）。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g24-media /Users/flanchan/.codex/worktrees/refactor-g24-media/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G25：MCP 运行时包清理（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G25：MCP 运行时包清理。

【本 goal】
工作项：T5，以及 T2、T6 在 mcpserver/auth、mcpserver/pixiv/{accounts,gallery,mediaproxy}（Manager、LoginManager 按职责改名） 中的部分（规格 3.1.2、3.4）。
- 按 3.4.2 的全部标准检查生产代码，按 3.4.1 检查测试。
- 被其他包调用的导出 API 需要修改时，一并修改调用处。
- 本 goal 在 G22 之前进行：现有的 internal/logging 诊断调用保持原样（可以随代码移动），不改成 slog；日志改造由 G22 在本 goal 合并后的代码上完成。

【验收】
- PR 中按 3.4.3 的表格列出删除或合并的测试和符号。
- 用户可见行为不变；触及生命周期、持久化或并发时 go test -race ./... 通过。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g25-mcp-runtime
- 路径：/Users/flanchan/.codex/worktrees/refactor-g25-mcp-runtime/pixiv-cli
- 分支：refactor/g25-mcp-runtime
- 开始前用 gh 确认依赖已满足：G05（依赖已于 2026-10-23 调整，以本条为准）。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g25-mcp-runtime /Users/flanchan/.codex/worktrees/refactor-g25-mcp-runtime/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G26：账号与会话包清理（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G26：账号与会话包清理。

【本 goal】
工作项：T5，以及 T2、T6 在 pixivaccount、pixivaccount/pool、fanboxaccount、session 中的部分（规格 3.1.2、3.4）。
- 按 3.4.2 的全部标准检查生产代码，按 3.4.1 检查测试。
- 被其他包调用的导出 API 需要修改时，一并修改调用处。
- 本 goal 在 G22 之前进行：现有的 internal/logging 诊断调用保持原样（可以随代码移动），不改成 slog；日志改造由 G22 在本 goal 合并后的代码上完成。

【验收】
- PR 中按 3.4.3 的表格列出删除或合并的测试和符号。
- 用户可见行为不变；触及生命周期、持久化或并发时 go test -race ./... 通过。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g26-accounts
- 路径：/Users/flanchan/.codex/worktrees/refactor-g26-accounts/pixiv-cli
- 分支：refactor/g26-accounts
- 开始前用 gh 确认依赖已满足：G05（依赖已于 2026-10-23 调整，以本条为准）。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g26-accounts /Users/flanchan/.codex/worktrees/refactor-g26-accounts/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G27：登录命令清理（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G27：登录命令清理。

【本 goal】
工作项：T5，以及 T2、T6 在 cli/auth、cli/auth/loginhelper（16 个 SetXxx 全局注入点改为结构体字段，见 3.4.2 的例子） 中的部分（规格 3.1.2、3.4）。
- 按 3.4.2 的全部标准检查生产代码，按 3.4.1 检查测试。
- 被其他包调用的导出 API 需要修改时，一并修改调用处。

【验收】
- PR 中按 3.4.3 的表格列出删除或合并的测试和符号。
- 用户可见行为不变；触及生命周期、持久化或并发时 go test -race ./... 通过。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g27-cli-auth
- 路径：/Users/flanchan/.codex/worktrees/refactor-g27-cli-auth/pixiv-cli
- 分支：refactor/g27-cli-auth
- 开始前用 gh 确认依赖已满足：G12（依赖已于 2026-10-23 调整，以本条为准）。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g27-cli-auth /Users/flanchan/.codex/worktrees/refactor-g27-cli-auth/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G28：全仓库扫尾

```text
/goal 执行 pixiv-cli 重构规格的 G28：全仓库扫尾。

【本 goal】
工作项：重新扫描只被测试引用的导出函数（T2），处理剩余的无对应生产文件测试（T3）、scripts/ 与 .github/ 以外的编号引用（T4；scripts/ 与 .github/ 由 G21 负责），以及其余兼容层，包括 illustration 别名（T6，写成 changelog 条目）。
- 全仓库生产代码中，超过约 100 行的编排函数，按真实阶段拆出有名字的步骤函数（只拆能用一个名字说清职责的部分，不为缩短而拆）；超过约 600 行、包含多个主题的文件按主题拆分。先处理此前 goal 未覆盖的部分，例如 mcpserver/pixiv/tools 中 recommendedHandler、artworkMediaHandler、showGalleryHandler（各约 140–150 行），cli/search 的 run（约 136 行），cli/bookmark/bookmark.go（906 行）。media/ugoira/jpeg 解码器照搬标准库 image/jpeg 的算法结构，保持原样。
- sdk/fanbox 按 G08 在 sdk/pixiv 中的做法组织文件：请求类型移到使用它的方法旁边，删除 request.go；只被一个领域使用的响应结构与映射放进 creator.go、post.go 等领域文件，共用部分放在 response.go，删除 wire.go；models.go 保留。改名与移动单独提交。
- 编号引用：清理代码与文档中指向仓库外设计文档的编号，以及指向本次重构 goal、阶段的说明（例如“G08 删除”“阶段 3 并入”）。RFC 章节号（RFC 6749 §4.1.2 等）和函数内按阶段编号的步骤注释是有效引用，保留。
- 删除生产代码中与真实包名不一致的 import 别名（例如 protocol→pixivapi/fanboxapi、commands/requirements→command、config/configapp→settings、accountpixiv→pixivaccount、filesecret→secretfile、filereplace→atomicfile、versionpkg→version），调用处改用真实包名；以合并时的 main 重新统计。
- 全仓库生产代码按规格 3.4.2 的 P-m 扫描过度防御，覆盖此前各 goal 已合并的代码：从 if x == nil || x.y == nil 这类复合判空（2026-10-24 统计 64 处）、重复的参数校验、返回零值的降级分支入手。信任边界上的校验、公开 SDK 参数校验、凭据拷贝保留。

【验收】
- PR 中列出删除的防御性判断及依据；保留下来但看起来像防御的判断，说明它守护的是哪一类边界。
- PR 中按 3.4.3 的表格列出每项删除及依据。
- 以合并时的 main 重新 grep，scripts/ 与 .github/ 以外，除 RFC 章节号外不再有指向仓库外文档或重构 goal 的编号。
- PR 中列出拆分的函数和文件（原行数、拆出的步骤），以及删除的 import 别名数量；仍超过约 100 行而未拆的函数逐个说明理由。
- 用户可见行为不变（changelog 条目除外）。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g28-sweep
- 路径：/Users/flanchan/.codex/worktrees/refactor-g28-sweep/pixiv-cli
- 分支：refactor/g28-sweep
- 开始前用 gh 确认依赖已满足：G22、G23、G24、G25、G26、G27（依赖已于 2026-10-23 调整，以本条为准）。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g28-sweep /Users/flanchan/.codex/worktrees/refactor-g28-sweep/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G29：注释整理：sdk/、cmd/（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G29：注释整理：sdk/、cmd/。

【本 goal】
工作项：C3 与注释整理，范围 sdk/、cmd/（规格 3.5、3.5.1，以及 pixiv-cli-code-commenting）。
- 补齐文件头注释。
- 按 3.5.1 逐条整理范围内的注释：删除复述名字与签名、没有信息量的注释；“X 是……”“不与……共享……”这类生硬句式改成自然的说法；保留并写清调用约定、原因、不变量、副作用与所有权。
- sdk/ 为每个导出符号保留文档注释（pkg.go.dev 展示）。
- 只改注释，不改代码；编译器与工具指令（//go:build、//go:embed、//go:generate 等）不动。

【验收】
- 范围内每个需要头注释的文件都有 1–3 行职责说明；Go 文件头注释与 package 子句之间有空行，//go:build 位于最前。
- PR 中给出若干条改写前后的对照，并列出删除的注释数量。
- go build ./... 和 go vet ./... 通过。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g29-headers-sdk
- 路径：/Users/flanchan/.codex/worktrees/refactor-g29-headers-sdk/pixiv-cli
- 分支：refactor/g29-headers-sdk
- 开始前用 gh 确认依赖已满足：G39、G40、G41、G42（依赖已于 2026-10-24 调整，以本条为准）。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g29-headers-sdk /Users/flanchan/.codex/worktrees/refactor-g29-headers-sdk/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G30：注释整理：internal/cli/（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G30：注释整理：internal/cli/。

【本 goal】
工作项：C3 与注释整理，范围 internal/cli/（规格 3.5、3.5.1，以及 pixiv-cli-code-commenting）。
- 补齐文件头注释。
- 按 3.5.1 逐条整理范围内的注释：删除复述名字与签名、没有信息量的注释；“X 是……”“不与……共享……”这类生硬句式改成自然的说法；保留并写清调用约定、原因、不变量、副作用与所有权。
- 只改注释，不改代码；编译器与工具指令（//go:build、//go:embed、//go:generate 等）不动。

【验收】
- 范围内每个需要头注释的文件都有 1–3 行职责说明；Go 文件头注释与 package 子句之间有空行，//go:build 位于最前。
- PR 中给出若干条改写前后的对照，并列出删除的注释数量。
- go build ./... 和 go vet ./... 通过。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g30-headers-cli
- 路径：/Users/flanchan/.codex/worktrees/refactor-g30-headers-cli/pixiv-cli
- 分支：refactor/g30-headers-cli
- 开始前用 gh 确认依赖已满足：G39、G40、G41、G42（依赖已于 2026-10-24 调整，以本条为准）。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g30-headers-cli /Users/flanchan/.codex/worktrees/refactor-g30-headers-cli/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G31：注释整理：internal/mcpserver/（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G31：注释整理：internal/mcpserver/。

【本 goal】
工作项：C3 与注释整理，范围 internal/mcpserver/（规格 3.5、3.5.1，以及 pixiv-cli-code-commenting）。
- 补齐文件头注释。
- 按 3.5.1 逐条整理范围内的注释：删除复述名字与签名、没有信息量的注释；“X 是……”“不与……共享……”这类生硬句式改成自然的说法；保留并写清调用约定、原因、不变量、副作用与所有权。
- 只改注释，不改代码；编译器与工具指令（//go:build、//go:embed、//go:generate 等）不动。

【验收】
- 范围内每个需要头注释的文件都有 1–3 行职责说明；Go 文件头注释与 package 子句之间有空行，//go:build 位于最前。
- PR 中给出若干条改写前后的对照，并列出删除的注释数量。
- go build ./... 和 go vet ./... 通过。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g31-headers-mcp
- 路径：/Users/flanchan/.codex/worktrees/refactor-g31-headers-mcp/pixiv-cli
- 分支：refactor/g31-headers-mcp
- 开始前用 gh 确认依赖已满足：G39、G40、G41、G42（依赖已于 2026-10-24 调整，以本条为准）。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g31-headers-mcp /Users/flanchan/.codex/worktrees/refactor-g31-headers-mcp/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G32：注释整理：internal/ 中除 cli、mcpserver 以外的目录，以及 frontend/（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G32：注释整理：internal/ 中除 cli、mcpserver 以外的目录，以及 frontend/。

【本 goal】
工作项：C3 与注释整理，范围 internal/ 中除 cli、mcpserver 以外的目录，以及 frontend/（规格 3.5、3.5.1，以及 pixiv-cli-code-commenting）。
- 补齐文件头注释。
- 按 3.5.1 逐条整理范围内的注释：删除复述名字与签名、没有信息量的注释；“X 是……”“不与……共享……”这类生硬句式改成自然的说法；保留并写清调用约定、原因、不变量、副作用与所有权。
- frontend/ 的 HTML、JS、CSS 使用各自的注释语法。
- 只改注释，不改代码；编译器与工具指令（//go:build、//go:embed、//go:generate 等）不动。

【验收】
- 范围内每个需要头注释的文件都有 1–3 行职责说明；Go 文件头注释与 package 子句之间有空行，//go:build 位于最前。
- PR 中给出若干条改写前后的对照，并列出删除的注释数量。
- go build ./... 和 go vet ./... 通过。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g32-headers-internal
- 路径：/Users/flanchan/.codex/worktrees/refactor-g32-headers-internal/pixiv-cli
- 分支：refactor/g32-headers-internal
- 开始前用 gh 确认依赖已满足：G39、G40、G41、G42（依赖已于 2026-10-24 调整，以本条为准）。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g32-headers-internal /Users/flanchan/.codex/worktrees/refactor-g32-headers-internal/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G33：注释整理：scripts/、.github/（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G33：注释整理：scripts/、.github/。

【本 goal】
工作项：C3 与注释整理，范围 scripts/、.github/（规格 3.5、3.5.1，以及 pixiv-cli-code-commenting）。
- 补齐文件头注释。
- 按 3.5.1 逐条整理范围内的注释：删除复述名字与签名、没有信息量的注释；“X 是……”“不与……共享……”这类生硬句式改成自然的说法；保留并写清调用约定、原因、不变量、副作用与所有权。
- Shell 脚本写在 shebang 之后，workflow YAML 写在 name: 之前。
- 只改注释，不改代码；编译器与工具指令（//go:build、//go:embed、//go:generate 等）不动。

【验收】
- 范围内每个需要头注释的文件都有 1–3 行职责说明；Go 文件头注释与 package 子句之间有空行，//go:build 位于最前。
- PR 中给出若干条改写前后的对照，并列出删除的注释数量。
- go build ./... 和 go vet ./... 通过。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g33-headers-scripts
- 路径：/Users/flanchan/.codex/worktrees/refactor-g33-headers-scripts/pixiv-cli
- 分支：refactor/g33-headers-scripts
- 开始前用 gh 确认依赖已满足：G21、G39、G40、G41、G42（依赖已于 2026-10-24 调整，以本条为准）。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g33-headers-scripts /Users/flanchan/.codex/worktrees/refactor-g33-headers-scripts/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G35：测试精简：sdk、pixivapi、fanboxapi（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G35：测试精简：sdk、pixivapi、fanboxapi。

【本 goal】
工作项：测试精简（规格 3.4.1，含 T-g、T-h 与契约分层表；3.4.3）。范围：sdk/、internal/pixivapi、internal/fanboxapi 的测试。
- 对范围内每个测试文件套用 3.4.1 的全部标准：删除、去重合并，或移到对应生产文件的 x_test.go。
- 重点是 T-g：sdk/pixiv、sdk/fanbox 中共用一个实现的端点族（user_previews、illusts、novels、comments 等），每族保留一个正常用例和一张失败模式表；不再保留 A3、A4 时期逐端点的同一组断言。现状：wire_test.go 2120 行、pixiv_test.go 2043 行。
- 映射正确性只在 SDK 断言（T-h）；CLI、MCP 层的重复映射断言由 G36、G37 删除。
- 真实环境测试（live_test.go、live_mutation_test.go）：保留保护真实账号的离线测试和写入后的清理确认，合并重复的流程与辅助函数。
- 只删减和合并测试，不改生产代码；确需为测试改动生产代码的，在 PR 中说明理由。
- 不新增测试，除非删除后某个长期契约失去覆盖。

【验收】
- go test ./... 与 go vet ./... 通过；真实环境测试在未开启时全部 SKIP。
- PR 中按 3.4.3 的表格列出删除或合并的测试、依据，以及仍覆盖该契约的测试。
- 凭据不泄露、数据完整性与持久化、取消与错误传播相关的保留测试，断言不变。
- PR 中列出范围内测试与生产代码的行数变化。预期测试约降到生产代码的一半；明显高于这个比例时说明原因（参考，不是门槛）。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g35-tests-sdk
- 路径：/Users/flanchan/.codex/worktrees/refactor-g35-tests-sdk/pixiv-cli
- 分支：refactor/g35-tests-sdk
- 开始前用 gh 确认依赖已满足：G28。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g35-tests-sdk /Users/flanchan/.codex/worktrees/refactor-g35-tests-sdk/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G36：测试精简：internal/mcpserver（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G36：测试精简：internal/mcpserver。

【本 goal】
工作项：测试精简（规格 3.4.1，含 T-g、T-h 与契约分层表；3.4.3）。范围：internal/mcpserver/ 的测试。
- 对范围内每个测试文件套用 3.4.1 的全部标准：删除、去重合并，或移到对应生产文件的 x_test.go。
- MCP 层只断言 tool schema、结构化输出外形、isError、注册与鉴权、stdio 纯净、凭据与签名 URL 不进入输出（例如 TestToolOutputValuesDoNotLeakResourceTransportCanary）。重复检查 SDK 映射字段的测试删除（T-h），例如 pixiv/pixiv_sdk_wire_test.go（1140 行）。
- 没有对应生产文件的测试（T3）合并到其所有者的 x_test.go，合并时不搬运重复断言。现状：internal/mcpserver 测试 15.6k 行、生产代码 11.2k 行。
- 只删减和合并测试，不改生产代码；确需为测试改动生产代码的，在 PR 中说明理由。
- 不新增测试，除非删除后某个长期契约失去覆盖。

【验收】
- go test ./... 与 go vet ./... 通过；真实环境测试在未开启时全部 SKIP。
- PR 中按 3.4.3 的表格列出删除或合并的测试、依据，以及仍覆盖该契约的测试。
- 凭据不泄露、数据完整性与持久化、取消与错误传播相关的保留测试，断言不变。
- PR 中列出范围内测试与生产代码的行数变化。预期测试约降到生产代码的一半；明显高于这个比例时说明原因（参考，不是门槛）。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g36-tests-mcp
- 路径：/Users/flanchan/.codex/worktrees/refactor-g36-tests-mcp/pixiv-cli
- 分支：refactor/g36-tests-mcp
- 开始前用 gh 确认依赖已满足：G28。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g36-tests-mcp /Users/flanchan/.codex/worktrees/refactor-g36-tests-mcp/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G37：测试精简：internal/cli、cmd（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G37：测试精简：internal/cli、cmd。

【本 goal】
工作项：测试精简（规格 3.4.1，含 T-g、T-h 与契约分层表；3.4.3）。范围：internal/cli/、cmd/ 的测试。
- 对范围内每个测试文件套用 3.4.1 的全部标准：删除、去重合并，或移到对应生产文件的 x_test.go。
- CLI 层只断言 flag 解析、文本/JSON/NDJSON 输出外形、退出码、stdin 输入、非 TTY 行为；重复检查 SDK 映射字段的测试删除（T-h）。现状：root_test.go 1405 行，search、detail、bookmark 的测试各约 800–960 行。
- cmd/pixiv 中编译二进制后执行的测试保留，只合并重复的环境隔离辅助函数。
- 只删减和合并测试，不改生产代码；确需为测试改动生产代码的，在 PR 中说明理由。
- 不新增测试，除非删除后某个长期契约失去覆盖。

【验收】
- go test ./... 与 go vet ./... 通过；真实环境测试在未开启时全部 SKIP。
- PR 中按 3.4.3 的表格列出删除或合并的测试、依据，以及仍覆盖该契约的测试。
- 凭据不泄露、数据完整性与持久化、取消与错误传播相关的保留测试，断言不变。
- PR 中列出范围内测试与生产代码的行数变化。预期测试约降到生产代码的一半；明显高于这个比例时说明原因（参考，不是门槛）。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g37-tests-cli
- 路径：/Users/flanchan/.codex/worktrees/refactor-g37-tests-cli/pixiv-cli
- 分支：refactor/g37-tests-cli
- 开始前用 gh 确认依赖已满足：G28。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g37-tests-cli /Users/flanchan/.codex/worktrees/refactor-g37-tests-cli/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G38：测试精简：internal 其余目录（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G38：测试精简：internal 其余目录。

【本 goal】
工作项：测试精简（规格 3.4.1，含 T-g、T-h 与契约分层表；3.4.3）。范围：internal/ 中除 cli、mcpserver、pixivapi、fanboxapi 以外的目录（media、selfupdate、reversesearch、config、pixivaccount、fanboxaccount、session、storage、browsercookies、loginrelay、pagination、record、searchfilter、logging 等） 的测试。
- 对范围内每个测试文件套用 3.4.1 的全部标准：删除、去重合并，或移到对应生产文件的 x_test.go。
- 保留持久化、revision 冲突、事务回滚、文件原子替换与权限、凭据不泄露、取消传播的测试，断言不变。
- 其余按 3.4.1 处理；同一行为的多组输入合并为表驱动或只留代表性用例。现状比例偏高的目录：pagination（测试约为生产代码的 2.1 倍）、record（1.6 倍）、session（1.5 倍）、reversesearch（1.2 倍）。
- 只删减和合并测试，不改生产代码；确需为测试改动生产代码的，在 PR 中说明理由。
- 不新增测试，除非删除后某个长期契约失去覆盖。

【验收】
- go test ./... 与 go vet ./... 通过；真实环境测试在未开启时全部 SKIP。
- PR 中按 3.4.3 的表格列出删除或合并的测试、依据，以及仍覆盖该契约的测试。
- 凭据不泄露、数据完整性与持久化、取消与错误传播相关的保留测试，断言不变。
- PR 中列出范围内测试与生产代码的行数变化。预期测试约降到生产代码的一半；明显高于这个比例时说明原因（参考，不是门槛）。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g38-tests-internal
- 路径：/Users/flanchan/.codex/worktrees/refactor-g38-tests-internal/pixiv-cli
- 分支：refactor/g38-tests-internal
- 开始前用 gh 确认依赖已满足：G28。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g38-tests-internal /Users/flanchan/.codex/worktrees/refactor-g38-tests-internal/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G39：逐包审查与修正：sdk、pixivapi、fanboxapi（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G39：逐包审查与修正：sdk、pixivapi、fanboxapi。

【本 goal】
工作项：逐包审查与修正，范围 sdk/、internal/pixivapi、internal/fanboxapi。建议由能力更强的模型执行。
- 逐包审查：范围内每个生产包都按规格 3.1（命名与放置）、3.3（横切关注点）、3.4.2 的 P-a 到 P-m 对照一遍；重点是主要为测试而存在的生产代码（P-a）、同一套方法或机制在多个类型上各写一份（P-d）、过度防御（P-m）。发现的问题直接修正。
- 测试：按规格 3.4.4 的清单核对范围内的测试；清单内缺失的按 3.4.4 的上限补回最小用例，清单外的不新增。
- 规格与事实不符的地方按 7.0 第 8 条处理，并写进最终汇报，供 G34 更新文档和 skill。
- 公开 SDK：比对 v1.2.1 tag 与当前 main 的 go doc -all（sdk、sdk/pixiv、sdk/fanbox），列出全部破坏性变更，作为 changelog 的 SDK 部分写进最终汇报。
- 映射：核对 G08 映射取舍带来的变化——推荐用户中作品样本新增的可选字段；上游缺少 tags 时模型中是 nil 还是空切片（G05 的契约是映射层保证切片非 nil）。不符合契约的在映射层修正。
- 日志：pixivapi、fanboxapi 记录的日志不包含 query、请求头、响应体、token、Cookie、签名 URL。

【验收】
- PR 或最终汇报中附逐包表：包名、发现的问题、处理结果（已修正 / 不修正及原因）；范围内每个生产包都在表中。
- 每类问题的修正 PR 都已合并。
- go test ./... -count=1、go vet ./...、sh scripts/build.sh 通过；修正涉及并发、持久化或共享运行时时，go test -race ./... 通过。
【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g39-audit
- 路径：/Users/flanchan/.codex/worktrees/refactor-g39-audit/pixiv-cli
- 分支：refactor/g39-audit
- 开始前用 gh 确认依赖已满足：G20、G28、G35、G36、G37、G38、G44（依赖已于 2026-10-25 调整，以本条为准）。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g39-audit /Users/flanchan/.codex/worktrees/refactor-g39-audit/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行；本 goal 可以分成多个小 PR（每类问题一个），每个 PR 单独通过检查后合并：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试；新旧二进制的输出比对使用 fixture 与 httptest 夹具。用户另行授权只读账号后，才可以用真实读取补充比对。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G40：逐包审查与修正：internal/mcpserver（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G40：逐包审查与修正：internal/mcpserver。

【本 goal】
工作项：逐包审查与修正，范围 internal/mcpserver/。建议由能力更强的模型执行。
- 逐包审查：范围内每个生产包都按规格 3.1（命名与放置）、3.3（横切关注点）、3.4.2 的 P-a 到 P-m 对照一遍；重点是主要为测试而存在的生产代码（P-a）、同一套方法或机制在多个类型上各写一份（P-d）、过度防御（P-m）。发现的问题直接修正。
- 测试：按规格 3.4.4 的清单核对范围内的测试；清单内缺失的按 3.4.4 的上限补回最小用例，清单外的不新增。
- 规格与事实不符的地方按 7.0 第 8 条处理，并写进最终汇报，供 G34 更新文档和 skill。
- 对外输出：比对 v1.2.1 与当前 main 的 MCP tools/list（名称、schema、annotations）以及代表性 tool 的结构化输出；每个差异确认是预期内并写进最终汇报。
- 安全：stdio stdout 只有 JSON-RPC；HTTP 必须带 OAuth bearer；运行失败返回 isError=true 的结构化结果；tool 输出不含签名 URL 与 Cookie；执行边界记录的 error 属性不带凭据。

【验收】
- PR 或最终汇报中附逐包表：包名、发现的问题、处理结果（已修正 / 不修正及原因）；范围内每个生产包都在表中。
- 每类问题的修正 PR 都已合并。
- go test ./... -count=1、go vet ./...、sh scripts/build.sh 通过；修正涉及并发、持久化或共享运行时时，go test -race ./... 通过。
【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g40-audit-mcp
- 路径：/Users/flanchan/.codex/worktrees/refactor-g40-audit-mcp/pixiv-cli
- 分支：refactor/g40-audit-mcp
- 开始前用 gh 确认依赖已满足：G20、G28、G35、G36、G37、G38、G44（依赖已于 2026-10-25 调整，以本条为准）。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g40-audit-mcp /Users/flanchan/.codex/worktrees/refactor-g40-audit-mcp/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行；本 goal 可以分成多个小 PR（每类问题一个），每个 PR 单独通过检查后合并：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试；新旧二进制的输出比对使用 fixture 与 httptest 夹具。用户另行授权只读账号后，才可以用真实读取补充比对。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G41：逐包审查与修正：internal/cli、cmd

```text
/goal 执行 pixiv-cli 重构规格的 G41：逐包审查与修正：internal/cli、cmd。

【本 goal】
工作项：逐包审查与修正，范围 internal/cli/、cmd/。建议由能力更强的模型执行。
- 逐包审查：范围内每个生产包都按规格 3.1（命名与放置）、3.3（横切关注点）、3.4.2 的 P-a 到 P-m 对照一遍；重点是主要为测试而存在的生产代码（P-a）、同一套方法或机制在多个类型上各写一份（P-d）、过度防御（P-m）。发现的问题直接修正。
- 测试：按规格 3.4.4 的清单核对范围内的测试；清单内缺失的按 3.4.4 的上限补回最小用例，清单外的不新增。
- 规格与事实不符的地方按 7.0 第 8 条处理，并写进最终汇报，供 G34 更新文档和 skill。
- 对外输出：用本机 Homebrew 安装的 v1.2.1 与当前 main 构建的二进制，对同一批输入比对 --help 与各命令的 text/JSON/NDJSON 输出；逐项确认差异都是预期内（包括 G08 映射变化在 CLI 中的表现）。
- 安全：CLI 数据 stdout 中没有日志；错误输出不含凭据。
- changelog 草稿：按 G43 合并后的规则（只写用户能感知的变化、用用户的话写），汇总 v1.2.1 以来的 SDK 破坏性变更（取自 G39）、CLI/MCP 可见变化（取自本 goal 与 G40）、依赖与交互变化（例如 Select 改为编号选择、stderr 诊断格式），每条注明来源 PR，写进最终汇报供 G34 使用。

【验收】
- PR 或最终汇报中附逐包表：包名、发现的问题、处理结果（已修正 / 不修正及原因）；范围内每个生产包都在表中。
- 每类问题的修正 PR 都已合并。
- go test ./... -count=1、go vet ./...、sh scripts/build.sh 通过；修正涉及并发、持久化或共享运行时时，go test -race ./... 通过。
【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g41-audit-cli
- 路径：/Users/flanchan/.codex/worktrees/refactor-g41-audit-cli/pixiv-cli
- 分支：refactor/g41-audit-cli
- 开始前用 gh 确认依赖已满足：G20、G28、G35、G36、G37、G38、G39、G40、G43、G44（依赖已于 2026-10-25 调整，以本条为准）。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g41-audit-cli /Users/flanchan/.codex/worktrees/refactor-g41-audit-cli/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行；本 goal 可以分成多个小 PR（每类问题一个），每个 PR 单独通过检查后合并：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试；新旧二进制的输出比对使用 fixture 与 httptest 夹具。用户另行授权只读账号后，才可以用真实读取补充比对。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G42：逐包审查与修正：internal 其余目录与 scripts（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G42：逐包审查与修正：internal 其余目录与 scripts。

【本 goal】
工作项：逐包审查与修正，范围 internal/ 中除 cli、mcpserver、pixivapi、fanboxapi 以外的目录，以及 scripts/。建议由能力更强的模型执行。
- 逐包审查：范围内每个生产包都按规格 3.1（命名与放置）、3.3（横切关注点）、3.4.2 的 P-a 到 P-m 对照一遍；重点是主要为测试而存在的生产代码（P-a）、同一套方法或机制在多个类型上各写一份（P-d）、过度防御（P-m）。发现的问题直接修正。
- 测试：按规格 3.4.4 的清单核对范围内的测试；清单内缺失的按 3.4.4 的上限补回最小用例，清单外的不新增。
- 规格与事实不符的地方按 7.0 第 8 条处理，并写进最终汇报，供 G34 更新文档和 skill。
- 凭据脱敏收归一个类型（P-d）：pixivaccount.Account、pixivaccount.AccountWithToken、fanboxaccount.Account 不再各自实现 String、GoString、Format；凭据字段改用一个自我脱敏的类型（实现 String、GoString、Format 与 slog.LogValuer），可复用或推广 browsercookies.Secret 的做法，类型放在按职责命名的位置。保留一个测试，确认 %v、%+v、%#v 与 slog 都不会输出凭据。
- 发布链路：scripts/release、scripts/pr 的每个子命令都有 workflow 或脚本调用，workflow 中每处调用都与工具一致；scripts/ci/test-package-release.sh 通过。

【验收】
- PR 或最终汇报中附逐包表：包名、发现的问题、处理结果（已修正 / 不修正及原因）；范围内每个生产包都在表中。
- 每类问题的修正 PR 都已合并。
- go test ./... -count=1、go vet ./...、sh scripts/build.sh 通过；修正涉及并发、持久化或共享运行时时，go test -race ./... 通过。
【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g42-audit-internal
- 路径：/Users/flanchan/.codex/worktrees/refactor-g42-audit-internal/pixiv-cli
- 分支：refactor/g42-audit-internal
- 开始前用 gh 确认依赖已满足：G20、G28、G35、G36、G37、G38、G43、G44（依赖已于 2026-10-25 调整，以本条为准）。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g42-audit-internal /Users/flanchan/.codex/worktrees/refactor-g42-audit-internal/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行；本 goal 可以分成多个小 PR（每类问题一个），每个 PR 单独通过检查后合并：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试；新旧二进制的输出比对使用 fixture 与 httptest 夹具。用户另行授权只读账号后，才可以用真实读取补充比对。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G43：面向用户的 changelog（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G43：面向用户的 changelog。

【本 goal】
工作项：changelog 改为面向用户（2026-10-24 与维护者确认）。读者是 CLI 用户、MCP 用户与 SDK 使用方。本 goal 是规格 7.0 第 7 条（不修改 changelog）的例外。
- 规则：
  - 只写用户能感知的变化，章节为“破坏性变更、新增、变更、修复、安全”，空章节不输出；删除“维护”章节。CI、测试、内部重构、PR 治理、维护者文档不写；用户文档（README、安装、使用说明）的实质变化可以写进“变更”。
  - 用用户的话写：先说用户看到什么、需要做什么，必要时再用一句话说明原因；写出具体命令、参数或 tool 名；不出现内部术语（smoke worker、handoff、check suite、可信变更分类等）。
  - 每条保留来源链接：PR 链接，没有 PR 的 direct commit 用 commit 链接；保留版本号、日期、compare 链接与新贡献者章节；中英文内容一致。
- 工具：scripts/release 的 notes 校验仍要求覆盖范围内每个 PR 与 direct commit，但允许把内部改动明确标记为不进入正文（标记方式由本 goal 选择最简单的一种，例如文件末尾不渲染的 HTML 注释列出内部 PR 编号），GitHub Release 正文中不显示。
- 重写全部历史版本：changelog/v0.1.0 到 v1.2.1 的 en.md 与 zh-CN.md、changelog/unreleased/，按新规则重写；被删除的内部条目改为标记。同步 changelog/README.md、README.zh-CN.md 与 pixiv-cli-release-notes skill 中的规则。
- GitHub Release 页面：只运行 sync-history 的 dry-run，把将要变化的版本与差异摘要写进 PR；sync-history --apply 修改已发布的正文，需要维护者另行明确授权，本 goal 不执行。

【验收】
- 用新的校验规则对每个历史版本运行 notes 校验，全部通过。
- 重写后的正文中没有 CI、测试、内部重构类条目，也没有上面列出的内部术语；中英文条目一一对应。
- PR 中给出两三个版本改写前后的对照，以及 sync-history dry-run 的输出摘要。
- go test ./scripts/... 与 git diff --check 通过。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g43-changelog
- 路径：/Users/flanchan/.codex/worktrees/refactor-g43-changelog/pixiv-cli
- 分支：refactor/g43-changelog
- 开始前用 gh 确认依赖已满足：无。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g43-changelog /Users/flanchan/.codex/worktrees/refactor-g43-changelog/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行；规则、校验工具与历史重写放在同一个 PR 中合并（校验规则改变后，未重写的历史文件会通不过校验）：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G44：测试重写

```text
/goal 执行 pixiv-cli 重构规格的 G44：测试重写。

【本 goal】
工作项：规格 3.4.4（2026-10-25 与维护者确认）。建议由能力更强的模型执行。
- 删除全仓库全部 *_test.go，以及不再被使用的 testdata 和测试专用代码；然后只按 3.4.4 的清单重写测试，测试点到为止。
- 遵守 3.4.4 的上限：单个测试文件不超过 200 行，单个测试函数不超过 40 行，全仓库测试文件不超过 25 个；不新建测试辅助包、fixture 构造器或 mock 层级；不为测试导出生产符号。优先用 httptest 与临时目录这类真实边界。
- workflow 按名称或路径运行的测试保留原名重写（3.4.4 清单最后一行）；scripts/citest 至少保留一个测试文件，因为 platform-smoke、container-smoke 与 release.yml 都运行它。删掉的测试名如果仍被 workflow 引用，同步修改 workflow，并按 7.0 第 10 条处理 smoke workflow。
- 真实环境测试（sdk/pixiv、sdk/fanbox 的 live_test.go、live_mutation_test.go 等）删除，不重写；中英文 development.md 中真实环境测试的说明改为手动验证步骤。
- 更新 pixiv-cli-test skill：把 3.4.4 的清单与上限写成长期规则，取代原来“无数量上限”的说法；同步中英文 development.md 的测试布局章节与 AGENTS.md 中相关的测试条款。
- 只改测试、testdata、测试相关文档与 workflow 中的测试引用；生产代码中只为测试存在的部分（导出符号、注入点）一并删除，其余生产代码不改。

【验收】
- 全仓库测试文件不超过 25 个，每个不超过 200 行；PR 中列出每个测试文件、行数与它覆盖的清单项。
- 3.4.4 清单的每一项都有对应测试；清单外没有测试。
- go test ./... -count=1、go test -race ./... -count=1、go vet ./...、sh scripts/build.sh 通过；platform-smoke、container-smoke 在本 PR 上通过。
- PR 中列出测试总行数变化（G35–G38 后为 52.7k 行）。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g44-tests-rewrite
- 路径：/Users/flanchan/.codex/worktrees/refactor-g44-tests-rewrite/pixiv-cli
- 分支：refactor/g44-tests-rewrite
- 开始前用 gh 确认依赖已满足：G35、G36、G37、G38。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g44-tests-rewrite /Users/flanchan/.codex/worktrees/refactor-g44-tests-rewrite/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行；删除与重写放在同一个 PR 中合并，main 上不出现没有测试的中间状态：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```

## G34：文档、skill 收尾并删除 refactor 目录

```text
/goal 执行 pixiv-cli 重构规格的 G34：文档、skill 收尾并删除 refactor 目录。

【本 goal】
工作项：C2（规格 C 组）。
- 删除 .agents/refactor/ 整个目录。
- 在 pixiv-cli-develop 与 pixiv-cli-review 中写入过度防御的判定（规格 3.4.2 的 P-m：删除什么、保留哪些边界），作为长期规则。
- 确认 pixiv-cli-test 与规格 3.4.4 的清单和上限一致（由 G44 写入），作为长期规则。
- 在 pixiv-cli-code-commenting 中写入 3.5.1 的注释规则与正反例，在 pixiv-cli-develop、pixiv-cli-review 中写入 P-d 的扩充（同一机制收归到字段类型或值类型），作为长期规则。
- 中英文 docs/*/sdk.md 改为描述新的 API；更新 architecture.md、development.md 和各 skill 中与重构结果相关的路径和放置规则；把规格第 6 节的示例整理到 pixiv-cli-develop 的 references/go-examples.md（删除前先从本目录复制需要的内容），示例中的文件名按重构后的实际命名改写（例如 wire.go 已改为领域文件与 response.go）。
- 以 G41 产出的 changelog 草稿为基础、按 G43 的规则，汇总已合并各 goal 的 PR 描述中的 changelog 条目，写进本 PR 描述作为 release notes 草稿；不创建 changelog/vX.Y.Z/。

【验收】
- 文档中的链接和引用路径都有效；中英文内容一致。
- .agents/refactor/ 已删除。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g34-finish
- 路径：/Users/flanchan/.codex/worktrees/refactor-g34-finish/pixiv-cli
- 分支：refactor/g34-finish
- 开始前用 gh 确认依赖已满足：G29、G30、G31、G32、G33、G39、G40、G41、G42、G43（依赖已于 2026-10-24 调整，以本条为准）。未满足就标为阻塞并结束。
- 满足后执行 git fetch origin main，再用 git worktree add -b refactor/g34-finish /Users/flanchan/.codex/worktrees/refactor-g34-finish/pixiv-cli origin/main 创建。
- 全部工作都在这个 worktree 中进行。goal-N/ 工作目录、临时探针和基线比对输出加入 .git/info/exclude，不提交。

【提交与合并】
按规格 7.0「每个 goal 的提交要求」执行：
- 按逻辑单元分次提交。
- 只修改本 goal 负责的目录。
- 验收要点全部有证据后，跑一次第 9 节的完整检查，按 pixiv-cli-pr 写 PR 描述（包括 changelog 条目），并用 prmeta 校验，然后向 main 创建 PR。
- 当前 head 的必需检查全部通过、机器人评审意见逐条处理之后，再 squash 合并。
- 合并后删除远端分支和 worktree。
- 无法合并时不强推、不绕过分支保护，写明原因后停止。

【真实账号】
本 goal 不运行真实环境测试。

【授权】
- 已授权：本 goal 的 commit、push、创建 PR，验收与检查通过后 squash 合并到 main，以及上面列出的真实账号测试。
- 未授权：发布版本、打 tag、修改分支保护、force push、引入规格第 4 节以外的依赖、修改 .agents/refactor/（G34 除外）。

【不确定时】
- 不向用户提问，不修改规格中的设计决策。
- 选择不改变对外行为的最保守做法继续推进，并把假设写进工作记录和 PR 描述。
- 无法在假设下继续的部分标为阻塞，在最终汇报中列出。
- 不做其他 goal 的工作项。本 goal 触及的目录顺带按规格 3.4 处理 T2、T3、T6。

【完成标准】
- PR 已合并进 main，上面的每条验收要点都有证据。
- 最终汇报包括：PR 链接、合并提交、实际运行的检查及结果、假设与阻塞项、依赖本 goal 的后续 goal 需要知道的事。
```
