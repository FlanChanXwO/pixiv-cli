# v1.1.0 — 2026-09-20

## 新增

- 扩展稳定的 Pixiv public surface：SDK、CLI 与 MCP 现完整覆盖 artwork/novel 收藏操作、artwork/novel 评论创建/回复/贴图/删除、贴图列表、novel 排行与收藏读取、user/follow/mypixiv 读取，以及基于 App API 的 feed/recommendation 路由。([#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82))
- 新增统一的 command-scoped Pixiv target resolver 与 search filter，使 ID、受支持的 Pixiv URL、entity type、artwork subtype、收藏数范围、日期、宽高比、分辨率和绘图工具等输入在执行前采用同一套校验。([#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82))

## 变更

- 稳定 Pixiv App API、public SDK、CLI owner 与 MCP owner 的 typed contract。分页统一使用 opaque cursor 与共享 continuation 处理，aggregate read 保持 failure-atomic 行为；不支持或无效输入会 fail closed，不再通过其他路径静默 fallback 或 replay。([#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82))
- 将 search、detail、series、ranking、recommendation、timeline、bookmark、comment、user、follow 与 mypixiv 命令行为对齐到已验证的 App API contract，包括 logical multi-batch traversal，以及 CLI/MCP 一致的稳定 structured output。([#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82))

## 修复

- 修正 artwork/novel 读取和 mutation 的 Pixiv wire adapter 与 continuation 处理，包括多页 artwork index、series cursor、recommendation continuation 参数、bookmark detail 空状态归一化，以及正数 cursor 校验。([#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82))
- 在不推断权限语义的前提下完整保留评论 metadata：当前评论日期映射已验证的 wire 字段，numeric `comment_access_control` 保留在 `access_control.comment_access_control`，同时继续兼容 legacy access metadata。([#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82))
- 父 context 取消时立即中止 remote-login relay，而不是继续等待 HTTP graceful shutdown，避免 active handler 在 race instrumentation 下拖住整个取消路径。([#86](https://github.com/FlanChanXwO/pixiv-cli/pull/86))

## 安全

- Pixiv failure、cursor state、认证决策与 mutation boundary 继续 fail closed：不恢复匿名 Web fallback，敏感值不进入公开输出，replay/continuation 在发起请求前校验绑定的 operation 与输入。([#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82))

## 文档

- 更新英文与简体中文 CLI、MCP、SDK、架构、开发、README 和产品 skill 文档，使其与稳定后的 Pixiv contract 及新增公开操作保持一致。([#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82))

## 维护

- 扩展 Pixiv endpoint ownership、SDK compatibility、cursor binding、pagination、CLI/MCP projection、comments、bookmarks、users、feeds 与 mutation evidence 的离线及 live-manifest 回归覆盖，同时继续确保 credential 和 private response 不进入仓库。([#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82))
- 准备 v1.1.0 双语发布元信息，并将分发的产品 skill 版本与 release tag 对齐。([#83](https://github.com/FlanChanXwO/pixiv-cli/pull/83))
- 正式声明 Go `1.27.1` 为 v1.1.0 的源码构建与 SDK 开发基线，并同步仓库文档与发布元信息。Release CI 现在要求每个原生 matrix gate 都实际执行：Windows ARM64 不再跳过 race step，而是必须精确匹配 Go 官方“不支持 race”的诊断；其他 race 失败仍保持失败。([#84](https://github.com/FlanChanXwO/pixiv-cli/pull/84))
- 将人工发布审批收敛为最终发布边界的一次审批。build、SDK E2E、provenance、source trust、production 与 container 检查全部自动执行；通过唯一的 `release-approval` Environment 审批后，GitHub Release 以及后续 registry/Homebrew 发布链自动继续，不再重复要求审批。([#85](https://github.com/FlanChanXwO/pixiv-cli/pull/85))

**完整变更**：[v1.0.2...v1.1.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.0.2...v1.1.0)
