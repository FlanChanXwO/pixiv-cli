# Pixiv Provider HTTP 接口文档

## 目标

本文档定义一个给外部应用实现的 HTTP Provider 契约。另一个应用只要实现这些接口，`atri-setu-api` 后续就可以把当前 Pixiv Web Ajax 对接替换为该 Provider，而图库、采集、审核和图片代理等内部业务继续使用同一套规范化数据。

本文档是目标接口规格，不表示当前代码已经实现这些 HTTP 调用。实现时应新增一个 Provider client，把本文档接口映射到当前 `pixiv.Client` 的同等能力。

## 基本约定

- Base URL 由部署配置提供，例如 `https://provider.example.com`。
- 所有 JSON 接口使用 `Content-Type: application/json; charset=utf-8`。
- 时间统一使用 RFC3339 字符串，例如 `2026-07-09T12:00:00Z`。
- 作品 ID、作者 ID 使用 JSON number 表达；实现方必须保证不超过 int64。
- Provider 返回的 `resource_url` 可以是完整 URL，也可以是 Provider 自有资源 ID，但必须能被资源代理接口消费。
- `cursor` 是 Provider 私有 JSON object；调用方只负责原样保存和回传。
- 认证建议使用 `Authorization: Bearer <token>`。如 Provider 部署在可信内网，也可以由反向代理负责认证。
- Provider 不得在错误响应中返回 cookie、token、代理密码等机密。

## 通用响应

成功响应直接返回业务 JSON，不再包一层 envelope。

错误响应统一为：

```json
{
  "error": {
    "code": "upstream_unavailable",
    "message": "作品不可用",
    "retryable": false,
    "details": {
      "pid": 123456
    }
  }
}
```

错误码约定：

| code | HTTP | retryable | 含义 |
| --- | --- | --- | --- |
| `invalid_argument` | 400 | false | 参数格式错误、缺少必填字段、mode 不支持 |
| `unauthorized` | 401 | false | Provider 调用凭据缺失或无效 |
| `forbidden` | 403 | false | 调用方无权访问该 Provider 能力 |
| `unsupported` | 422 | false | Provider 明确不支持该能力或模式 |
| `artwork_unavailable` | 404 | false | 作品被删、私密、受限或不存在 |
| `rate_limited` | 429 | true | Provider 或其上游限流 |
| `upstream_error` | 502 | true | 上游返回非预期错误 |
| `upstream_unavailable` | 503 | true | 上游网络不可达、代理不可用、服务不可用 |
| `malformed_upstream_response` | 502 | false | 上游响应结构无法解析 |
| `internal_error` | 500 | true | Provider 内部错误 |

## 能力发现

### GET `/v1/capabilities`

返回 Provider 支持的能力，用于管理后台展示和 source probe 的前置判断。

响应：

```json
{
  "provider": "example-pixiv-provider",
  "version": "1.0.0",
  "capabilities": {
    "detail": true,
    "pages": true,
    "ugoira": true,
    "image_proxy": true,
    "discovery": {
      "bookmarks": true,
      "user_artworks": true,
      "search": true,
      "ranking": true,
      "following_new": false,
      "rss_feed": false,
      "crawler": false
    }
  }
}
```

## 采集源探测

### POST `/v1/probe`

用指定 source 配置执行一次轻量探测。此接口不写入图库，也不推进正式 cursor。

请求：

```json
{
  "source_type": "pixiv_app_api",
  "params": {
    "mode": "search",
    "word": "初音ミク",
    "search_target": "tags_partial",
    "sort": "date_desc",
    "duration": "within_last_week",
    "limit": 5
  },
  "filters": {
    "r18_mode": "sfw_only",
    "media_type": ["illust", "manga"]
  }
}
```

响应：

```json
{
  "status": "ok",
  "checks": [
    {
      "field": "params.word",
      "status": "ok",
      "message": "搜索关键词可用"
    }
  ],
  "preview": {
    "summary": "找到 5 个样例作品",
    "total": 5,
    "pids": [123456, 234567],
    "samples": [
      {
        "pid": 123456,
        "title": "sample",
        "author_id": 3456,
        "author_name": "artist"
      }
    ]
  }
}
```

`status` 取值：

| status | 含义 |
| --- | --- |
| `ok` | 参数和上游能力可用，预览有结果 |
| `warning` | 调用成功但结果为空、能力部分受限或配置可能不完整 |
| `error` | 参数错误、认证失败、上游失败 |
| `unsupported` | Provider 明确不支持该 source type 或 mode |

## PID 发现

### POST `/v1/discover`

正式采集入口。Provider 按 source type、params、cursor 和 budget 返回一批 PID hit。

请求：

```json
{
  "source_id": "pixiv-search-miku",
  "source_type": "pixiv_app_api",
  "params": {
    "mode": "search",
    "word": "初音ミク",
    "search_target": "tags_partial",
    "sort": "date_desc",
    "duration": "",
    "limit": 50
  },
  "cursor": {
    "page": 1
  },
  "budget": {
    "max_pages": 1,
    "max_artworks": 50
  }
}
```

