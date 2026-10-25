# v0.8.0 — 2026-07-28

## 破坏性变更

- 公开 SDK 的下载与构造 API、配置 API，以及 CLI/MCP 下载输入已改为显式选项和并发模型。SDK 使用方需要更新调用代码。（[`3145bf3`](https://github.com/FlanChanXwO/pixiv-cli/commit/3145bf3)）

## 新增

- 下载支持持久缓存重验证与续传、有序批量结果、APNG 输出，以及作品、用户和 CDN 引用；CLI/MCP 新增书签分类（pool）、动态流和跨设备 OAuth 回调 relay 配置。（[`3145bf3`](https://github.com/FlanChanXwO/pixiv-cli/commit/3145bf3)）

## 变更

- README 说明了 pixiv-cli 在 Pixiv 工具生态中的定位，产品 skill 也指向正确的工作流。（[#35](https://github.com/FlanChanXwO/pixiv-cli/pull/35)）

## 修复

- 跨设备认证恢复在浏览器中显示成功与失败结果页。（[`3145bf3`](https://github.com/FlanChanXwO/pixiv-cli/commit/3145bf3)）

**完整变更**：[v0.7.2...v0.8.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v0.7.2...v0.8.0)

<!-- internal:
  [#36](https://github.com/FlanChanXwO/pixiv-cli/pull/36)
-->
