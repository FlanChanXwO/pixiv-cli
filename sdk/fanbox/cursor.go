package fanbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
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

// identityScopedOps are FANBOX operations whose pagination state is tied to the
// authenticated FANBOX account: Home, Supporting, and the supporting/following
// creator lists all return pages scoped to the current session's user. Their
// cursors bind the verified non-secret FANBOX user id so a cursor minted under
// one account cannot be replayed against another account's feed.
var identityScopedOps = map[string]bool{
	"Home":         true,
	"Supporting":   true,
	"Creators":     true,
	"CreatorPosts": false,
	"TaggedPosts":  false,
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

func (c *Client) buildCursor(ctx context.Context, op string, baseQuery url.Values, nextURL string) (sdk.Cursor, error) {
	if nextURL == "" {
		return sdk.Cursor{}, nil
	}
	payload, err := json.Marshal(continuationEnvelope{NextURL: nextURL})
	if err != nil {
		return sdk.Cursor{}, newError(op, sdk.UpstreamError, err)
	}
	var opts []sdk.CursorOption
	if identityScopedOps[op] {
		userID, idErr := c.verifiedUserID(ctx)
		if idErr != nil {
			return sdk.Cursor{}, idErr
		}
		opts = append(opts, sdk.WithCursorIdentity(strconv.FormatInt(userID, 10)))
	}
	return sdk.NewCursor(product, op, cursorBindingVersion, queryDigest(baseQuery), payload, opts...)
}

func (c *Client) continuationURL(ctx context.Context, op string, baseQuery url.Values, cur sdk.Cursor) (string, error) {
	if cur.IsZero() {
		return "", nil
	}
	if err := sdk.ValidateCursor(cur, product, op, cursorBindingVersion, queryDigest(baseQuery)); err != nil {
		return "", newError(op, sdk.InvalidCursor, err)
	}
	if identityScopedOps[op] {
		if identity, ok := sdk.CursorIdentity(cur); ok {
			userID, idErr := c.verifiedUserID(ctx)
			if idErr != nil {
				return "", idErr
			}
			if strconv.FormatInt(userID, 10) != identity {
				return "", newError(op, sdk.InvalidCursor, errors.New("cursor belongs to a different account"))
			}
		} else {
			// An identity-scoped cursor without an identity binding cannot have
			// been produced by this client; fail closed rather than mixing feeds.
			return "", newError(op, sdk.InvalidCursor, errors.New("cursor is not bound to an account"))
		}
	}
	payload, err := sdk.CursorPayload(cur)
	if err != nil {
		return "", newError(op, sdk.InvalidCursor, err)
	}
	var envelope continuationEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil || envelope.NextURL == "" {
		return "", newError(op, sdk.InvalidCursor, errors.New("cursor payload is malformed"))
	}
	return envelope.NextURL, nil
}

// verifiedUserID returns the authenticated FANBOX user id, verifying it once
// per client and caching the result. The user id is non-secret and may be bound
// into a cursor; the session cookie never leaves the protocol layer. A verified
// id is required for identity-scoped operations so their cursors cannot be
// replayed across accounts.
func (c *Client) verifiedUserID(ctx context.Context) (int64, error) {
	c.identityMu.Lock()
	defer c.identityMu.Unlock()
	if c.userID > 0 {
		return c.userID, nil
	}
	identity, err := c.session.CurrentUser(ctx)
	if err != nil {
		return 0, classifyError("CurrentUser", err)
	}
	if identity.UserID <= 0 {
		return 0, newError("CurrentUser", sdk.MalformedUpstreamResponse, errors.New("FANBOX identity has no valid user id"))
	}
	c.userID = identity.UserID
	return c.userID, nil
}
