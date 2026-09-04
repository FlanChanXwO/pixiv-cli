package pixiv

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/novel"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/resource"
	atomicfile "github.com/FlanChanXwO/pixiv-cli/internal/storage/file/atomic"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// resourceRefPayload is the opaque identity payload embedded in a Pixiv
// ResourceRef. It carries only stable identity fields; a resolved or signed
// URL is kept in the client registry or reconstructed from trusted metadata.
type resourceRefPayload struct {
	Kind    string `json:"k"`
	ID      int64  `json:"id"`
	Page    int    `json:"p,omitempty"`
	Variant string `json:"v,omitempty"`
}

// defaultResourceHosts are the official Pixiv media hosts whose HTTPS URLs are
// accepted for resource reads.
var defaultResourceHosts = []string{"i.pximg.net", "s.pximg.net", "i-f.pximg.net"}

// newResource builds a validated sdk.Resource from an upstream media URL. It
// returns ResourceForbidden when the URL does not satisfy the resource
// policy and InvalidArgument when the identity payload is invalid.
func (c *Client) newResource(kind string, id int64, page int, rawURL string) (sdk.Resource, error) {
	return c.newResourceWithVariant(kind, id, page, "", rawURL)
}

func (c *Client) newResourceWithVariant(kind string, id int64, page int, variant, rawURL string) (sdk.Resource, error) {
	if rawURL == "" {
		return sdk.Resource{}, nil
	}
	if err := c.validateResourceURL(rawURL); err != nil {
		return sdk.Resource{}, err
	}
	payload, err := json.Marshal(resourceRefPayload{Kind: kind, ID: id, Page: page, Variant: variant})
	if err != nil {
		return sdk.Resource{}, newError("resource", sdk.UpstreamError, "cannot encode resource reference")
	}
	ref, err := sdk.NewResourceRef(product, payload)
	if err != nil {
		return sdk.Resource{}, newError("resource", sdk.UpstreamError, "cannot encode resource reference")
	}
	c.resourceMu.Lock()
	c.resourceURLs[ref.String()] = rawURL
	c.resourceMu.Unlock()
	return sdk.Resource{
		Ref:            ref,
		URL:            rawURL,
		RequestHeaders: map[string]string{"Referer": protocol.AppReferer},
	}, nil
}

// ArtworkVariantResource 返回同一页的另一个质量变体的 ResourceRef。
// 它解码既有 ref 的稳定身份并替换 variant，使 SaveResource 走与 original
// 相同的 revalidation / re-resolution 路径。variant 为空或 "original" 时
// 原样返回 original.Ref。只支持 artwork kind；其他 kind 报错。
func ArtworkVariantResource(original sdk.Resource, variant string) (sdk.ResourceRef, error) {
	if variant == "" || variant == "original" {
		return original.Ref, nil
	}
	payload, err := sdk.ResourceRefPayload(original.Ref)
	if err != nil {
		return sdk.ResourceRef{}, err
	}
	var rp resourceRefPayload
	if err := json.Unmarshal(payload, &rp); err != nil || rp.Kind == "" || rp.ID <= 0 {
		return sdk.ResourceRef{}, newError("resource", sdk.InvalidArgument, "cannot decode resource reference")
	}
	if rp.Kind != "artwork" {
		return sdk.ResourceRef{}, newError("resource", sdk.InvalidArgument, "only artwork resources support quality variants")
	}
	rp.Variant = variant
	encoded, err := json.Marshal(rp)
	if err != nil {
		return sdk.ResourceRef{}, newError("resource", sdk.UpstreamError, "cannot encode resource reference")
	}
	ref, err := sdk.NewResourceRef(product, encoded)
	if err != nil {
		return sdk.ResourceRef{}, newError("resource", sdk.UpstreamError, "cannot encode resource reference")
	}
	return ref, nil
}