响应：

```json
{
  "hits": [
    {
      "pid": 123456,
      "discovered_at": "2026-07-09T12:00:00Z",
      "meta": {
        "mode": "search",
        "title": "sample",
        "author_id": 3456,
        "author_name": "artist",
        "tags": ["初音ミク"],
        "page_count": 1,
        "x_restrict": 0,
        "ai_type": 1,
        "illust_type": 0,
        "upload_date": "2026-07-08T12:00:00Z"
      }
    }
  ],
  "cursor": {
    "page": 2
  },
  "budget_used": {
    "pages": 1
  }
}
```

支持的 `source_type`：

| source_type | 说明 |
| --- | --- |
| `pixiv_app_api` | Pixiv 等价列表发现能力 |
| `rss_feed` | RSS/Atom feed 中解析 PID |
| `pixiv_crawler` | HTTP(S) 页面中按正则解析 PID |

`pixiv_app_api.params.mode`：

| mode | 必填参数 | 可选参数 | cursor |
| --- | --- | --- | --- |
| `bookmarks` | `user_id` | `restrict`, `limit` | `{ "offset": 0 }` |
| `user_artworks` | `user_id` | `limit` | `{ "offset": 0 }` |
| `search` | `word` | `search_target`, `sort`, `duration`, `limit` | `{ "page": 1 }` |
| `ranking` | 无 | `mode_rank`, `date`, `limit` | `{ "page": 1 }` |
| `following_new` | 无 | `restrict`, `limit` | `{ "page": 1 }` |

参数枚举：

| 参数 | 允许值 |
| --- | --- |
| `restrict` | `public`, `private` |
| `search_target` | `tags_partial`, `title`, `title_desc`, `content`, `keyword` |
| `sort` | `date_desc`, `date_asc`, `popularity` |
| `duration` | 空串, `within_last_day`, `within_last_week`, `within_last_month` |
| `mode_rank` | `day`, `week`, `month`, `day_male`, `day_female`, `week_original`, `week_rookie` |
| `date` | `YYYYMMDD` |

## 作品详情

### GET `/v1/artworks/{pid}`

返回单个作品的规范化详情。入库、stage 2 filter 和单页作品页面构造依赖此接口。

响应：

```json
{
  "pid": 123456,
  "title": "sample",
  "author_id": 3456,
  "author_name": "artist",
  "x_restrict": 0,
  "r18": false,
  "ai_type": 1,
  "illust_type": 0,
  "media_type": "illust",
  "page_count": 1,
  "bookmark_count": 1200,
  "view_count": 30000,
  "upload_date": "2026-07-08T12:00:00Z",
  "width": 1200,
  "height": 1800,
  "original_url": "https://i.pximg.net/img-original/img/2026/07/08/12/00/00/123456_p0.jpg",
  "tags": [
    {
      "name": "初音ミク",
      "translation_en": "Hatsune Miku"
    }
  ]
}
```

字段约束：

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `pid` | 是 | Pixiv 兼容作品 ID |
| `title` | 是 | 作品标题，可为空串但不能缺失 |
| `author_id` | 是 | 作者 UID；未知时填 0 |
| `author_name` | 是 | 作者名；未知时填空串 |
| `x_restrict` | 是 | Pixiv 兼容限制等级；大于 0 视作 R18 |
| `r18` | 是 | 与 `x_restrict > 0` 保持一致 |
| `ai_type` | 是 | 0 未标、1 非 AI、2/3 AI |
| `illust_type` | 是 | 0 插画、1 漫画、2 ugoira |
| `media_type` | 是 | `illust`, `manga`, `ugoira` |
| `page_count` | 是 | 总页数，至少 1 |
| `bookmark_count` | 是 | 收藏数，不可为负 |
| `view_count` | 是 | 浏览数，不可为负 |
| `upload_date` | 是 | RFC3339；未知时可用零时间但不推荐 |
| `width` / `height` | 是 | 首图尺寸 |
| `original_url` | 是 | 首图原图资源 |
| `tags` | 是 | 标签数组；无标签时为空数组 |

## 作品分页

### GET `/v1/artworks/{pid}/pages`

返回作品所有页面。`page_index` 必须从 0 开始且按升序排列。

响应：

```json
{
  "pages": [
    {
      "page_index": 0,
      "original_url": "https://i.pximg.net/img-original/img/2026/07/08/12/00/00/123456_p0.jpg",
      "ext": "jpg",
      "width": 1200,
      "height": 1800
    }
  ]
}
```

## Ugoira 元数据

### GET `/v1/artworks/{pid}/ugoira`

仅 `illust_type=2` 的作品需要此接口。非 ugoira 作品可以返回 404 `artwork_unavailable` 或 422 `unsupported`，调用方不会把它当作普通图片必需能力。

响应：

