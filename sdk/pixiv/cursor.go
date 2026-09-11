package pixiv

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// cursorBindingVersion is the Pixiv cursor binding version. Bump it only when
// the continuation semantics change within the v1 major so that older cursors
// fail closed instead of being misinterpreted.
const cursorBindingVersion = 1

// 搜索新增批内 checkpoint 与账号绑定、recommended 改为多参数 continuation
// 回放，二者都只使各自旧 cursor 显式失效（fail-closed）。
func operationCursorBindingVersion(op string) int {
	switch op {
	case "SearchArtworks", "RecommendedArtworks", "RecommendedNovels":
		return 2
	}
	return cursorBindingVersion
}

// continuationEnvelope is the opaque payload embedded in a Pixiv cursor. It
// never contains tokens, cookies, signed URLs, search text, or local paths.
// Params 仅承载 recommended 这类上游以多参数表达续页的 endpoint 的结构化
// 续页参数（offset、bookmark 游标、viewed 下标数组等），不含 next_url 原文。
type continuationEnvelope struct {
	Key      string     `json:"k"`
	Value    int64      `json:"v"`
	Consumed int        `json:"s,omitempty"`
	Params   url.Values `json:"p,omitempty"`
}

// identityScopedOps are operations whose pagination state is tied to the
// current authenticated account. Their cursors bind the verified non-secret
// identity; when no identity could be verified the cursor is ephemeral and only
// valid for the same client instance.
var identityScopedOps = map[string]bool{
	"SearchArtworks":      true,
	"CurrentUser":         true,
	"FollowingArtworks":   true,
	"FollowingNovels":     true,
	"MyPixivArtworks":     true,
	"MyPixivNovels":       true,
	"MyPixivUsers":        true,
	"RecommendedArtworks": true,
	"RecommendedNovels":   true,
	"RecommendedUsers":    true,
	"RelatedArtworks":     true,
	"RelatedUsers":        true,
	"ArtworkRanking":      false,
	"UserFollowing":       true,
	"UserFollowers":       true,
	"UserBlockedUsers":    true,
}

// queryDigest returns a stable digest of the request's query parameters. The
// continuation parameter is excluded before hashing so that callers can repeat
// the original query when continuing pagination.
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

// buildCursor encodes the next-page continuation for an operation. When exists
// is false it returns the zero cursor. baseQuery is the request's query without
// the continuation parameter.
func (c *Client) buildCursor(op string, baseQuery url.Values, key string, value int64, exists bool) (sdk.Cursor, error) {
	if !exists {
		return sdk.Cursor{}, nil
	}
	return c.buildContinuationCursor(op, baseQuery, continuationEnvelope{Key: key, Value: value})
}

func (c *Client) buildContinuationCursor(op string, baseQuery url.Values, state continuationEnvelope) (sdk.Cursor, error) {
	payload, err := json.Marshal(state)
	if err != nil {
		return sdk.Cursor{}, newError(op, sdk.UpstreamError, "cannot encode cursor")
	}
	var opts []sdk.CursorOption
	if identityScopedOps[op] {
		if c.userID > 0 {
			opts = append(opts, sdk.WithCursorIdentity(strconv.FormatInt(c.userID, 10)))
		} else {
			if c.cursorInstance == "" {
				return sdk.Cursor{}, newError(op, sdk.LocalStateError, "cursor instance is not configured")
			}
			opts = append(opts, sdk.WithCursorEphemeralInstance(c.cursorInstance))
		}
	}
	return sdk.NewCursor(product, op, operationCursorBindingVersion(op), queryDigest(baseQuery), payload, opts...)
}

// continuationFromCursor decodes and validates a caller-provided cursor against
// the repeated base query. It returns the continuation key and value to append
// to the next request.
func (c *Client) continuationFromCursor(op string, baseQuery url.Values, cur sdk.Cursor) (key string, value int64, err error) {
	state, err := c.continuationState(op, baseQuery, cur)
	return state.Key, state.Value, err
}

