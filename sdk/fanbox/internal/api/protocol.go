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
	// HeaderOrigin carries the FANBOX web origin on product requests.
	HeaderOrigin = "Origin"
	// HeaderReferer identifies the FANBOX homepage on API and media requests.
	HeaderReferer = "Referer"
	// HeaderUserAgent carries the native or solved browser identity.
	HeaderUserAgent = "User-Agent"
	// HeaderAccept selects the expected representation.
	HeaderAccept = "Accept"
	// HeaderCookie carries credentials only to the product's approved cookie host.
	HeaderCookie = "Cookie"
	// HeaderLocation supplies the next redirect target for validation.
	HeaderLocation = "Location"
	// HeaderMitigated identifies an explicit Cloudflare challenge.
	HeaderMitigated = "Cf-Mitigated"
	// HeaderServer helps distinguish Cloudflare HTML from product failures.
	HeaderServer = "Server"
	// HeaderRay identifies responses processed by Cloudflare.
	HeaderRay = "Cf-Ray"
	// HeaderRetryAfter supplies the upstream wait duration without scheduling retries.
	HeaderRetryAfter = "Retry-After"
)

const (
	webOrigin               = "https://www.fanbox.cc"
	acceptHTML              = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
	challengeReadBufferSize = 32 * 1024
)

type requestProfile struct {
	accept  string
	method  string
	media   bool
	headers http.Header
}

func (c *Client) do(ctx context.Context, operation, endpoint string, profile requestProfile) (*http.Response, error) {
	state := c.nativeState()
	response, err := c.native(ctx, operation, endpoint, profile, state)
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
	response, err = c.native(ctx, operation, endpoint, profile, state)
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

func (c *Client) native(ctx context.Context, operation, endpoint string, profile requestProfile, state *cloudflare.Clearance) (*http.Response, error) {
	visited := make(map[string]bool)
	redirected := false
	for {
		target, err := allowedURL(endpoint, profile.media)
		if err != nil {
			return nil, failure(operation, sdk.ResourceForbidden, "FANBOX URL is not allowed", nil)
		}
		canonical := target.String()
		if visited[canonical] {
			return nil, failure(operation, sdk.UpstreamError, "FANBOX redirect loop", nil)
		}
		visited[canonical] = true
		request, err := http.NewRequestWithContext(ctx, profile.method, canonical, nil)
		if err != nil {
			return nil, externalFailure(ctx, operation, "build FANBOX request failed", err)
		}
		agent, clearance := c.agent, ""
		if state != nil {
			agent, clearance = state.UserAgent, state.Cookie.Value()
		}
		request.Header.Set(HeaderOrigin, webOrigin)
		request.Header.Set(HeaderReferer, WebBaseURL)
		request.Header.Set(HeaderUserAgent, agent)
		request.Header.Set(HeaderAccept, profile.accept)
		for name, values := range profile.headers {
			for _, value := range values {
				request.Header.Add(name, value)
			}
		}
		host := strings.ToLower(target.Hostname())
		cookieAllowed := profile.media && host == DownloadsHost || !profile.media && (host == "www.fanbox.cc" || host == "api.fanbox.cc")
		if !redirected && cookieAllowed {
			cookie := sessionCookieName + "=" + c.session.Value()
			if clearance != "" {
				cookie += "; " + clearanceCookieName + "=" + clearance
			}
			request.Header.Set(HeaderCookie, cookie)
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
			location := response.Header.Get(HeaderLocation)
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
		if profile.media && response.StatusCode == http.StatusNotModified {
			return response, nil
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
		htmlChallenge := strings.HasPrefix(strings.ToLower(response.Header.Get(HeaderContentType)), "text/html") && (strings.Contains(strings.ToLower(response.Header.Get(HeaderServer)), "cloudflare") || response.Header.Get(HeaderRay) != "")
		if challenge || strings.Contains(strings.ToLower(response.Header.Get(HeaderMitigated)), "challenge") || htmlChallenge {
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
	if raw := response.Header.Get(HeaderRetryAfter); raw != "" {
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
