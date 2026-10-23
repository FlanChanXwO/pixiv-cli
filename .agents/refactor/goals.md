# pixiv-cli 重构：goal 语句

规格见同目录的 `spec.md` 第 7 节 7.0。每条语句可以独立粘贴。开始一个 goal 前，它依赖的 goal 必须已合并进 main；同一阶段内没有依赖关系的 goal 可以同时运行，每个都在自己的 worktree 中进行。

标题带“（可并行）”的 goal 可以与其他 goal 同时运行：阶段 1 的各条线路之间、G23–G27 之间、G29–G33 之间互相并行，G19–G21 还可以与阶段 0 同时进行。同一线路内仍按依赖顺序执行，例如 G07 要等 G06 合并。

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
| G09 | `refactor-g09-sdk-fanbox` | G08 | 是 |
| G10 | `refactor-g10-mcp-tools` | G05 | 是 |
| G11 | `refactor-g11-cli-commands` | G05 | 是 |
| G12 | `refactor-g12-cli-prompt` | G11 | 是 |
| G13 | `refactor-g13-config-tests` | G05 | 是 |
| G14 | `refactor-g14-config-toml` | G13 | 是 |
| G15 | `refactor-g15-transport` | G05 | 是 |
| G16 | `refactor-g16-storage` | G05 | 是 |
| G17 | `refactor-g17-live-tests` | G05 | 是 |
| G18 | `refactor-g18-leaf-packages` | G05 | 是 |
| G19 | `refactor-g19-scripts-move` | 用 gh 确认最近一次 release tag 对应的全部发布 workflow 已成功（不依赖其他 goal） | 是 |
| G20 | `refactor-g20-scripts-slim` | G19 | 是 |
| G21 | `refactor-g21-workflow-refs` | G20 | 是 |
| G22 | `refactor-g22-logging` | G09、G10、G11、G14、G15 | 否 |
| G23 | `refactor-g23-shared-mechanisms` | G22，以及 G01–G21 全部 | 是 |
| G24 | `refactor-g24-media` | G22，以及 G01–G21 全部 | 是 |
| G25 | `refactor-g25-mcp-runtime` | G22，以及 G01–G21 全部 | 是 |
| G26 | `refactor-g26-accounts` | G22，以及 G01–G21 全部 | 是 |
| G27 | `refactor-g27-cli-auth` | G22，以及 G01–G21 全部 | 是 |
| G28 | `refactor-g28-sweep` | G23、G24、G25、G26、G27 | 否 |
| G29 | `refactor-g29-headers-sdk` | G28 | 是 |
| G30 | `refactor-g30-headers-cli` | G28 | 是 |
| G31 | `refactor-g31-headers-mcp` | G28 | 是 |
| G32 | `refactor-g32-headers-internal` | G28 | 是 |
| G33 | `refactor-g33-headers-scripts` | G28、G21 | 是 |
| G34 | `refactor-g34-finish` | G29、G30、G31、G32、G33 | 否 |

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
工作项：A3 的 user 及其余 Pixiv 端点；完成后删除 services/pixiv/endpoint/（规格 6.1、3.3、附录 A 的 F4/F5）。
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
- services/fanbox/endpoint/** 并入 sdk/fanbox，完成后 services/ 目录消失。
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
- 开始前用 gh 确认依赖已满足：G08。未满足就标为阻塞并结束。
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
- 开始前用 gh 确认依赖已满足：用 gh 确认最近一次 release tag 对应的全部发布 workflow 已成功（不依赖其他 goal）。未满足就标为阻塞并结束。
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

## G20：scripts 精简（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G20：scripts 精简。

【本 goal】
工作项：S2（规格 S 组中 S2 的测试规则）。
- 去掉 licensebundle 的暂存、备份、回滚和 bundleFileOps；licensebundle --check 从 7 处减为 2 处；按规则精简 scripts 的测试。

【验收】
- licensebundle 生成结果与改动前逐字节一致。
- PR 中列出删除的测试及依据。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g20-scripts-slim
- 路径：/Users/flanchan/.codex/worktrees/refactor-g20-scripts-slim/pixiv-cli
- 分支：refactor/g20-scripts-slim
- 开始前用 gh 确认依赖已满足：G19。未满足就标为阻塞并结束。
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
- 把 §28、R14 一类指向仓库外文档的引用改为直接写出原因。

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
- 开始前用 gh 确认依赖已满足：G20。未满足就标为阻塞并结束。
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
- 开始前用 gh 确认依赖已满足：G09、G10、G11、G14、G15。未满足就标为阻塞并结束。
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

【验收】
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
- 开始前用 gh 确认依赖已满足：G22，以及 G01–G21 全部。未满足就标为阻塞并结束。
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
- 开始前用 gh 确认依赖已满足：G22，以及 G01–G21 全部。未满足就标为阻塞并结束。
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
- 开始前用 gh 确认依赖已满足：G22，以及 G01–G21 全部。未满足就标为阻塞并结束。
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
- 开始前用 gh 确认依赖已满足：G22，以及 G01–G21 全部。未满足就标为阻塞并结束。
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
- 开始前用 gh 确认依赖已满足：G22，以及 G01–G21 全部。未满足就标为阻塞并结束。
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
工作项：重新扫描只被测试引用的导出函数（T2），处理剩余的无对应生产文件测试（T3）、Go 代码中的编号引用（T4），以及其余兼容层，包括 illustration 别名（T6，写成 changelog 条目）。

【验收】
- PR 中按 3.4.3 的表格列出每项删除及依据。
- 用户可见行为不变（changelog 条目除外）。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g28-sweep
- 路径：/Users/flanchan/.codex/worktrees/refactor-g28-sweep/pixiv-cli
- 分支：refactor/g28-sweep
- 开始前用 gh 确认依赖已满足：G23、G24、G25、G26、G27。未满足就标为阻塞并结束。
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

## G29：文件头注释：sdk/、cmd/（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G29：文件头注释：sdk/、cmd/。

【本 goal】
工作项：C3 在 sdk/、cmd/ 中的部分（规格 3.5，以及 pixiv-cli-code-commenting 的 File header comments）。
- 只添加或修正文件头注释，不改其他内容。

【验收】
- 范围内每个需要头注释的文件都有 1–3 行职责说明；Go 文件头注释与 package 子句之间有空行，//go:build 位于最前。
- go build ./... 和 go vet ./... 通过。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g29-headers-sdk
- 路径：/Users/flanchan/.codex/worktrees/refactor-g29-headers-sdk/pixiv-cli
- 分支：refactor/g29-headers-sdk
- 开始前用 gh 确认依赖已满足：G28。未满足就标为阻塞并结束。
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

## G30：文件头注释：internal/cli/（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G30：文件头注释：internal/cli/。

【本 goal】
工作项：C3 在 internal/cli/ 中的部分（规格 3.5，以及 pixiv-cli-code-commenting 的 File header comments）。
- 只添加或修正文件头注释，不改其他内容。

【验收】
- 范围内每个需要头注释的文件都有 1–3 行职责说明；Go 文件头注释与 package 子句之间有空行，//go:build 位于最前。
- go build ./... 和 go vet ./... 通过。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g30-headers-cli
- 路径：/Users/flanchan/.codex/worktrees/refactor-g30-headers-cli/pixiv-cli
- 分支：refactor/g30-headers-cli
- 开始前用 gh 确认依赖已满足：G28。未满足就标为阻塞并结束。
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

## G31：文件头注释：internal/mcpserver/（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G31：文件头注释：internal/mcpserver/。

【本 goal】
工作项：C3 在 internal/mcpserver/ 中的部分（规格 3.5，以及 pixiv-cli-code-commenting 的 File header comments）。
- 只添加或修正文件头注释，不改其他内容。

【验收】
- 范围内每个需要头注释的文件都有 1–3 行职责说明；Go 文件头注释与 package 子句之间有空行，//go:build 位于最前。
- go build ./... 和 go vet ./... 通过。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g31-headers-mcp
- 路径：/Users/flanchan/.codex/worktrees/refactor-g31-headers-mcp/pixiv-cli
- 分支：refactor/g31-headers-mcp
- 开始前用 gh 确认依赖已满足：G28。未满足就标为阻塞并结束。
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

## G32：文件头注释：internal/ 中除 cli、mcpserver 以外的目录（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G32：文件头注释：internal/ 中除 cli、mcpserver 以外的目录。

【本 goal】
工作项：C3 在 internal/ 中除 cli、mcpserver 以外的目录 中的部分（规格 3.5，以及 pixiv-cli-code-commenting 的 File header comments）。
- 只添加或修正文件头注释，不改其他内容。

【验收】
- 范围内每个需要头注释的文件都有 1–3 行职责说明；Go 文件头注释与 package 子句之间有空行，//go:build 位于最前。
- go build ./... 和 go vet ./... 通过。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g32-headers-internal
- 路径：/Users/flanchan/.codex/worktrees/refactor-g32-headers-internal/pixiv-cli
- 分支：refactor/g32-headers-internal
- 开始前用 gh 确认依赖已满足：G28。未满足就标为阻塞并结束。
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

## G33：文件头注释：scripts/、.github/、frontend/（可并行）

```text
/goal 执行 pixiv-cli 重构规格的 G33：文件头注释：scripts/、.github/、frontend/。

【本 goal】
工作项：C3 在 scripts/、.github/、frontend/ 中的部分（规格 3.5，以及 pixiv-cli-code-commenting 的 File header comments）。
- 只添加或修正文件头注释，不改其他内容。

【验收】
- 范围内每个需要头注释的文件都有 1–3 行职责说明；Go 文件头注释与 package 子句之间有空行，//go:build 位于最前。
- go build ./... 和 go vet ./... 通过。

【依据】
- 规格：仓库内的 .agents/refactor/spec.md，以 origin/main 上的版本为准。先通读第 1–3 节、第 7 节的 7.0 和本 goal 涉及的工作项、第 9 节、第 10 节。第 6 节示例只固定职责划分和数据流向，签名以当时的代码为准。
- 仓库：github.com/FlanChanXwO/pixiv-cli（本机路径 /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli）。先读 AGENTS.md，以及 .agents/skills/ 下的 pixiv-cli-develop、pixiv-cli-test、pixiv-cli-code-commenting、pixiv-cli-review、pixiv-cli-pr、pixiv-cli-commit-message。
- 规格中的统计数字基于 ef4accb9；以当前 origin/main 的实际代码为准，有出入时写进工作记录和 PR 描述。

【worktree 与分支】
- 名称：refactor-g33-headers-scripts
- 路径：/Users/flanchan/.codex/worktrees/refactor-g33-headers-scripts/pixiv-cli
- 分支：refactor/g33-headers-scripts
- 开始前用 gh 确认依赖已满足：G28、G21。未满足就标为阻塞并结束。
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

## G34：文档、skill 收尾并删除 refactor 目录

```text
/goal 执行 pixiv-cli 重构规格的 G34：文档、skill 收尾并删除 refactor 目录。

【本 goal】
工作项：C2（规格 C 组）。
- 删除 .agents/refactor/ 整个目录。
- 中英文 docs/*/sdk.md 改为描述新的 API；更新 architecture.md、development.md 和各 skill 中与重构结果相关的路径和放置规则；把规格第 6 节的示例整理到 pixiv-cli-develop 的 references/go-examples.md（删除前先从本目录复制需要的内容）。
- 汇总 G01–G33 PR 描述中的 changelog 条目，写进本 PR 描述作为 release notes 草稿；不创建 changelog/vX.Y.Z/。

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
- 开始前用 gh 确认依赖已满足：G29、G30、G31、G32、G33。未满足就标为阻塞并结束。
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
