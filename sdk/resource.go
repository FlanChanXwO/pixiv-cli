package sdk

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ResourceMethod names an OpenResource verb. The zero value means GET.
type ResourceMethod string

// ResourceMethodGet and ResourceMethodHead are the two accepted verbs.
const (
	ResourceMethodGet  ResourceMethod = "GET"
	ResourceMethodHead ResourceMethod = "HEAD"
)

// Resource describes a first-party media item returned by product models. URL
// is a currently-usable upstream locator that a caller may stream directly or
// proxy without buffering to disk; Ref is the stable opaque identity to hand
// back to OpenResource or SaveResource. The two serve different purposes and
// neither is derived from the other. The JSON form exposes only Ref and
// RequiresCredentials, so runtime locators never cross an output boundary.
//
// URL must be validated (scheme, host, userinfo, allowed resource path) before
// it enters a public model. RequestHeaders contains only non-secret headers a
// caller must forward, such as a Pixiv image referer; it must never carry
// cookies, authorization, or proxy credentials. When the resource still needs
// product credentials invisible to the caller, RequiresCredentials is true and
// callers should use OpenResource instead of treating URL as a public link.
// ExpiresAt is set only when the upstream or protocol can reliably determine
// it; callers must not treat URL as a permanent identity.
type Resource struct {
	Ref                 ResourceRef       `json:"ref"`
	URL                 string            `json:"url"`
	RequestHeaders      map[string]string `json:"request_headers,omitempty"`
	ExpiresAt           *time.Time        `json:"expires_at,omitempty"`
	RequiresCredentials bool              `json:"requires_credentials,omitempty"`
}

// MarshalJSON emits only the opaque reference and the credential requirement,
// never URL, request headers, or expiry metadata. A resource without a
// reference marshals as null.
func (r Resource) MarshalJSON() ([]byte, error) {
	if r.Ref.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(struct {
		Ref                 ResourceRef `json:"ref"`
		RequiresCredentials bool        `json:"requires_credentials,omitempty"`
	}{r.Ref, r.RequiresCredentials})
}

// Copy returns a deep copy of the resource. RequestHeaders is returned to
// callers as a fresh map so mutating it never changes the client's internal
// state.
func (r *Resource) Copy() Resource {
	out := *r
	if r.RequestHeaders != nil {
		out.RequestHeaders = make(map[string]string, len(r.RequestHeaders))
		for k, v := range r.RequestHeaders {
			out.RequestHeaders[k] = v
		}
	}
	if r.ExpiresAt != nil {
		t := *r.ExpiresAt
		out.ExpiresAt = &t
	}
	return out
}

// OpenResourceRequest describes one OpenResource call. The zero Method means
// GET. The header fields are validated with Validate, so SDKs never accept
// arbitrary header maps.
type OpenResourceRequest struct {
	Ref             ResourceRef
	Method          ResourceMethod
	Range           string
	IfNoneMatch     string
	IfModifiedSince string
	IfRange         string
}

// Validate checks that the method is allowed and that request header fields
// contain no control characters, returning InvalidArgument otherwise.
func (r OpenResourceRequest) Validate() error {
	switch r.Method {
	case "", ResourceMethodGet, ResourceMethodHead:
	default:
		return NewError("", "OpenResourceRequest.Validate", InvalidArgument, WithDetail("unsupported resource method"))
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{"range", r.Range},
		{"if-none-match", r.IfNoneMatch},
		{"if-modified-since", r.IfModifiedSince},
		{"if-range", r.IfRange},
	} {
		if hasControlChars(field.value) {
			return NewError("", "OpenResourceRequest.Validate", InvalidArgument, WithDetail(field.name+" header contains control characters"))
		}
	}
	return nil
}

func hasControlChars(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f {
			return true
		}
	}
	return false
}

// resourceHeaderAllowlist lists the upstream response headers the resource
// contract exposes. Location, Set-Cookie, authentication headers, and internal
// URLs are never forwarded.
var resourceHeaderAllowlist = []string{
	"Content-Type",
	"Content-Length",
	"Content-Range",
	"Accept-Ranges",
	"ETag",
	"Last-Modified",
	"Cache-Control",
}

// ResourceResponse reports the result of opening a resource. The caller owns
// Body and must close it; it is a non-nil empty stream for HEAD, 204, and 304
// responses. Header and the typed getters expose only allowlisted representation
// headers.
type ResourceResponse struct {
	// Filename is an upstream-suggested attachment name, including its extension
	// when provided. Empty means unavailable. It is metadata, not a trusted path;
	// callers must normalize it before choosing a filesystem destination.
	Filename   string
	StatusCode int
	Body       io.ReadCloser
	header     http.Header
}

// NewResourceResponse builds a response carrying only the allowlisted
// representation headers copied from src. The caller owns body and must close
// it.
func NewResourceResponse(statusCode int, src http.Header, body io.ReadCloser) *ResourceResponse {
	h := make(http.Header)
	for _, name := range resourceHeaderAllowlist {
		if values, ok := src[http.CanonicalHeaderKey(name)]; ok {
			for _, v := range values {
				h.Add(name, v)
			}
		}
	}
	return &ResourceResponse{StatusCode: statusCode, Body: body, header: h}
}

// Header returns a copy of the allowlisted representation headers.
func (r *ResourceResponse) Header() http.Header {
	out := make(http.Header, len(r.header))
	for k, vs := range r.header {
		out[k] = append([]string(nil), vs...)
	}
	return out
}

// ContentType returns the upstream media type, or empty when not provided.
func (r *ResourceResponse) ContentType() string { return r.header.Get("Content-Type") }

// ContentLength returns the upstream body size in bytes, or zero when absent or
// malformed.
func (r *ResourceResponse) ContentLength() int64 {
	v := r.header.Get("Content-Length")
	if v == "" {
		return 0
	}
	n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// ContentRange returns the upstream byte-range header, or empty when absent.
func (r *ResourceResponse) ContentRange() string { return r.header.Get("Content-Range") }

// AcceptRanges returns the upstream range support header, or empty when absent.
func (r *ResourceResponse) AcceptRanges() string { return r.header.Get("Accept-Ranges") }

// ETag returns the upstream entity tag, or empty when absent.
func (r *ResourceResponse) ETag() string { return r.header.Get("ETag") }

// LastModified returns the upstream modification time in HTTP date form, or
// empty when absent.
func (r *ResourceResponse) LastModified() string { return r.header.Get("Last-Modified") }

// CacheControl returns the upstream caching directive, or empty when absent.
func (r *ResourceResponse) CacheControl() string { return r.header.Get("Cache-Control") }
