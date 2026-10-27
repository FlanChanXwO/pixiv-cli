package search

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/internal/searchfilter"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// artworkSearchClient 是 bookmark 过滤所需的最小 public SDK capability。
type artworkSearchClient interface {
	SearchArtworks(context.Context, pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	CheckpointSearchArtworks(pixiv.SearchArtworksRequest, int) (sdk.Cursor, error)
}

type artworkSearchOutcome struct {
	Page   sdk.Page[pixiv.Artwork]
	Filter *searchfilter.BookmarkOutcome
}

type artworkSearchRequest struct {
	Query      pixiv.SearchArtworksRequest
	Plan       pagination.PagePlan
	Membership searchfilter.BookmarkMembership
	Strategy   searchfilter.BookmarkStrategy
	Include    func(pixiv.Artwork) (bool, error)
}

// searchArtworks 在 CLI 的 execution scope 内执行 bookmark 候选筛选。
// auto 只在当前已验证策略下解析为 local；server 不做未经证实的降级。
func searchArtworks[C artworkSearchClient](ctx context.Context, execute pagination.Execute[C], request artworkSearchRequest) (artworkSearchOutcome, error) {
	var outcome artworkSearchOutcome
	if err := validateArtworkPagePlan(request.Plan); err != nil {
		return outcome, err
	}
	membership, err := searchfilter.NormalizeBookmarkMembership(string(request.Membership))
	if err != nil {
		return outcome, bookmarkSearchError(err)
	}
	filter := searchfilter.BookmarkRange{Min: request.Query.BookmarkMin, Max: request.Query.BookmarkMax, Membership: membership}
	strategy, err := searchfilter.ResolveBookmarkStrategy(string(request.Strategy), membership, filter.Active())
	if err != nil {
		return outcome, bookmarkSearchError(err)
	}
	filter.Strategy = strategy
	hasRange := filter.Active()
	if execute == nil {
		return outcome, sdk.NewError("pixiv", "SearchArtworks", sdk.LocalStateError,
			sdk.WithDetail("sdk pooled operation is not configured"))
	}

	err = execute(ctx, func(ctx context.Context, client C) (bool, error) {
		if !hasRange {
			page, err := searchArtworkPages(ctx, client, request.Query, request.Plan, request.Include)
			if err != nil {
				return false, err
			}
			outcome = artworkSearchOutcome{Page: page}
			return false, nil
		}

		candidateQuery := request.Query
		candidateQuery.CursorContext = combineCursorContexts(candidateQuery.CursorContext, filter.CursorContext())
		if filter.Local() {
			// App API bounds 只是 candidate 条件；local 必须枚举正常候选流再本地复核。
			candidateQuery.BookmarkMin = nil
			candidateQuery.BookmarkMax = nil
		}
		items, next, pageResult, err := pagination.CollectFilteredPagesFrom(
			ctx,
			request.Plan,
			candidateQuery.Cursor,
			func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
				candidateQuery.Cursor = cursor
				page, err := client.SearchArtworks(ctx, candidateQuery)
				if err != nil {
					return nil, sdk.Cursor{}, err
				}
				return page.Items, page.Next, nil
			},
			func(item pixiv.Artwork) (bool, error) {
				if item.TotalBookmarks < 0 {
					return false, sdk.NewError("pixiv", "SearchArtworks", sdk.MalformedUpstreamResponse,
						sdk.WithDetail("artwork bookmark count is negative"))
				}
				keep := filter.Matches(item.TotalBookmarks)
				if !keep || request.Include == nil {
					return keep, nil
				}
				return request.Include(item)
			},
			func(cursor sdk.Cursor, consumed int) (sdk.Cursor, error) {
				checkpointQuery := candidateQuery
				checkpointQuery.Cursor = cursor
				return client.CheckpointSearchArtworks(checkpointQuery, consumed)
			},
		)
		if err != nil {
			return false, err
		}
		filtered := filter.Outcome(pageResult.HasMore || !next.IsZero())
		outcome = artworkSearchOutcome{
			Page:   sdk.Page[pixiv.Artwork]{Items: items, Next: next},
			Filter: &filtered,
		}
		return false, nil
	})
	if err != nil {
		return artworkSearchOutcome{}, err
	}
	return outcome, nil
}

func validateArtworkPagePlan(plan pagination.PagePlan) error {
	if plan.Skip < 0 || plan.Limit < 0 {
		return sdk.NewError("pixiv", "SearchArtworks", sdk.InvalidArgument,
			sdk.WithDetail("page skip and limit must be zero or positive"))
	}
	return nil
}

// bookmarkSearchError maps shared bookmark validation to this command's public
// SDK error class while keeping the established messages.
func bookmarkSearchError(err error) error {
	kind := sdk.InvalidArgument
	switch {
	case errors.Is(err, searchfilter.ErrBookmarkServer):
		kind = sdk.UpstreamUnavailable
	case errors.Is(err, searchfilter.ErrBookmarkNonPremium):
		kind = sdk.Forbidden
	}
	return sdk.NewError("pixiv", "SearchArtworks", kind, sdk.WithDetail(err.Error()))
}

func searchArtworkPages[C artworkSearchClient](ctx context.Context, client C, query pixiv.SearchArtworksRequest, plan pagination.PagePlan, include func(pixiv.Artwork) (bool, error)) (sdk.Page[pixiv.Artwork], error) {
	if include == nil {
		include = func(pixiv.Artwork) (bool, error) { return true, nil }
	}
	items, next, _, err := pagination.CollectFilteredPagesFrom(ctx, plan, query.Cursor, func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		fetchQuery := query
		fetchQuery.Cursor = cursor
		page, err := client.SearchArtworks(ctx, fetchQuery)
		return page.Items, page.Next, err
	}, include, func(cursor sdk.Cursor, consumed int) (sdk.Cursor, error) {
		checkpointQuery := query
		checkpointQuery.Cursor = cursor
		return client.CheckpointSearchArtworks(checkpointQuery, consumed)
	})
	if err != nil {
		return sdk.Page[pixiv.Artwork]{}, err
	}
	return sdk.Page[pixiv.Artwork]{Items: items, Next: next}, nil
}

// combineCursorContexts 将多个已经脱敏的本地语义摘要绑定为一个摘要；空值
// 保持原摘要，以兼容没有组合本地筛选时既有的 bookmark cursor 语义。
func combineCursorContexts(values ...string) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			parts = append(parts, value)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}
	payload := "search/v1\n"
	for _, part := range parts {
		payload += part + "\n"
	}
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}
