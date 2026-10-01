package auth

import (
	"context"
	"errors"
	"net/http"
	"time"

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
)

// Handler 同时持有 OAuth 路由及 canonical resource 的 bearer 边界，不管理 listener。
type Handler struct {
	routes                http.Handler
	store                 Store
	resource, metadataURL string
}

// ServeHTTP 处理 discovery、注册、授权与 token 请求。
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.routes.ServeHTTP(w, r) }

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
	for _, grant := range state.Grants {
		expiry, exists := grant.AccessTokens[hash]
		if exists && !grant.Revoked && grant.Resource == h.resource && time.Now().Before(expiry) {
			return &sdkauth.TokenInfo{Scopes: []string{grant.Scope}, Expiration: expiry, UserID: "owner"}, nil
		}
	}
	return nil, sdkauth.ErrInvalidToken
}
