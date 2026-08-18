package search_illust

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/pagination"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/traversal"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	product "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// artworkSearchClient 是 bookmark 过滤所需的最小 public SDK capability。
type artworkSearchClient interface {
	SearchArtworks(context.Context, product.SearchArtworksRequest) (sdk.Page[product.Artwork], error)
}

type artworkSearchOutcome struct {
	Page   sdk.Page[product.Artwork]
	Filter *bookmarkFilterOutcome
}

type bookmarkMembership string

const (
	bookmarkMembershipPremium    bookmarkMembership = "premium"
	bookmarkMembershipNonPremium bookmarkMembership = "non_premium"
	bookmarkMembershipUnknown    bookmarkMembership = "unknown"
)

type bookmarkFilterStrategy string

const (
	bookmarkFilterStrategyAuto       bookmarkFilterStrategy = "auto"
	bookmarkFilterStrategyServer     bookmarkFilterStrategy = "server"
	bookmarkFilterStrategyLocal      bookmarkFilterStrategy = "local"
	bookmarkFilterStrategyBestEffort bookmarkFilterStrategy = "best_effort"
)

type bookmarkFilterCompleteness string

const (
	bookmarkFilterCompletenessPartial           bookmarkFilterCompleteness = "partial"
	bookmarkFilterCompletenessCompleteForSource bookmarkFilterCompleteness = "complete_for_source"
)

type bookmarkFilterOutcome struct {
	Min          *int
	Max          *int
	Membership   bookmarkMembership
	Strategy     bookmarkFilterStrategy
	Completeness bookmarkFilterCompleteness
}

type artworkSearchRequest struct {
	Query      product.SearchArtworksRequest
	Plan       pagination.PagePlan
	Membership bookmarkMembership
	Strategy   bookmarkFilterStrategy
}

// searchArtworks 在 MCP adapter 内执行 bookmark 候选筛选。
// auto 只在当前已验证策略下解析为 local；server 不做未经证实的降级。
func searchArtworks[C artworkSearchClient](ctx context.Context, execute traversal.Execute[C], request artworkSearchRequest) (artworkSearchOutcome, error) {
	var outcome artworkSearchOutcome
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
	if execute == nil {
		return outcome, sdk.NewError("pixiv", "SearchArtworks", sdk.LocalStateError,
			sdk.WithDetail("sdk pooled operation is not configured"))
	}

	err = execute(ctx, func(ctx context.Context, client C) (bool, error) {
		if !hasRange {
			page, err := searchArtworkPages(ctx, client, request.Query, request.Plan)
			if err != nil {
				return false, err
			}
			outcome = artworkSearchOutcome{Page: page}
			return false, nil
		}

		candidateQuery := request.Query
		if strategy == bookmarkFilterStrategyLocal {
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
		completeness := bookmarkFilterCompletenessCompleteForSource
		if pageResult.HasMore || !next.IsZero() {
			completeness = bookmarkFilterCompletenessPartial
		}
		outcome = artworkSearchOutcome{
			Page: sdk.Page[product.Artwork]{Items: items, Next: next},
			Filter: &bookmarkFilterOutcome{
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
		return artworkSearchOutcome{}, err
	}
	return outcome, nil
}

func bookmarkFilterFrom(value *bookmarkFilterOutcome) *outputs.BookmarkFilter {
	if value == nil {
		return nil
	}
	return &outputs.BookmarkFilter{
		Min:          value.Min,
		Max:          value.Max,
		Membership:   string(value.Membership),
		Strategy:     string(value.Strategy),
		Completeness: string(value.Completeness),
	}
}

func validateArtworkBookmarkSearch(request artworkSearchRequest) (min, max *int, hasRange bool, err error) {
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

func normalizeBookmarkMembership(value bookmarkMembership) (bookmarkMembership, error) {
	switch value {
	case "", bookmarkMembershipUnknown:
		return bookmarkMembershipUnknown, nil
	case bookmarkMembershipPremium, bookmarkMembershipNonPremium:
		return value, nil
	default:
		return "", sdk.NewError("pixiv", "SearchArtworks", sdk.InvalidArgument,
			sdk.WithDetail("unknown bookmark membership"))
	}
}

func resolveBookmarkStrategy(requested bookmarkFilterStrategy, membership bookmarkMembership, hasRange bool) (bookmarkFilterStrategy, error) {
	if requested == "" {
		requested = bookmarkFilterStrategyAuto
	}
	switch requested {
	case bookmarkFilterStrategyAuto:
		if !hasRange {
			return bookmarkFilterStrategyAuto, nil
		}
		return bookmarkFilterStrategyLocal, nil
	case bookmarkFilterStrategyLocal, bookmarkFilterStrategyBestEffort:
		return requested, nil
	case bookmarkFilterStrategyServer:
		if membership == bookmarkMembershipNonPremium {
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

func searchArtworkPages[C artworkSearchClient](ctx context.Context, client C, query product.SearchArtworksRequest, plan pagination.PagePlan) (sdk.Page[product.Artwork], error) {
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
