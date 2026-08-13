// Package pixiv 拥有 Pixiv 搜索的逻辑分页、bookmark 策略和完整性语义。
package pixiv

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	product "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// Operation 在产品 session 提供的重放边界内执行一次搜索或列表读取。它只
// 约定 commit 回传，不拥有账号选择、网络连接或具体 client 构造。
type Operation[C any] func(context.Context, func(context.Context, C) (committed bool, err error)) error

// PagedReadResult 是入口层可安全公开的逻辑分页状态；opaque cursor 不离开
// workflow，HasMore 只表达还有下一逻辑页。
type PagedReadResult[T any] struct {
	Items   []T
	HasMore bool
}

// RunPooledPagedRead 在调用方提供的 session operation 内执行 logical
// pagination。consume 返回 committed=true 表示已经尝试交付结果，失败后不得
// 由账号池安全重放，避免重复请求或重复输出。
func RunPooledPagedRead[C any, T any](ctx context.Context, operation Operation[C], plan pagination.PagePlan, begin func() error, fetch func(context.Context, C, sdk.Cursor) ([]T, sdk.Cursor, error), consume func([]T) (committed bool, err error)) error {
	if operation == nil {
		return sdk.NewError("pixiv", "PagedRead", sdk.LocalStateError,
			sdk.WithDetail("sdk pooled operation is not configured"))
	}
	return operation(ctx, func(ctx context.Context, client C) (bool, error) {
		if begin != nil {
			if err := begin(); err != nil {
				return false, err
			}
		}
		committed := false
		_, err := pagination.TraversePagesFrom(ctx, plan, sdk.Cursor{}, func(ctx context.Context, cursor sdk.Cursor) ([]T, sdk.Cursor, error) {
			return fetch(ctx, client, cursor)
		}, func(items []T) error {
			published, err := consume(items)
			committed = committed || published
			return err
		})
		return committed, err
	})
}

// CollectPooledPagedRead 在一个可重放 operation 中收集 typed 读取。每次开始
// 新尝试都会清空前一次未提交结果，避免跨账号混入；失败时保留当前尝试的
// 收集状态并原样返回错误，调用方决定是否消费该状态。
func CollectPooledPagedRead[C any, T any](ctx context.Context, operation Operation[C], plan pagination.PagePlan, fetch func(context.Context, C, sdk.Cursor) ([]T, sdk.Cursor, error)) (PagedReadResult[T], error) {
	result := PagedReadResult[T]{Items: make([]T, 0)}
	var next sdk.Cursor
	err := RunPooledPagedRead(ctx, operation, plan, func() error {
		result.Items = result.Items[:0]
		result.HasMore = false
		next = sdk.Cursor{}
		return nil
	}, func(ctx context.Context, client C, cursor sdk.Cursor) ([]T, sdk.Cursor, error) {
		items, cursorNext, err := fetch(ctx, client, cursor)
		if err == nil {
			next = cursorNext
		}
		return items, cursorNext, err
	}, func(items []T) (bool, error) {
		result.Items = append(result.Items, items...)
		return false, nil
	})
	result.HasMore = !next.IsZero()
	return result, err
}

// ArtworkSearchClient 是 bookmark 搜索编排所需的唯一 public SDK capability。
type ArtworkSearchClient interface {
	SearchArtworks(context.Context, product.SearchArtworksRequest) (sdk.Page[product.Artwork], error)
}

// SearchOutcome 只在需要说明本地过滤策略和结果完整性时包装 public SDK page。
type SearchOutcome[T any] struct {
	Page   sdk.Page[T]
	Filter *BookmarkFilterOutcome
}

// BookmarkMembership 是本地已知的账号会员状态。空值按 unknown 处理，unknown
// 不能被推断为 non_premium。
type BookmarkMembership string

const (
	BookmarkMembershipPremium    BookmarkMembership = "premium"
	BookmarkMembershipNonPremium BookmarkMembership = "non_premium"
	BookmarkMembershipUnknown    BookmarkMembership = "unknown"
)

// BookmarkFilterStrategy 是收藏数量筛选的编排策略。
type BookmarkFilterStrategy string

