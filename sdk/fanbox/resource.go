// FANBOX 资源协议：把已验证的媒体定位封装成 sdk.Resource，并按引用重新解析。
// 会话与 locator 缓存在 internal/fanboxapi；下载写入与进度在下方。

package fanbox

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	protocol "github.com/FlanChanXwO/pixiv-cli/internal/fanboxapi"
	atomicfile "github.com/FlanChanXwO/pixiv-cli/internal/storage/atomicfile"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// fanboxReferer is the non-secret referer FANBOX media requests carry.
const fanboxReferer = protocol.Referer

// resourceRefPayload is the opaque identity payload embedded in a FANBOX
// ResourceRef. It carries only stable identity (kind, owning creator or post,
// and attachment id) so locator rotation never changes the cache key and a
// stored ref can be reopened across sessions. A resolved URL is never encoded
// here: OpenResource resolves a fresh, validated locator from trusted
// metadata. PostID is empty for creator-scoped kinds (icon/cover); AssetID is
// empty for creator-scoped kinds and is the attachment id for post assets.
type resourceRefPayload struct {
	Kind    string `json:"k"`
	Creator string `json:"c,omitempty"`
	Post    string `json:"p,omitempty"`
	Asset   string `json:"a,omitempty"`
}

func (c *Client) validateResourceURL(rawURL string) error {
	if err := protocol.ValidateURL(rawURL); err != nil {
		return newError("resource", sdk.ResourceForbidden, errors.New("resource host is not allowed"))
	}
	return nil
}

// newResource builds a validated sdk.Resource from an upstream media URL and
// the stable identity that owns it. The URL is exposed as the currently-usable
// locator (Resource.URL) and cached in-session keyed by the opaque ref; the ref
// envelope encodes only stable identity. RequiresCredentials reflects whether
// the locator lives on the credentialed downloads host.
func (c *Client) newResource(kind, creatorID, postID, assetID, rawURL, filename string) (sdk.Resource, error) {
	if rawURL == "" {
		return sdk.Resource{}, nil
	}
	if err := c.validateResourceURL(rawURL); err != nil {
		return sdk.Resource{}, err
	}
	payload, err := json.Marshal(resourceRefPayload{Kind: kind, Creator: creatorID, Post: postID, Asset: assetID})
	if err != nil {
		return sdk.Resource{}, newError("resource", sdk.UpstreamError, err)
	}
	ref, err := sdk.NewResourceRef(product, payload)
	if err != nil {
		return sdk.Resource{}, newError("resource", sdk.UpstreamError, err)
	}
	c.app.CacheLocator(ref.String(), protocol.Locator{URL: rawURL, Filename: filename})
	return sdk.Resource{
		Ref:                 ref,
		URL:                 rawURL,
		RequestHeaders:      map[string]string{"Referer": fanboxReferer},
		RequiresCredentials: protocol.RequiresCredentials(rawURL),
	}, nil
}

// OpenResource opens a resource by its opaque reference. The caller owns and
// must close response.Body. Resource requests carry the session cookie only on
// the credentialed downloads host; redirects are revalidated against the media
// host allowlist. The ref's stable identity is re-resolved to a fresh locator
// so a stored ref never reopens a stale embedded URL.
func (c *Client) OpenResource(ctx context.Context, request sdk.OpenResourceRequest) (*sdk.ResourceResponse, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	locator, err := c.resolveResource(ctx, request.Ref, "OpenResource")
	if err != nil {
		return nil, err
	}
	if err := c.validateResourceURL(locator.URL); err != nil {
		return nil, newError("OpenResource", sdk.ResourceForbidden, errors.New("resource URL is not allowed"))
	}
	response, err := c.app.OpenMedia(ctx, locator.URL, protocol.MediaRequest{
		Method:          string(request.Method),
		Range:           request.Range,
		IfNoneMatch:     request.IfNoneMatch,
		IfModifiedSince: request.IfModifiedSince,
		IfRange:         request.IfRange,
	})
	if err != nil {
		return nil, classifyError("OpenResource", err)
	}
	result := sdk.NewResourceResponse(response.StatusCode, response.Header, response.Body)
	result.Filename = locator.Filename
	return result, nil
}

