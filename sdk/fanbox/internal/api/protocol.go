package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/internal/cloudflare"
)

const (
	headerOrigin            = "Origin"
	headerReferer           = "Referer"
	headerUserAgent         = "User-Agent"
	headerAccept            = "Accept"
	headerCookie            = "Cookie"
	headerLocation          = "Location"
	headerContentType       = "Content-Type"
	headerMitigated         = "Cf-Mitigated"
	headerServer            = "Server"
	headerRay               = "Cf-Ray"
	headerRetryAfter        = "Retry-After"
	webOrigin               = "https://www.fanbox.cc"
	acceptHTML              = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
	challengeReadBufferSize = 32 * 1024
)

func (c *Client) do(ctx context.Context, operation, endpoint, accept string) (*http.Response, error) {
	state := c.nativeState()
	response, err := c.native(ctx, operation, endpoint, accept, state)
	if sdk.ReasonOf(err) != sdk.ChallengeRequired || !c.solverEnabled {
		return response, err
	}
	// Only a verified challenge permits one anonymous solve and one native replay.
	c.invalidate(state)
	solved, err := c.browser.Solve(ctx, WebBaseURL)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	state = &solved
	c.clearance = state
	c.mu.Unlock()
	slog.DebugContext(ctx, "Replaying FANBOX request with fresh clearance", "operation", operation)
	response, err = c.native(ctx, operation, endpoint, accept, state)
	if sdk.ReasonOf(err) == sdk.ChallengeRequired {
		c.invalidate(state)
	}
	return response, err
}

func (c *Client) invalidate(rejected *cloudflare.Clearance) {
	c.mu.Lock()
	defer c.mu.Unlock()
	current := c.clearance
	// A delayed challenge can reject only the state that request actually used.
	// Shared solve waiters may have adopted equivalent copies of that state.
	if rejected != nil && current != nil && current.UserAgent == rejected.UserAgent && current.Cookie.Value() == rejected.Cookie.Value() && current.ExpiresAt.Equal(rejected.ExpiresAt) {
		c.clearance = nil
		c.browser.Invalidate()
	}
}

func (c *Client) nativeState() *cloudflare.Clearance {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.clearance != nil && !c.clearance.ExpiresAt.IsZero() && !time.Now().Before(c.clearance.ExpiresAt) {
		c.clearance = nil
	}
	return c.clearance
}

func (c *Client) native(ctx context.Context, operation, endpoint, accept string, state *cloudflare.Clearance) (*http.Response, error) {
	visited := make(map[string]bool)
	redirected := false
	for {
		target, err := allowedURL(endpoint, false)
		if err != nil {
			return nil, failure(operation, sdk.ResourceForbidden, "FANBOX URL is not allowed", nil)
		}
		canonical := target.String()
		if visited[canonical] {
			return nil, failure(operation, sdk.UpstreamError, "FANBOX redirect loop", nil)
		}
		visited[canonical] = true
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, canonical, nil)
		if err != nil {
			return nil, externalFailure(ctx, operation, "build FANBOX request failed", err)
		}
		agent, clearance := c.agent, ""
		if state != nil {
			agent, clearance = state.UserAgent, state.Cookie.Value()
		}
		request.Header.Set(headerOrigin, webOrigin)
		request.Header.Set(headerReferer, WebBaseURL)
		request.Header.Set(headerUserAgent, agent)
		request.Header.Set(headerAccept, accept)
		host := strings.ToLower(target.Hostname())
		if !redirected && (host == "www.fanbox.cc" || host == "api.fanbox.cc") {
			cookie := sessionCookieName + "=" + c.session.Value()
			if clearance != "" {
				cookie += "; " + clearanceCookieName + "=" + clearance
			}
			request.Header.Set(headerCookie, cookie)
		}
		// Revalidate each redirect and never forward credentials or a caller's jar.
		client := *c.http
		client.Jar = nil
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		started := time.Now()
		response, err := client.Do(request)
		if err != nil {
			slog.DebugContext(ctx, "FANBOX request failed", "operation", operation, "host", target.Host, "path", target.Path)
			return nil, externalFailure(ctx, operation, "FANBOX request failed", err)
		}
		slog.DebugContext(ctx, "FANBOX request completed", "operation", operation, "host", target.Host, "path", target.Path, "status", response.StatusCode, "duration", time.Since(started))
		switch response.StatusCode {
		case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
			location := response.Header.Get(headerLocation)
			if err := closeResponse(ctx, operation, response); err != nil {
				return nil, err
			}
			next, err := target.Parse(location)
			if err != nil || strings.TrimSpace(location) == "" {
				return nil, failure(operation, sdk.MalformedUpstreamResponse, "invalid FANBOX redirect", nil)
			}
			endpoint, redirected = next.String(), true
			continue
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			return nil, responseFailure(ctx, operation, response)
		}
		return response, nil
	}
}

