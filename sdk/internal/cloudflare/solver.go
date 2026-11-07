package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/credential"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

const (
	solverAPIPath       = "/v1"
	solverSessionPrefix = "pixiv-cli-cloudflare-"
	commandCreate       = "sessions.create"
	commandGet          = "request.get"
	commandDestroy      = "sessions.destroy"
	statusOK            = "ok"
	statusError         = "error"
	clearanceCookieName = "cf_clearance"
	headerContentType   = "Content-Type"
	headerAccept        = "Accept"
	jsonMediaType       = "application/json"
	// Retain ascii2d's browser-protocol maxTimeout; Go requests use caller cancellation.
	sessionMaxTimeout = 180000
)

// Clearance carries only the solver's user agent, cf_clearance, and optional
// expiry. Other solver cookies, HTML, and upstream headers are discarded.
// Cookie's Secret value redacts itself in formatting and slog; read Value only
// while constructing a request for the product's validated cookie host.
type Clearance struct {
	UserAgent string
	Cookie    credential.Secret `json:"-"`
	ExpiresAt time.Time         // Zero when the solver did not supply an expiry.
}

func (s Clearance) expired() bool { return !s.ExpiresAt.IsZero() && !time.Now().Before(s.ExpiresAt) }

type solverRequest struct {
	Command    string       `json:"cmd"`
	Session    string       `json:"session,omitempty"`
	URL        string       `json:"url,omitempty"`
	Proxy      *solverProxy `json:"proxy,omitempty"`
	MaxTimeout int          `json:"maxTimeout,omitempty"`
}

type solverProxy struct {
	URL string `json:"url"`
}

type solverResponse struct {
	Status   string          `json:"status"`
	Solution *solverSolution `json:"solution"`
}

type solverSolution struct {
	UserAgent string         `json:"userAgent"`
	Cookies   []solverCookie `json:"cookies"`
}

type solverCookie struct {
	Name    string          `json:"name"`
	Value   string          `json:"value"`
	Expires json.RawMessage `json:"expires"`
	Expiry  json.RawMessage `json:"expiry"`
}

func (c *Client) solve(ctx context.Context, target string) (Clearance, error) {
	request := solverRequest{Command: commandGet, URL: target}
	if c.session != "" {
		if err := c.createSession(ctx); err != nil {
			return Clearance{}, err
		}
		request.Session, request.MaxTimeout = c.session, sessionMaxTimeout
	} else if c.solverProxy != "" {
		request.Proxy = &solverProxy{URL: c.solverProxy}
	}
	response, err := c.call(ctx, request)
	if err != nil {
		return Clearance{}, err
	}
	return validateSolution(response.Solution)
}

func (c *Client) createSession(ctx context.Context) error {
	// Serializing creation prevents concurrent Close calls from destroying a new session.
	c.sessionMu.Lock()
	defer c.sessionMu.Unlock()
	if c.created {
		return nil
	}
	request := solverRequest{Command: commandCreate, Session: c.session}
	if c.solverProxy != "" {
		request.Proxy = &solverProxy{URL: c.solverProxy}
	}
	_, err := c.call(ctx, request)
	if err == nil {
		c.created = true
	}
	return err
}

func (c *Client) call(ctx context.Context, payload solverRequest) (solverResponse, error) {
	operation := operationSolve
	if payload.Command == commandDestroy {
		operation = operationClose
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return solverResponse{}, failure(operation, sdk.InvalidArgument, "could not encode solver request", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return solverResponse{}, failure(operation, sdk.InvalidArgument, "invalid solver request", err)
	}
	request.Header.Set(headerContentType, jsonMediaType)
	request.Header.Set(headerAccept, jsonMediaType)
	response, err := c.control.Do(request)
	if err != nil {
		return solverResponse{}, failure(operation, sdk.UpstreamUnavailable, "FlareSolverr service unavailable", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		err := failure(operation, sdk.ChallengeRequired, "FlareSolverr returned an HTTP failure", nil)
		err.HTTPStatus = response.StatusCode
		return solverResponse{}, err
	}
	decoder := json.NewDecoder(response.Body)
	var decoded solverResponse
	if err := decoder.Decode(&decoded); err != nil {
		return solverResponse{}, failure(operation, sdk.MalformedUpstreamResponse, "malformed FlareSolverr response", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return solverResponse{}, failure(operation, sdk.MalformedUpstreamResponse, "trailing FlareSolverr response data", err)
	}
	switch decoded.Status {
	case statusOK:
		return decoded, nil
	case statusError:
		return solverResponse{}, failure(operation, sdk.ChallengeRequired, "FlareSolverr could not solve challenge", nil)
	default:
		return solverResponse{}, failure(operation, sdk.MalformedUpstreamResponse, "unknown FlareSolverr status", nil)
	}
}
func validateSolution(solution *solverSolution) (Clearance, error) {
	malformed := func() (Clearance, error) {
		return Clearance{}, failure(operationSolve, sdk.MalformedUpstreamResponse, "invalid FlareSolverr clearance", nil)
	}
	if solution == nil || !validHeaderValue(solution.UserAgent) {
		return malformed()
	}
	var value string
	var expiry json.RawMessage
	for _, cookie := range solution.Cookies {
		if cookie.Name != clearanceCookieName {
			continue
		}
		if value != "" || cookie.Value == "" || !validCookieValue(cookie.Value) {
			return malformed()
		}
		value = cookie.Value
		expiry = bytes.TrimSpace(cookie.Expiry)
		if len(expiry) == 0 || bytes.Equal(expiry, []byte("null")) {
			expiry = bytes.TrimSpace(cookie.Expires)
		}
	}
	if value == "" {
		return malformed()
	}
	state := Clearance{UserAgent: solution.UserAgent, Cookie: credential.New([]byte(value))}
	if len(expiry) != 0 && !bytes.Equal(expiry, []byte("null")) {
		var err error
		state.ExpiresAt, err = parseExpiry(expiry)
		if err != nil {
			return Clearance{}, err
		}
		if state.expired() {
			return malformed()
		}
	}
	return state, nil
}

func validHeaderValue(value string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			return false
		}
	}
	return true
}

func validCookieValue(value string) bool {
	for _, char := range value {
		// RFC 6265 cookie-octet excludes whitespace, controls, quotes, comma, semicolon, and backslash.
		if char < 0x21 || char > 0x7e || strings.ContainsRune("\",;\\", char) {
			return false
		}
	}
	return true
}

func parseExpiry(raw json.RawMessage) (time.Time, error) {
	var numeric json.Number
	if err := json.Unmarshal(raw, &numeric); err == nil {
		value, err := numeric.Int64()
		if err == nil && value > 0 {
			return time.Unix(value, 0), nil
		}
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		for _, layout := range []string{time.RFC3339, http.TimeFormat} {
			if value, err := time.Parse(layout, text); err == nil {
				return value, nil
			}
		}
	}
	return time.Time{}, failure(operationSolve, sdk.MalformedUpstreamResponse, "invalid FlareSolverr cookie expiry", nil)
}
