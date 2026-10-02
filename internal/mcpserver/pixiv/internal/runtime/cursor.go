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
	encoded, err := json.Marshal(bindingInput)
	if err != nil {
		return nil, PaginationOut{}, err
	}
	digest := sha256.Sum256(encoded)
	binding := hex.EncodeToString(digest[:])
	var initial sdk.Cursor
	if cursor != nil {
		envelope, err := sdk.ParseCursor(*cursor)
		if err != nil {
			return nil, PaginationOut{}, err
		}
		if err := sdk.ValidateCursor(envelope, "pixiv-mcp", operation, 1, binding); err != nil {
			return nil, PaginationOut{}, err
		}
		payload, err := sdk.CursorPayload(envelope)
		if err != nil {
			return nil, PaginationOut{}, err
		}
		initial, err = sdk.ParseCursor(string(payload))
		if err != nil {
			return nil, PaginationOut{}, err
		}
	}
	items, next, err := CollectBatchWith(ctx, app, initial, fetch)
	if err != nil {
		return nil, PaginationOut{}, err
	}
	page := ListPagination(plan, nil, len(items), !next.IsZero())
	if !next.IsZero() {
		envelope, err := sdk.NewCursor("pixiv-mcp", operation, 1, binding, []byte(next.String()))
		if err != nil {
			return nil, PaginationOut{}, err
		}
		page.NextCursor = envelope.String()
	}
	return items, page, nil
}