// ValidateMediaURL checks a first-party media locator without fetching it.
func ValidateMediaURL(raw string) error {
	_, err := allowedURL(raw, true)
	return err
}

func allowedURL(raw string, media bool) (*url.URL, error) {
	target, err := url.Parse(raw)
	if err != nil || target.Scheme != "https" || target.Hostname() == "" || target.User != nil {
		return nil, errors.New("invalid FANBOX URL")
	}
	host := strings.ToLower(target.Hostname())
	allowed := host == "fanbox.cc" || strings.HasSuffix(host, ".fanbox.cc")
	if media {
		allowed = allowed || strings.HasSuffix(host, ".pximg.net") || host == "fanbox.pixiv.net" || strings.HasSuffix(host, ".fanbox.pixiv.net")
		if target.Path == "" || target.Path == "/" {
			return nil, errors.New("invalid FANBOX media path")
		}
	}
	if !allowed {
		return nil, errors.New("invalid FANBOX host")
	}
	return target, nil
}

func readResponse(ctx context.Context, operation string, response *http.Response) ([]byte, error) {
	body, readErr := io.ReadAll(response.Body)
	var err error
	if readErr != nil {
		err = externalFailure(ctx, operation, "read FANBOX response failed", readErr)
	}
	err = errors.Join(err, closeResponse(ctx, operation, response))
	return body, err
}
func closeResponse(ctx context.Context, operation string, response *http.Response) error {
	if err := response.Body.Close(); err != nil {
		return externalFailure(ctx, operation, "close FANBOX response failed", err)
	}
	return nil
}

func responseFailure(ctx context.Context, operation string, response *http.Response) error {
	challenge := false
	var readErr error
	if response.StatusCode == http.StatusForbidden {
		challenge, readErr = scanChallenge(response.Body)
	}
	closeErr := closeResponse(ctx, operation, response)
	if readErr != nil {
		return errors.Join(externalFailure(ctx, operation, "read FANBOX failure response failed", readErr), closeErr)
	}
	if closeErr != nil {
		return closeErr
	}
	reason := sdk.UpstreamError
	switch response.StatusCode {
	case http.StatusUnauthorized:
		reason = sdk.CredentialsExpired
	case http.StatusForbidden:
		reason = sdk.Forbidden
		htmlChallenge := strings.HasPrefix(strings.ToLower(response.Header.Get(headerContentType)), "text/html") && (strings.Contains(strings.ToLower(response.Header.Get(headerServer)), "cloudflare") || response.Header.Get(headerRay) != "")
		if challenge || strings.Contains(strings.ToLower(response.Header.Get(headerMitigated)), "challenge") || htmlChallenge {
			reason = sdk.ChallengeRequired
		}
	case http.StatusNotFound:
		reason = sdk.NotFound
	case http.StatusTooManyRequests:
		reason = sdk.RateLimited
	default:
		if response.StatusCode >= http.StatusInternalServerError {
			reason = sdk.UpstreamUnavailable
		}
	}
	err := failure(operation, reason, "FANBOX returned an HTTP failure", nil)
	err.HTTPStatus = response.StatusCode
	if raw := response.Header.Get(headerRetryAfter); raw != "" {
		if seconds, parseErr := strconv.ParseInt(raw, 10, 64); parseErr == nil && seconds >= 0 && seconds <= int64((1<<63-1)/time.Second) {
			wait := time.Duration(seconds) * time.Second
			err.RetryAfter = &wait
		} else if date, parseErr := http.ParseTime(raw); parseErr == nil {
			wait := max(time.Until(date), 0)
			err.RetryAfter = &wait
		}
	}
	return err
}

func scanChallenge(body io.Reader) (bool, error) {
	buffer := make([]byte, challengeReadBufferSize)
	tail, found := "", false
	const markerTailLength = len("cf-chl") - 1
	for {
		count, err := body.Read(buffer)
		if count > 0 {
			text := tail + strings.ToLower(string(buffer[:count]))
			found = found || strings.Contains(text, "cf-chl") || strings.Contains(text, "cf_chl")
			tail = text[max(0, len(text)-markerTailLength):]
		}
		if errors.Is(err, io.EOF) {
			return found, nil
		}
		if err != nil {
			return false, err
		}
	}
}

func failure(operation string, reason sdk.Reason, message string, cause error) *sdk.Error {
	return &sdk.Error{Operation: operation, Reason: reason, Message: message, Err: cause}
}
func externalFailure(ctx context.Context, operation, message string, cause error) *sdk.Error {
	// External errors may embed signed URLs or cookies; cancellation remains inspectable.
	switch {
	case ctx.Err() != nil:
		cause = ctx.Err()
	case errors.Is(cause, context.Canceled):
		cause = context.Canceled
	case errors.Is(cause, context.DeadlineExceeded):
		cause = context.DeadlineExceeded
	default:
		cause = nil
	}
	return failure(operation, sdk.UpstreamUnavailable, message, cause)
}
