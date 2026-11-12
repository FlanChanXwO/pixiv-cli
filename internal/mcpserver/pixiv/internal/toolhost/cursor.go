package toolhost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"

	legacysdk "github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk"
	"github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/toolrun"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// CollectCursorWith 让 discovery 共用查询绑定与完整批次续读。bindingInput 必须是清掉
// cursor 后的原始 typed input，否则筛选字段会从绑定里丢失。外层 envelope 不授权，
// SDK 仍会校验内部 cursor 的 operation/query/账号绑定。
func CollectCursorWith[T any](ctx context.Context, app *App, operation string, bindingInput any, cursor *string, input toolrun.ListIn, fetch func(context.Context, *pixiv.Client, legacysdk.Cursor) ([]T, legacysdk.Cursor, error)) ([]T, PaginationOut, error) {
	plan, err := toolrun.ParseListPlan(input)
	if err != nil {
		return nil, PaginationOut{}, err
	}
	if cursor != nil && !plan.OneBatch {
		return nil, PaginationOut{}, errors.New("cursor requires omitted page and limit")
	}
	if !plan.OneBatch {
		items, more, err := CollectWith(ctx, app, plan, fetch)
		return items, ListPagination(plan, input.Limit, len(items), more), err
	}
	binding, err := BindCursor(operation, bindingInput)
	if err != nil {
		return nil, PaginationOut{}, err
	}
	initial, err := binding.Decode(cursor)
	if err != nil {
		return nil, PaginationOut{}, err
	}
	items, next, err := CollectBatchWith(ctx, app, initial, fetch)
	if err != nil {
		return nil, PaginationOut{}, err
	}
	page := ListPagination(plan, nil, len(items), !next.IsZero())
	page.NextCursor, err = binding.Encode(next)
	if err != nil {
		return nil, PaginationOut{}, err
	}
	return items, page, nil
}

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

// Decode 先校验外层 tool 与 query，再交给 SDK 校验内部 continuation，返回其中的
// SDK cursor；cursor 为 nil 时返回零值。
func (b CursorBinding) Decode(cursor *string) (legacysdk.Cursor, error) {
	if cursor == nil {
		return legacysdk.Cursor{}, nil
	}
	envelope, err := b.envelope(*cursor)
	if err != nil {
		return legacysdk.Cursor{}, err
	}
	payload, err := legacysdk.CursorPayload(envelope)
	if err != nil {
		return legacysdk.Cursor{}, err
	}
	return legacysdk.ParseCursor(string(payload))
}

// Encode 保留完整 SDK checkpoint；next 为零值时返回空串，表示没有下一页。
func (b CursorBinding) Encode(next pagination.Cursor) (string, error) {
	if next.IsZero() {
		return "", nil
	}
	envelope, err := legacysdk.NewCursor("pixiv-mcp", b.operation, 1, b.query, []byte(next.String()))
	if err != nil {
		return "", err
	}
	return envelope.String(), nil
}

// EncodeSDK 保存实际选号，续页时复用同一账号；SDK 仍校验内部账号绑定。
func (b CursorBinding) EncodeSDK(next sdk.Cursor, userID int64) (string, error) {
	if next.IsZero() {
		return "", nil
	}
	envelope, err := legacysdk.NewCursor("pixiv-mcp", b.operation, 1, b.query, []byte(next.String()), legacysdk.WithCursorIdentity(strconv.FormatInt(userID, 10)))
	if err != nil {
		return "", err
	}
	return envelope.String(), nil
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
	identity, _ := legacysdk.CursorIdentity(envelope)
	userID, err := strconv.ParseInt(identity, 10, 64)
	if err != nil || userID <= 0 {
		return sdk.Cursor{}, 0, &sdk.Error{Reason: sdk.InvalidArgument, Operation: b.operation, Message: "cursor account is invalid"}
	}
	payload, err := legacysdk.CursorPayload(envelope)
	if err != nil {
		return sdk.Cursor{}, 0, err
	}
	cursor, err := sdk.ParseCursor(string(payload))
	return cursor, userID, err
}

func (b CursorBinding) envelope(token string) (legacysdk.Cursor, error) {
	envelope, err := legacysdk.ParseCursor(token)
	if err != nil {
		return legacysdk.Cursor{}, err
	}
	if err := legacysdk.ValidateCursor(envelope, "pixiv-mcp", b.operation, 1, b.query); err != nil {
		return legacysdk.Cursor{}, err
	}
	return envelope, nil
}
