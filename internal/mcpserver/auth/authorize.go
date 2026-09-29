package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"html/template"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

type authorizationRequest struct {
	ClientID, RedirectURI, Resource, Scope, Challenge, State string
	HasState                                                 bool
}

type ownerSession struct {
	Generation string
	Verified   bool
}

type consentForm struct {
	Request               authorizationRequest
	SessionID, Generation string
}

// authorizationCode 仅存内存；兑换时须核验请求绑定、generation 与截止时间。
type authorizationCode struct {
	Request    authorizationRequest
	Generation string
	ExpiresAt  time.Time
}

type authorizer struct {
	generation string
	codes      map[[32]byte]authorizationCode
	now        func() time.Time

	store                                Store
	issuer, origin, endpoint, cookiePath string
	secure                               bool
	mu                                   sync.Mutex
	sessions                             map[string]ownerSession
	forms                                map[string]consentForm
}

type consentPage struct {
	Action, ClientName, RedirectOrigin, RedirectURI, CSRF string
	Verified                                              bool
}

// authorizationReply 把锁内状态变更与可能阻塞的 HTTP 写入分开。
type authorizationReply struct {
	Status          int
	Error, Location string
	Cookie          *http.Cookie
	Page            *consentPage
}

func newAuthorizer(base *url.URL, store Store) *authorizer {
	return &authorizer{
		now: time.Now, codes: map[[32]byte]authorizationCode{}, store: store,
		issuer: base.String(), origin: browserOrigin(base.Scheme + "://" + base.Host),
		// 完整 URL 防止双斜线 prefix 被浏览器解释为跨 origin action。
		endpoint: base.String() + "/oauth/authorize", cookiePath: base.EscapedPath() + "/oauth",
		secure: base.Scheme == "https", sessions: map[string]ownerSession{}, forms: map[string]consentForm{},
	}
}

func (a *authorizer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	// no-referrer 会让浏览器表单 POST 的 Origin 变成 null；strict-origin 保留 origin 但不泄漏 query。
	w.Header().Set("Referrer-Policy", "strict-origin")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	var reply authorizationReply
	switch r.Method {
	case http.MethodGet:
		reply = a.prepare(r)
	case http.MethodPost:
		reply = a.submit(r)
	default:
		w.Header().Set("Allow", "GET, POST")
		reply = authorizationReply{Status: 405, Error: "invalid_request"}
	}
	if reply.Cookie != nil {
		http.SetCookie(w, reply.Cookie)
	}
	if reply.Location != "" {
		w.Header().Set("Location", reply.Location)
		w.WriteHeader(http.StatusFound)
		return
	}
	if reply.Page != nil {
		// Chromium 同时约束表单后的重定向；只放行已精确匹配的 callback origin。
		// 注册 origin 不能成为 CSP 通配符或额外 directive。
		source := strings.NewReplacer(";", "%3B", "*", "%2A", "'", "%27").Replace(reply.Page.RedirectOrigin)
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self' "+source+"; frame-ancestors 'none'; base-uri 'none'")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = consentTemplate.Execute(w, reply.Page)
		return
	}
	writeJSON(w, reply.Status, map[string]string{"error": reply.Error})
}

