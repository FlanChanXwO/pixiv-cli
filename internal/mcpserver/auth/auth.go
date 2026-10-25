// auth.go 组装 OAuth 路由与入站保护，提供各授权阶段共用的随机值生成。

package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

// OAuth control messages contain metadata and opaque identifiers, never media.
// Bound unauthenticated allocation and persisted DCR growth for this single-owner server.
const maxOAuthBody = 64 << 10
const maxOAuthEntries = 1024
const controlBodyReadTimeout = 10 * time.Second

var errOAuthCapacity = errors.New("OAuth capacity reached")

// Handler 同时持有 OAuth 路由及 canonical resource 的 bearer 边界，不管理 listener。
type Handler struct {
	routes                http.Handler
	store                 Store
	resource, metadataURL string
}

// ServeHTTP 处理 discovery、注册、授权与 token 请求。
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.routes.ServeHTTP(w, r) }

func randomAuthorizationValue() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

// RequireBearer 包装 MCP handler；鉴权只影响新请求，不取消已授权请求。
func (h *Handler) RequireBearer(next http.Handler) http.Handler {
	protected := sdkauth.RequireBearerToken(h.verify, &sdkauth.RequireBearerTokenOptions{
		ResourceMetadataURL: h.metadataURL, Scopes: []string{"mcp"},
	})(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		protected.ServeHTTP(privateResponseWriter{w}, r)
	})
}

// SDK 会重设 SSE 缓存头；在实际提交时守住私有结果边界，而非只在进入 handler 前设置。
type privateResponseWriter struct{ http.ResponseWriter }

func (w privateResponseWriter) WriteHeader(status int) {
	w.Header().Set("Cache-Control", "no-store")
	w.ResponseWriter.WriteHeader(status)
}

func (w privateResponseWriter) Write(p []byte) (int, error) {
	w.Header().Set("Cache-Control", "no-store")
	return w.ResponseWriter.Write(p)
}

func (w privateResponseWriter) FlushError() error {
	w.Header().Set("Cache-Control", "no-store")
	return http.NewResponseController(w.ResponseWriter).Flush()
}

