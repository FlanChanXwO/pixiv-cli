package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// CollectCursorWith 为 discovery 复用查询绑定与完整批次续读；bindingInput
// 必须是清除 cursor 后的原始 typed input，不能丢失 owner 的筛选字段。
// envelope 不提供授权；SDK 仍验证内部 cursor 的 operation/query/账号绑定。
func CollectCursorWith[T any](ctx context.Context, app *App, operation string, bindingInput any, cursor *string, input PageLimitIn, fetch func(context.Context, *pixiv.Client, sdk.Cursor) ([]T, sdk.Cursor, error)) ([]T, PaginationOut, error) {
	plan, err := ParseListPlan(input)
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

// CursorBinding 只封装参数摘要与 wire codec，不提供身份或访问权限。
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

// Decode 校验外层 tool/query，再交由 SDK 验证内部 continuation。
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

// Encode 保留完整 SDK checkpoint；耗尽时不生成 wire cursor。
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
