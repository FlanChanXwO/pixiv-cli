# pixiv-cli v0.2.0 恢复与 v0.3.0 实施计划

## 目标与约束

- 项目、仓库、Go module、Homebrew formula、Release 资产和 CLI 命令均保持 `pixiv-cli` / `pixiv`。
- 先恢复不可变 `v0.2.0` tag 的正式 Release；随后开发、验收并发布 `v0.3.0`。
- 公开 SDK 从 `pkg/pixiv` 迁移到顶层 `pixiv`，不保留兼容 package；目标 import path 是 `github.com/FlanChanXwO/pixiv-cli/pixiv`。
- Like、Cookie/Web 登录、浏览器凭据读取及 `doctor` 明确不实现。
- CLI/MCP 是精选入口；任何已暴露的 Pixiv 内容能力必须调用同一公开 SDK，不为表面对齐增加无用户需求命令。
- 上游协议更新只通过代码和签名 Release 交付；不使用远程 endpoint manifest、插件 ABI 或静默 fallback。

## 交付内容

1. 恢复 v0.2.0：修复 Windows 权限测试并纳入受审计 recovery overlay，dispatch 原 tag，验证 Release/Homebrew/更新检查。
2. SDK 路径迁移：移动公开 package，更新 import、文档、测试、release recovery policy。
3. 协议隔离：保留 DTO→internal model→public model，集中 endpoint/profile，统一脱敏 upstream failure，拆分 web adapter，消除 legacy Source 生产双栈。
4. User Detail：稳定的 User/Profile/ProfilePublicity/Workspace 模型；CLI `pixiv user detail USER_ID`；MCP `user_detail`。
5. 推荐：SDK 原子支持 illust/manga/novel/user；CLI 强制 `pixiv recommended all|illust|manga|novel|user`；MCP `recommended` 的必填 `kind`。
6. 推荐仅公开个性化列表、作者 preview 和 opaque cursor；不公开 ranking、privacy policy、contest UI、原始 next_url。
7. 文档、ADR、能力矩阵、仓库 skills、CHANGELOG 与知识图谱同步。

## 关键接口决定

- User detail：必需 envelope 缺失或用户 ID 非法时返回稳定 malformed upstream error；可选 URL 维持可选、隐藏文本/未知计数为零值。
- 推荐：漫画使用 `/v1/illust/recommended?content_type=manga`，不回退 `/v1/manga/recommended`；四类 API 使用独立 opaque cursor。
- `recommended all`：在一个 SDK/account snapshot 内顺序读取四类；`--limit` 与 `--page` 分别作用于每条流；无 `--limit` 一类一批，`--limit 0` 读到 cursor 结束；文本/JSON 全部成功前不写 stdout。
- MCP `recommended.kind` 是必填枚举 `all|illust|manga|novel|user`；保留旧 `illust_recommended` 与随机下载插画 tool 的兼容行为。

## 验证与回滚

- 每一功能切片使用 RED→GREEN→REFACTOR；测试主要经 SDK、CLI 和 MCP 公开行为验证。
- 每个子任务由实现代理、规格审查代理、质量审查代理依次完成；发现问题必须修复并复审。
- 默认 CI 不访问真实 Pixiv；提供显式 opt-in 的 App API canary，绝不打印 refresh token。
- Release 失败时保留 tag 不变，遵守已有 audited recovery workflow；不 force push、不移动 tag。
