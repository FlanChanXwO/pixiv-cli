// Package api owns FANBOX session headers, identity parsing, redirects, and
// challenge recovery through the shared Chrome TLS and FlareSolverr client.
//
// [New] builds a [Client] without I/O. Caller-supplied HTTP clients remain
// caller-owned. [Client.CurrentUser] validates the session against homepage
// metadata; all product requests retain this client's fixed session.
package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/FlanChanXwO/pixiv-cli/internal/credential"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/internal/cloudflare"
)

const (
	// WebBaseURL names the anonymous homepage used for identity and solver requests.
	WebBaseURL          = webOrigin + "/"
	defaultUserAgent    = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:148.0) Gecko/20100101 Firefox/148.0"
	sessionCookieName   = "FANBOXSESSID"
	clearanceCookieName = "cf_clearance"
)

// Options configures native requests and the independent solver browser route.
// HTTPClient, when supplied, must have an explicit Transport and remains owned
// by the caller. Its Jar and redirect policy are bypassed for product requests.
// ProxyURL accepts an HTTP(S) proxy without userinfo. The solver service accepts
// an HTTP(S) root URL without userinfo; its browser proxy accepts HTTP/SOCKS4/5
// without userinfo. Empty FlareSolverrURL disables recovery.
type Options struct {
	HTTPClient           *http.Client
	ProxyURL             string
	UserAgent            string // Empty preserves the FANBOX request-header baseline.
	FlareSolverrURL      string
	FlareSolverrProxyURL string
}

// Client owns one immutable session and its per-client clearance. The mutex
// protects clearance adoption and expiry; shared solver flights are owned by
// cloudflare.Client. Finish native requests before closing the client.
type Client struct {
	http          *http.Client
	browser       *cloudflare.Client
	session       *credential.Secret
	agent         string
	solverEnabled bool
	mu            sync.Mutex
	clearance     *cloudflare.Clearance // Protected by mu; never persisted or logged.
	identityGate  chan struct{}         // Serializes identity verification while allowing waiters to cancel.
	userID        int64                 // Protected by identityGate; only the verified non-secret ID is cached.
}

// New validates settings and the cookie value without contacting any endpoint.
func New(sessionID string, options Options) (*Client, error) {
	if strings.ContainsAny(sessionID, "\r\n") {
		return nil, failure("New", sdk.InvalidArgument, "session value contains line breaks", nil)
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || !validCookieValue(sessionID) {
		return nil, failure("New", sdk.InvalidArgument, "a valid FANBOXSESSID value is required", nil)
	}
	agent := options.UserAgent
	if agent == "" {
		agent = defaultUserAgent
	}
	if !validHeaderValue(agent) {
		return nil, failure("New", sdk.InvalidArgument, "invalid User-Agent header", nil)
	}
	proxyURL, err := validateOptions(options)
	if err != nil {
		return nil, err
	}
	if options.HTTPClient != nil && options.HTTPClient.Transport == nil {
		return nil, failure("New", sdk.InvalidArgument, "injected HTTP client requires an explicit transport", nil)
	}
	browser, err := cloudflare.New(cloudflare.Options{ProxyURL: proxyURL, FlareSolverrURL: options.FlareSolverrURL, FlareSolverrProxyURL: options.FlareSolverrProxyURL, Origin: WebBaseURL})
	if err != nil {
		return nil, err
	}
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Transport: browser}
	}
	session := credential.New([]byte(sessionID))
	return &Client{http: client, browser: browser, session: &session, agent: agent, solverEnabled: options.FlareSolverrURL != "", identityGate: make(chan struct{}, 1)}, nil
}

// Format keeps credentials and connection settings out of diagnostics.
func (*Client) Format(state fmt.State, _ rune) { fmt.Fprint(state, "api.Client{}") }

// CloseIdleConnections releases only SDK-owned native and solver connections.
func (c *Client) CloseIdleConnections() { c.browser.CloseIdleConnections() }

// Close cancels and joins pending solves and releases SDK-owned connections.
// Caller-owned transports remain open; finish native requests before closing.
func (c *Client) Close(ctx context.Context) error { return c.browser.Close(ctx) }

func validateOptions(options Options) (string, error) {
	proxyURL := ""
	for _, field := range []struct {
		value, name string
		schemes     []string
		root        bool
	}{
		{strings.TrimSpace(options.ProxyURL), "native proxy", []string{"http", "https"}, false},
		{options.FlareSolverrURL, "solver service", []string{"http", "https"}, true},
		{options.FlareSolverrProxyURL, "solver browser proxy", []string{"http", "socks4", "socks5"}, true},
	} {
		if field.value == "" {
			continue
		}
		parsed, err := url.Parse(field.value)
		validScheme := false
		if err == nil {
			for _, scheme := range field.schemes {
				validScheme = validScheme || parsed.Scheme == scheme
			}
		}
		if err != nil || !validScheme || parsed.Hostname() == "" || parsed.User != nil || (field.root && ((parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "")) {
			return "", failure("New", sdk.InvalidArgument, "invalid "+field.name+" URL", nil)
		}
		if !field.root {
			// The existing CONNECT proxy contract uses only scheme and authority.
			proxyURL = parsed.Scheme + "://" + parsed.Host
		}
	}
	return proxyURL, nil
}

func validHeaderValue(value string) bool {
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			return false
		}
	}
	return true
}
func validCookieValue(value string) bool {
	for _, char := range value {
		if char < 0x21 || char > 0x7e || strings.ContainsRune("\",;\\", char) {
			return false
		}
	}
	return true
}
