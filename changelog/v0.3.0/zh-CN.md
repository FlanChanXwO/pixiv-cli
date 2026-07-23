# v0.3.0 — 2026-07-15

## 变更

- Breaking: 公开 Go SDK 已迁移至 `github.com/FlanChanXwO/pixiv-cli/pixiv`；旧导入路径不保留兼容 package。
- Breaking: 认证入口只接受原始 Pixiv App API refresh token；网页 Cookie（包括 `refresh_token=...`）不再被解析、提取或转换。
- Breaking: `pixiv recommended` 现要求 `all|illust|manga|novel|user` kind；`all` 原子返回四类个性化推荐。

## 新增

- MCP 新增 `recommended`：以必填 kind 返回插画、漫画、小说或作者推荐；`all` 以每流独立分页的 structured output 返回四类推荐。
- MCP 新增 `user_detail`，以必填 `user_id` 返回完整稳定的用户详情 structured output。
- 新增 `pixiv user detail USER_ID`；可用 `--json` 输出完整、稳定的用户详情 SDK envelope。
- `pixiv search` 新增 `--rating`、`--type` 与 `--ai-type` 本地结果过滤；带 `--limit`/`--page` 时会按匹配结果继续读取 opaque cursor。

## 修复

- 修复真实 App API 四类推荐的 `next_url` 返回 `offset=0` 时被误判为 malformed 的问题；opaque cursor 继续保持种类、查询与账号来源隔离。
- 修复 App API 用户详情把 `profile_publicity` 的 `public`/`private` wire 值误判为 malformed 的问题；公开 SDK 继续稳定输出 bool。
- `auth login` 的浏览器 callback 页现在明确提示“授权已收到、正在回到 CLI 完成登录”；非 JSON 的登录成功输出精简为单行。
- 默认日志级别改为 `warn`，避免普通 CLI 成功命令把 INFO 操作诊断写入 stderr；显式 `info` 配置和环境覆盖保持不变。
- 修复显式与自动更新检查未向 GitHub Releases API 发送项目识别性 `User-Agent` 而可能收到 HTTP 403 的兼容性问题。

## 安全

- `auth login` 不再启动受管 Chromium、连接 DevTools/CDP、读取浏览器历史/会话/存储或扫描活动标签页；只保留本轮 loopback、受控 `pixiv://` helper 和用户显式手动回填。
- SDK、CLI、MCP、环境变量和已存账号在 OAuth 请求前统一拒绝 Cookie 形态凭据，且不回显输入内容。
