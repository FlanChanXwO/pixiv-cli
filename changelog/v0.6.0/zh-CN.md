# v0.6.0 — 2026-07-24

## 新增

- 新增认证 App API 小说搜索：CLI `pixiv novel search WORD`、public SDK `SearchNovel` 与 MCP `search_novel` 同步支持关键词匹配、日期排序、时间范围、分级、正文长度和仅原创筛选；小说结果提供稳定小说 URL、分级、正文长度与原创标记。
- 新增 CLI `pixiv user search WORD`，并将 MCP `search_user` 升级为 `{source,user_previews,pagination,text}`。结果明确标识官方 App 用户搜索（`app_search`）或匿名相关插画作者 fallback（`related_illust_authors`），避免误称后者为用户名搜索。
- 补齐日语 public SDK 与 MCP tool reference，并完善三语交叉导航。

## 变更

- **Breaking：**持久的本地应用数据统一直接位于用户主目录下：macOS/Linux 为 `~/.pixiv-cli`，Windows 为 `%USERPROFILE%\.pixiv-cli`；其中包括认证、配置、回调桥接状态、日志、Release 检查缓存和 macOS 回调 helper。旧存储路径不会被读取或迁移。
- 作品详情新增 `caption`：SDK/CLI JSON/MCP 保留 Pixiv 原始 HTML，普通 CLI 详情安全显示纯文本；列表输出不增加作品说明。
- 发布 tag 现在必须先通过受保护 `pixiv-e2e` Environment 的完整认证 E2E；PR 与 `main` 常规 CI 仍保持离线、无 secret。真实回归失败会阻止后续生产构建与发布。
- GitHub Release body 现在同时包含对应的英文与简体中文发布说明。
- CLI、MCP、SDK、下载器与 App API 的 operation diagnostics 统一使用安全结构化事件；事件不记录 token、URL、原始 header、请求输入或 response body。

## 修复

- 修复桌面 Linux 与 Windows 的浏览器 OAuth 回调：`pixiv://` 现在只在当前登录期间注册，结束后恢复原有用户关联。SSH `--no-open --addr` fallback 页面现在会由提交它的浏览器继续已校验 Pixiv relay，因此本机 `ssh -L` tunnel 可将最终 callback 回传无 GUI 服务器，无需在浏览器机器安装第二份 pixiv。
- 修复经显式 HTTP(S) 代理下载静态图或 ugoira 时可能出现的 HTTP/2 资源流中断：资源传输现在单独协商 HTTP/1.1，App API、OAuth 和 Web 元数据请求保持原有协议协商。
- 文件日志现使用无 `pixiv-` 前缀的纯文本 `YYYY-MM-DD.txt`；JSONL 输出及无实际作用的 `log_format` / `PIXIV_LOG_FORMAT` 开关已删除。OAuth 登录、回调、成功和失败页面的浏览器标题统一为 `pixiv-cli`。
- OAuth operation diagnostics 现在保留安全的 `transport_kind` 分类（包括 typed network timeout），使没有 HTTP 状态码的上游失败可被诊断，同时不记录 URL、凭据、header 或 response body。