func (c *Client) validateResourceURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return newError("resource", sdk.ResourceForbidden, "invalid resource URL")
	}
	if parsed.Scheme != "https" || parsed.User != nil {
		return newError("resource", sdk.ResourceForbidden, "resource URL must be https without userinfo")
	}
	if parsed.Path == "" || parsed.Path == "/" {
		return newError("resource", sdk.ResourceForbidden, "resource URL has no path")
	}
	host := strings.ToLower(parsed.Hostname())
	if !allowedResourceHost(host, c.opts.ResourcePolicy.AllowedHosts) {
		return newError("resource", sdk.ResourceForbidden, "resource host is not allowed")
	}
	return nil
}

func allowedResourceHost(host string, extra []string) bool {
	for _, allowed := range defaultResourceHosts {
		if host == allowed {
			return true
		}
	}
	for _, allowed := range extra {
		if host == strings.ToLower(strings.TrimSpace(allowed)) {
			return true
		}
	}
	return false
}

// OpenResource opens a resource by its opaque reference and returns an
// allowlisted response. The caller owns and must close response.Body. The
// resource URL, redirects, headers, and identity are revalidated before any
// bytes are streamed; cookies are never sent for resource requests.
func (c *Client) OpenResource(ctx context.Context, request sdk.OpenResourceRequest) (*sdk.ResourceResponse, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	rawURL, err := c.resolveResourceURL(ctx, request.Ref, "OpenResource")
	if err != nil {
		return nil, err
	}
	if err := c.validateResourceURL(rawURL); err != nil {
		return nil, newError("OpenResource", sdk.ResourceForbidden, "resource URL is not allowed")
	}
	header := http.Header{}
	header.Set("Referer", protocol.AppReferer)
	for _, field := range []struct{ name, value string }{
		{"Range", request.Range},
		{"If-None-Match", request.IfNoneMatch},
		{"If-Modified-Since", request.IfModifiedSince},
		{"If-Range", request.IfRange},
	} {
		if field.value != "" {
			header.Set(field.name, field.value)
		}
	}
	method := string(request.Method)
	if method == "" {
		method = http.MethodGet
	}
	response, err := c.resClient.Open(ctx, resource.OpenRequest{
		URL:            rawURL,
		Method:         method,
		Header:         header,
		Validate:       c.validateResourceURL,
		DisableCookies: true,
	})
	if err != nil {
		return nil, classifyAppError(err, "OpenResource")
	}
	return sdk.NewResourceResponse(response.StatusCode, response.Header, response.Body), nil
}

