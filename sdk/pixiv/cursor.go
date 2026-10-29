package pixiv

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// appAPIHost is the only host accepted in a continuation next_url.
const appAPIHost = "app-api.pixiv.net"

// cursorBindingVersion is the Pixiv cursor binding version. Bump it only when
// the continuation semantics change within the v1 major so that older cursors
// fail closed instead of being misinterpreted.
const cursorBindingVersion = 1

// Search gained in-batch checkpoints and account binding, while recommended and
// related now replay multi-parameter continuations. Each change retires only the
// cursors of the operations it affects, and they fail with InvalidCursor rather
// than restarting silently.
func operationCursorBindingVersion(op string) int {
	switch op {
	case "SearchArtworks", "RecommendedArtworks", "RecommendedNovels", "RelatedArtworks":
		return 2
	}
	return cursorBindingVersion
}

// continuationEnvelope is the opaque payload carried by a Pixiv cursor. It never
// contains tokens, cookies, signed URLs, search text, or local paths. Params
// holds the structured continuation parameters of endpoints such as
// recommended, which express the next page as offset, bookmark cursor, and
// viewed index arrays; the raw next_url is never stored.
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
	return sdk.NewCursor(product, op, operationCursorBindingVersion(op), sdk.CursorQueryHash(baseQuery), payload, opts...)
}

// continuationFromCursor decodes and validates a caller-provided cursor against
// the repeated base query. It returns the continuation key and value to append
// to the next request.
func (c *Client) continuationFromCursor(op string, baseQuery url.Values, cur sdk.Cursor) (key string, value int64, err error) {
	state, err := c.continuationState(op, baseQuery, cur)
	return state.Key, state.Value, err
}

