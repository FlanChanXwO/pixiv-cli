// FANBOX 分页读取与游标绑定：共用首尾页请求以及会话、参数作用域校验。

package fanbox

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/fanboxapi"
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

func (c *Client) buildCursor(ctx context.Context, op string, baseQuery url.Values, nextURL string) (sdk.Cursor, error) {
	if nextURL == "" {
		return sdk.Cursor{}, nil
	}
	if err := fanboxapi.ValidateAPIURL(nextURL); err != nil {
		return sdk.Cursor{}, newError(op, sdk.MalformedUpstreamResponse, errors.New("FANBOX continuation URL is invalid"))
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
	return sdk.NewCursor(product, op, cursorBindingVersion, sdk.CursorQueryHash(baseQuery), payload, opts...)
}

func (c *Client) continuationURL(ctx context.Context, op string, baseQuery url.Values, cur sdk.Cursor) (string, error) {
	if cur.IsZero() {
		return "", nil
	}
	if err := sdk.ValidateCursor(cur, product, op, cursorBindingVersion, sdk.CursorQueryHash(baseQuery)); err != nil {
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
	if err := fanboxapi.ValidateAPIURL(envelope.NextURL); err != nil {
		return "", newError(op, sdk.InvalidCursor, errors.New("cursor continuation URL is invalid"))
	}
	return envelope.NextURL, nil
}

// verifiedUserID returns the authenticated FANBOX user id, verifying it once
// per client. A verified id is required for identity-scoped operations so their
// cursors cannot be replayed across accounts.
func (c *Client) verifiedUserID(ctx context.Context) (int64, error) {
	userID, err := c.app.VerifiedUserID(ctx)
	if err != nil {
		return 0, classifyError("CurrentUser", err)
	}
	return userID, nil
}

// getListBody fetches one list page body, reusing a server continuation when
// the caller supplied one and sending query on the first page otherwise.
func (c *Client) getListBody(ctx context.Context, path string, query url.Values, nextURL string, body *json.RawMessage, operation string) error {
	var response struct {
		Body json.RawMessage `json:"body"`
	}
	var err error
	if nextURL != "" {
		err = c.app.GetJSONURL(ctx, nextURL, &response)
	} else {
		err = c.app.GetJSON(ctx, path, query, &response)
	}
	if err != nil {
		return classifyError(operation, err)
	}
	*body = response.Body
	return nil
}

// firstContinuation prefers the server pageUrls entry over nextUrl.
func firstContinuation(pageURLs []string, nextURL string) string {
	if len(pageURLs) > 0 {
		return pageURLs[0]
	}
	return nextURL
}