func (a *authorizer) prepare(r *http.Request) authorizationReply {
	a.mu.Lock()
	defer a.mu.Unlock()
	state, err := a.readState(r.Context())
	if err != nil {
		return authorizationReply{Status: 500, Error: "server_error"}
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query["client_id"]) != 1 || len(query["redirect_uri"]) != 1 {
		return authorizationReply{Status: 400, Error: "invalid_request"}
	}
	client, ok := state.Clients[query.Get("client_id")]
	if !ok || !slices.Contains(client.RedirectURIs, query.Get("redirect_uri")) || !validRedirect(query.Get("redirect_uri")) {
		// 未确认 client/redirect 时绝不跳转，防止授权端点成为 open redirect。
		return authorizationReply{Status: 400, Error: "invalid_request"}
	}
	redirect, _ := url.Parse(query.Get("redirect_uri"))
	callbackQuery, err := url.ParseQuery(redirect.RawQuery)
	if err != nil {
		return authorizationReply{Status: 400, Error: "invalid_request"}
	}
	// 不覆盖注册 URI 的 query，也不把预置 code/error 当作本次授权结果。
	for _, key := range []string{"code", "error", "error_description", "error_uri", "state", "iss"} {
		if callbackQuery.Has(key) {
			return authorizationReply{Status: 400, Error: "invalid_request"}
		}
	}
	request := authorizationRequest{ClientID: query.Get("client_id"), RedirectURI: query.Get("redirect_uri"), Resource: query.Get("resource"), Scope: query.Get("scope"), Challenge: query.Get("code_challenge"), State: query.Get("state"), HasState: len(query["state"]) == 1}
	for _, key := range []string{"response_type", "resource", "scope", "code_challenge", "code_challenge_method", "state"} {
		if len(query[key]) > 1 {
			return a.callback(request, "", "invalid_request")
		}
	}
	if query.Get("response_type") != "code" {
		return a.callback(request, "", "unsupported_response_type")
	}
	if request.Resource != a.issuer+"/mcp" {
		return a.callback(request, "", "invalid_target")
	}
	if !query.Has("scope") {
		request.Scope = "mcp"
	}
	if request.Scope != "mcp" {
		return a.callback(request, "", "invalid_scope")
	}
	challenge, err := base64.RawURLEncoding.Strict().DecodeString(request.Challenge)
	if query.Get("code_challenge_method") != "S256" || len(request.Challenge) != 43 || err != nil || len(challenge) != sha256.Size {
		return a.callback(request, "", "invalid_request")
	}
	var sessionID string
	if cookie, err := r.Cookie("pixiv_mcp_owner"); err == nil {
		sessionID = cookie.Value
	}
	session, exists := a.sessions[sessionID]
	var cookie *http.Cookie
	if !exists || session.Generation != state.OwnerVerifier {
		sessionID, err = randomAuthorizationValue()
		if err != nil {
			return authorizationReply{Status: 500, Error: "server_error"}
		}
		session = ownerSession{Generation: state.OwnerVerifier}
		a.sessions[sessionID] = session
		cookie = a.cookie(sessionID)
	}
	csrf, err := randomAuthorizationValue()
	if err != nil {
		return authorizationReply{Status: 500, Error: "server_error"}
	}
	a.forms[csrf] = consentForm{Request: request, SessionID: sessionID, Generation: state.OwnerVerifier}
	origin := redirect.Scheme + ":"
	if redirect.Host != "" {
		origin += "//" + redirect.Host
	}
	return authorizationReply{Cookie: cookie, Page: &consentPage{Action: a.endpoint, ClientName: client.ClientName, RedirectOrigin: origin, RedirectURI: request.RedirectURI, CSRF: csrf, Verified: session.Verified}}
}

