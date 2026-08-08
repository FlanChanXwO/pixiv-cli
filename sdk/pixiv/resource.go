package pixiv

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/filesystem"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/resource"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// resourceRefPayload is the opaque identity payload embedded in a Pixiv
// ResourceRef. It carries the stable identity plus the current valid URL so
// OpenResource and SaveResource can revalidate and safely open the resource
// without an extra API round trip.
type resourceRefPayload struct {
	Kind string `json:"k"`
	ID   int64  `json:"id"`
	Page int    `json:"p,omitempty"`
	URL  string `json:"u"`
}

// defaultResourceHosts are the official Pixiv media hosts whose HTTPS URLs are
// accepted for resource reads.
var defaultResourceHosts = []string{"i.pximg.net", "s.pximg.net", "i-f.pximg.net"}

// newResource builds a validated sdk.Resource from an upstream media URL. It
// returns ResourceForbidden when the URL does not satisfy the resource
// policy and InvalidArgument when the identity payload is invalid.
func (c *Client) newResource(kind string, id int64, page int, rawURL string) (sdk.Resource, error) {
	if rawURL == "" {
		return sdk.Resource{}, nil
	}
	if err := c.validateResourceURL(rawURL); err != nil {
		return sdk.Resource{}, err
	}
	payload, err := json.Marshal(resourceRefPayload{Kind: kind, ID: id, Page: page, URL: rawURL})
	if err != nil {
		return sdk.Resource{}, newError("resource", sdk.UpstreamError, "cannot encode resource reference")
	}
	ref, err := sdk.NewResourceRef(product, payload)
	if err != nil {
		return sdk.Resource{}, newError("resource", sdk.UpstreamError, "cannot encode resource reference")
	}
	return sdk.Resource{
		Ref:            ref,
		URL:            rawURL,
		RequestHeaders: map[string]string{"Referer": protocol.AppReferer},
	}, nil
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
	rp, err := c.decodeResourceRef(request.Ref)
	if err != nil {
		return nil, newError("OpenResource", sdk.InvalidArgument, "invalid resource reference")
	}
	if err := c.validateResourceURL(rp.URL); err != nil {
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
		URL:            rp.URL,
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
	rp, err := c.decodeResourceRef(ref)
	if err != nil {
		return sdk.SavedResource{}, newError("SaveResource", sdk.InvalidArgument, "invalid resource reference")
	}
	if err := c.validateResourceURL(rp.URL); err != nil {
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
	size, err := filesystem.Write(ctx, options.Path, &progressReader{src: response.Body, progress: options.Progress})
	if err != nil {
		return sdk.SavedResource{}, newError("SaveResource", sdk.LocalStateError, "cannot write resource")
	}
	return sdk.SavedResource{Path: options.Path, Size: size}, nil
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