// SaveResource writes a single resource by its opaque reference to Path through
// an atomic destination. It does not expand posts, creators, archives, or
// sidecars; use the downloader for those.
func (c *Client) SaveResource(ctx context.Context, ref sdk.ResourceRef, options sdk.SaveOptions) (sdk.SavedResource, error) {
	if strings.TrimSpace(options.Path) == "" {
		return sdk.SavedResource{}, newError("SaveResource", sdk.InvalidArgument, errors.New("destination path is required"))
	}
	response, err := c.OpenResource(ctx, sdk.OpenResourceRequest{Ref: ref, Method: sdk.ResourceMethodGet})
	if err != nil {
		return sdk.SavedResource{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return sdk.SavedResource{}, newError("SaveResource", sdk.UpstreamError, errors.New("resource returned a non-success status"))
	}
	size, err := atomicfile.Write(ctx, options.Path, &progressReader{src: response.Body, progress: options.Progress})
	if err != nil {
		return sdk.SavedResource{}, newError("SaveResource", sdk.LocalStateError, errors.New("cannot write resource"))
	}
	return sdk.SavedResource{Path: options.Path, Size: size}, nil
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
	if err := json.Unmarshal(payload, &rp); err != nil || rp.Kind == "" {
		return resourceRefPayload{}, errors.New("malformed resource reference")
	}
	return rp, nil
}

// resolveResource keeps the URL and attachment name in the same cache entry.
// Reopened references resolve metadata by kind and stable identity, never by URL.
func (c *Client) resolveResource(ctx context.Context, ref sdk.ResourceRef, operation string) (protocol.Locator, error) {
	rp, err := c.decodeResourceRef(ref)
	if err != nil {
		return protocol.Locator{}, newError(operation, sdk.InvalidArgument, errors.New("invalid resource reference"))
	}
	if locator, ok := c.app.CachedLocator(ref.String()); ok && locator.URL != "" {
		return locator, nil
	}
	var locator protocol.Locator
	switch rp.Kind {
	case "creator_icon", "creator_cover":
		locator.URL, err = c.resolveCreatorAssetURL(ctx, rp)
	case "post_image", "post_file":
		locator, err = c.resolvePostAsset(ctx, rp)
	default:
		return protocol.Locator{}, newError(operation, sdk.InvalidArgument, errors.New("resource kind is unsupported"))
	}
	if err != nil {
		if _, ok := err.(*sdk.Error); ok {
			return protocol.Locator{}, err
		}
		return protocol.Locator{}, newError(operation, sdk.MalformedUpstreamResponse, errors.New("resource metadata has no usable URL"))
	}
	if err := c.validateResourceURL(locator.URL); err != nil {
		return protocol.Locator{}, newError(operation, sdk.ResourceForbidden, errors.New("resolved resource URL is not allowed"))
	}
	c.app.CacheLocator(ref.String(), locator)
	return locator, nil
}

// resolveCreatorAssetURL re-resolves a creator icon or cover locator by
// re-fetching the creator profile and selecting the field matching the kind.
func (c *Client) resolveCreatorAssetURL(ctx context.Context, rp resourceRefPayload) (string, error) {
	if rp.Creator == "" {
		return "", errors.New("creator resource reference is missing creator id")
	}
	profile, err := c.creatorProfile(ctx, rp.Creator)
	if err != nil {
		return "", classifyError("resource", err)
	}
	switch rp.Kind {
	case "creator_icon":
		if iconURL := strings.TrimSpace(profile.User.IconURL); iconURL != "" {
			return iconURL, nil
		}
		return "", errors.New("creator icon is unavailable")
	case "creator_cover":
		if coverURL := strings.TrimSpace(profile.CoverImageURL); coverURL != "" {
			return coverURL, nil
		}
		return "", errors.New("creator cover is unavailable")
	default:
		return "", errors.New("unsupported creator resource kind")
	}
}

// resolvePostAsset selects kind as well as ID: image and file ID spaces may overlap.
func (c *Client) resolvePostAsset(ctx context.Context, rp resourceRefPayload) (protocol.Locator, error) {
	if rp.Post == "" || rp.Asset == "" {
		return protocol.Locator{}, errors.New("post resource reference is missing post or attachment id")
	}
	source, err := c.postInfo(ctx, rp.Post)
	if err != nil {
		return protocol.Locator{}, classifyError("resource", err)
	}
	if source.Body == nil {
		return protocol.Locator{}, errors.New("post body is unavailable")
	}
	images, files, err := bodyMedia(source.Body)
	if err != nil {
		return protocol.Locator{}, classifyError("resource", err)
	}
	if rp.Kind == "post_image" {
		for _, image := range images {
			if image.ID == rp.Asset && image.OriginalURL != "" {
				return protocol.Locator{URL: image.OriginalURL}, nil
			}
		}
	} else if rp.Kind == "post_file" {
		for _, file := range files {
			if file.ID == rp.Asset && file.URL != "" {
				return protocol.Locator{URL: file.URL, Filename: attachmentFilename(file.Name, file.Extension)}, nil
			}
		}
	}
	return protocol.Locator{}, errors.New("post attachment is unavailable")
}

// attachmentFilename retains upstream naming, without guessing a name from a locator.
func attachmentFilename(name, extension string) string {
	if name == "" || extension == "" || strings.HasSuffix(strings.ToLower(name), "."+strings.ToLower(extension)) {
		return name
	}
	return name + "." + extension
}

type progressReader struct {
	src      io.Reader
	progress func(sdk.SaveProgress)
	done     int64
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.src.Read(p)
	if n > 0 {
		r.done += int64(n)
		if r.progress != nil {
			r.progress(sdk.SaveProgress{Done: r.done})
		}
	}
	return n, err
}
