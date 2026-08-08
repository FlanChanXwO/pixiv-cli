# v1.0.0 SDK 使用伪代码

本页验证公开契约能支持常见的嵌入场景。代码是 Go-like pseudocode，用于设计和实现计划，
不是当前版本可编译的 example；v1 实现完成后应把对应场景改成 public external tests/examples。

## 从收藏中均匀随机抽取作品

这里的“随机收藏”指从指定用户可见的完整收藏集合中均匀抽取一个作品。不能只随机当前页，
也不能为了速度加入固定页数或条数上限。下面使用 reservoir sampling，因此不需要提前知道总数，
抽样状态保持常数级；重复 cursor 防护另外保存已访问 cursor，空间随页数增长。取消和整体运行
时间由调用方 context 控制。

```go
func RandomBookmark(
	ctx context.Context,
	client *pixiv.Client,
	userID int64,
	restrict pixiv.Restrict,
) (pixiv.Artwork, error) {
	request := pixiv.UserArtworkBookmarksRequest{
		UserID:   userID,
		Restrict: restrict,
	}

	var selected pixiv.Artwork
	var seen int64
	visited := map[string]struct{}{}

	for {
		page, err := client.UserArtworkBookmarks(ctx, request)
		if err != nil {
			return pixiv.Artwork{}, err
		}

		for _, artwork := range page.Items {
			seen++

			// 第 n 个元素以 1/n 概率替换当前结果，最终对全部元素均匀。
			draw, err := cryptorand.Int(cryptorand.Reader, big.NewInt(seen))
			if err != nil {
				return pixiv.Artwork{}, err
			}
			if draw.Sign() == 0 {
				selected = artwork
			}
		}

		if page.Next.IsZero() {
			break
		}

		encoded, err := page.Next.MarshalText()
		if err != nil {
			return pixiv.Artwork{}, err
		}
		key := string(encoded)
		if _, repeated := visited[key]; repeated {
			return pixiv.Artwork{}, ErrRepeatedCursor
		}
		visited[key] = struct{}{}
		request.Cursor = page.Next
	}

	if seen == 0 {
		return pixiv.Artwork{}, ErrNoBookmarks
	}
	return selected, nil
}
```

若需求是“从搜索结果随机选择一项并执行收藏”，复用相同遍历/抽样方式，把 page source 换成
`SearchArtworks`，得到候选后显式调用 mutation：

```go
candidate, err := RandomSearchResult(ctx, client, searchRequest)
if err != nil {
	return err
}

return client.AddBookmark(ctx, pixiv.AddBookmarkRequest{
	ArtworkID: candidate.ID,
	Restrict:  pixiv.RestrictPublic,
})
```

随机选择与 mutation 必须是两个可观察步骤；搜索、分页或随机源失败时不得继续执行收藏。

## Pixiv 图片反向代理

公开 `Resource` 同时提供两条路径：Go 服务可以把 opaque `ResourceRef` 交回 SDK 做受控读取；其他
runtime 可以使用 SDK 已验证的当前 `URL` 与非 secret `RequestHeaders` 直接流式反代。两条路径都
不要求资源落盘或完整缓冲。下游 route 仍需执行应用自身的用户鉴权；SDK 只保护上游 Pixiv 资源
边界，不决定谁可以访问你的代理。

### Go：通过 `ResourceRef` 读取

业务先通过 `Artwork`、`ArtworkPages` 或其他 SDK 模型取得 ref，再把其 Text codec 放进自己的
route/token。反代入口只接受 SDK 生成的 opaque ref，不接受任意 URL：

```go
func ServePixivImage(
	w http.ResponseWriter,
	r *http.Request,
	client *pixiv.Client,
) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// route 参数是 ResourceRef codec；禁止接受 https://... 形式的原始上游 URL。
	ref, err := sdk.ParseResourceRef(r.PathValue("resource_ref"))
	if err != nil {
		writeProxyError(w, http.StatusBadRequest, sdk.InvalidArgument)
		return
	}

	method := sdk.ResourceMethodGet
	if r.Method == http.MethodHead {
		method = sdk.ResourceMethodHead
	}

	response, err := client.OpenResource(r.Context(), sdk.OpenResourceRequest{
		Ref:             ref,
		Method:          method,
		Range:           r.Header.Get("Range"),
		IfNoneMatch:     r.Header.Get("If-None-Match"),
		IfModifiedSince: r.Header.Get("If-Modified-Since"),
		IfRange:         r.Header.Get("If-Range"),
	})
	if err != nil {
		// 只根据稳定 sdk.Reason 映射状态；不要把 Error() 或 cause 直接写给下游。
		writeProxySDKError(w, err)
		return
	}
	defer response.Body.Close()

	// Header 已由 SDK allowlist；仍使用 Set，避免和本地 middleware 的值叠加。
	for _, name := range []string{
		"Content-Type",
		"Content-Length",
		"Content-Range",
		"Accept-Ranges",
		"ETag",
		"Last-Modified",
		"Cache-Control",
	} {
		if value := response.Header.Get(name); value != "" {
			w.Header().Set(name, value)
		}
	}

	w.WriteHeader(response.StatusCode)
	if r.Method == http.MethodHead || response.StatusCode == http.StatusNotModified {
		return
	}

	if _, err := io.Copy(w, response.Body); err != nil {
		// Header/body 可能已提交，不能再伪造 JSON success/error body。
		// request context cancellation 会中止上游读取；仅记录脱敏的本地分类。
		recordCommittedProxyFailure(err)
	}
}
```

