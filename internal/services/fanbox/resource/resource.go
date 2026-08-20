// Package resource owns FANBOX media URL policy and the narrow resource
// transport adapter. Opaque public ResourceRef decoding remains in sdk/fanbox.
package resource

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/protocol"
)

// Referer is the non-secret referer required by FANBOX media requests.
const Referer = protocol.WebBaseURL

// DownloadsHost is the first-party FANBOX attachment host whose media URLs
// require the FANBOXSESSID cookie. Resources on this host are credentialed and
// callers must open them through OpenResource rather than treating the URL as
// a public link; Pixiv CDN and other allowlisted media hosts are public.
const DownloadsHost = "downloads.fanbox.cc"

// RequiresCredentials reports whether rawURL points at a credentialed FANBOX
// attachment host. It parses rawURL defensively and returns false on any parse
// error so callers treat unparseable locators as public (and let the host
// allowlist reject them later) rather than falsely advertising credentials.
func RequiresCredentials(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Hostname(), DownloadsHost)
}

// Transport is the narrow product-session capability required to open media.
type Transport interface {
	OpenMediaWithRequest(context.Context, string, protocol.MediaRequest) (*http.Response, error)
}

// Request is the controlled resource request surface used by the public SDK.
type Request struct {
	Method          string
	Range           string
	IfNoneMatch     string
	IfModifiedSince string
	IfRange         string
}

// Client owns resource host validation and delegates the request to the
// product session transport.
type Client struct {
	transport Transport
}

// New constructs a resource endpoint client.
func New(transport Transport) *Client { return &Client{transport: transport} }

// ValidateURL validates a FANBOX asset locator without opening it.
func ValidateURL(rawURL string) error {
	if strings.TrimSpace(rawURL) == "" {
		return errors.New("FANBOX resource URL is empty")
	}
	if err := protocol.ValidateMediaURL(rawURL); err != nil {
		return err
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Path == "" || parsed.Path == "/" {
		return errors.New("FANBOX resource URL must contain a path")
	}
	return nil
}

// Open validates and opens one media resource.
func (c *Client) Open(ctx context.Context, rawURL string, request Request) (*http.Response, error) {
	if c == nil || c.transport == nil {
		return nil, errors.New("FANBOX resource transport is not configured")
	}
	if err := ValidateURL(rawURL); err != nil {
		return nil, err
	}
	return c.transport.OpenMediaWithRequest(ctx, rawURL, protocol.MediaRequest{
		Method:          request.Method,
		Range:           request.Range,
		IfNoneMatch:     request.IfNoneMatch,
		IfModifiedSince: request.IfModifiedSince,
		IfRange:         request.IfRange,
	})
}
