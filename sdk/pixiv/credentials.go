package pixiv

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/credential"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api/oauth"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// Credentials carries one account's versioned OAuth state. Token fields redact
// themselves in formatting and slog, and JSON omits them. Read their Value only
// for private persistence or explicit credential transfer. Revision is positive.
// A store may omit access tokens on Load; the client caches them by revision.
type Credentials struct {
	UserID       int64 // Zero only for a refresh-only seed whose identity is not verified yet.
	DisplayName  string
	Revision     int64
	AccessToken  credential.Secret `json:"-"`
	RefreshToken credential.Secret `json:"-"`
	ExpiresAt    time.Time         // Zero means the upstream did not supply an access-token expiry.
}

// NewCredentials seeds revision 1 from an App API refresh token, without I/O.
// Credential stores validate the seed before use. LoginSession returns full
// credentials directly; persistent stores restore their saved identity/revision.
func NewCredentials(refreshToken string) Credentials {
	value := Credentials{Revision: 1}
	value.SetTokens("", refreshToken)
	return value
}

// SetTokens copies raw OAuth tokens into their redacting fields. Stores that
// restore access state also set UserID, Revision, and ExpiresAt on the snapshot.
// Validation happens when the snapshot enters a store or a client operation.
func (value *Credentials) SetTokens(accessToken, refreshToken string) {
	value.AccessToken = credential.New([]byte(strings.TrimSpace(accessToken)))
	value.RefreshToken = credential.New([]byte(strings.TrimSpace(refreshToken)))
}

// CredentialStore owns one account's storage. Each method releases all storage
// locks before returning; the SDK performs OAuth between these method calls.
// Preserve an established positive UserID across every replacement.
// Store errors must omit token values; the SDK preserves their causes.
type CredentialStore interface {
	// Load returns the latest revision and refresh token, optionally with access state.
	// A classified [sdk.Error] is preserved, allowing a store to reject an expired lease.
	Load(ctx context.Context) (Credentials, error)
	// CompareAndSwap atomically persists replacement only if expectedRevision
	// matches. Replacement.Revision is expectedRevision+1; return false, nil for
	// a conflict, or an error for a failed save. Report success after persistence.
	CompareAndSwap(ctx context.Context, expectedRevision int64, replacement Credentials) (bool, error)
}

// Credentials prepares usable credentials without a content request. It reads
// the latest saved revision and commits any refresh before returning, using the
// same credential gate and revision comparison as content operations.
func (c *Client) Credentials(ctx context.Context) (Credentials, error) {
	return c.credentials(ctx, Credentials{})
}

type memoryCredentials struct {
	mu    sync.Mutex
	value Credentials
}

// NewMemoryCredentials creates a concurrency-safe store with no file access.
// Use NewCredentials(refreshToken) as a seed, or pass credentials from login.
func NewMemoryCredentials(initial Credentials) (CredentialStore, error) {
	if err := validateCredentials(initial); err != nil {
		return nil, err
	}
	return &memoryCredentials{value: initial}, nil
}

// Format keeps stored tokens out of diagnostics, including %#v.
func (*memoryCredentials) Format(state fmt.State, _ rune) {
	fmt.Fprint(state, "pixiv.MemoryCredentials{}")
}

// Load returns an immutable credential snapshot; Secret values expose only copies.
func (m *memoryCredentials) Load(ctx context.Context) (Credentials, error) {
	if err := ctx.Err(); err != nil {
		return Credentials{}, newError("credentials.Load", sdk.LocalStateError, "credential read canceled", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.value, nil
}

// CompareAndSwap saves replacement only when the current revision matches.
func (m *memoryCredentials) CompareAndSwap(ctx context.Context, expected int64, replacement Credentials) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, newError("credentials.CompareAndSwap", sdk.LocalStateError, "credential save canceled", err)
	}
	if err := validateCredentials(replacement); err != nil {
		return false, err
	}
	if replacement.Revision != expected+1 {
		return false, newError("credentials.CompareAndSwap", sdk.InvalidArgument, "replacement must advance revision by one", nil)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.value.Revision != expected {
		return false, nil
	}
	if m.value.UserID > 0 && m.value.UserID != replacement.UserID {
		return false, newError("credentials.CompareAndSwap", sdk.LocalStateError, "credential identity changed", nil)
	}
	m.value = replacement
	return true, nil
}

func validateCredentials(value Credentials) error {
	if value.Revision <= 0 || value.UserID < 0 || (value.AccessToken.Empty() && value.RefreshToken.Empty()) || (!value.AccessToken.Empty() && value.UserID == 0) {
		return newError("credentials", sdk.InvalidArgument, "credentials require a positive revision, tokens, and an identity for access tokens", nil)
	}
	if !value.RefreshToken.Empty() && oauth.New(value.RefreshToken.Value(), oauth.Config{}).RefreshTokenValue() == "" {
		return newError("credentials", sdk.InvalidArgument, "provide an App API refresh token, not browser cookies", nil)
	}
	return nil
}

func (value Credentials) usable() bool {
	return !value.AccessToken.Empty() && (value.ExpiresAt.IsZero() || time.Now().Before(value.ExpiresAt))
}