### Node.js：使用 `Resource.URL` 无落盘流式反代

下面的 `resource` 必须来自应用可信存储或 Go SDK 序列化的模型，不能由请求方提交任意
`url`/`request_headers` JSON。示例拒绝 redirect；若业务需要跟随 redirect，应逐跳执行与 SDK
等价的 scheme/host/path 校验，不能直接改成自动跟随。

```ts
import { Readable } from "node:stream";

type Resource = {
  ref: string;
  url: string;
  request_headers?: Record<string, string>;
  expires_at?: string;
  requires_credentials?: boolean;
};

async function serveResource(req, res, resource: Resource) {
  if (req.method !== "GET" && req.method !== "HEAD") {
    res.writeHead(405).end();
    return;
  }
  if (resource.requires_credentials) {
    // secret 不会出现在 Resource 中；此类资源必须转给持有 Client 的 OpenResource 路径。
    res.writeHead(502).end();
    return;
  }

  const headers = new Headers(resource.request_headers ?? {});
  for (const name of ["range", "if-none-match", "if-modified-since", "if-range"]) {
    const value = req.headers[name];
    if (typeof value === "string") headers.set(name, value);
  }

  const abort = new AbortController();
  req.once("aborted", () => abort.abort());
  res.once("close", () => abort.abort());

  const upstream = await fetch(resource.url, {
    method: req.method,
    headers,
    redirect: "error",
    signal: abort.signal,
  });

  for (const name of [
    "content-type", "content-length", "content-range", "accept-ranges",
    "etag", "last-modified", "cache-control",
  ]) {
    const value = upstream.headers.get(name);
    if (value !== null) res.setHeader(name, value);
  }

  res.writeHead(upstream.status);
  if (req.method === "HEAD" || upstream.status === 204 || upstream.status === 304) {
    await upstream.body?.cancel();
    res.end();
    return;
  }
  if (upstream.body === null) {
    res.end();
    return;
  }

  // Readable.fromWeb 直接 backpressure-aware pipe，不写文件、不把完整图片读入内存。
  Readable.fromWeb(upstream.body).pipe(res);
}
```

`Resource.URL` 是当前定位符，不是永久 ID；过期后重新执行对应 detail/pages operation 获取新的
`Resource`。cache identity 使用 `ResourceRef`，不能使用可能轮换的签名 URL。不得把
`RequestHeaders` 与下游 Cookie/Authorization 合并，也不得在日志中记录完整 signed query。

推荐的稳定 HTTP 映射：

```text
invalid_argument / invalid_cursor → 400
unauthorized / credentials_expired → 401
forbidden / resource_forbidden → 403
not_found → 404
content_unavailable → 410
rate_limited → 429（仅在 HasAfter 时输出计算后的 Retry-After）
challenge_required → 503
upstream_error / malformed_upstream_response → 502
upstream_unavailable → 503
```

反代实现还必须遵守：

- 不使用 `httputil.ReverseProxy` 接收调用方提供的 target。
- 不把下游 `Authorization`、Cookie、Referer 或任意 header 转发给 Pixiv。
- SDK 逐跳验证 redirect，最终 upstream URL 永不出现在 response、日志或 cache key。
- cache key 使用稳定 `ResourceRef` 与 representation 条件，不使用签名 URL。
- Range/conditional request 保持状态码和 header 语义，不预读或按固定大小截断 body。
- 代理关闭或客户端取消时沿 request context 取消上游读取，不添加固定流式 timeout。
- 如果代理可能暴露私密收藏、R18 或受账号权限控制的资源，必须在调用 SDK 前完成下游授权。