func (c *Client) continuationState(op string, baseQuery url.Values, cur sdk.Cursor) (continuationEnvelope, error) {
	if err := sdk.ValidateCursor(cur, product, op, operationCursorBindingVersion(op), sdk.CursorQueryHash(baseQuery)); err != nil {
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
	// A payload is either a single-key continuation (Key+Value) or a
	// multi-parameter set (Params) for the recommended family; neither may be
	// empty.
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

// continuationParams decodes a multi-parameter continuation from the
// recommended family. A zero cursor means the first page and returns nil; a
// payload without Params returns InvalidCursor instead of restarting silently.
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
	if !validContinuationParams(op, state.Params) {
		return nil, newError(op, sdk.InvalidCursor, "cursor continuation params are malformed")
	}
	return state.Params, nil
}

func validContinuationParams(op string, params url.Values) bool {
	if len(params) == 0 {
		return false
	}
	switch op {
	case "RecommendedArtworks":
		allowed := map[string]bool{
			"offset": true, "min_bookmark_id_for_recent_illust": true,
			"max_bookmark_id_for_recommend": true, "include_ranking_illusts": true,
			"include_privacy_policy": true,
		}
		for key, values := range params {
			if !allowed[key] || len(values) != 1 || values[0] == "" {
				return false
			}
			switch key {
			case "offset":
				value, err := strconv.ParseInt(values[0], 10, 64)
				if err != nil || value < 0 {
					return false
				}
			case "min_bookmark_id_for_recent_illust", "max_bookmark_id_for_recommend":
				value, err := strconv.ParseInt(values[0], 10, 64)
				if err != nil || value <= 0 {
					return false
				}
			case "include_ranking_illusts", "include_privacy_policy":
				if values[0] != "true" && values[0] != "false" {
					return false
				}
			}
		}
		return true
	case "RecommendedNovels":
		allowed := map[string]bool{
			api.ParamOffset: true, api.ParamAlreadyRecommended: true,
			api.ParamMaxRecommend: true, api.ParamIncludeRankingNovels: true,
			api.ParamIncludePrivacy: true,
		}
		for key, values := range params {
			if !allowed[key] || len(values) != 1 || values[0] == "" {
				return false
			}
			switch key {
			case api.ParamOffset:
				value, err := strconv.ParseInt(values[0], 10, 64)
				if err != nil || value < 0 {
					return false
				}
			case api.ParamMaxRecommend:
				value, err := strconv.ParseInt(values[0], 10, 64)
				if err != nil || value <= 0 {
					return false
				}
			case api.ParamIncludeRankingNovels, api.ParamIncludePrivacy:
				if values[0] != "true" && values[0] != "false" {
					return false
				}
			}
		}
		return true
	case "RelatedArtworks":
		if len(params["illust_id"]) != 1 || !validPositiveCursorValues(params["illust_id"]) {
			return false
		}
		if offset, hasOffset := params["offset"]; hasOffset {
			return len(params) == 2 && len(offset) == 1 && validPositiveCursorValues(offset)
		}
		return len(params) == 3 && validPositiveCursorValues(params["seed_illust_ids[]"]) && validPositiveCursorValues(params["viewed[]"])
	default:
		return false
	}
}

func validPositiveCursorValues(values []string) bool {
	if len(values) == 0 {
		return false
	}
	for _, raw := range values {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value <= 0 {
			return false
		}
	}
	return true
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

// continuationSpec lists the next_url query keys one endpoint accepts and how
// to read its continuation values.
type continuationSpec struct {
	path      string
	keys      []string
	allowZero bool
	query     []string
	prefixes  []string
	ignored   []string
}

// parseContinuation validates next_url and extracts its single typed
// continuation value. The URL must point at the fixed App API host and the
// endpoint's path, each query key must be registered and appear once, and any
// unknown key or out-of-range value is rejected as malformed.
func parseContinuation(rawURL string, spec continuationSpec) (string, int64, error) {
	if rawURL == "" || spec.path == "" || len(spec.keys) == 0 {
		return "", 0, api.MalformedResponse()
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Fragment != "" || parsed.Opaque != "" ||
		parsed.Hostname() != appAPIHost || parsed.Port() != "" || parsed.Path != spec.path || parsed.EscapedPath() != spec.path {
		return "", 0, api.MalformedResponse()
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return "", 0, api.MalformedResponse()
	}
	allowed := make(map[string]struct{}, len(spec.query)+len(spec.keys))
	for _, key := range spec.keys {
		if key == "" {
			return "", 0, api.MalformedResponse()
		}
		allowed[key] = struct{}{}
	}
	for _, key := range spec.query {
		allowed[key] = struct{}{}
	}
	for key, entries := range values {
		if _, ok := allowed[key]; !ok || len(entries) != 1 {
			return "", 0, api.MalformedResponse()
		}
	}
	continuationKey := ""
	for _, key := range spec.keys {
		if _, ok := values[key]; !ok {
			continue
		}
		if continuationKey != "" {
			return "", 0, api.MalformedResponse()
		}
		continuationKey = key
	}
	if continuationKey == "" {
		return "", 0, api.MalformedResponse()
	}
	value, err := strconv.ParseInt(values[continuationKey][0], 10, 64)
	if err != nil || value < 0 || (!spec.allowZero && value == 0) {
		return "", 0, api.MalformedResponse()
	}
	return continuationKey, value, nil
}

// parseContinuationParams validates next_url and returns its full query set so
// endpoints such as recommended can replay it as a unit. An empty rawURL means
// there is no next page.
func parseContinuationParams(rawURL string, spec continuationSpec) (url.Values, bool, error) {
	if rawURL == "" {
		return nil, false, nil
	}
	if spec.path == "" {
		return nil, false, api.MalformedResponse()
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Fragment != "" || parsed.Opaque != "" ||
		parsed.Hostname() != appAPIHost || parsed.Port() != "" || parsed.Path != spec.path || parsed.EscapedPath() != spec.path {
		return nil, false, api.MalformedResponse()
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return nil, false, api.MalformedResponse()
	}
	for key := range values {
		for _, prefix := range spec.ignored {
			if prefix != "" && strings.HasPrefix(key, prefix) {
				delete(values, key)
				break
			}
		}
	}
	if err := validateContinuationParams(values, spec); err != nil {
		return nil, false, api.MalformedResponse()
	}
	return values, true, nil
}

// validateContinuationParams checks an already structured parameter set.
// Unknown keys, empty values, and repeated scalar values fail closed before the
// request reaches the transport.
func validateContinuationParams(values url.Values, spec continuationSpec) error {
	if len(values) == 0 {
		return api.MalformedResponse()
	}
	allowed := make(map[string]struct{}, len(spec.query)+len(spec.keys))
	for _, key := range spec.keys {
		if key == "" {
			return api.MalformedResponse()
		}
		allowed[key] = struct{}{}
	}
	for _, key := range spec.query {
		if key == "" {
			return api.MalformedResponse()
		}
		allowed[key] = struct{}{}
	}
	for key, entries := range values {
		if _, ok := allowed[key]; !ok {
			matched := false
			for _, prefix := range spec.prefixes {
				if prefix != "" && strings.HasPrefix(key, prefix) {
					matched = true
					break
				}
			}
			if !matched {
				return api.MalformedResponse()
			}
		}
		if len(entries) != 1 || entries[0] == "" {
			return api.MalformedResponse()
		}
	}
	return nil
}

// next reads the typed value of a single continuation. A nil rawURL means there
// is no next page; a spec with allowZero accepts zero, and every other spec
// rejects it in parseContinuation.
func (s continuationSpec) next(rawURL *string) (int64, bool, error) {
	_, value, ok, err := s.parseValue(rawURL)
	return value, ok, err
}

// parseValue validates the next_url path, query, and continuation value. An
// offset must convert to int without loss so it matches the public cursor's
// offset semantics.
func (s continuationSpec) parseValue(rawURL *string) (string, int64, bool, error) {
	if rawURL == nil {
		return "", 0, false, nil
	}
	key, value, err := parseContinuation(*rawURL, s)
	if err != nil {
		return "", 0, false, err
	}
	if key == api.ParamOffset && int64(int(value)) != value {
		return "", 0, false, api.MalformedResponse()
	}
	return key, value, true, nil
}

// offset accepts only a single offset continuation, and the offset must be a
// positive int.
func (s continuationSpec) offset(rawURL *string) (int, bool, error) {
	value, hasNext, err := s.value(rawURL)
	if err != nil || !hasNext {
		return 0, hasNext, err
	}
	if value <= 0 || int64(int(value)) != value {
		return 0, false, api.MalformedResponse()
	}
	return int(value), true, nil
}

// keyValue returns the key and value of a single continuation for endpoints
// such as series and latest, which accept either continuation form.
func (s continuationSpec) keyValue(rawURL *string) (string, int64, bool, error) {
	key, value, ok, err := s.parseValue(rawURL)
	if err != nil || !ok {
		return "", 0, ok, err
	}
	if value <= 0 {
		return "", 0, false, api.MalformedResponse()
	}
	return key, value, true, nil
}

// value resolves a single typed continuation key and rejects missing or
// repeated keys.
func (s continuationSpec) value(rawURL *string) (int64, bool, error) {
	_, value, ok, err := s.parseValue(rawURL)
	if err != nil || !ok {
		return 0, ok, err
	}
	if value <= 0 {
		return 0, false, api.MalformedResponse()
	}
	return value, true, nil
}

// params replays a multi-parameter next page, returning the complete query set
// from the upstream next_url.
func (s continuationSpec) params(rawURL *string) (url.Values, bool, error) {
	if rawURL == nil {
		return nil, false, nil
	}
	values, hasNext, err := parseContinuationParams(*rawURL, s)
	if err != nil || !hasNext {
		return nil, false, api.MalformedResponse()
	}
	return values, true, nil
}

// continuationKeyValue decodes a multi-key continuation cursor; the zero cursor
// means the first page.
func (c *Client) continuationKeyValue(operation string, base url.Values, cur sdk.Cursor, spec continuationSpec) (string, int64, error) {
	if cur.IsZero() {
		return spec.keys[0], 0, nil
	}
	key, value, err := c.continuationFromCursor(operation, base, cur)
	if err != nil {
		return "", 0, err
	}
	for _, allowed := range spec.keys {
		if key == allowed {
			if value <= 0 {
				return "", 0, newError(operation, sdk.InvalidCursor, "cursor continuation value must be positive")
			}
			if key == api.ParamOffset && int64(int(value)) != value {
				return "", 0, newError(operation, sdk.InvalidCursor, "cursor continuation offset is out of range")
			}
			return key, value, nil
		}
	}
	return "", 0, newError(operation, sdk.InvalidCursor, "cursor continuation kind mismatch")
}

func cloneValues(values url.Values) url.Values {
	cloned := make(url.Values, len(values))
	for key, entries := range values {
		cloned[key] = append([]string(nil), entries...)
	}
	return cloned
}
