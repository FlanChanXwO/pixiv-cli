package pixiv

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"

	"github.com/FlanChanXwO/pixiv-cli/internal/credential"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api/oauth"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

const (
	loginStateBytes      = 32
	loginChallenge       = "code_challenge"
	loginChallengeMethod = "code_challenge_method"
	loginClient          = "client"
	loginState           = "state"
	loginCode            = "code"
	loginS256            = "S256"
	loginAndroid         = "pixiv-android"
	loginAppCallback     = "pixiv://account/login"
)

// LoginSession owns a one-shot PKCE exchange. Copies share the completion gate.
// AuthorizationURL explicitly exposes the login URL to the caller; formatting
// and JSON omit the nonce, verifier, callback, and credentials.
// BeginLogin creates the session; Complete returns credentials for caller-owned
// persistence, and CloseIdleConnections releases session-owned idle sockets.
type LoginSession struct {
	http          *http.Client
	ownsHTTP      bool
	authorization string
	verifier      credential.Secret
	nonce         credential.Secret
	used          *atomic.Bool
}

// BeginLogin generates PKCE and state without network I/O, browser access or a
// listener. Options.HTTPClient and RequestInterval control the later exchange;
// AcceptLanguage and ResourceHosts apply only to content/resource clients.
func BeginLogin(options Options) (*LoginSession, error) {
	verifier, challenge, err := oauth.GeneratePKCEPair()
	if err != nil {
		return nil, newError("BeginLogin", sdk.LocalStateError, "could not generate PKCE", err)
	}
	nonce, err := oauth.RandomURLToken(loginStateBytes)
	if err != nil {
		return nil, newError("BeginLogin", sdk.LocalStateError, "could not generate login state", err)
	}
	query := url.Values{loginChallenge: {challenge}, loginChallengeMethod: {loginS256}, loginClient: {loginAndroid}, loginState: {nonce}}
	client, owned := api.NewHTTPClient(options.HTTPClient, options.RequestInterval, protocolLogger())
	return &LoginSession{http: client, ownsHTTP: owned, authorization: api.AppAPIBase + api.PathLogin + "?" + query.Encode(), verifier: credential.New([]byte(verifier)), nonce: credential.New([]byte(nonce)), used: &atomic.Bool{}}, nil
}

// AuthorizationURL returns the login URL to open in the caller's browser; an
// unbound session returns "". The URL contains state and must not enter logs.
func (s *LoginSession) AuthorizationURL() string {
	if s == nil {
		return ""
	}
	return s.authorization
}

// AcceptsCallbackURL checks a callback without I/O or consuming the session.
// Official HTTPS and pixiv://account/login callbacks may omit state; any supplied
// state must match. Other callbacks require matching state. Bare codes return false.
func (s *LoginSession) AcceptsCallbackURL(rawURL string) bool {
	if s == nil || s.used == nil {
		return false
	}
	_, err := s.callbackCode(rawURL, false)
	return err == nil
}

// Complete exchanges a callback URL or bare authorization code. A valid input
// consumes the session before I/O, including when the exchange fails or is
// canceled; concurrent or repeated completion returns InvalidArgument. Invalid
// input leaves it available. Persist returned credentials before issuing requests.
func (s *LoginSession) Complete(ctx context.Context, callbackOrCode string) (Credentials, error) {
	if s == nil || s.used == nil {
		return Credentials{}, newError("LoginSession.Complete", sdk.InvalidArgument, "login session is unbound", nil)
	}
	code, err := s.callbackCode(callbackOrCode, true)
	if err != nil {
		return Credentials{}, err
	}
	// Consume before I/O because a failed response can hide a successful exchange.
	if !s.used.CompareAndSwap(false, true) {
		return Credentials{}, newError("LoginSession.Complete", sdk.InvalidArgument, "login session was already consumed", nil)
	}
	auth := oauth.New("", oauth.Config{HTTPClient: s.http, Logger: protocolLogger(), PreferConfiguredLogger: true})
	token, err := auth.ExchangeAuthorizationCode(ctx, code, s.verifier.Value())
	if err != nil {
		return Credentials{}, classifyError("LoginSession.Complete", err, true)
	}
	if token.UserID <= 0 || strings.TrimSpace(token.AccessToken) == "" || strings.TrimSpace(token.RefreshToken) == "" {
		return Credentials{}, newError("LoginSession.Complete", sdk.MalformedUpstreamResponse, "OAuth response is missing credentials or account identity", nil)
	}
	value := NewCredentials(token.RefreshToken)
	value.SetTokens(token.AccessToken, token.RefreshToken)
	value.UserID, value.DisplayName, value.ExpiresAt = token.UserID, token.Username, token.ExpiresAt
	if err := validateCredentials(value); err != nil {
		return Credentials{}, newError("LoginSession.Complete", sdk.MalformedUpstreamResponse, "OAuth response contains invalid credentials", err)
	}
	slog.DebugContext(ctx, "OAuth login completed")
	return value, nil
}

// CloseIdleConnections releases only SDK-owned idle sockets. Finish an exchange
// before closing; an injected HTTP client remains the caller's responsibility.
func (s *LoginSession) CloseIdleConnections() {
	if s != nil && s.ownsHTTP {
		s.http.CloseIdleConnections()
	}
}

// Format omits PKCE and callback data, including with %#v.
func (LoginSession) Format(state fmt.State, _ rune) { fmt.Fprint(state, "pixiv.LoginSession{}") }

func (s *LoginSession) callbackCode(input string, allowBare bool) (string, error) {
	invalid := newError("LoginSession.Complete", sdk.InvalidArgument, "login callback is invalid", nil)
	input = strings.TrimSpace(input)
	parsed, err := url.Parse(input)
	if err != nil || parsed.Scheme == "" {
		if allowBare && input != "" && !strings.ContainsAny(input, "?#&") {
			return input, nil
		}
		return "", invalid
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(query[loginCode]) != 1 || len(query[loginState]) > 1 {
		return "", invalid
	}
	code, state := strings.TrimSpace(query.Get(loginCode)), strings.TrimSpace(query.Get(loginState))
	if code == "" {
		return "", invalid
	}
	route := strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host) + parsed.EscapedPath()
	official := parsed.User == nil && parsed.Opaque == "" && (route == api.OAuthRedirectURI || route == loginAppCallback)
	if (!official && state == "") || (state != "" && state != s.nonce.Value()) {
		return "", invalid
	}
	return code, nil
}