func (c *Client) continuationState(op string, baseQuery url.Values, cur sdk.Cursor) (continuationEnvelope, error) {
	if err := sdk.ValidateCursor(cur, product, op, operationCursorBindingVersion(op), queryDigest(baseQuery)); err != nil {
		return continuationEnvelope{}, newError(op, sdk.InvalidCursor, "cursor does not match this operation and query")
	}
	if identityScopedOps[op] {
		if identity, ok := sdk.CursorIdentity(cur); ok {
			if strconv.FormatInt(c.userID, 10) != identity {
				return continuationEnvelope{}, newError(op, sdk.InvalidCursor, "cursor belongs to a different account")
			}
		} else if err := sdk.ValidateCursorInstance(cur, c.cursorInstance); err != nil {
			return continuationEnvelope{}, newError(op, sdk.InvalidCursor, "cursor belongs to a different client instance")
		}
	}
	payload, err := sdk.CursorPayload(cur)
	if err != nil {
		return continuationEnvelope{}, newError(op, sdk.InvalidCursor, "cursor payload is unavailable")
	}
	var envelope continuationEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil || envelope.Value < 0 || envelope.Consumed < 0 {
		return continuationEnvelope{}, newError(op, sdk.InvalidCursor, "cursor payload is malformed")
	}
	// 两种合法 payload 形态：单键 continuation（Key+Value）与 recommended
	// 家族的多参数集（Params）；二者都不允许为空。
	if envelope.Key == "" && len(envelope.Params) == 0 {
		return continuationEnvelope{}, newError(op, sdk.InvalidCursor, "cursor payload is malformed")
	}
	if envelope.Key != "" && len(envelope.Params) != 0 {
		return continuationEnvelope{}, newError(op, sdk.InvalidCursor, "cursor payload is malformed")
	}
	return envelope, nil
}

// continuationOffset decodes an offset-keyed cursor. A zero cursor means the
// first page and returns offset zero.
func (c *Client) continuationOffset(op string, baseQuery url.Values, cur sdk.Cursor) (int, error) {
	if cur.IsZero() {
		return 0, nil
	}
	key, value, err := c.continuationFromCursor(op, baseQuery, cur)
	if err != nil {
		return 0, err
	}
	if key != "offset" {
		return 0, newError(op, sdk.InvalidCursor, "cursor continuation kind mismatch")
	}
	return int(value), nil
}

// continuationPositiveOffset decodes an offset cursor whose continuation
// value must be positive. A zero cursor still represents the first page.
func (c *Client) continuationPositiveOffset(op string, baseQuery url.Values, cur sdk.Cursor) (int, error) {
	if cur.IsZero() {
		return 0, nil
	}
	state, err := c.continuationState(op, baseQuery, cur)
	if err != nil {
		return 0, err
	}
	if state.Key != "offset" {
		return 0, newError(op, sdk.InvalidCursor, "cursor continuation kind mismatch")
	}
	if state.Value <= 0 || int64(int(state.Value)) != state.Value {
		return 0, newError(op, sdk.InvalidCursor, "cursor continuation offset must be positive")
	}
	return int(state.Value), nil
}

// continuationPositiveValue decodes an explicit continuation value that must
// be positive. A zero cursor still represents the first page.
func (c *Client) continuationPositiveValue(op string, baseQuery url.Values, cur sdk.Cursor, expectedKey string) (int64, error) {
	if cur.IsZero() {
		return 0, nil
	}
	state, err := c.continuationState(op, baseQuery, cur)
	if err != nil {
		return 0, err
	}
	if state.Key != expectedKey {
		return 0, newError(op, sdk.InvalidCursor, "cursor continuation kind mismatch")
	}
	if state.Value <= 0 {
		return 0, newError(op, sdk.InvalidCursor, "cursor continuation value must be positive")
	}
	return state.Value, nil
}

// continuationOffsetExists is continuationOffset for operations whose adapter
// distinguishes "first page" from "continuation exists".
func (c *Client) continuationOffsetExists(op string, baseQuery url.Values, cur sdk.Cursor) (int, bool, error) {
	if cur.IsZero() {
		return 0, false, nil
	}
	offset, err := c.continuationOffset(op, baseQuery, cur)
	if err != nil {
		return 0, false, err
	}
	return offset, true, nil
}

// continuationParams 解码多参数 continuation（recommended 家族）。零 cursor
// 表示首页并返回 nil；payload 未携带 Params 时显式 InvalidCursor，不静默重启。
func (c *Client) continuationParams(op string, baseQuery url.Values, cur sdk.Cursor) (url.Values, error) {
	if cur.IsZero() {
		return nil, nil
	}
	state, err := c.continuationState(op, baseQuery, cur)
	if err != nil {
		return nil, err
	}
	if len(state.Params) == 0 {
		return nil, newError(op, sdk.InvalidCursor, "cursor continuation params are missing")
	}
	return state.Params, nil
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

func newCursorInstanceID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
