package toolhost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/toolrun"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// CollectCursorWith 让 discovery 共用查询绑定与完整批次续读。bindingInput 必须是清掉
// cursor 后的原始 typed input，否则筛选字段会从绑定里丢失。外层 envelope 不授权，
// SDK 仍会校验内部 cursor 的 operation/query/账号绑定。
func CollectCursorWith[T any](ctx context.Context, app *App, operation string, bindingInput any, cursor *string, input toolrun.ListIn, fetch func(context.Context, *pixiv.Client, sdk.Cursor) ([]T, sdk.Cursor, error)) ([]T, PaginationOut, error) {
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
func (b CursorBinding) Decode(cursor *string) (sdk.Cursor, error) {
	if cursor == nil {
		return sdk.Cursor{}, nil
	}
	envelope, err := sdk.ParseCursor(*cursor)
	if err != nil {
		return sdk.Cursor{}, err
	}
	if err := sdk.ValidateCursor(envelope, "pixiv-mcp", b.operation, 1, b.query); err != nil {
		return sdk.Cursor{}, err
	}
	payload, err := sdk.CursorPayload(envelope)
	if err != nil {
		return sdk.Cursor{}, err
	}
	return sdk.ParseCursor(string(payload))
}

// Encode 保留完整 SDK checkpoint；next 为零值时返回空串，表示没有下一页。
func (b CursorBinding) Encode(next sdk.Cursor) (string, error) {
	if next.IsZero() {
		return "", nil
	}
	envelope, err := sdk.NewCursor("pixiv-mcp", b.operation, 1, b.query, []byte(next.String()))
	if err != nil {
		return "", err
	}
	return envelope.String(), nil
}