// SaveResource conditionally reads a single resource by its opaque reference
// and writes it to Path through an atomic destination, so a partially
// transferred file never appears at the final path. It does not expand creators,
// archives, sidecars, or ugoira batches; use the downloader for those.
func (c *Client) SaveResource(ctx context.Context, ref sdk.ResourceRef, options sdk.SaveOptions) (sdk.SavedResource, error) {
	if strings.TrimSpace(options.Path) == "" {
		return sdk.SavedResource{}, newError("SaveResource", sdk.InvalidArgument, "destination path is required")
	}
	rawURL, err := c.resolveResourceURL(ctx, ref, "SaveResource")
	if err != nil {
		return sdk.SavedResource{}, err
	}
	if err := c.validateResourceURL(rawURL); err != nil {
		return sdk.SavedResource{}, newError("SaveResource", sdk.ResourceForbidden, "resource URL is not allowed")
	}
	response, err := c.OpenResource(ctx, sdk.OpenResourceRequest{Ref: ref, Method: sdk.ResourceMethodGet})
	if err != nil {
		return sdk.SavedResource{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return sdk.SavedResource{}, newError("SaveResource", sdk.UpstreamError, "resource returned a non-success status")
	}
	size, err := atomicfile.Write(ctx, options.Path, &progressReader{src: response.Body, total: response.ContentLength(), progress: options.Progress})
	if err != nil {
		return sdk.SavedResource{}, newError("SaveResource", sdk.LocalStateError, "cannot write resource")
	}
	return sdk.SavedResource{Path: options.Path, Size: size, ContentType: response.ContentType()}, nil
}

// SaveResourceURL 将经过资源策略校验的 Pixiv CDN URL 保存到 Path。它复用
// ResourceClient 的 Referer、重定向校验和禁用 cookie 行为，并通过原子目标
// 写入，避免失败传输留下最终文件。
func (c *Client) SaveResourceURL(ctx context.Context, rawURL string, options sdk.SaveOptions) (sdk.SavedResource, error) {
	const operation = "SaveResourceURL"
	if strings.TrimSpace(options.Path) == "" {
		return sdk.SavedResource{}, newError(operation, sdk.InvalidArgument, "destination path is required")
	}
	if err := c.validateResourceURL(rawURL); err != nil {
		return sdk.SavedResource{}, newError(operation, sdk.ResourceForbidden, "resource URL is not allowed")
	}

	stream, err := c.resClient.Open(ctx, resource.OpenRequest{
		URL:            rawURL,
		Method:         http.MethodGet,
		Header:         http.Header{"Referer": {protocol.AppReferer}},
		Validate:       c.validateResourceURL,
		DisableCookies: true,
	})
	if err != nil {
		return sdk.SavedResource{}, classifyAppError(err, operation)
	}
	response := sdk.NewResourceResponse(stream.StatusCode, stream.Header, stream.Body)
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return sdk.SavedResource{}, newError(operation, sdk.UpstreamError, "resource returned a non-success status")
	}

	size, err := atomicfile.Write(ctx, options.Path, &progressReader{
		src:      response.Body,
		total:    response.ContentLength(),
		progress: options.Progress,
	})
	if err != nil {
		return sdk.SavedResource{}, newError(operation, sdk.LocalStateError, "cannot write resource")
	}
	return sdk.SavedResource{Path: options.Path, Size: size, ContentType: response.ContentType()}, nil
}

type progressReader struct {
	src      io.Reader
	total    int64
	progress func(sdk.SaveProgress)
	done     int64
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.src.Read(p)
	if n > 0 {
		r.done += int64(n)
		if r.progress != nil {
			r.progress(sdk.SaveProgress{Total: r.total, Done: r.done})
		}
	}
	return n, err
}

func (c *Client) decodeResourceRef(ref sdk.ResourceRef) (resourceRefPayload, error) {
	if ref.IsZero() {
		return resourceRefPayload{}, errors.New("zero resource reference")
	}
	refProduct, err := sdk.ResourceRefProduct(ref)
	if err != nil || refProduct != product {
		return resourceRefPayload{}, errors.New("resource reference belongs to another product")
	}
	payload, err := sdk.ResourceRefPayload(ref)
	if err != nil {
		return resourceRefPayload{}, err
	}
	var rp resourceRefPayload
	if err := json.Unmarshal(payload, &rp); err != nil || rp.Kind == "" || rp.ID <= 0 || rp.Page < -1 {
		return resourceRefPayload{}, errors.New("malformed resource reference")
	}
	return rp, nil
}

