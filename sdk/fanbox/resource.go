package fanbox

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	creatorlist "github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/creator/creators"
	postinfo "github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/post/info"
	fanboxresource "github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/resource"
	atomicfile "github.com/FlanChanXwO/pixiv-cli/internal/storage/file/atomic"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// fanboxReferer is the non-secret referer FANBOX media requests carry.
const fanboxReferer = fanboxresource.Referer

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
	if err := fanboxresource.ValidateURL(rawURL); err != nil {
		return newError("resource", sdk.ResourceForbidden, errors.New("resource host is not allowed"))
	}
	return nil
}

// newResource builds a validated sdk.Resource from an upstream media URL and
// the stable identity that owns it. The URL is exposed as the currently-usable
// locator (Resource.URL) and cached in-session keyed by the opaque ref; the ref
// envelope encodes only stable identity. RequiresCredentials reflects whether
// the locator lives on the credentialed downloads host.
func (c *Client) newResource(kind, creatorID, postID, assetID, rawURL string) (sdk.Resource, error) {
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
	c.resourceMu.Lock()
	c.resourceURLs[ref.String()] = rawURL
	c.resourceMu.Unlock()
	return sdk.Resource{
		Ref:                 ref,
		URL:                 rawURL,
		RequestHeaders:      map[string]string{"Referer": fanboxReferer},
		RequiresCredentials: fanboxresource.RequiresCredentials(rawURL),
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
	rawURL, err := c.resolveResourceURL(ctx, request.Ref, "OpenResource")
	if err != nil {
		return nil, err
	}
	if err := c.validateResourceURL(rawURL); err != nil {
		return nil, newError("OpenResource", sdk.ResourceForbidden, errors.New("resource URL is not allowed"))
	}
	response, err := c.resource.Open(ctx, rawURL, fanboxresource.Request{
		Method:          string(request.Method),
		Range:           request.Range,
		IfNoneMatch:     request.IfNoneMatch,
		IfModifiedSince: request.IfModifiedSince,
		IfRange:         request.IfRange,
	})
	if err != nil {
		return nil, classifyError("OpenResource", err)
	}
	return sdk.NewResourceResponse(response.StatusCode, response.Header, response.Body), nil
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

// resolveResourceURL returns the currently-usable locator for a ref. It first
// consults the in-session locator cache (the URL the resource was created
// with); when no cached locator exists it re-resolves a fresh locator from
// trusted metadata by re-fetching the owning post or creator and locating the
// attachment by its stable id. The resolved URL is always revalidated against
// the media host allowlist before use.
func (c *Client) resolveResourceURL(ctx context.Context, ref sdk.ResourceRef, operation string) (string, error) {
	rp, err := c.decodeResourceRef(ref)
	if err != nil {
		return "", newError(operation, sdk.InvalidArgument, errors.New("invalid resource reference"))
	}
	c.resourceMu.RLock()
	rawURL := c.resourceURLs[ref.String()]
	c.resourceMu.RUnlock()
	if rawURL != "" {
		return rawURL, nil
	}

	var resolved string
	switch rp.Kind {
	case "creator_icon", "creator_cover":
		resolved, err = c.resolveCreatorAssetURL(ctx, rp)
	case "post_image", "post_file":
		resolved, err = c.resolvePostAssetURL(ctx, rp)
	default:
		return "", newError(operation, sdk.InvalidArgument, errors.New("resource kind is unsupported"))
	}
	if err != nil {
		if _, ok := err.(*sdk.Error); ok {
			return "", err
		}
		return "", newError(operation, sdk.MalformedUpstreamResponse, errors.New("resource metadata has no usable URL"))
	}
	if err := c.validateResourceURL(resolved); err != nil {
		return "", newError(operation, sdk.ResourceForbidden, errors.New("resolved resource URL is not allowed"))
	}
	c.resourceMu.Lock()
	c.resourceURLs[ref.String()] = resolved
	c.resourceMu.Unlock()
	return resolved, nil
}

// resolveCreatorAssetURL re-resolves a creator icon or cover locator by
// re-fetching the creator profile and selecting the field matching the kind.
func (c *Client) resolveCreatorAssetURL(ctx context.Context, rp resourceRefPayload) (string, error) {
	if rp.Creator == "" {
		return "", errors.New("creator resource reference is missing creator id")
	}
	profile, err := c.creators.Profile(ctx, creatorlist.ProfileRequest{CreatorID: rp.Creator})
	if err != nil {
		return "", classifyError("resource", err)
	}
	switch rp.Kind {
	case "creator_icon":
		if profile.IconURL == "" {
			return "", errors.New("creator icon is unavailable")
		}
		return profile.IconURL, nil
	case "creator_cover":
		if profile.CoverURL == "" {
			return "", errors.New("creator cover is unavailable")
		}
		return profile.CoverURL, nil
	default:
		return "", errors.New("unsupported creator resource kind")
	}
}

// resolvePostAssetURL re-resolves a post image or file locator by re-fetching
// the post and locating the attachment by its stable id across the merged
// image/file maps (mirroring mapPost's asset merge so block-only attachments
// resolve identically).
func (c *Client) resolvePostAssetURL(ctx context.Context, rp resourceRefPayload) (string, error) {
	if rp.Post == "" || rp.Asset == "" {
		return "", errors.New("post resource reference is missing post or attachment id")
	}
	source, err := c.postInfo.Get(ctx, postinfo.Request{PostID: rp.Post})
	if err != nil {
		return "", classifyError("resource", err)
	}
	if source.Body == nil {
		return "", errors.New("post body is unavailable")
	}
	for _, image := range mergePostImages(*source.Body) {
		if image.ID == rp.Asset && image.OriginalURL != "" {
			return image.OriginalURL, nil
		}
	}
	for _, file := range mergePostFiles(*source.Body) {
		if file.ID == rp.Asset && file.URL != "" {
			return file.URL, nil
		}
	}
	return "", errors.New("post attachment is unavailable")
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