// browserOrigin 只比较 scheme/host/有效端口，不把路径或 opaque Origin 当作同源。
func browserOrigin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Opaque != "" || u.Path != "" ||
		strings.ContainsAny(raw, `?#\`) || (u.Scheme != "http" && u.Scheme != "https") || !validHTTPAuthority(u) {
		return ""
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return u.Scheme + "://" + net.JoinHostPort(strings.ToLower(u.Hostname()), port)
}

func (a *authorizer) submit(r *http.Request) authorizationReply {
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		return authorizationReply{Status: 400, Error: "invalid_request"}
	}
	origins := r.Header.Values("Origin")
	if len(origins) > 1 || (len(origins) == 1 && browserOrigin(origins[0]) != a.origin) || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return authorizationReply{Status: 403, Error: "access_denied"}
	}

	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/x-www-form-urlencoded" {
		return authorizationReply{Status: 400, Error: "invalid_request"}
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return authorizationReply{Status: 400, Error: "invalid_request"}
	}
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return authorizationReply{Status: 400, Error: "invalid_request"}
	}
	for key, fields := range values {
		if (key != "csrf_token" && key != "owner_secret" && key != "decision") || len(fields) != 1 {
			return authorizationReply{Status: 400, Error: "invalid_request"}
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	state, err := a.readState(r.Context())
	if err != nil {
		return authorizationReply{Status: 500, Error: "server_error"}
	}
	cookie, err := r.Cookie("pixiv_mcp_owner")
	if err != nil {
		return authorizationReply{Status: 403, Error: "access_denied"}
	}
	session, exists := a.sessions[cookie.Value]
	form, pending := a.forms[values.Get("csrf_token")]
	if !exists || !pending || form.SessionID != cookie.Value || session.Generation != state.OwnerVerifier || form.Generation != state.OwnerVerifier {
		return authorizationReply{Status: 403, Error: "access_denied"}
	}
	delete(a.forms, values.Get("csrf_token"))
	client, registered := state.Clients[form.Request.ClientID]
	if !registered || !slices.Contains(client.RedirectURIs, form.Request.RedirectURI) {
		return authorizationReply{Status: 400, Error: "invalid_request"}
	}
	if values.Get("decision") == "deny" {
		return a.callback(form.Request, "", "access_denied")
	}
	if values.Get("decision") != "allow" {
		return authorizationReply{Status: 400, Error: "invalid_request"}
	}
	if !session.Verified {
		digest := sha256.Sum256([]byte(values.Get("owner_secret")))
		expected, _ := hex.DecodeString(state.OwnerVerifier)
		if subtle.ConstantTimeCompare(digest[:], expected) != 1 {
			return authorizationReply{Status: 403, Error: "access_denied"}
		}
	}
	code, err := randomAuthorizationValue()
	if err != nil {
		return authorizationReply{Status: 500, Error: "server_error"}
	}
	reply := a.callback(form.Request, code, "")
	if !session.Verified {
		id, err := randomAuthorizationValue()
		if err != nil {
			return authorizationReply{Status: 500, Error: "server_error"}
		}
		// 身份升级时更换浏览器会话，旧匿名 cookie/表单不能继承 owner 身份。
		delete(a.sessions, cookie.Value)
		for csrf, other := range a.forms {
			if other.SessionID == cookie.Value {
				delete(a.forms, csrf)
			}
		}
		a.sessions[id] = ownerSession{Generation: state.OwnerVerifier, Verified: true}
		reply.Cookie = a.cookie(id)
	}
	now := a.now()
	// RFC 6749 §4.1.2：有效期从签发起计，不给浏览器等待或业务请求加超时。
	a.codes[sha256.Sum256([]byte(code))] = authorizationCode{Request: form.Request, Generation: state.OwnerVerifier, ExpiresAt: now.Add(10 * time.Minute)}
	return reply
}

// readState 仅在持有 mu 时调用，reset 后连同旧表单一起丢弃内存身份与 code。
func (a *authorizer) readState(ctx context.Context) (State, error) {
	state, err := a.store.Read(ctx)
	if err != nil {
		return State{}, err
	}
	if a.generation != state.OwnerVerifier {
		clear(a.sessions)
		clear(a.forms)
		clear(a.codes)
		a.generation = state.OwnerVerifier
	}
	now := a.now()
	for hash, code := range a.codes {
		if !now.Before(code.ExpiresAt) {
			delete(a.codes, hash)
		}
	}
	return state, nil
}

func (a *authorizer) callback(request authorizationRequest, code, failure string) authorizationReply {
	destination, _ := url.Parse(request.RedirectURI)
	query := destination.Query()
	query.Set("iss", a.issuer)
	if request.HasState {
		query.Set("state", request.State)
	}
	if failure != "" {
		query.Set("error", failure)
	} else {
		query.Set("code", code)
	}
	destination.RawQuery = query.Encode()
	return authorizationReply{Location: destination.String()}
}

func (a *authorizer) cookie(value string) *http.Cookie {
	return &http.Cookie{Name: "pixiv_mcp_owner", Value: value, Path: a.cookiePath, HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode}
}

func randomAuthorizationValue() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

var consentTemplate = template.Must(template.New("consent").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Authorize pixiv-cli</title><style>
body{font:16px/1.6 system-ui,sans-serif;background:#f5f6f8;color:#202838;margin:0;padding:24px}main{max-width:560px;margin:6vh auto;background:white;padding:32px;border-radius:12px;border:1px solid #d9dfe7}h1{font-size:26px;margin-top:0}code{overflow-wrap:anywhere}label{display:block;font-weight:600}input{box-sizing:border-box;width:100%;font:inherit;padding:10px;margin:8px 0 20px;border:1px solid #778396;border-radius:6px}button{font:inherit;padding:10px 22px;margin:8px 8px 0 0;cursor:pointer;border:1px solid #44546e;border-radius:6px;background:white}button[value=allow]{background:#243f70;color:white}button:focus-visible,input:focus-visible{outline:3px solid #6188d0;outline-offset:3px}small{display:block;color:#506078}
</style></head><body><main><h1>Authorize pixiv-cli</h1>
<p>Only approve connectors you trust. This grants access to this instance's MCP tools.</p>
<p>Unverified client name: <strong>{{.ClientName}}</strong></p>
<p>Redirect origin: <code>{{.RedirectOrigin}}</code><br>Redirect URI: <code>{{.RedirectURI}}</code></p>
<form method="post" action="{{.Action}}">
<input type="hidden" name="csrf_token" value="{{.CSRF}}">
{{if .Verified}}<p>Owner verified in this browser. Your explicit approval is still required.</p>{{else}}
<label for="owner-secret">Owner secret</label><small>Use the secret from pixiv mcp auth init. Do not share it in chat.</small>
<input id="owner-secret" name="owner_secret" type="password" autocomplete="current-password">{{end}}
<button type="submit" name="decision" value="allow">Allow</button><button type="submit" name="decision" value="deny">Deny</button>
</form></main></body></html>`))