const (
	BookmarkFilterStrategyAuto       BookmarkFilterStrategy = "auto"
	BookmarkFilterStrategyServer     BookmarkFilterStrategy = "server"
	BookmarkFilterStrategyLocal      BookmarkFilterStrategy = "local"
	BookmarkFilterStrategyBestEffort BookmarkFilterStrategy = "best_effort"
)

// BookmarkFilterCompleteness 说明本次是否已经消费完当前 candidate source。
// complete_for_source 不代表对关键词结果全集作出全局完备性承诺。
type BookmarkFilterCompleteness string

const (
	BookmarkFilterCompletenessPartial           BookmarkFilterCompleteness = "partial"
	BookmarkFilterCompletenessCompleteForSource BookmarkFilterCompleteness = "complete_for_source"
)

// BookmarkFilterOutcome 是不含凭据或 upstream locator 的安全诊断语义。
type BookmarkFilterOutcome struct {
	Min          *int
	Max          *int
	Membership   BookmarkMembership
	Strategy     BookmarkFilterStrategy
	Completeness BookmarkFilterCompleteness
}

// ArtworkSearchRequest 组合 public SDK 原子请求与 internal logical pagination。
// session account 的选择留在 Operation，不能通过本 request 另行指定。
type ArtworkSearchRequest struct {
	Query      product.SearchArtworksRequest
	Plan       pagination.PagePlan
	Membership BookmarkMembership
	Strategy   BookmarkFilterStrategy
}

// SearchArtworks 在 session Operation 内执行 bookmark 候选筛选。当前没有可靠
// server bookmark strategy 证据，因此 auto 始终走 local，server 明确失败而不
// 偷换协议或静默退化。
func SearchArtworks[C ArtworkSearchClient](ctx context.Context, operation Operation[C], request ArtworkSearchRequest) (SearchOutcome[product.Artwork], error) {
	var outcome SearchOutcome[product.Artwork]
	min, max, hasRange, err := validateArtworkBookmarkSearch(request)
	if err != nil {
		return outcome, err
	}
	membership, err := normalizeBookmarkMembership(request.Membership)
	if err != nil {
		return outcome, err
	}
	strategy, err := resolveBookmarkStrategy(request.Strategy, membership, hasRange)
	if err != nil {
		return outcome, err
	}
	if operation == nil {
		return outcome, sdk.NewError("pixiv", "SearchArtworks", sdk.LocalStateError,
			sdk.WithDetail("sdk pooled operation is not configured"))
	}

	err = operation(ctx, func(ctx context.Context, client C) (bool, error) {
		if !hasRange {
			page, err := searchArtworkPages(ctx, client, request.Query, request.Plan)
			if err != nil {
				return false, err
			}
			outcome = SearchOutcome[product.Artwork]{Page: page}
			return false, nil
		}

		candidateQuery := request.Query
		if strategy == BookmarkFilterStrategyLocal {
			// App API bounds 只是 candidate 条件；local 必须枚举正常候选流再本地复核。
			candidateQuery.BookmarkMin = nil
			candidateQuery.BookmarkMax = nil
		}
		items, next, pageResult, err := pagination.CollectFilteredPagesFrom(
			ctx,
			request.Plan,
			candidateQuery.Cursor,
			func(ctx context.Context, cursor sdk.Cursor) ([]product.Artwork, sdk.Cursor, error) {
				candidateQuery.Cursor = cursor
				page, err := client.SearchArtworks(ctx, candidateQuery)
				if err != nil {
					return nil, sdk.Cursor{}, err
				}
				return page.Items, page.Next, nil
			},
			func(item product.Artwork) (bool, error) {
				if item.TotalBookmarks < 0 {
					return false, sdk.NewError("pixiv", "SearchArtworks", sdk.MalformedUpstreamResponse,
						sdk.WithDetail("artwork bookmark count is negative"))
				}
				return (min == nil || item.TotalBookmarks >= *min) && (max == nil || item.TotalBookmarks <= *max), nil
			},
		)
		if err != nil {
			return false, err
		}
		completeness := BookmarkFilterCompletenessCompleteForSource
		if pageResult.HasMore || !next.IsZero() {
			completeness = BookmarkFilterCompletenessPartial
		}
		outcome = SearchOutcome[product.Artwork]{
			Page: sdk.Page[product.Artwork]{Items: items, Next: next},
			Filter: &BookmarkFilterOutcome{
				Min:          cloneIntPointer(min),
				Max:          cloneIntPointer(max),
				Membership:   membership,
				Strategy:     strategy,
				Completeness: completeness,
			},
		}
		return false, nil
	})
	if err != nil {
		return SearchOutcome[product.Artwork]{}, err
	}
	return outcome, nil
}

