package auth

import (
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

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

// NewHandler 构造独立 discovery/DCR/authorize handler；canonicalBase 不从请求头推断。
// 不启动 listener 或读取文件；注册与授权要求 store 已初始化，并在每次请求内读取最新状态。
func NewHandler(canonicalBase string, store Store) (http.Handler, error) {
	base, err := url.Parse(strings.TrimRight(canonicalBase, "/"))
	if err != nil || base.User != nil || base.Opaque != "" ||
		(base.Scheme != "http" && base.Scheme != "https") || !validHTTPAuthority(base) ||
		base.RawQuery != "" || base.ForceQuery || strings.ContainsAny(canonicalBase, `#\`) {
		return nil, errors.New("canonical MCP base URL must be an absolute HTTP(S) URL without credentials, query or fragment")
	}
	// 客户端会归一化 dot segments；拒绝可能使 issuer 与 discovery 路径不一致的配置。
	for _, segment := range strings.Split(base.Path, "/") {
		if segment == "." || segment == ".." {
			return nil, errors.New("canonical MCP base URL path must not contain dot segments")
		}
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
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		case prefix + "/oauth/register":
			register(w, r, store)
		default:
			http.NotFound(w, r)
		}
	}), nil
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
		Client
		Method    json.RawMessage `json:"token_endpoint_auth_method"`
		Grants    json.RawMessage `json:"grant_types"`
		Responses json.RawMessage `json:"response_types"`
		Scope     json.RawMessage `json:"scope"`
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
	return input.Client, ""
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