func (c *Client) resolveResourceURL(ctx context.Context, ref sdk.ResourceRef, operation string) (string, error) {
	rp, err := c.decodeResourceRef(ref)
	if err != nil {
		return "", newError(operation, sdk.InvalidArgument, "invalid resource reference")
	}
	c.resourceMu.RLock()
	rawURL := c.resourceURLs[ref.String()]
	c.resourceMu.RUnlock()
	if rawURL != "" {
		return rawURL, nil
	}

	switch rp.Kind {
	case "artwork":
		detail, appErr := c.artworkDetail.Artwork(ctx, rp.ID)
		if appErr != nil {
			return "", classifyAppError(appErr, operation)
		}
		rawURL, err = artworkResourceURL(detail.Artwork, rp.Page, rp.Variant)
	case "novel_cover":
		detail, appErr := c.novelDetail.Detail(ctx, rp.ID)
		if appErr != nil {
			return "", classifyAppError(appErr, operation)
		}
		rawURL, err = imageURLsResourceURL(detail.Novel.ImageURLs, rp.Variant)
	case "user_profile":
		detail, appErr := c.userDetail.Detail(ctx, rp.ID)
		if appErr != nil {
			return "", classifyAppError(appErr, operation)
		}
		rawURL, err = profileImageResourceURL(detail.User.ProfileImageURLs)
	case "ugoira_archive":
		result, appErr := c.artworkDetail.UgoiraMetadata(ctx, rp.ID)
		if appErr != nil {
			return "", classifyAppError(appErr, operation)
		}
		rawURL, err = ugoiraResourceURL(result.Metadata, rp.Variant)
	case "novel_image", "novel_file":
		raw, appErr := c.novelDetail.Content(ctx, rp.ID)
		if appErr != nil {
			return "", classifyAppError(appErr, operation)
		}
		rawURL, err = c.novelContentResourceURL(rp, raw)
	default:
		return "", newError(operation, sdk.InvalidArgument, "resource kind is unsupported")
	}
	if err != nil {
		if _, ok := err.(*sdk.Error); ok {
			return "", err
		}
		return "", newError(operation, sdk.MalformedUpstreamResponse, "resource metadata has no usable URL")
	}
	if err := c.validateResourceURL(rawURL); err != nil {
		return "", newError(operation, sdk.ResourceForbidden, "resolved resource URL is not allowed")
	}
	c.resourceMu.Lock()
	c.resourceURLs[ref.String()] = rawURL
	c.resourceMu.Unlock()
	return rawURL, nil
}

func artworkResourceURL(illust artwork.Artwork, page int, variant string) (string, error) {
	if page >= 0 {
		for _, candidate := range illust.MetaPages {
			if candidate.PageIndex == page {
				if variant == "original" || variant == "" {
					if value := firstArtworkImageURL(candidate.ImageURLs); value != "" {
						return value, nil
					}
					return "", errors.New("artwork page is unavailable")
				}
				return artworkImageURLsResourceURL(candidate.ImageURLs, variant)
			}
		}
		if page == 0 && illust.MetaSinglePage.OriginalImageURL != "" {
			if variant != "" && variant != "original" {
				if derived, ok := deriveImageVariantURL(illust.MetaSinglePage.OriginalImageURL, variant); ok {
					return derived, nil
				}
				return "", errors.New("requested image variant is unavailable")
			}
			return illust.MetaSinglePage.OriginalImageURL, nil
		}
		return "", errors.New("artwork page is unavailable")
	}
	return artworkImageURLsResourceURL(illust.ImageURLs, variant)
}

func imageURLsResourceURL(urls novel.ImageURLs, variant string) (string, error) {
	values := map[string]string{
		"original":      urls.Original,
		"large":         urls.Large,
		"medium":        urls.Medium,
		"square_medium": urls.SquareMedium,
	}
	if variant != "" {
		if value := values[variant]; value != "" {
			return value, nil
		}
		return "", errors.New("requested image variant is unavailable")
	}
	if value := firstAvailable(urls); value != "" {
		return value, nil
	}
	return "", errors.New("image URL is unavailable")
}

func artworkImageURLsResourceURL(urls artwork.ImageURLs, variant string) (string, error) {
	values := map[string]string{
		"original":      urls.Original,
		"large":         urls.Large,
		"medium":        urls.Medium,
		"square_medium": urls.SquareMedium,
	}
	if variant != "" {
		if value := values[variant]; value != "" {
			return value, nil
		}
		if derived, ok := deriveImageVariantURL(urls.Original, variant); ok {
			return derived, nil
		}
		return "", errors.New("requested image variant is unavailable")
	}
	if value := firstArtworkImageURL(urls); value != "" {
		return value, nil
	}
	return "", errors.New("image URL is unavailable")
}

