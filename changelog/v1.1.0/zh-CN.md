# v1.1.0 — 2026-09-20

## 新增

- SDK、CLI 和 MCP 新增 Pixiv 作品与小说收藏、评论（创建、回复、贴图、删除、贴图列表）、小说排行与收藏、用户/关注/MyPixiv 读取，以及动态流与推荐接口。（[#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82)）
- 命令输入支持 ID、受支持的 Pixiv URL、实体类型、作品子类型、收藏数范围、日期、宽高比、分辨率和绘图工具筛选，并在执行前完成校验。（[#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82)）

## 变更

- 搜索、详情、系列、排行、推荐、动态、收藏、评论、用户、关注和 MyPixiv 命令已按验证过的 App API 契约统一，支持不透明 cursor 的多批次分页。无效或不支持的输入会直接报错，不再静默改走其他请求路径。（[#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82)）

## 修复

- 修正作品与小说的读取和写入映射，包括多页页号、系列 cursor、推荐续页参数和收藏详情归一化。（[#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82)）
- 评论日期改用已验证的字段，保留 `comment_access_control`，已有的访问元数据仍可读取。（[#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82)）
- `pixiv auth` 远程登录在取消时会立即停止 relay，不再等待 HTTP server 关闭而卡住。（[#86](https://github.com/FlanChanXwO/pixiv-cli/pull/86)）

## 安全

- Pixiv 请求失败时会直接报错，不会改走匿名 Web 路径；敏感值不会进入公开输出，cursor 也不能被换到其他账号或操作上重放。（[#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82)）

**完整变更**：[v1.0.2...v1.1.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.0.2...v1.1.0)

<!-- internal:
  [#83](https://github.com/FlanChanXwO/pixiv-cli/pull/83)
  [#84](https://github.com/FlanChanXwO/pixiv-cli/pull/84)
  [#85](https://github.com/FlanChanXwO/pixiv-cli/pull/85)
-->
