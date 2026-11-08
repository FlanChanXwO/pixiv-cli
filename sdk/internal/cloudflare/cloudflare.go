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
// an expiring cache. Close cancels pending solves and cleans up attempted
// persistent sessions, including creation whose acknowledgment was lost.
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

	"github.com/FlanChanXwO/pixiv-cli/internal/credential"
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

type solverCredentials struct {
	proxy   *solverProxy
	session credential.Secret
}

// Options separates the native route from the solver's browser route. The
// product supplies its anonymous Origin; product cookies never reach the solver.
type Options struct {
	ProxyURL             string // Empty selects direct native transport; HTTP(S) and SOCKS5 are supported.
	FlareSolverrURL      string // Empty disables solver recovery; a service path prefix is allowed.
	FlareSolverrProxyURL string // Browser-only proxy; HTTP supports authentication, SOCKS4/5 are unauthenticated.
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
	credentials *solverCredentials // Pointer keeps private secret fields opaque in value diagnostics.
	sessionGate chan struct{}
	phase       sessionPhase // Protected by sessionGate; uncertain creation still requires cleanup.
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
	client := &Client{credentials: &solverCredentials{}, sessionGate: make(chan struct{}, 1)}
	if options.FlareSolverrURL != "" {
		service, err := httpURL(options.FlareSolverrURL)
		if err != nil {
			return nil, err
		}
		if service.RawQuery != "" || service.ForceQuery {
			return nil, failure(operationSolve, sdk.InvalidArgument, "solver service URL must not contain a query", nil)
		}
		origin, err := httpURL(options.Origin)
		if err != nil {
			return nil, err
		}
		if (origin.Path != "" && origin.Path != "/") || origin.RawQuery != "" || origin.ForceQuery {
			return nil, failure(operationSolve, sdk.InvalidArgument, "clearance origin must not contain a path or query", nil)
		}
		if options.FlareSolverrProxyURL != "" {
			client.credentials.proxy, err = solverProxyURL(options.FlareSolverrProxyURL)
			if err != nil {
				return nil, err
			}
		}
		client.endpoint = strings.TrimRight(service.String(), "/") + solverAPIPath
		client.origin = origin
		client.control = &http.Client{Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		if options.PersistentSession {
			client.credentials.session = credential.New([]byte(solverSessionPrefix + rand.Text()))
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

// Close cancels and joins pending solves, cleans up attempted browser sessions,
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
	if err := c.lockSession(ctx); err != nil {
		return failure(operationClose, sdk.LocalStateError, "solver shutdown canceled", err)
	}
	defer func() { <-c.sessionGate }()
	if c.phase == sessionUnused {
		return nil
	}
	_, err := c.call(ctx, solverRequest{Command: commandDestroy, Session: c.credentials.session.Value()})
	if err == nil {
		c.phase = sessionUnused
	}
	return err
}

func (c *Client) lockSession(ctx context.Context) error {
	// A caller can stop waiting while another create/destroy request owns the gate.
	select {
	case c.sessionGate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
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
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
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

func solverProxyURL(raw string) (*solverProxy, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "socks4" && parsed.Scheme != "socks5") || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return nil, failure(operationSolve, sdk.InvalidArgument, "solver browser proxy URL is invalid", nil)
	}
	result := &solverProxy{}
	if parsed.User != nil {
		if parsed.Scheme != "http" {
			return nil, failure(operationSolve, sdk.InvalidArgument, "Chromium does not support authenticated SOCKS proxies", nil)
		}
		username := parsed.User.Username()
		password, _ := parsed.User.Password()
		result.Username, result.Password = &username, &password
		parsed.User = nil
	}
	result.URL = parsed.String()
	return result, nil
}
