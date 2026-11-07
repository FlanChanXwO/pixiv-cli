// Package cloudflare shares Chrome TLS transport and FlareSolverr clearance
// acquisition between product SDKs.
//
// [New] creates a [Client] without network I/O. Use it as an http.RoundTripper;
// product clients still own headers, redirects, challenge detection, and replay.
// After a verified challenge, Solve obtains a [Clearance] for a protected URL
// within Origin.
// FlareSolverr control requests are direct and carry no product credentials.
// PersistentSession selects the session protocol required by ascii2d; FANBOX
// uses a direct request.get. Concurrent solves share one cancellable request and
// an expiring cache. Close cancels pending solves and destroys a created session.
package cloudflare

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	tlsclient "github.com/bogdanfinn/tls-client"
)

const (
	operationTransport = "cloudflare.RoundTrip"
	operationSolve     = "cloudflare.Solve"
	operationClose     = "cloudflare.Close"
)

type sessionPhase uint8

const (
	sessionUnused sessionPhase = iota
	sessionUncertain
	sessionReady
)

// Options separates the native route from the solver's browser route. The
// product supplies its anonymous Origin; product cookies never reach the solver.
type Options struct {
	ProxyURL             string // Empty selects direct native transport; HTTP(S) and SOCKS5 are supported.
	FlareSolverrURL      string // Empty disables solver recovery; a service path prefix is allowed.
	FlareSolverrProxyURL string // Browser-only proxy; HTTP, SOCKS4, or SOCKS5.
	Origin               string // Required with FlareSolverrURL; HTTP(S) origin for clearance sharing.
	PersistentSession    bool   // Create/destroy a browser session and keep it across solves.
}

// Client owns the browser transport, solver control client, and clearance cache.
// One instance belongs to one product origin. Use Close(ctx) to end
// its solver lifecycle; CloseIdleConnections only releases idle connections.
type Client struct {
	native      tlsclient.HttpClient
	control     *http.Client
	endpoint    string
	origin      *url.URL
	solverProxy string
	session     string
	sessionMu   sync.Mutex
	phase       sessionPhase // Protected by sessionMu; uncertain creation still requires cleanup.
	mu          sync.Mutex
	state       *Clearance
	active      *solveCall
	closed      bool
}

// New validates connection options and creates clients without contacting any
// endpoint. Product-specific proxy restrictions remain with the product owner.
func New(options Options) (*Client, error) {
	proxyURL, err := nativeProxy(options.ProxyURL)
	if err != nil {
		return nil, err
	}
	client := &Client{}
	if options.FlareSolverrURL != "" {
		service, err := httpURL(options.FlareSolverrURL)
		if err != nil {
			return nil, err
		}
		origin, err := httpURL(options.Origin)
		if err != nil {
			return nil, err
		}
		if origin.Path != "" && origin.Path != "/" {
			return nil, failure(operationSolve, sdk.InvalidArgument, "clearance origin must not contain a path", nil)
		}
		if options.FlareSolverrProxyURL != "" {
			if err := validateSolverProxy(options.FlareSolverrProxyURL); err != nil {
				return nil, err
			}
		}
		client.endpoint = strings.TrimRight(service.String(), "/") + solverAPIPath
		client.origin, client.solverProxy = origin, options.FlareSolverrProxyURL
		client.control = &http.Client{Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		if options.PersistentSession {
			client.session = solverSessionPrefix + rand.Text()
		}
	}
	client.native, err = newBrowserTransport(proxyURL)
	if err != nil {
		return nil, failure(operationTransport, sdk.UpstreamUnavailable, "could not create Chrome TLS transport", err)
	}
	return client, nil
}

// Format omits proxy settings, sessions, and cached cookies from diagnostics.
func (*Client) Format(state fmt.State, _ rune) { fmt.Fprint(state, "cloudflare.Client{}") }

// CloseIdleConnections releases idle native and solver-control connections.
func (c *Client) CloseIdleConnections() {
	c.native.CloseIdleConnections()
	if c.control != nil {
		c.control.CloseIdleConnections()
	}
}

// Close cancels and joins pending solves, destroys an existing browser session,
// and releases idle connections. A failed destroy can be retried with Close.
// The caller must finish native requests before closing their transport.
func (c *Client) Close(ctx context.Context) error {
	defer c.CloseIdleConnections()
	c.mu.Lock()
	c.closed, c.state = true, nil
	active := c.active
	if active != nil {
		active.cancel()
	}
	c.mu.Unlock()
	if active != nil {
		select {
		case <-active.done: // Join before destroying the browser session it may be using.
		case <-ctx.Done():
			return failure(operationClose, sdk.LocalStateError, "solver shutdown canceled", ctx.Err())
		}
	}
	c.sessionMu.Lock()
	defer c.sessionMu.Unlock()
	if c.phase == sessionUnused {
		return nil
	}
	_, err := c.call(ctx, solverRequest{Command: commandDestroy, Session: c.session})
	if err == nil {
		c.phase = sessionUnused
	}
	return err
}

func failure(operation string, reason sdk.Reason, message string, cause error) *sdk.Error {
	// net/http wraps causes with request URLs, which may contain signed queries.
	var requestError *url.Error
	for errors.As(cause, &requestError) {
		cause = requestError.Err
	}
	return &sdk.Error{Reason: reason, Operation: operation, Message: message, Err: cause}
}

func httpURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, failure(operationSolve, sdk.InvalidArgument, "solver service or challenge URL is invalid", nil)
	}
	return parsed, nil
}

func nativeProxy(raw string) (*url.URL, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https" && parsed.Scheme != "socks5" && parsed.Scheme != "socks5h") || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return nil, failure(operationTransport, sdk.InvalidArgument, "native proxy URL is invalid", nil)
	}
	return parsed, nil
}

func validateSolverProxy(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "socks4" && parsed.Scheme != "socks5") || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return failure(operationSolve, sdk.InvalidArgument, "solver browser proxy URL is invalid", nil)
	}
	return nil
}
