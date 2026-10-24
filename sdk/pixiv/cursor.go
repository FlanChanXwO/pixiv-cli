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

	"github.com/FlanChanXwO/pixiv-cli/internal/pixivapi"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// appAPIHost 是 continuation next_url 唯一接受的 host。
const appAPIHost = "app-api.pixiv.net"

// cursorBindingVersion is the Pixiv cursor binding version. Bump it only when
// the continuation semantics change within the v1 major so that older cursors
// fail closed instead of being misinterpreted.
const cursorBindingVersion = 1

// 搜索新增批内 checkpoint 与账号绑定，recommended/related 改为多参数
// continuation 回放；这些变更都只使对应 operation 的旧 cursor 显式失效。
func operationCursorBindingVersion(op string) int {
	switch op {
	case "SearchArtworks", "RecommendedArtworks", "RecommendedNovels", "RelatedArtworks":
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
			pixivapi.ParamOffset: true, pixivapi.ParamAlreadyRecommended: true,
			pixivapi.ParamMaxRecommend: true, pixivapi.ParamIncludeRankingNovels: true,
			pixivapi.ParamIncludePrivacy: true,
		}
		for key, values := range params {
			if !allowed[key] || len(values) != 1 || values[0] == "" {
				return false
			}
			switch key {
			case pixivapi.ParamOffset:
				value, err := strconv.ParseInt(values[0], 10, 64)
				if err != nil || value < 0 {
					return false
				}
			case pixivapi.ParamMaxRecommend:
				value, err := strconv.ParseInt(values[0], 10, 64)
				if err != nil || value <= 0 {
					return false
				}
			case pixivapi.ParamIncludeRankingNovels, pixivapi.ParamIncludePrivacy:
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

// continuationSpec 描述单条 artwork endpoint 的 next_url 白名单。
type continuationSpec struct {
	path      string
	keys      []string
	allowZero bool
	query     []string
	prefixes  []string
	ignored   []string
}

// parseContinuation 校验 next_url 并提取唯一的 typed continuation 值。
// next_url 必须指向固定的 App API host 与当前端点 path；query 只允许出现
// 已登记的键，每个键一次，未知键或越界值一律 malformed。
func parseContinuation(rawURL string, spec continuationSpec) (string, int64, error) {
	if rawURL == "" || spec.path == "" || len(spec.keys) == 0 {
		return "", 0, pixivapi.MalformedResponse()
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Fragment != "" || parsed.Opaque != "" ||
		parsed.Hostname() != appAPIHost || parsed.Port() != "" || parsed.Path != spec.path || parsed.EscapedPath() != spec.path {
		return "", 0, pixivapi.MalformedResponse()
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return "", 0, pixivapi.MalformedResponse()
	}
	allowed := make(map[string]struct{}, len(spec.query)+len(spec.keys))
	for _, key := range spec.keys {
		if key == "" {
			return "", 0, pixivapi.MalformedResponse()
		}
		allowed[key] = struct{}{}
	}
	for _, key := range spec.query {
		allowed[key] = struct{}{}
	}
	for key, entries := range values {
		if _, ok := allowed[key]; !ok || len(entries) != 1 {
			return "", 0, pixivapi.MalformedResponse()
		}
	}
	continuationKey := ""
	for _, key := range spec.keys {
		if _, ok := values[key]; !ok {
			continue
		}
		if continuationKey != "" {
			return "", 0, pixivapi.MalformedResponse()
		}
		continuationKey = key
	}
	if continuationKey == "" {
		return "", 0, pixivapi.MalformedResponse()
	}
	value, err := strconv.ParseInt(values[continuationKey][0], 10, 64)
	if err != nil || value < 0 || (!spec.allowZero && value == 0) {
		return "", 0, pixivapi.MalformedResponse()
	}
	return continuationKey, value, nil
}

// parseContinuationParams 校验 next_url 并返回完整查询集，供 recommended
// 这类“上游用多参数表达续页”的端点整体回放；rawURL 为空表示没有下一页。
func parseContinuationParams(rawURL string, spec continuationSpec) (url.Values, bool, error) {
	if rawURL == "" {
		return nil, false, nil
	}
	if spec.path == "" {
		return nil, false, pixivapi.MalformedResponse()
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Fragment != "" || parsed.Opaque != "" ||
		parsed.Hostname() != appAPIHost || parsed.Port() != "" || parsed.Path != spec.path || parsed.EscapedPath() != spec.path {
		return nil, false, pixivapi.MalformedResponse()
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return nil, false, pixivapi.MalformedResponse()
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
		return nil, false, pixivapi.MalformedResponse()
	}
	return values, true, nil
}

// validateContinuationParams 校验已结构化的 continuation 参数集：未知、空值
// 或重复的标量参数 fail closed，不交给传输层。
func validateContinuationParams(values url.Values, spec continuationSpec) error {
	if len(values) == 0 {
		return pixivapi.MalformedResponse()
	}
	allowed := make(map[string]struct{}, len(spec.query)+len(spec.keys))
	for _, key := range spec.keys {
		if key == "" {
			return pixivapi.MalformedResponse()
		}
		allowed[key] = struct{}{}
	}
	for _, key := range spec.query {
		if key == "" {
			return pixivapi.MalformedResponse()
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
				return pixivapi.MalformedResponse()
			}
		}
		if len(entries) != 1 || entries[0] == "" {
			return pixivapi.MalformedResponse()
		}
	}
	return nil
}

// next 读取单一 continuation 的 typed value；rawURL 为空表示没有下一页。
// allowZero 的 Spec 接受 0，其余 Spec 由 parseContinuation 拒绝 0。
func (s continuationSpec) next(rawURL *string) (int64, bool, error) {
	_, value, ok, err := s.parseValue(rawURL)
	return value, ok, err
}

// parseValue 校验 next_url 的 path、query 与 continuation 值。offset 必须能
// 无损转换为 int，与公开游标的 offset 语义保持一致。
func (s continuationSpec) parseValue(rawURL *string) (string, int64, bool, error) {
	if rawURL == nil {
		return "", 0, false, nil
	}
	key, value, err := parseContinuation(*rawURL, s)
	if err != nil {
		return "", 0, false, err
	}
	if key == pixivapi.ParamOffset && int64(int(value)) != value {
		return "", 0, false, pixivapi.MalformedResponse()
	}
	return key, value, true, nil
}

// offset 解析只允许单一 offset 的续页；offset 必须是正 int。
func (s continuationSpec) offset(rawURL *string) (int, bool, error) {
	value, hasNext, err := s.value(rawURL)
	if err != nil || !hasNext {
		return 0, hasNext, err
	}
	if value <= 0 || int64(int(value)) != value {
		return 0, false, pixivapi.MalformedResponse()
	}
	return int(value), true, nil
}

// keyValue 返回单一 continuation 的 key 与值，供 series 与 latest 这类允许
// 两种续页形式的端点构建 cursor。
func (s continuationSpec) keyValue(rawURL *string) (string, int64, bool, error) {
	key, value, ok, err := s.parseValue(rawURL)
	if err != nil || !ok {
		return "", 0, ok, err
	}
	if value <= 0 {
		return "", 0, false, pixivapi.MalformedResponse()
	}
	return key, value, true, nil
}

// value 解析单一 typed continuation key，并拒绝缺失与重复。
func (s continuationSpec) value(rawURL *string) (int64, bool, error) {
	_, value, ok, err := s.parseValue(rawURL)
	if err != nil || !ok {
		return 0, ok, err
	}
	if value <= 0 {
		return 0, false, pixivapi.MalformedResponse()
	}
	return value, true, nil
}

// params 整体回放多参数续页，返回上游 next_url 的完整查询集。
func (s continuationSpec) params(rawURL *string) (url.Values, bool, error) {
	if rawURL == nil {
		return nil, false, nil
	}
	values, hasNext, err := parseContinuationParams(*rawURL, s)
	if err != nil || !hasNext {
		return nil, false, pixivapi.MalformedResponse()
	}
	return values, true, nil
}

// continuationKeyValue 解码多 key 续页游标；零游标表示首页。
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
			if key == pixivapi.ParamOffset && int64(int(value)) != value {
				return "", 0, newError(operation, sdk.InvalidCursor, "cursor continuation offset is out of range")
			}
			return key, value, nil
		}
	}
	return "", 0, newError(operation, sdk.InvalidCursor, "cursor continuation kind mismatch")
}

// continuationValue 读取单一 key 的续页游标；零游标表示首页。
func (c *Client) continuationValue(operation string, base url.Values, cur sdk.Cursor, spec continuationSpec) (string, int64, error) {
	if cur.IsZero() {
		return spec.keys[0], 0, nil
	}
	return c.continuationKeyValue(operation, base, cur, spec)
}

func cloneValues(values url.Values) url.Values {
	if values == nil {
		return url.Values{}
	}
	cloned := make(url.Values, len(values))
	for key, entries := range values {
		cloned[key] = append([]string(nil), entries...)
	}
	return cloned
}