func validateArtworkBookmarkSearch(request ArtworkSearchRequest) (min, max *int, hasRange bool, err error) {
	if request.Query.BookmarkMin != nil && *request.Query.BookmarkMin < 0 {
		return nil, nil, false, sdk.NewError("pixiv", "SearchArtworks", sdk.InvalidArgument,
			sdk.WithDetail("bookmark minimum must be zero or positive"))
	}
	if request.Query.BookmarkMax != nil && *request.Query.BookmarkMax < 0 {
		return nil, nil, false, sdk.NewError("pixiv", "SearchArtworks", sdk.InvalidArgument,
			sdk.WithDetail("bookmark maximum must be zero or positive"))
	}
	if request.Query.BookmarkMin != nil && request.Query.BookmarkMax != nil && *request.Query.BookmarkMin > *request.Query.BookmarkMax {
		return nil, nil, false, sdk.NewError("pixiv", "SearchArtworks", sdk.InvalidArgument,
			sdk.WithDetail("bookmark minimum must not exceed maximum"))
	}
	if request.Plan.Skip < 0 || request.Plan.Limit < 0 {
		return nil, nil, false, sdk.NewError("pixiv", "SearchArtworks", sdk.InvalidArgument,
			sdk.WithDetail("page skip and limit must be zero or positive"))
	}
	return request.Query.BookmarkMin, request.Query.BookmarkMax, request.Query.BookmarkMin != nil || request.Query.BookmarkMax != nil, nil
}

func normalizeBookmarkMembership(value BookmarkMembership) (BookmarkMembership, error) {
	switch value {
	case "", BookmarkMembershipUnknown:
		return BookmarkMembershipUnknown, nil
	case BookmarkMembershipPremium, BookmarkMembershipNonPremium:
		return value, nil
	default:
		return "", sdk.NewError("pixiv", "SearchArtworks", sdk.InvalidArgument,
			sdk.WithDetail("unknown bookmark membership"))
	}
}

func resolveBookmarkStrategy(requested BookmarkFilterStrategy, membership BookmarkMembership, hasRange bool) (BookmarkFilterStrategy, error) {
	if requested == "" {
		requested = BookmarkFilterStrategyAuto
	}
	switch requested {
	case BookmarkFilterStrategyAuto:
		if !hasRange {
			return BookmarkFilterStrategyAuto, nil
		}
		return BookmarkFilterStrategyLocal, nil
	case BookmarkFilterStrategyLocal, BookmarkFilterStrategyBestEffort:
		return requested, nil
	case BookmarkFilterStrategyServer:
		if membership == BookmarkMembershipNonPremium {
			return "", sdk.NewError("pixiv", "SearchArtworks", sdk.Forbidden,
				sdk.WithDetail("server bookmark strategy is not available for non-premium membership"))
		}
		return "", sdk.NewError("pixiv", "SearchArtworks", sdk.UpstreamUnavailable,
			sdk.WithDetail("server bookmark strategy requires verified premium membership and evidence"))
	default:
		return "", sdk.NewError("pixiv", "SearchArtworks", sdk.InvalidArgument,
			sdk.WithDetail("unknown bookmark filter strategy"))
	}
}

func searchArtworkPages[C ArtworkSearchClient](ctx context.Context, client C, query product.SearchArtworksRequest, plan pagination.PagePlan) (sdk.Page[product.Artwork], error) {
	items := make([]product.Artwork, 0)
	var next sdk.Cursor
	_, err := pagination.TraversePagesFrom(ctx, plan, query.Cursor, func(ctx context.Context, cursor sdk.Cursor) ([]product.Artwork, sdk.Cursor, error) {
		query.Cursor = cursor
		page, err := client.SearchArtworks(ctx, query)
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		next = page.Next
		return page.Items, page.Next, nil
	}, func(page []product.Artwork) error {
		items = append(items, page...)
		return nil
	})
	if err != nil {
		return sdk.Page[product.Artwork]{}, err
	}
	return sdk.Page[product.Artwork]{Items: items, Next: next}, nil
}

func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
