package pixiv

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// Credentials carries the verified Pixiv identity and the tokens produced by
// Open or a completed login. The token values are only reachable through
// AccessToken and RefreshToken; every formatting and JSON path is redacted so
// credentials can be safely logged or embedded in debug output.
//
// Callers must persist the rotated refresh token before issuing content
// requests, because the Client returned by Open holds only the access token and
// never refreshes on its own.
type Credentials struct {
	UserID    int64
	Username  string
	ExpiresAt time.Time

	accessToken  string
	refreshToken string
}

// AccessToken returns the current access token.
func (c Credentials) AccessToken() string { return c.accessToken }

// RefreshToken returns the rotated refresh token, or the empty string when the
// credentials came from a New client and no refresh token is known.
func (c Credentials) RefreshToken() string { return c.refreshToken }

// String returns a redacted summary that never contains token values.
func (c Credentials) String() string {
	return fmt.Sprintf("pixiv.Credentials{UserID:%d Username:%q ExpiresAt:%s}",
		c.UserID, c.Username, c.ExpiresAt.Format(time.RFC3339))
}

// GoString returns the same redacted summary as String.
func (c Credentials) GoString() string { return c.String() }

// Format renders the redacted summary for any formatting verb; token values are
// never written.
func (c Credentials) Format(state fmt.State, verb rune) {
	_, _ = io.WriteString(state, c.String())
}

// MarshalJSON emits only the non-secret identity fields. Token values are
// never serialized.
func (c Credentials) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		UserID    int64     `json:"user_id"`
		Username  string    `json:"username"`
		ExpiresAt time.Time `json:"expires_at"`
	}{
		UserID:    c.UserID,
		Username:  c.Username,
		ExpiresAt: c.ExpiresAt,
	})
}
