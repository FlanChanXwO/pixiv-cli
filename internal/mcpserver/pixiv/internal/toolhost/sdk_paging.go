package toolhost

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/toolrun"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// CollectSDKWith 在新 SDK 的账号重放边界内收集一个逻辑页。
func CollectSDKWith[T any](ctx context.Context, app *App, plan pagination.PagePlan, fetch func(context.Context, *pixiv.Client, sdk.Cursor) ([]T, sdk.Cursor, error)) ([]T, bool, error) {
	return collectFrom(ctx, app.SDKReadExecutor(), plan, sdk.Cursor{}, fetch)
}

// CollectSDKCursorWith 保留 MCP 外层查询绑定，SDK 自行校验内部账号与查询。
func CollectSDKCursorWith[T any](ctx context.Context, app *App, operation string, bindingInput any, token *string, input toolrun.ListIn, fetch func(context.Context, *pixiv.Client, sdk.Cursor) ([]T, sdk.Cursor, error)) ([]T, PaginationOut, error) {
	plan, err := toolrun.ParseListPlan(input)
	if err != nil {
		return nil, PaginationOut{}, err
	}
	if token != nil && !plan.OneBatch {
		return nil, PaginationOut{}, errors.New("cursor requires omitted page and limit")
	}
	if !plan.OneBatch {
		items, more, err := CollectSDKWith(ctx, app, plan, fetch)
		return items, ListPagination(plan, input.Limit, len(items), more), err
	}
	binding, err := BindCursor(operation, bindingInput)
	if err != nil {
		return nil, PaginationOut{}, err
	}
	initial, err := binding.DecodeSDK(token)
	if err != nil {
		return nil, PaginationOut{}, err
	}
	var next sdk.Cursor
	items, more, err := collectFrom(ctx, app.SDKReadExecutor(), plan, initial, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]T, sdk.Cursor, error) {
		items, continuation, err := fetch(ctx, client, cursor)
		if err == nil && !continuation.IsZero() && continuation == cursor {
			return nil, sdk.Cursor{}, errors.New("batch continuation cursor repeated")
		}
		next = continuation
		return items, continuation, err
	})
	if err != nil {
		return nil, PaginationOut{}, err
	}
	page := ListPagination(plan, nil, len(items), more)
	page.NextCursor, err = binding.Encode(next)
	return items, page, err
}