```json
{
  "pid": 123456,
  "src": "https://i.pximg.net/img-zip-ugoira/img/2026/07/08/12/00/00/123456_ugoira600x600.zip",
  "zip_url": "https://i.pximg.net/img-zip-ugoira/img/2026/07/08/12/00/00/123456_ugoira1920x1080.zip",
  "mime_type": "image/jpeg",
  "frames": [
    {
      "file": "000000.jpg",
      "delay": 60
    }
  ]
}
```

## 资源代理

### GET `/v1/resources`

Provider 负责把资源流式返回给 `atri-setu-api`，后者再把响应转发给客户端或审核 worker。

请求 query：

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `url` | 是 | 来自 `original_url`、分页 `original_url` 或 `zip_url` 的资源地址 |

请求头：

| Header | 说明 |
| --- | --- |
| `Range` | 可选，断点续传 |
| `If-None-Match` | 可选，缓存条件请求 |
| `If-Modified-Since` | 可选，缓存条件请求 |

响应：

- 成功时直接返回二进制 body。
- 应保留 `Content-Type`、`Content-Length`、`Cache-Control`、`ETag`、`Last-Modified`、`Expires`、`Age`、`Accept-Ranges`、`Content-Encoding`、`Vary` 等缓存和内容相关响应头。
- 不得返回上游 `Set-Cookie` 给调用方。

资源安全要求：

- Provider 必须只允许已知作品资源类型，例如原图、master 缩略图、裁剪缩略图、ugoira zip。
- 如果 `url` 是完整 URL，Provider 必须校验 scheme、host、path、query、fragment 和 userinfo，禁止 SSRF。
- 如果 Provider 使用自有资源 ID，应保证资源 ID 不可被构造成任意本地文件或内网 URL。

## Feed 和 Crawler 兼容

若 Provider 支持 `rss_feed`：

```json
{
  "source_type": "rss_feed",
  "params": {
    "url": "https://example.com/feed.xml",
    "pid_pattern": "artworks/(\\d+)",
    "source_hint": "pixiv/rss_self",
    "max_items": 50
  }
}
```

若 Provider 支持 `pixiv_crawler`：

```json
{
  "source_type": "pixiv_crawler",
  "params": {
    "url": "https://example.com/page.html",
    "pid_pattern": "(?:artworks/|i/|illust_id=)(\\d+)",
    "max_items": 50
  }
}
```

这两类能力只要求返回 PID hit。详情、分页和 ugoira 仍通过 `/v1/artworks/{pid}`、`/v1/artworks/{pid}/pages`、`/v1/artworks/{pid}/ugoira` 获取。

## 最小实现清单

另一个应用若想成为可用 Provider，最小需要实现：

- `GET /v1/capabilities`
- `POST /v1/probe`
- `POST /v1/discover`
- `GET /v1/artworks/{pid}`
- `GET /v1/artworks/{pid}/pages`
- `GET /v1/artworks/{pid}/ugoira`
- `GET /v1/resources?url=...`

第一阶段可以显式不支持 `following_new`、`rss_feed`、`pixiv_crawler`，但必须在 capabilities 和 probe 中返回 `unsupported`，不能伪装成空结果。

## 与当前项目字段映射

| Provider 字段 | 当前内部用途 |
| --- | --- |
| `pid` | `pixiv_artwork.pid`、`pixiv_page.pid`、`source_hit.pid` |
| `title` | `pixiv_artwork.title`、标题正则过滤 |
| `author_id` | `pixiv_artwork.author_id`、UID 黑白名单 |
| `author_name` | `pixiv_artwork.author_name` |
| `x_restrict` / `r18` | `pixiv_artwork.r18`、R18 filter、公开 API NSFW 过滤 |
| `ai_type` | `pixiv_artwork.ai_type`、AI 生成过滤 |
| `illust_type` / `media_type` | `pixiv_artwork.artwork_type`、media type filter、ugoira 分支 |
| `page_count` | 页数过滤、多页分页请求 |
| `bookmark_count` | `pixiv_artwork.bookmark_count`、收藏阈值过滤 |
| `view_count` | `pixiv_artwork.view_count`、浏览阈值过滤 |
| `upload_date` | `pixiv_artwork.upload_date`、时间窗过滤、排序 |
| `width` / `height` | `pixiv_page.width` / `height`、尺寸与长宽比过滤、审核输入 |
| `original_url` | `pixiv_page.original_url`、图片代理、缩略图派生 |
| `tags[].name` | `pixiv_tag.name`、`pixiv_artwork_tag`、tag filter/search |
| `tags[].translation_en` | `pixiv_tag.translations` |
| `frames` | `pixiv_ugoira_meta.frame_data`、ugoira 帧数过滤 |
| `zip_url` | `pixiv_ugoira_meta.zip_url`、ugoira zip 代理 |

## 兼容性要求

- Provider 返回字段可以增加，但不得删除本文档标为必填的字段。
- 调用方必须忽略未知字段。
- Provider 如需引入破坏性变更，应提升主版本，例如 `/v2/...`。
- `cursor` 内部结构只属于 Provider，同一 Provider 版本内应保持向后兼容。
- `meta` 内可以包含更多来源信息，但不得包含凭据或敏感 URL query。
