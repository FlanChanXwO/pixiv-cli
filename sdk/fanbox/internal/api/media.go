package api

import (
	"context"
	"io"
	"net/http"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

const (
	downloadsHost         = "downloads.fanbox.cc"
	acceptMedia           = "*/*"
	headerRange           = "Range"
	headerIfNoneMatch     = "If-None-Match"
	headerIfModifiedSince = "If-Modified-Since"
	headerIfRange         = "If-Range"
)

// Allowed representation header names are shared with the public media boundary.
const (
	// HeaderContentType identifies the representation's media type.
	HeaderContentType = "Content-Type"
	// HeaderContentLength reports the representation size in bytes.
	HeaderContentLength = "Content-Length"
	// HeaderContentRange describes a partial representation.
	HeaderContentRange = "Content-Range"
	// HeaderAcceptRanges reports supported byte ranges.
	HeaderAcceptRanges = "Accept-Ranges"
	// HeaderETag identifies the representation version.
	HeaderETag = "ETag"
	// HeaderLastModified reports the modification time.
	HeaderLastModified = "Last-Modified"
	// HeaderCacheControl carries representation caching policy.
	HeaderCacheControl = "Cache-Control"
)

// MediaRequest selects GET (the zero value) or HEAD and the allowed conditional headers.
type MediaRequest struct {
	Method          string
	Range           string
	IfNoneMatch     string
	IfModifiedSince string
	IfRange         string
}

// OpenMedia opens a validated media locator with only product-approved headers.
// Redirects retain the method and conditions while stripping all cookies.
func (c *Client) OpenMedia(ctx context.Context, operation, raw string, request MediaRequest) (*http.Response, error) {
	method := request.Method
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodHead {
		return nil, failure(operation, sdk.InvalidArgument, "media method must be GET or HEAD", nil)
	}
	headers := make(http.Header)
	for _, field := range []struct{ name, value string }{
		{headerRange, request.Range}, {headerIfNoneMatch, request.IfNoneMatch}, {headerIfModifiedSince, request.IfModifiedSince}, {headerIfRange, request.IfRange},
	} {
		if !validHeaderValue(field.value) {
			return nil, failure(operation, sdk.InvalidArgument, "invalid media condition header", nil)
		}
		if field.value != "" {
			headers.Set(field.name, field.value)
		}
	}
	response, err := c.do(ctx, operation, raw, requestProfile{accept: acceptMedia, method: method, media: true, headers: headers})
	if err != nil {
		return nil, err
	}
	response.Body = &mediaReadCloser{ctx: ctx, operation: operation, body: response.Body}
	return response, nil
}

type mediaReadCloser struct {
	ctx       context.Context
	operation string
	body      io.ReadCloser
}

// Read preserves bytes and sanitizes external stream failures.
func (r *mediaReadCloser) Read(data []byte) (int, error) {
	n, err := r.body.Read(data)
	if err != nil && err != io.EOF {
		return n, externalFailure(r.ctx, r.operation, "read FANBOX media failed", err)
	}
	return n, err
}

// Close reports external close failure without exposing credential-bearing text.
func (r *mediaReadCloser) Close() error {
	if err := r.body.Close(); err != nil {
		return externalFailure(r.ctx, r.operation, "close FANBOX media failed", err)
	}
	return nil
}
