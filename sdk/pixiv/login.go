package pixiv

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/oauth"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// LoginOptions configure a programmatic browser login session.
type LoginOptions struct {
	// HTTPClient performs the authorization-code exchange. It belongs to the
	// caller and is never closed by the session; when nil a session-owned client
	// is created for the one exchange.
	HTTPClient *http.Client
}

// LoginSession is a self-contained, one-shot PKCE login session. It is not
// bound to a Client or any local account store. Copying the small handle shares
// the same session state and one-time gate; completing it twice or concurrently
// returns a stable error. Formatting never exposes the verifier, state, code, or
// callback URL.
type LoginSession struct {
	state *loginSessionState
}

type loginSessionState struct {
	httpClient       *http.Client
	selfHTTP         bool
	authorizationURL string
	verifier         string
	state            string
	used             atomic.Bool
}

// BeginLogin creates a one-shot programmatic login session. The session does
// not open a browser, start a loopback server, or read a TTY; callers open the
// AuthorizationURL themselves.
func BeginLogin(options LoginOptions) (*LoginSession, error) {
	verifier, challenge, err := oauth.GeneratePKCEPair()
	if err != nil {
		return nil, newError("BeginLogin", sdk.CodeUpstreamUnavailable, "cannot create oauth login session")
	}
	state, err := oauth.RandomURLToken(32)
	if err != nil {
		return nil, newError("BeginLogin", sdk.CodeUpstreamUnavailable, "cannot create oauth login session")
	}
	httpClient, selfHTTP := loginHTTPClient(options.HTTPClient)
	values := url.Values{}
	values.Set("code_challenge", challenge)
	values.Set("code_challenge_method", "S256")
	values.Set("client", "pixiv-android")
	values.Set("state", state)
	return &LoginSession{state: &loginSessionState{
		httpClient:       httpClient,
		selfHTTP:         selfHTTP,
		authorizationURL: strings.TrimRight(protocol.AppAPIBase, "/") + protocol.AppLogin + "?" + values.Encode(),
		verifier:         verifier,
		state:            state,
	}}, nil
}

// AuthorizationURL returns the Pixiv authorization URL the caller should open
// in a browser.
func (s *LoginSession) AuthorizationURL() string {
	if s == nil || s.state == nil {
		return ""
	}
	return s.state.authorizationURL
}

// Complete exchanges the authorization code carried by callbackURL (a full
// callback URL or a bare code) for credentials. The session is one-shot; a
// second completion returns CodeInvalidArgument. The callback state is checked
// against this session's state for official callback URLs.
func (s *LoginSession) Complete(ctx context.Context, callbackURL string) (Credentials, error) {
	if s == nil || s.state == nil {
		return Credentials{}, newError("Complete", sdk.CodeInvalidArgument, "login session is nil")
	}
	code, err := loginCode(callbackURL, s.state.state)
	if err != nil {
		return Credentials{}, newError("Complete", sdk.CodeInvalidArgument, "login callback is invalid")
	}
	if !s.state.used.CompareAndSwap(false, true) {
		return Credentials{}, newError("Complete", sdk.CodeInvalidArgument, "login session was already used")
	}
	oauthClient := oauth.New("", oauth.WithHTTPClient(s.state.httpClient))
	token, err := oauthClient.ExchangeAuthorizationCode(ctx, code, s.state.verifier)
	if err != nil {
		return Credentials{}, classifyOAuthError(err, "Complete")
	}
	if token.UserID <= 0 || strings.TrimSpace(token.RefreshToken) == "" {
		return Credentials{}, newError("Complete", sdk.CodeMalformedUpstreamResponse, "oauth response did not include account identity")
	}
	return Credentials{
		UserID:       token.UserID,
		Username:     token.Username,
		ExpiresAt:    token.ExpiresAt,
		accessToken:  token.AccessToken,
		refreshToken: token.RefreshToken,
	}, nil
}

// CloseIdleConnections releases idle connections of a session-owned HTTP
// client. It is a no-op when a caller-provided client was injected.
func (s *LoginSession) CloseIdleConnections() {
	if s == nil || s.state == nil || !s.state.selfHTTP {
		return
	}
	if transport, ok := s.state.httpClient.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
}

func loginHTTPClient(injected *http.Client) (*http.Client, bool) {
	if injected != nil {
		return injected, false
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	return &http.Client{Transport: transport}, true
}

// String, GoString, and Format never expose the verifier, state, code, or
// callback; the authorization URL is reachable only through AuthorizationURL.
func (LoginSession) String() string   { return "pixiv.LoginSession{}" }
func (LoginSession) GoString() string { return "pixiv.LoginSession{}" }
func (LoginSession) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "pixiv.LoginSession{}")
}

// loginCode extracts the authorization code from a callback URL or a bare code,
// validating the session state for official callbacks.
func loginCode(input, expectedState string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("empty input")
	}
	parsed, err := url.Parse(input)
	if err != nil || parsed.Scheme == "" {
		if strings.ContainsAny(input, "?#&") {
			return "", fmt.Errorf("malformed callback")
		}
		return input, nil
	}
	code := strings.TrimSpace(parsed.Query().Get("code"))
	if code == "" {
		return "", fmt.Errorf("missing code")
	}
	state := strings.TrimSpace(parsed.Query().Get("state"))
	if loginCallbackRequiresState(parsed) {
		if state == "" || state != expectedState {
			return "", fmt.Errorf("state mismatch")
		}
	} else if state != "" && state != expectedState {
		return "", fmt.Errorf("state mismatch")
	}
	return code, nil
}

func loginCallbackRequiresState(parsed *url.URL) bool {
	if parsed == nil {
		return true
	}
	if strings.EqualFold(parsed.Scheme, "pixiv") && strings.EqualFold(parsed.Host, "account") && parsed.Path == "/login" {
		return false
	}
	callback, err := url.Parse(protocol.OAuthRedirectURI)
	if err != nil {
		return true
	}
	return !(strings.EqualFold(parsed.Scheme, callback.Scheme) && strings.EqualFold(parsed.Host, callback.Host) && parsed.Path == callback.Path)
}
