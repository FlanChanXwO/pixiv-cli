package fanbox

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/fanbox/api"
	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// resourceRefPayload is the identity a FANBOX ResourceRef encodes: kind, owning
// creator or post, and attachment ID. Stable fields keep the cache key valid when
// a locator rotates and let a stored ref reopen in another session. A resolved
// URL is never encoded; OpenResource derives a fresh, validated locator from
// this metadata. PostID is empty for creator-scoped kinds (icon and cover), and
// AssetID is empty there but holds the attachment ID for post assets.
type resourceRefPayload struct {
	Kind    string `json:"k"`
	Creator string `json:"c,omitempty"`
	Post    string `json:"p,omitempty"`
	Asset   string `json:"a,omitempty"`
}

func (c *Client) validateResourceURL(rawURL string) error {
	if err := api.ValidateURL(rawURL); err != nil {
		return newError("resource", sdk.ResourceForbidden, errors.New("resource host is not allowed"))
	}
	return nil
}

// newResource validates an upstream media URL against the stable identity that
// owns it. The URL becomes the currently usable Resource.URL and is cached
// in-session under the opaque ref, which encodes only stable identity.
// RequiresCredentials reports whether the locator is on the credentialed
// downloads host.
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
	c.app.CacheLocator(ref.String(), api.Locator{URL: rawURL, Filename: filename})
	return sdk.Resource{
		Ref:                 ref,
		URL:                 rawURL,
		RequestHeaders:      map[string]string{"Referer": api.Referer},
		RequiresCredentials: api.RequiresCredentials(rawURL),
	}, nil
}

// OpenResource opens a resource by its opaque reference. The caller owns and
// must close response.Body. The session cookie goes only to the credentialed
// downloads host, redirects are revalidated against the media host allowlist, and
// the ref's identity is resolved to a fresh locator so a stored ref never
// reopens a stale embedded URL.
func (c *Client) OpenResource(ctx context.Context, request sdk.OpenResourceRequest) (*sdk.ResourceResponse, error) {
	ctx = logging.WithOperation(ctx, "OpenResource")
	if err := request.Validate(); err != nil {
		return nil, err
	}
	locator, err := c.resolveResource(ctx, request.Ref, "OpenResource")
	if err != nil {
		return nil, err
	}
	response, err := c.app.OpenMedia(ctx, locator.URL, api.MediaRequest{
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

// SaveResource writes one resource, addressed by its opaque reference, to Path
// through an atomic destination. It does not expand posts, creators, archives, or
// sidecars; the downloader handles those.
func (c *Client) SaveResource(ctx context.Context, ref sdk.ResourceRef, options sdk.SaveOptions) (sdk.SavedResource, error) {
	ctx = logging.WithOperation(ctx, "SaveResource")
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
func (c *Client) resolveResource(ctx context.Context, ref sdk.ResourceRef, operation string) (api.Locator, error) {
	rp, err := c.decodeResourceRef(ref)
	if err != nil {
		return api.Locator{}, newError(operation, sdk.InvalidArgument, errors.New("invalid resource reference"))
	}
	if locator, ok := c.app.CachedLocator(ref.String()); ok {
		return locator, nil
	}
	var locator api.Locator
	switch rp.Kind {
	case "creator_icon", "creator_cover":
		locator.URL, err = c.resolveCreatorAssetURL(ctx, rp)
	case "post_image", "post_file":
		locator, err = c.resolvePostAsset(ctx, rp)
	default:
		return api.Locator{}, newError(operation, sdk.InvalidArgument, errors.New("resource kind is unsupported"))
	}
	if err != nil {
		if _, ok := err.(*sdk.Error); ok {
			return api.Locator{}, err
		}
		return api.Locator{}, newError(operation, sdk.MalformedUpstreamResponse, errors.New("resource metadata has no usable URL"))
	}
	if err := c.validateResourceURL(locator.URL); err != nil {
		return api.Locator{}, newError(operation, sdk.ResourceForbidden, errors.New("resolved resource URL is not allowed"))
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
func (c *Client) resolvePostAsset(ctx context.Context, rp resourceRefPayload) (api.Locator, error) {
	if rp.Post == "" || rp.Asset == "" {
		return api.Locator{}, errors.New("post resource reference is missing post or attachment id")
	}
	source, err := c.postInfo(ctx, rp.Post)
	if err != nil {
		return api.Locator{}, classifyError("resource", err)
	}
	if source.Body == nil {
		return api.Locator{}, errors.New("post body is unavailable")
	}
	images, files, err := bodyMedia(source.Body)
	if err != nil {
		return api.Locator{}, classifyError("resource", err)
	}
	if rp.Kind == "post_image" {
		for _, image := range images {
			if image.ID == rp.Asset {
				return api.Locator{URL: image.OriginalURL}, nil
			}
		}
	} else if rp.Kind == "post_file" {
		for _, file := range files {
			if file.ID == rp.Asset {
				return api.Locator{URL: file.URL, Filename: attachmentFilename(file.Name, file.Extension)}, nil
			}
		}
	}
	return api.Locator{}, errors.New("post attachment is unavailable")
}

// attachmentFilename keeps the upstream name instead of guessing one from a
// locator.
func attachmentFilename(name, extension string) string {
	if name == "" || extension == "" || strings.HasSuffix(strings.ToLower(name), "."+strings.ToLower(extension)) {
		return name
	}
	return name + "." + extension
}
