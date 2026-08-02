package fanbox

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/utils/atomicfile"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// fanboxReferer is the non-secret referer FANBOX media requests carry.
const fanboxReferer = "https://www.fanbox.cc/"

// resourceRefPayload is the opaque identity payload embedded in a FANBOX
// ResourceRef.
type resourceRefPayload struct {
	Kind string `json:"k"`
	ID   string `json:"id"`
	URL  string `json:"u"`
}

// allowedMediaHost reports whether host is an allowed FANBOX media host.
func allowedMediaHost(host string) bool {
	host = strings.ToLower(host)
	if host == "i.pximg.net" || host == "s.pximg.net" || strings.HasSuffix(host, ".pximg.net") {
		return true
	}
	if host == "fanbox.pixiv.net" || strings.HasSuffix(host, ".fanbox.pixiv.net") {
		return true
	}
	if host == "www.fanbox.cc" || host == "api.fanbox.cc" || strings.HasSuffix(host, ".fanbox.cc") {
		return true
	}
	return false
}

func (c *Client) validateResourceURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return newError("resource", sdk.CodeResourceForbidden, errors.New("invalid resource URL"))
	}
	if parsed.Scheme != "https" || parsed.User != nil || parsed.Path == "" || parsed.Path == "/" {
		return newError("resource", sdk.CodeResourceForbidden, errors.New("resource URL must be https without userinfo and with a path"))
	}
	if !allowedMediaHost(parsed.Hostname()) {
		return newError("resource", sdk.CodeResourceForbidden, errors.New("resource host is not allowed"))
	}
	return nil
}

func (c *Client) newResource(kind, id, rawURL string) (sdk.Resource, error) {
	if rawURL == "" {
		return sdk.Resource{}, nil
	}
	if err := c.validateResourceURL(rawURL); err != nil {
		return sdk.Resource{}, err
	}
	payload, err := json.Marshal(resourceRefPayload{Kind: kind, ID: id, URL: rawURL})
	if err != nil {
		return sdk.Resource{}, newError("resource", sdk.CodeUpstreamError, err)
	}
	ref, err := sdk.NewResourceRef(product, payload)
	if err != nil {
		return sdk.Resource{}, newError("resource", sdk.CodeUpstreamError, err)
	}
	return sdk.Resource{
		Ref:            ref,
		URL:            rawURL,
		RequestHeaders: map[string]string{"Referer": fanboxReferer},
	}, nil
}

// OpenResource opens a resource by its opaque reference. The caller owns and
// must close response.Body. Resource requests never carry cookies; redirects
// are revalidated against the media host allowlist.
func (c *Client) OpenResource(ctx context.Context, request sdk.OpenResourceRequest) (*sdk.ResourceResponse, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	rp, err := c.decodeResourceRef(request.Ref)
	if err != nil {
		return nil, newError("OpenResource", sdk.CodeInvalidArgument, err)
	}
	if err := c.validateResourceURL(rp.URL); err != nil {
		return nil, newError("OpenResource", sdk.CodeResourceForbidden, errors.New("resource URL is not allowed"))
	}
	response, err := c.session.OpenMedia(ctx, rp.URL)
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
		return sdk.SavedResource{}, newError("SaveResource", sdk.CodeInvalidArgument, errors.New("destination path is required"))
	}
	response, err := c.OpenResource(ctx, sdk.OpenResourceRequest{Ref: ref, Method: sdk.ResourceMethodGet})
	if err != nil {
		return sdk.SavedResource{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return sdk.SavedResource{}, newError("SaveResource", sdk.CodeUpstreamError, errors.New("resource returned a non-success status"))
	}
	size, err := atomicfile.Write(ctx, options.Path, &progressReader{src: response.Body, progress: options.Progress})
	if err != nil {
		return sdk.SavedResource{}, newError("SaveResource", sdk.CodeLocalStateError, errors.New("cannot write resource"))
	}
	return sdk.SavedResource{Path: options.Path, Size: size}, nil
}

func (c *Client) decodeResourceRef(ref sdk.ResourceRef) (resourceRefPayload, error) {
	if ref.IsZero() {
		return resourceRefPayload{}, errors.New("zero resource reference")
	}
	payload, err := sdk.ResourceRefPayload(ref)
	if err != nil {
		return resourceRefPayload{}, err
	}
	var rp resourceRefPayload
	if err := json.Unmarshal(payload, &rp); err != nil || rp.URL == "" {
		return resourceRefPayload{}, errors.New("malformed resource reference")
	}
	return rp, nil
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
