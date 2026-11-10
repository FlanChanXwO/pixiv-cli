package fanbox

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/storage/atomicfile"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox/internal/api"
)

// OpenRequest selects GET (the zero value) or HEAD and optional HTTP conditions.
// Only Range, IfNoneMatch, IfModifiedSince and IfRange headers are accepted.
type OpenRequest = api.MediaRequest

// MediaResponse exposes the status, allowlisted representation headers and stream from Open.
// The caller must close Body. HEAD and 304 have an empty body; Filename is
// upstream metadata and must be sanitized before choosing a destination path.
type MediaResponse struct {
	StatusCode int
	Header     http.Header
	Body       io.ReadCloser
	Filename   string
}

func (c *Client) openMedia(ctx context.Context, operation, raw string, request OpenRequest) (*MediaResponse, error) {
	if c == nil {
		return nil, newError(operation, sdk.InvalidArgument, "media is unbound; retrieve it through a client-bound creator or post")
	}
	response, err := c.app.OpenMedia(ctx, operation, raw, request)
	if err != nil {
		return nil, err
	}
	headers := make(http.Header)
	for _, name := range []string{api.HeaderContentType, api.HeaderContentLength, api.HeaderContentRange, api.HeaderAcceptRanges, api.HeaderETag, api.HeaderLastModified, api.HeaderCacheControl} {
		for _, value := range response.Header.Values(name) {
			headers.Add(name, value)
		}
	}
	return &MediaResponse{StatusCode: response.StatusCode, Header: headers, Body: response.Body}, nil
}

func (c *Client) saveMedia(ctx context.Context, operation, raw, path string) error {
	if strings.TrimSpace(path) == "" {
		return newError(operation, sdk.InvalidArgument, "destination path is required")
	}
	response, err := c.openMedia(ctx, operation, raw, OpenRequest{})
	if err != nil {
		return err
	}
	source := &saveSource{body: response.Body}
	if response.StatusCode != http.StatusOK {
		failure := newError(operation, sdk.UpstreamError, "media save requires HTTP 200")
		failure.HTTPStatus = response.StatusCode
		return errors.Join(failure, source.Close())
	}
	_, writeErr := atomicfile.AtomicWrite(ctx, path, source)
	if err := errors.Join(writeErr, source.Close()); err != nil {
		var upstream *sdk.Error
		if errors.As(err, &upstream) {
			return err
		}
		return &sdk.Error{Operation: operation, Reason: sdk.LocalStateError, Message: "save FANBOX media failed", Err: err}
	}
	return nil
}

// Closing the source at EOF makes close failure abort the atomic replacement.
type saveSource struct {
	body   io.ReadCloser
	closed bool
}

// Read closes the downloaded stream before atomic publication at EOF.
func (s *saveSource) Read(data []byte) (int, error) {
	n, err := s.body.Read(data)
	if err == io.EOF {
		if closeErr := s.Close(); closeErr != nil {
			return n, closeErr
		}
	} else if err != nil {
		err = errors.Join(err, s.Close())
	}
	return n, err
}

// Close releases the source once, including early filesystem or read failures.
func (s *saveSource) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.body.Close()
}
