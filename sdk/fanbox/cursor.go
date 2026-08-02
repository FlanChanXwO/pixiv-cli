package fanbox

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// cursorBindingVersion is the FANBOX cursor binding version.
const cursorBindingVersion = 1

// continuationEnvelope is the opaque cursor payload. It stores the upstream
// pagination URL (which carries no token, cookie, or signature) so the adapter
// can fetch the next page.
type continuationEnvelope struct {
	NextURL string `json:"u"`
}

// queryDigest returns a stable digest of the base request query parameters.
func queryDigest(query url.Values) string {
	keys := make([]string, 0, len(query))
	for key := range query {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	for _, key := range keys {
		values := append([]string(nil), query[key]...)
		sort.Strings(values)
		for _, value := range values {
			builder.WriteString(url.QueryEscape(key))
			builder.WriteByte('=')
			builder.WriteString(url.QueryEscape(value))
			builder.WriteByte('&')
		}
	}
	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:])
}

func (c *Client) buildCursor(op string, baseQuery url.Values, nextURL string) (sdk.Cursor, error) {
	if nextURL == "" {
		return sdk.Cursor{}, nil
	}
	payload, err := json.Marshal(continuationEnvelope{NextURL: nextURL})
	if err != nil {
		return sdk.Cursor{}, newError(op, sdk.CodeUpstreamError, err)
	}
	return sdk.NewCursor(product, op, cursorBindingVersion, queryDigest(baseQuery), payload)
}

func (c *Client) continuationURL(op string, baseQuery url.Values, cur sdk.Cursor) (string, error) {
	if cur.IsZero() {
		return "", nil
	}
	if err := sdk.ValidateCursor(cur, product, op, cursorBindingVersion, queryDigest(baseQuery)); err != nil {
		return "", newError(op, sdk.CodeInvalidCursor, err)
	}
	payload, err := sdk.CursorPayload(cur)
	if err != nil {
		return "", newError(op, sdk.CodeInvalidCursor, err)
	}
	var envelope continuationEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil || envelope.NextURL == "" {
		return "", newError(op, sdk.CodeInvalidCursor, errors.New("cursor payload is malformed"))
	}
	return envelope.NextURL, nil
}
