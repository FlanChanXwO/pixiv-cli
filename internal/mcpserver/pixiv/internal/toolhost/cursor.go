package toolhost

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// CursorBinding 封装参数摘要与 wire codec，本身不携带身份或访问权限。
type CursorBinding struct {
	operation string
	query     string
}

// BindCursor 绑定清除 cursor 后的原始 typed input，供不同分页策略共用。
func BindCursor(operation string, input any) (CursorBinding, error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return CursorBinding{}, err
	}
	digest := sha256.Sum256(encoded)
	return CursorBinding{operation: operation, query: hex.EncodeToString(digest[:])}, nil
}

// EncodeSDK 保存实际选号，续页时复用同一账号；SDK 仍校验内部账号绑定。
func (b CursorBinding) EncodeSDK(next sdk.Cursor, userID int64) (string, error) {
	if next.IsZero() {
		return "", nil
	}
	envelope, err := json.Marshal(cursorEnvelope{Version: 1, Product: "pixiv-mcp", Operation: b.operation, Binding: 1, QueryHash: b.query, Identity: strconv.FormatInt(userID, 10), Payload: []byte(next.String())})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(envelope), nil
}

// DecodeSDK 返回新版 SDK cursor 及其生成账号；nil 表示开始一次池调度。
func (b CursorBinding) DecodeSDK(token *string) (sdk.Cursor, int64, error) {
	if token == nil {
		return sdk.Cursor{}, 0, nil
	}
	envelope, err := b.envelope(*token)
	if err != nil {
		return sdk.Cursor{}, 0, err
	}
	identity := envelope.Identity
	userID, err := strconv.ParseInt(identity, 10, 64)
	if err != nil || userID <= 0 {
		return sdk.Cursor{}, 0, &sdk.Error{Reason: sdk.InvalidArgument, Operation: b.operation, Message: "cursor account is invalid"}
	}
	payload := envelope.Payload
	cursor, err := sdk.ParseCursor(string(payload))
	return cursor, userID, err
}

// cursorEnvelope 保留 MCP 的查询与账号绑定格式；内部续页状态由 SDK 校验。
type cursorEnvelope struct {
	Version   int    `json:"v"`
	Product   string `json:"p"`
	Operation string `json:"o"`
	Binding   int    `json:"b"`
	QueryHash string `json:"q"`
	Identity  string `json:"id,omitempty"`
	Ephemeral bool   `json:"e,omitempty"`
	Instance  string `json:"i,omitempty"`
	Payload   []byte `json:"pl"`
}

func (b CursorBinding) envelope(token string) (cursorEnvelope, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return cursorEnvelope{}, &sdk.Error{Operation: b.operation, Reason: sdk.InvalidArgument, Message: "invalid cursor encoding", Err: err}
	}
	var envelope cursorEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return cursorEnvelope{}, &sdk.Error{Operation: b.operation, Reason: sdk.InvalidArgument, Message: "malformed cursor envelope"}
	}
	if envelope.Version != 1 || envelope.Product != "pixiv-mcp" || envelope.Operation != b.operation || envelope.Binding != 1 || envelope.QueryHash != b.query || len(envelope.Payload) == 0 {
		return cursorEnvelope{}, &sdk.Error{Operation: b.operation, Reason: sdk.InvalidArgument, Message: "cursor binding mismatch"}
	}
	return envelope, nil
}