// 让 ResponseController 的 deadline/full-duplex 等能力仍可到达底层连接。
func (w privateResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (h *Handler) verify(ctx context.Context, token string, request *http.Request) (*sdkauth.TokenInfo, error) {
	if len(request.Header.Values("Authorization")) != 1 {
		return nil, sdkauth.ErrInvalidToken
	}
	state, err := h.store.Read(ctx)
	if err != nil {
		return nil, errors.New("MCP authorization state unavailable")
	}
	hash := tokenHash(token)
	for grantID, grant := range state.Grants {
		expiry, exists := grant.AccessTokens[hash]
		if exists && grant.Usable(h.resource) && time.Now().Before(expiry) {
			return &sdkauth.TokenInfo{Scopes: []string{grant.Scope}, Expiration: expiry, UserID: "owner", Extra: map[string]any{"grant_id": grantID}}, nil
		}
	}
	return nil, sdkauth.ErrInvalidToken
}

// ParseBaseURL 校验并返回规范化的公开 MCP base URL（去掉末尾斜杠），只接受显式配置。
// 客户端会归一化 dot segments；拒绝可能使 issuer 与 discovery 路径不一致的配置。
func ParseBaseURL(canonicalBase string) (*url.URL, error) {
	base, err := url.Parse(strings.TrimRight(canonicalBase, "/"))
	if err != nil || base.User != nil || base.Opaque != "" ||
		(base.Scheme != "http" && base.Scheme != "https") || !validHTTPAuthority(base) ||
		base.RawQuery != "" || base.ForceQuery || strings.ContainsAny(canonicalBase, `#\`) {
		return nil, errors.New("canonical MCP base URL must be an absolute HTTP(S) URL without credentials, query or fragment")
	}
	for _, segment := range strings.Split(base.Path, "/") {
		if segment == "." || segment == ".." {
			return nil, errors.New("canonical MCP base URL path must not contain dot segments")
		}
	}
	return base, nil
}

// NewHandler 构造独立 discovery/DCR/authorize/token handler 及 bearer 包装器；canonicalBase 不从请求头推断。
// 不启动 listener 或读取文件；注册、授权与 token 操作要求 store 已初始化，并在每次请求内读取最新状态。
func NewHandler(canonicalBase string, store Store) (*Handler, error) {
	base, err := ParseBaseURL(canonicalBase)
	if err != nil {
		return nil, err
	}
	if store.Path == "" {
		return nil, errors.New("MCP state path is required")
	}
	authorize := newAuthorizer(base, store)
	issuer := base.String()
	prefix := base.EscapedPath()
	resource := sdkauth.ProtectedResourceMetadataHandler(&oauthex.ProtectedResourceMetadata{
		Resource: issuer + "/mcp", AuthorizationServers: []string{issuer}, ScopesSupported: []string{"mcp"},
	})
	// RFC 8414 §3 / RFC 9728 §3：well-known 插入 host 与 path 之间。
	metadata := map[string]any{
		"authorization_response_iss_parameter_supported": true,
		"issuer":                                issuer,
		"authorization_endpoint":                issuer + "/oauth/authorize",
		"token_endpoint":                        issuer + "/oauth/token",
		"registration_endpoint":                 issuer + "/oauth/register",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"scopes_supported":                      []string{"mcp"},
	}
	routes := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status := readControlRequest(w, r); status != 0 {
			// 保留失败读取的截止时间并关闭连接，避免 net/http 在返回后无限排空请求体。
			r.Close = true
			w.Header().Set("Connection", "close")
			code := "invalid_request"
			if status == http.StatusInternalServerError {
				code = "server_error"
			}
			writeJSON(w, status, map[string]string{"error": code})
			return
		}
		switch r.URL.EscapedPath() {
		case "/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource" + prefix + "/mcp":
			resource.ServeHTTP(w, r)
		case "/.well-known/oauth-authorization-server" + prefix:
			if r.Method != http.MethodGet {
				w.Header().Set("Allow", "GET")
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(metadata)
		case prefix + "/oauth/authorize":
			authorize.serveHTTP(w, r)
		case prefix + "/oauth/token":
			authorize.token(w, r)
		case prefix + "/oauth/register":
			register(w, r, store)
		default:
			http.NotFound(w, r)
		}
	})
	return &Handler{routes: routes, store: store, resource: issuer + "/mcp", metadataURL: base.Scheme + "://" + base.Host + "/.well-known/oauth-protected-resource" + prefix + "/mcp"}, nil
}

// 仅缓冲最多 64 KiB 的 OAuth 控制消息；读取限时与请求头预算一致，不作用于 MCP/媒体。
func readControlRequest(w http.ResponseWriter, r *http.Request) int {
	controller := http.NewResponseController(w)
	if err := controller.SetReadDeadline(time.Now().Add(controlBodyReadTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return http.StatusInternalServerError
	}
	if len(r.URL.RawQuery) > maxOAuthBody {
		return http.StatusRequestURITooLong
	}
	if r.ContentLength > maxOAuthBody {
		return http.StatusRequestEntityTooLarge
	}
	defer r.Body.Close()
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxOAuthBody))
	if err != nil {
		var timeout net.Error
		var tooLarge *http.MaxBytesError
		switch {
		case errors.As(err, &timeout) && timeout.Timeout():
			return http.StatusRequestTimeout
		case errors.As(err, &tooLarge):
			return http.StatusRequestEntityTooLarge
		default:
			return http.StatusBadRequest
		}
	}
	if err := controller.SetReadDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return http.StatusInternalServerError
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return 0
}

func validHTTPAuthority(u *url.URL) bool {
	if u.Hostname() == "" || strings.HasSuffix(u.Host, ":") {
		return false
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		return err == nil && n > 0 && n <= 65535
	}
	return true
}

func register(w http.ResponseWriter, r *http.Request, store Store) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "invalid_request"})
		return
	}
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_client_metadata"})
		return
	}
	client, code := registrationClient(r.Body)
	if code != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": code})
		return
	}
	id, err := store.registerClient(r.Context(), client)
	if errors.Is(err, errOAuthCapacity) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "temporarily_unavailable"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_error"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id": id, "client_name": client.ClientName, "redirect_uris": client.RedirectURIs,
		"token_endpoint_auth_method": "none", "grant_types": []string{"authorization_code", "refresh_token"},
		"response_types": []string{"code"}, "scope": "mcp",
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func registrationClient(body io.Reader) (Client, string) {
	// 未使用的扩展 metadata 按 RFC 7591 忽略，不抓取 logo、JWKS 或 client URL。
	var input struct {
		ClientName   string          `json:"client_name"`
		RedirectURIs []string        `json:"redirect_uris"`
		Method       json.RawMessage `json:"token_endpoint_auth_method"`
		Grants       json.RawMessage `json:"grant_types"`
		Responses    json.RawMessage `json:"response_types"`
		Scope        json.RawMessage `json:"scope"`
	}
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&input); err != nil {
		return Client{}, "invalid_client_metadata"
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Client{}, "invalid_client_metadata"
	}
	if len(input.RedirectURIs) == 0 {
		return Client{}, "invalid_redirect_uri"
	}
	for _, redirect := range input.RedirectURIs {
		if !validRedirect(redirect) {
			return Client{}, "invalid_redirect_uri"
		}
	}
	for _, field := range []struct {
		raw  json.RawMessage
		want string
	}{{input.Method, "none"}, {input.Scope, "mcp"}} {
		if len(field.raw) == 0 {
			continue
		}
		var value string
		if json.Unmarshal(field.raw, &value) != nil || value != field.want {
			return Client{}, "invalid_client_metadata"
		}
	}
	for _, field := range []struct {
		raw     json.RawMessage
		allowed []string
	}{
		{input.Grants, []string{"authorization_code", "refresh_token"}},
		{input.Responses, []string{"code"}},
	} {
		if len(field.raw) == 0 {
			continue
		}
		var values []string
		if json.Unmarshal(field.raw, &values) != nil || len(values) == 0 {
			return Client{}, "invalid_client_metadata"
		}
		for _, value := range values {
			if !slices.Contains(field.allowed, value) {
				return Client{}, "invalid_client_metadata"
			}
		}
	}
	// 所有注册采用同一 public-client profile；响应明确返回实际采用的 metadata。
	return Client{ClientName: input.ClientName, RedirectURIs: input.RedirectURIs}, ""
}

func validRedirect(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.User != nil || strings.ContainsAny(raw, `#\`) ||
		strings.IndexFunc(raw, func(r rune) bool { return r <= 0x20 || r == 0x7f }) >= 0 {
		return false
	}
	switch u.Scheme {
	case "https":
		return u.Opaque == "" && validHTTPAuthority(u)
	case "http":
		return u.Opaque == "" && validHTTPAuthority(u) &&
			(strings.EqualFold(u.Hostname(), "localhost") || net.ParseIP(u.Hostname()).IsLoopback())
	default:
		// RFC 8252 §8.4 的 reverse-domain 私有 scheme 防止与执行性/通用 scheme 混用。
		return strings.Contains(u.Scheme, ".") && (u.Path != "" || u.Opaque != "" || u.Host != "")
	}
}

// RequireCanonical 将入站 Host/Origin 绑定到配置的部署地址，不信任转发头。
func (h *Handler) RequireCanonical(next http.Handler) http.Handler {
	// resource 只由已验证的 canonical base 构造。
	base, _ := url.Parse(h.resource)
	origin := browserOrigin(base.Scheme + "://" + base.Host)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origins := r.Header.Values("Origin")
		if browserOrigin(base.Scheme+"://"+r.Host) != origin || len(origins) > 1 ||
			(len(origins) == 1 && browserOrigin(origins[0]) != origin) {
			http.Error(w, "Forbidden: request does not match canonical MCP origin", http.StatusForbidden)
			return
		}
		// 标准库 trusted-origin 使用字符串比较；仅在同源校验通过后统一大小写/默认端口表示。
		if len(origins) == 1 && origins[0] != base.Scheme+"://"+base.Host {
			r = r.Clone(r.Context())
			r.Header.Set("Origin", base.Scheme+"://"+base.Host)
		}
		next.ServeHTTP(w, r)
	})
}