// deriveImageVariantURL 在 upstream 没有直接提供请求质量时，按 Pixiv 的公开
// img-master / 缩略图路径约定从 original locator 推导对应 URL。variant 取
// regular / small / thumb / mini（与下载器公开的 DownloadQuality 一致）；
// 这些 locator 不携带签名参数，cache key 由稳定身份决定。
func deriveImageVariantURL(originalURL, variant string) (string, bool) {
	original := strings.TrimSpace(originalURL)
	if original == "" {
		return "", false
	}
	parsed, err := url.Parse(original)
	if err != nil || parsed.Scheme != "https" || parsed.Path == "" {
		return "", false
	}
	prefix, crop, suffix, ok := variantPathComponents(variant)
	if !ok {
		return "", false
	}
	// img-original/img/.../FILENAME.ext → img-master/img/.../FILENAME_{suffix}.jpg
	marker := "/img-original/img/"
	index := strings.Index(parsed.Path, marker)
	if index < 0 {
		return "", false
	}
	rel := parsed.Path[index+len(marker):]
	base := rel[strings.LastIndex(rel, "/")+1:]
	stem := base
	if dot := strings.LastIndex(base, "."); dot > 0 {
		stem = base[:dot]
	}
	derivedPath := prefix + crop + "img-master/img/" + rel[:len(rel)-len(base)] + stem + suffix + ".jpg"
	clone := *parsed
	clone.Path = derivedPath
	clone.RawQuery = ""
	clone.Fragment = ""
	return clone.String(), true
}

// variantPathComponents 返回某质量的 img-master 前缀、裁剪段与文件名后缀。
// 前缀以 / 开头；crop 形如 "c/1200x1200/" 或为空。
func variantPathComponents(variant string) (prefix, crop, suffix string, ok bool) {
	switch variant {
	case "regular":
		return "/", "c/1200x1200/", "_master1200", true
	case "small":
		return "/", "c/540x540_70/", "_master1200", true
	case "thumb":
		return "/", "c/250x250_80_a2/", "_square1200", true
	case "mini":
		return "/", "c/48x48/", "_square1200", true
	default:
		return "", "", "", false
	}
}

func profileImageResourceURL(urls user.ProfileImageURLs) (string, error) {
	if urls.Medium == nil || *urls.Medium == "" {
		return "", errors.New("profile image is unavailable")
	}
	return *urls.Medium, nil
}

func ugoiraResourceURL(meta artwork.UgoiraMetadata, variant string) (string, error) {
	values := map[string]string{
		"medium":   meta.ZipURLs.Medium,
		"original": meta.ZipURLs.Original,
	}
	if variant != "" {
		if value := values[variant]; value != "" {
			return value, nil
		}
		return "", errors.New("requested ugoira archive is unavailable")
	}
	for _, value := range []string{values["original"], values["medium"]} {
		if value != "" {
			return value, nil
		}
	}
	return "", errors.New("ugoira archive is unavailable")
}

func (c *Client) novelContentResourceURL(rp resourceRefPayload, raw []byte) (string, error) {
	content, err := c.parseNovelContent(rp.ID, raw)
	if err != nil {
		return "", err
	}
	if rp.Page < 0 {
		return "", errors.New("novel resource index is invalid")
	}
	index := 0
	for _, block := range content.Blocks {
		switch rp.Kind {
		case "novel_image":
			if block.Kind != NovelBlockImage {
				continue
			}
			if index == rp.Page {
				if block.Image == nil {
					return "", errors.New("novel image block is unavailable")
				}
				return block.Image.Resource.URL, nil
			}
		case "novel_file":
			if block.Kind != NovelBlockFile {
				continue
			}
			if index == rp.Page {
				if block.File == nil {
					return "", errors.New("novel file block is unavailable")
				}
				return block.File.Resource.URL, nil
			}
		default:
			return "", errors.New("novel resource kind is unsupported")
		}
		index++
	}
	return "", errors.New("novel resource is unavailable")
}
