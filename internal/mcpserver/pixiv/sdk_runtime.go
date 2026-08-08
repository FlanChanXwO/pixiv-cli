package pixiv

import (
	"context"
	"errors"
	"math"

	paginationapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pagination"
	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// pageLimitIn 以指针区分未传 limit（兼容旧的“单上游批次”）和显式 limit=0（读取全部）。
// Cursor 始终是 SDK 内部细节，不出现在 MCP tool 输入或输出中。
type pageLimitIn struct {
	Page  *int `json:"page,omitempty" jsonschema:"1-based logical page; requires a positive limit"`
	Limit *int `json:"limit,omitempty" jsonschema:"maximum items; 0 returns all items"`
}

type mcpListPlan struct {
	page     int
	limit    int
	oneBatch bool
	skip     int
}

type paginationOut struct {
	Page     int  `json:"page"`
	Limit    *int `json:"limit"`
	Returned int  `json:"returned"`
	HasMore  bool `json:"has_more"`
	NextPage *int `json:"next_page"`
}

func parseMCPListPlan(input pageLimitIn) (mcpListPlan, error) {
	if input.Page != nil && *input.Page <= 0 {
		return mcpListPlan{}, errors.New("page must be a positive integer")
	}
	if input.Limit != nil && *input.Limit < 0 {
		return mcpListPlan{}, errors.New("limit must be zero or a positive integer")
	}
	if input.Page != nil {
		if input.Limit == nil || *input.Limit <= 0 {
			return mcpListPlan{}, errors.New("page requires limit to be a positive integer")
		}
		if *input.Page-1 > math.MaxInt / *input.Limit {
			return mcpListPlan{}, errors.New("page and limit overflow the logical result offset")
		}
		return mcpListPlan{page: *input.Page, limit: *input.Limit, skip: (*input.Page - 1) * *input.Limit}, nil
	}
	if input.Limit == nil {
		return mcpListPlan{page: 1, limit: -1, oneBatch: true}, nil
	}
	return mcpListPlan{page: 1, limit: *input.Limit}, nil
}

func listPagination(plan mcpListPlan, limit *int, returned int, hasMore bool) paginationOut {
	out := paginationOut{Page: plan.page, Limit: limit, Returned: returned, HasMore: hasMore}
	if hasMore && limit != nil && *limit > 0 && plan.page < math.MaxInt {
		next := plan.page + 1
		out.NextPage = &next
	}
	return out
}

// collectPages 仅把 MCP 的兼容 sentinel 映射到 application 分页语义；成功空结果
// 仍保持 non-nil slice，失败时共享 collector 会丢弃部分结果。
func collectPages[T any](ctx context.Context, plan mcpListPlan, fetch func(context.Context, sdk.Cursor) ([]T, sdk.Cursor, error)) ([]T, bool, error) {
	limit := plan.limit
	if limit < 0 {
		limit = 0
	}
	seen := make(map[string]struct{})
	items, result, err := paginationapp.CollectPages(ctx, paginationapp.PagePlan{
		Skip:     plan.skip,
		Limit:    limit,
		OneBatch: plan.oneBatch,
	}, func(ctx context.Context, cursor sdk.Cursor) ([]T, sdk.Cursor, error) {
		items, next, err := fetch(ctx, cursor)
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return filterRecordPage(ctx, items, seen), next, nil
	})
	if err != nil {
		return nil, false, err
	}
	return items, result.HasMore, nil
}

func (a *App) openSDKOperation(ctx context.Context) (client pixivapp.ClientSet, release func(), err error) {
	if a.sdk.NewClient == nil {
		return pixivapp.ClientSet{}, nil, errors.New("pixiv sdk is not configured")
	}
	if err = a.acquireSDKGate(ctx); err != nil {
		return pixivapp.ClientSet{}, nil, err
	}
	client, err = a.sdk.OpenOperation(ctx, a.sdkRequest)
	if err != nil {
		a.releaseSDKGate()
		return pixivapp.ClientSet{}, nil, err
	}
	return client, a.releaseSDKOperation, nil
}

func (a *App) currentSDKUser(ctx context.Context) (client pixivapp.ClientSet, userID int64, release func(), err error) {
	if a.sdk.NewClient == nil {
		return pixivapp.ClientSet{}, 0, nil, errors.New("pixiv sdk is not configured")
	}
	if err = a.acquireSDKGate(ctx); err != nil {
		return pixivapp.ClientSet{}, 0, nil, err
	}
	client, err = a.sdk.OpenOperation(ctx, a.sdkRequest)
	if err != nil {
		a.releaseSDKGate()
		return pixivapp.ClientSet{}, 0, nil, err
	}
	userID, err = client.CurrentUserID(ctx)
	if err != nil {
		a.releaseSDKGate()
		return pixivapp.ClientSet{}, 0, nil, err
	}
	return client, userID, a.releaseSDKOperation, nil
}

func (a *App) releaseSDKOperation() {
	a.releaseSDKGate()
}

func (a *App) acquireSDKGate(ctx context.Context) error {
	if a.sdkGate == nil {
		a.sdkGate = make(chan struct{}, 1)
	}
	select {
	case a.sdkGate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *App) releaseSDKGate() { <-a.sdkGate }

func resolveSDKUser(ctx context.Context, app *App, userID int64) (pixivapp.ClientSet, int64, func(), error) {
	if userID != 0 {
		client, release, err := app.openSDKOperation(ctx)
		return client, userID, release, err
	}
	return app.currentSDKUser(ctx)
}
