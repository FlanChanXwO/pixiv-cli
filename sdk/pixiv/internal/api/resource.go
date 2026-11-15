package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// OpenResource streams an allowed media URL without product credentials.
// Redirects are validated after the caller callback has finalized the target.
// The caller owns the returned body.
func (c *Client) OpenResource(ctx context.Context, rawURL string) (*http.Response, error) {
	if err := c.ValidateResourceURL(rawURL); err != nil {
		return nil, Failure{Kind: FailureResourceForbidden}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, Transport(err)
	}
	req.Header.Set("Referer", AppReferer)
	req.Header.Set("User-Agent", AppUserAgent)
	// Copy caller settings but strip the cookie jar before contacting media hosts.
	httpClient := *c.http
	httpClient.Jar = nil
	originalRedirect := httpClient.CheckRedirect
	httpClient.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if originalRedirect != nil {
			if err := originalRedirect(next, via); err != nil {
				return err
			}
		} else if len(via) >= 10 {
			// Preserve net/http.Client's default ten-redirect limit.
			return errors.New("stopped after 10 redirects")
		}
		// Validate the final target because the caller callback may rewrite it.
		if err := c.ValidateResourceURL(next.URL.String()); err != nil {
			return Failure{Kind: FailureResourceForbidden}
		}
		return nil
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, Transport(err)
	}
	return resp, nil
}

// ValidateResourceURL checks the media scheme, host allowlist, and nonempty path.
func (c *Client) ValidateResourceURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return errors.New("invalid resource URL")
	}
	return validateResourceURL(parsed, c.allowedResourceHosts)
}

// ValidStampURL requires an absolute URI on an official media host; caller-added
// resource hosts do not expand stamp policy.
func ValidStampURL(rawURL string) bool {
	parsed, err := url.ParseRequestURI(rawURL)
	return err == nil && validateResourceURL(parsed, nil) == nil
}

func validateResourceURL(parsed *url.URL, extra []string) error {
	if parsed.Scheme != "https" || parsed.User != nil {
		return errors.New("resource URL must be https without userinfo")
	}
	if parsed.Path == "" || parsed.Path == "/" {
		return errors.New("resource URL has no path")
	}
	host := strings.ToLower(parsed.Hostname())
	if !allowedResourceHost(host, extra) {
		return errors.New("resource host is not allowed")
	}
	return nil
}

var defaultResourceHosts = []string{"i.pximg.net", "s.pximg.net", "i-f.pximg.net"}

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
