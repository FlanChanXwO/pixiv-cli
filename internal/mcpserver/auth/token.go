package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/storage/file/lock"
)

var errInvalidGrant = errors.New("invalid_grant")

func (a *authorizer) token(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "invalid_request"})
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/x-www-form-urlencoded" || r.URL.RawQuery != "" || r.URL.ForceQuery || r.Header.Get("Authorization") != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	for _, values := range form {
		if len(values) != 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
			return
		}
	}
	required := []string{"client_id", "resource"}
	switch form.Get("grant_type") {
	case "authorization_code":
		required = append(required, "code", "redirect_uri", "code_verifier")
	case "refresh_token":
		required = append(required, "refresh_token")
	case "":
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_grant_type"})
		return
	}
	for _, key := range required {
		if form.Get(key) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
			return
		}
	}
	pair, err := a.exchange(r, form)
	if err != nil {
		status, code := http.StatusInternalServerError, "server_error"
		if errors.Is(err, errInvalidGrant) {
			status, code = http.StatusBadRequest, "invalid_grant"
		}
		writeJSON(w, status, map[string]string{"error": code})
		return
	}
	writeJSON(w, http.StatusOK, pair)
}

func (a *authorizer) exchange(r *http.Request, form url.Values) (map[string]any, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var pair map[string]any
	codeHash := sha256.Sum256([]byte(form.Get("code")))
	// 与 reset/注册共用文件锁；重新读取 generation，不能提交锁外旧快照。
	err := lock.WithPrivateLock(r.Context(), a.store.Path, func() error {
		state, err := a.readState(r.Context())
		if err != nil {
			return err
		}
		if form.Get("grant_type") == "refresh_token" {
			pair, err = a.refresh(r, form, state)
			return err
		}
		code, exists := a.codes[codeHash]
		client, registered := state.Clients[form.Get("client_id")]
		request := code.Request
		verifier := form.Get("code_verifier")
		challenge := sha256.Sum256([]byte(verifier))
		if !exists || !registered || code.Generation != state.OwnerVerifier ||
			request.ClientID != form.Get("client_id") || request.RedirectURI != form.Get("redirect_uri") ||
			!slices.Contains(client.RedirectURIs, request.RedirectURI) || request.Resource != form.Get("resource") ||
			(form.Has("scope") && form.Get("scope") != request.Scope) || !validVerifier(verifier) ||
			base64.RawURLEncoding.EncodeToString(challenge[:]) != request.Challenge {
			return errInvalidGrant
		}
		access, err := randomAuthorizationValue()
		if err != nil {
			return err
		}
		refresh, err := randomAuthorizationValue()
		if err != nil {
			return err
		}
		id := tokenHash(refresh)
		if _, exists := state.Grants[id]; exists {
			return errors.New("grant collision")
		}
		state.Grants[id] = Grant{ClientID: request.ClientID, Resource: request.Resource, Scope: request.Scope, AccessTokens: map[string]time.Time{tokenHash(access): a.now().Add(time.Hour)}, RefreshHash: tokenHash(refresh)}
		if err := a.store.save(r.Context(), state); err != nil {
			return err
		}
		// 写盘失败保留 code，客户端可重试；只有提交成功才消费和发布 token。
		delete(a.codes, codeHash)
		pair = map[string]any{"access_token": access, "refresh_token": refresh, "token_type": "Bearer", "expires_in": 3600, "scope": request.Scope}
		return nil
	})
	return pair, err
}

func tokenHash(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// RFC 7636 §4.1：43–128 个 unreserved ASCII 字符，不接受任意字符串。
func validVerifier(value string) bool {
	if len(value) < 43 || len(value) > 128 {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '.' || c == '_' || c == '~') {
			return false
		}
	}
	return true
}

// refresh 在调用者持有文件锁时旋转；重放撤销也必须先成功写盘。
func (a *authorizer) refresh(r *http.Request, form url.Values, state State) (map[string]any, error) {
	if form.Get("resource") != a.issuer+"/mcp" {
		return nil, errInvalidGrant
	}
	hash := tokenHash(form.Get("refresh_token"))
	// ponytail: 单 owner 的 grant/重放历史线性扫描；规模增大时再加 hash 索引。
	for id, grant := range state.Grants {
		used := slices.Contains(grant.UsedRefreshHashes, hash)
		if grant.RefreshHash != hash && !used {
			continue
		}
		if grant.Revoked || grant.ClientID != form.Get("client_id") || grant.Resource != form.Get("resource") ||
			(form.Has("scope") && form.Get("scope") != grant.Scope) {
			return nil, errInvalidGrant
		}
		if used {
			grant.Revoked = true
			state.Grants[id] = grant
			if err := a.store.save(r.Context(), state); err != nil {
				return nil, err
			}
			return nil, errInvalidGrant
		}
		access, err := randomAuthorizationValue()
		if err != nil {
			return nil, err
		}
		refresh, err := randomAuthorizationValue()
		if err != nil {
			return nil, err
		}
		grant.UsedRefreshHashes = append(grant.UsedRefreshHashes, grant.RefreshHash)
		grant.RefreshHash = tokenHash(refresh)
		now := a.now()
		for hash, expiry := range grant.AccessTokens {
			if !now.Before(expiry) {
				delete(grant.AccessTokens, hash)
			}
		}
		grant.AccessTokens[tokenHash(access)] = now.Add(time.Hour)
		state.Grants[id] = grant
		if err := a.store.save(r.Context(), state); err != nil {
			return nil, err
		}
		return map[string]any{"access_token": access, "refresh_token": refresh, "token_type": "Bearer", "expires_in": 3600, "scope": grant.Scope}, nil
	}
	return nil, errInvalidGrant
}
