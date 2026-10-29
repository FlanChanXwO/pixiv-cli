package pixiv

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
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

// newResource binds a policy-validated locator to its stable resource identity.
func (c *Client) newResource(kind string, id int64, page int, variant, rawURL string) (sdk.Resource, error) {
	payload, err := json.Marshal(resourceRefPayload{Kind: kind, ID: id, Page: page, Variant: variant})
	if err != nil {
		return sdk.Resource{}, newError("resource", sdk.UpstreamError, "cannot encode resource reference")
	}
	ref, err := sdk.NewResourceRef(product, payload)
	if err != nil {
		return sdk.Resource{}, newError("resource", sdk.UpstreamError, "cannot encode resource reference")
	}
	if err := c.app.CacheResource(ref.String(), rawURL); err != nil {
		return sdk.Resource{}, newError("resource", sdk.ResourceForbidden, err.Error())
	}
	return sdk.Resource{
		Ref:            ref,
		URL:            rawURL,
		RequestHeaders: map[string]string{"Referer": api.AppReferer},
	}, nil
}

// ArtworkVariantResource returns a ResourceRef for another quality variant of the
// same page. It decodes the stable identity from original.Ref and replaces the
// variant, so SaveResource revalidates and resolves it exactly as it does
// original. An empty variant or "original" returns original.Ref unchanged
// without inspecting the reference. Any other variant requires an artwork
// reference; other kinds return InvalidArgument.
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

// OpenResource opens a resource by its opaque reference and returns an
// allowlisted response. The caller owns and must close response.Body. The
// resource URL, redirects, headers, and identity are revalidated before any
// bytes are streamed; cookies are never sent for resource requests.
func (c *Client) OpenResource(ctx context.Context, request sdk.OpenResourceRequest) (*sdk.ResourceResponse, error) {
	ctx = logging.WithOperation(ctx, "OpenResource")
	if err := request.Validate(); err != nil {
		return nil, err
	}
	rawURL, err := c.resolveResourceURL(ctx, request.Ref, "OpenResource")
	if err != nil {
		return nil, err
	}
	return c.openResourceURL(ctx, request, rawURL)
}

