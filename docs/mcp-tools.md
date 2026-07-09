# MCP 工具

当前 server 通过 `pixiv mcp` 以 stdio 注册以下 MCP tools。

无 refresh token 且 `web_fallback_enabled=true` 时，`download`、`search_illust`、`illust_detail`、`illust_ranking`、`search_user`、`get_thumbnail_base64` 会走匿名 Pixiv web/ajax API fallback。有 refresh token 时仍优先 App API；App API 认证、网络或服务端错误不会自动 fallback。

## 配置与认证

- `set_download_path`
  - 参数：`path`
  - 设置默认下载目录。路径为空会返回错误；设置时会尝试创建目录。
- `refresh_token`
  - 参数：无
  - 使用已保存的 refresh token 刷新 Pixiv API token；成功时显示用户 ID，若 API 可提供则同时显示用户名。
- `set_refresh_token`
  - 参数：`refresh_token`
  - 在当前 MCP 会话设置 refresh token，并立即尝试认证；不会写入 `auth.json`。成功时显示用户 ID，若 API 可提供则同时显示用户名。参数可直接传原始 token，也可传包含 `refresh_token=...` 的 Cookie 字符串；仅包含 `PHPSESSID`/`device_token` 的网页 Cookie 不能用于 App API OAuth 刷新。

## 下载

- `download`
  - 参数：`illust_id` 或 `illust_ids`，可选 `delivery`
  - 同步下载作品，完成后返回中文摘要和 structured output。默认 `delivery` 为 `local_path`，会返回本地路径、`file://` URI、MIME 和文件大小。
  - 当 `delivery` 为 `image_content` 时，还会为每个已下载文件追加 MCP `ImageContent`。文本摘要和 structured output 仍会返回，供不支持图片输入的客户端或模型使用。
  - 匿名 fallback 下，静态作品通过 `/ajax/illust/{id}/pages` 的 `original` URL 下载；ugoira 通过 `/ajax/illust/{id}/ugoira_meta` 获取 zip 与 frames 后继续复用本地 `ffmpeg` 转 GIF。
- `download_random_from_recommendation`
  - 参数：`count`，可选 `delivery`
  - 从推荐作品中随机选择并同步下载。当前默认 5 个，最多 20 个，返回格式与 `download` 一致。

## 作品浏览

- `search_illust`
  - 参数：`word`、`search_target`、`sort`、`duration`、`offset`、`search_r18`、`include_thumbnail`
  - 搜索插画。`search_r18` 为 true 时会在搜索词后追加 `R-18`。
- `illust_detail`
  - 参数：`illust_id`
  - 获取单个作品详情，当前以格式化 JSON 返回。
- `illust_related`
  - 参数：`illust_id`、`offset`、`include_thumbnail`
  - 获取相关作品。
- `illust_ranking`
  - 参数：`mode`、`date`、`offset`、`include_thumbnail`
  - 浏览 Pixiv 排行榜。默认 `mode` 为 `day`。
- `illust_recommended`
  - 参数：`offset`、`include_thumbnail`
  - 获取个性化推荐，需要认证。
- `trending_tags_illust`
  - 参数：无
  - 获取当前热门插画标签。
- `illust_follow`
  - 参数：`restrict`、`offset`、`include_thumbnail`
  - 获取关注用户的新作品，需要认证。默认 `restrict` 为 `public`。
- `get_thumbnail_base64`
  - 参数：`illust_id`
  - 获取作品缩略图并返回 `data:image/jpeg;base64,...` 文本。

## 用户相关

- `search_user`
  - 参数：`word`、`offset`
  - 搜索 Pixiv 用户。匿名 fallback 下不是官方用户名搜索，而是通过作品搜索结果按 `userId` 去重返回“相关作品作者”。
- `user_bookmarks`
  - 参数：`user_id_to_check`、`restrict`、`tag`、`max_bookmark_id`
  - 查询用户收藏，需要认证。未提供用户 ID 时使用当前认证用户。
- `user_following`
  - 参数：`user_id_to_check`、`restrict`、`offset`
  - 查询用户关注列表，需要认证。未提供用户 ID 时使用当前认证用户。
