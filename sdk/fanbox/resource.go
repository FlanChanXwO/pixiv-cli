// FANBOX 资源协议：把已验证的媒体定位封装成 sdk.Resource，并按引用重新解析。
// 会话与 locator 缓存在 internal/fanboxapi；原子写入与进度共用 sdk.WriteResource。

package fanbox

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/fanboxapi"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

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
	if err := fanboxapi.ValidateURL(rawURL); err != nil {
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
	c.app.CacheLocator(ref.String(), fanboxapi.Locator{URL: rawURL, Filename: filename})
	return sdk.Resource{
		Ref:                 ref,
		URL:                 rawURL,
		RequestHeaders:      map[string]string{"Referer": fanboxapi.Referer},
		RequiresCredentials: fanboxapi.RequiresCredentials(rawURL),
	}, nil
}

// OpenResource opens a resource by its opaque reference. The caller owns and
// must close response.Body. Resource requests carry the session cookie only on
// the credentialed downloads host; redirects are revalidated against the media
// host allowlist. The ref's stable identity is re-resolved to a fresh locator
// so a stored ref never reopens a stale embedded URL.
func (c *Client) OpenResource(ctx context.Context, request sdk.OpenResourceRequest) (*sdk.ResourceResponse, error) {
	ctx = fanboxapi.WithOperation(ctx, "OpenResource")
	if err := request.Validate(); err != nil {
		return nil, err
	}
	locator, err := c.resolveResource(ctx, request.Ref, "OpenResource")
	if err != nil {
		return nil, err
	}
	response, err := c.app.OpenMedia(ctx, locator.URL, fanboxapi.MediaRequest{
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
	ctx = fanboxapi.WithOperation(ctx, "SaveResource")
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
	size, err := sdk.WriteResource(ctx, product, "SaveResource", response.Body, 0, options)
	if err != nil {
		return sdk.SavedResource{}, err
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
func (c *Client) resolveResource(ctx context.Context, ref sdk.ResourceRef, operation string) (fanboxapi.Locator, error) {
	rp, err := c.decodeResourceRef(ref)
	if err != nil {
		return fanboxapi.Locator{}, newError(operation, sdk.InvalidArgument, errors.New("invalid resource reference"))
	}
	if locator, ok := c.app.CachedLocator(ref.String()); ok {
		return locator, nil
	}
	var locator fanboxapi.Locator
	switch rp.Kind {
	case "creator_icon", "creator_cover":
		locator.URL, err = c.resolveCreatorAssetURL(ctx, rp)
	case "post_image", "post_file":
		locator, err = c.resolvePostAsset(ctx, rp)
	default:
		return fanboxapi.Locator{}, newError(operation, sdk.InvalidArgument, errors.New("resource kind is unsupported"))
	}
	if err != nil {
		if _, ok := err.(*sdk.Error); ok {
			return fanboxapi.Locator{}, err
		}
		return fanboxapi.Locator{}, newError(operation, sdk.MalformedUpstreamResponse, errors.New("resource metadata has no usable URL"))
	}
	if err := c.validateResourceURL(locator.URL); err != nil {
		return fanboxapi.Locator{}, newError(operation, sdk.ResourceForbidden, errors.New("resolved resource URL is not allowed"))
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
	profile, err := c.creatorProfile(ctx, "resource", rp.Creator)
	if err != nil {
		return "", err
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
func (c *Client) resolvePostAsset(ctx context.Context, rp resourceRefPayload) (fanboxapi.Locator, error) {
	if rp.Post == "" || rp.Asset == "" {
		return fanboxapi.Locator{}, errors.New("post resource reference is missing post or attachment id")
	}
	source, err := c.postInfo(ctx, rp.Post)
	if err != nil {
		return fanboxapi.Locator{}, classifyError("resource", err)
	}
	if source.Body == nil {
		return fanboxapi.Locator{}, errors.New("post body is unavailable")
	}
	images, files, err := bodyMedia(source.Body)
	if err != nil {
		return fanboxapi.Locator{}, classifyError("resource", err)
	}
	if rp.Kind == "post_image" {
		for _, image := range images {
			if image.ID == rp.Asset {
				return fanboxapi.Locator{URL: image.OriginalURL}, nil
			}
		}
	} else if rp.Kind == "post_file" {
		for _, file := range files {
			if file.ID == rp.Asset {
				return fanboxapi.Locator{URL: file.URL, Filename: attachmentFilename(file.Name, file.Extension)}, nil
			}
		}
	}
	return fanboxapi.Locator{}, errors.New("post attachment is unavailable")
}

// attachmentFilename retains upstream naming, without guessing a name from a locator.
func attachmentFilename(name, extension string) string {
	if name == "" || extension == "" || strings.HasSuffix(strings.ToLower(name), "."+strings.ToLower(extension)) {
		return name
	}
	return name + "." + extension
}