func (c *Client) openResourceURL(ctx context.Context, request sdk.OpenResourceRequest, rawURL string) (*sdk.ResourceResponse, error) {
	header := http.Header{}
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
	response, err := c.app.OpenResource(ctx, api.OpenRequest{
		URL:    rawURL,
		Method: string(request.Method),
		Header: header,
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
	ctx = logging.WithOperation(ctx, "SaveResource")
	if strings.TrimSpace(options.Path) == "" {
		return sdk.SavedResource{}, newError("SaveResource", sdk.InvalidArgument, "destination path is required")
	}
	rawURL, err := c.resolveResourceURL(ctx, ref, "SaveResource")
	if err != nil {
		return sdk.SavedResource{}, err
	}
	response, err := c.openResourceURL(ctx, sdk.OpenResourceRequest{Method: sdk.ResourceMethodGet}, rawURL)
	if err != nil {
		return sdk.SavedResource{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return sdk.SavedResource{}, newError("SaveResource", sdk.UpstreamError, "resource returned a non-success status")
	}
	size, err := sdk.WriteResource(ctx, product, "SaveResource", response.Body, response.ContentLength(), options)
	if err != nil {
		return sdk.SavedResource{}, err
	}
	return sdk.SavedResource{Path: options.Path, Size: size, ContentType: response.ContentType()}, nil
}

// SaveResourceURL saves a policy-checked Pixiv CDN URL to Path. It reuses the
// transport's Referer, redirect validation, and cookie suppression, and writes
// atomically so a failed transfer leaves no partial file: Path keeps its previous
// contents, or stays absent if it did not exist.
func (c *Client) SaveResourceURL(ctx context.Context, rawURL string, options sdk.SaveOptions) (sdk.SavedResource, error) {
	ctx = logging.WithOperation(ctx, "SaveResourceURL")
	const operation = "SaveResourceURL"
	if strings.TrimSpace(options.Path) == "" {
		return sdk.SavedResource{}, newError(operation, sdk.InvalidArgument, "destination path is required")
	}

	stream, err := c.app.OpenResource(ctx, api.OpenRequest{
		URL:    rawURL,
		Method: http.MethodGet,
	})
	if err != nil {
		return sdk.SavedResource{}, classifyAppError(err, operation)
	}
	response := sdk.NewResourceResponse(stream.StatusCode, stream.Header, stream.Body)
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return sdk.SavedResource{}, newError(operation, sdk.UpstreamError, "resource returned a non-success status")
	}

	size, err := sdk.WriteResource(ctx, product, operation, response.Body, response.ContentLength(), options)
	if err != nil {
		return sdk.SavedResource{}, err
	}
	return sdk.SavedResource{Path: options.Path, Size: size, ContentType: response.ContentType()}, nil
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
	if rawURL, ok := c.app.CachedResource(ref.String()); ok {
		return rawURL, nil
	}
	var rawURL string

	switch rp.Kind {
	case "artwork":
		detail, appErr := c.fetchArtworkDetail(ctx, operation, rp.ID)
		if appErr != nil {
			return "", appErr
		}
		rawURL, err = artworkResourceURL(*detail.Illust, rp.Page, rp.Variant)
	case "novel_cover":
		detail, appErr := c.fetchNovelDetail(ctx, operation, rp.ID)
		if appErr != nil {
			return "", appErr
		}
		rawURL, err = imageURLsResourceURL(detail.Novel.ImageURLs, rp.Variant)
	case "user_profile":
		detail, appErr := c.fetchUserDetail(ctx, operation, rp.ID, false)
		if appErr != nil {
			return "", appErr
		}
		rawURL, err = profileImageResourceURL(detail.User.Value.ProfileImageURLs)
	case "ugoira_archive":
		var wire ugoiraResponseWire
		if appErr := c.app.GetJSON(ctx, api.PathUgoiraMetadata, url.Values{
			api.ParamIllustID: {itoa(rp.ID)},
		}, &wire); appErr != nil {
			return "", classifyAppError(appErr, operation)
		}
		if !wire.Metadata.Present || !wire.Metadata.Valid {
			return "", newError(operation, sdk.MalformedUpstreamResponse, "ugoira metadata is missing")
		}
		rawURL, err = ugoiraResourceURL(wire.Metadata.Value, rp.Variant)
	case "novel_image", "novel_file":
		raw, appErr := c.fetchNovelContent(ctx, operation, rp.ID)
		if appErr != nil {
			return "", appErr
		}
		rawURL, err = c.novelContentResourceURL(rp, raw)
	case "stamp":
		items, appErr := c.fetchStamps(ctx, operation)
		if appErr != nil {
			return "", appErr
		}
		rawURL, err = stampResourceURL(items, rp.ID)
	default:
		return "", newError(operation, sdk.InvalidArgument, "resource kind is unsupported")
	}
	if err != nil {
		if _, ok := err.(*sdk.Error); ok {
			return "", err
		}
		return "", newError(operation, sdk.MalformedUpstreamResponse, "resource metadata has no usable URL")
	}
	if err := c.app.CacheResource(ref.String(), rawURL); err != nil {
		return "", newError(operation, sdk.ResourceForbidden, "resolved resource URL is not allowed")
	}
	return rawURL, nil
}

func stampResourceURL(items []stampWire, id int64) (string, error) {
	for _, item := range items {
		if item.ID == id {
			return item.URL, nil
		}
	}
	return "", errors.New("stamp is unavailable")
}

func artworkResourceURL(illust artworkWire, page int, variant string) (string, error) {
	if page >= 0 {
		for index, candidate := range illust.MetaPages {
			if index == page {
				if variant == "original" || variant == "" {
					if value := firstImageURL(candidate.ImageURLs); value != "" {
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

func imageURLsResourceURL(urls imageURLsWire, variant string) (string, error) {
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
	if value := firstImageURL(urls); value != "" {
		return value, nil
	}
	return "", errors.New("image URL is unavailable")
}

func artworkImageURLsResourceURL(urls imageURLsWire, variant string) (string, error) {
	raw, err := imageURLsResourceURL(urls, variant)
	if err == nil {
		return raw, nil
	}
	if variant != "" {
		if derived, ok := deriveImageVariantURL(urls.Original, variant); ok {
			return derived, nil
		}
	}
	return "", err
}

// deriveImageVariantURL derives the requested quality from an original locator
// using Pixiv's documented img-master and thumbnail path conventions when
// upstream provides no direct URL. variant is regular, small, thumb, or mini,
// matching the downloader's DownloadQuality. These locators carry no signature
// parameters, so the cache key comes from the stable identity.
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

// variantPathComponents returns the img-master prefix, crop segment, and filename
// suffix for one quality. The prefix starts with /; crop is empty or resembles
// "c/1200x1200/".
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

func profileImageResourceURL(urls profileImageURLsWire) (string, error) {
	if urls.Medium == nil || *urls.Medium == "" {
		return "", errors.New("profile image is unavailable")
	}
	return *urls.Medium, nil
}

func ugoiraResourceURL(meta ugoiraMetadataWire, variant string) (string, error) {
	values := map[string]string{
		"medium":   meta.ZipURLs.Value.Medium,
		"original": meta.ZipURLs.Value.Original,
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
