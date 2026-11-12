package search

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	legacysdk "github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/searchfilter"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

type artworkSearchOutcome struct {
	Page   sdk.Page[legacysdk.Artwork]
	Filter *searchfilter.BookmarkOutcome
}

type artworkSearchRequest struct {
	Word     string
	Query    pixiv.ArtworkSearchOptions
	Plan     pagination.PagePlan
	Strategy searchfilter.BookmarkStrategy
	Include  func(legacysdk.Artwork) (bool, error)
}

// searchArtworks 在 CLI 的 execution scope 内执行 bookmark 候选筛选。
// auto 只在当前已验证策略下解析为 local；server 不做未经证实的降级。
func searchArtworks(ctx context.Context, execute pagination.Execute[*pixiv.Client], request artworkSearchRequest) (artworkSearchOutcome, error) {
	var outcome artworkSearchOutcome
	filter := searchfilter.BookmarkRange{Min: request.Query.BookmarkMin, Max: request.Query.BookmarkMax, Membership: searchfilter.BookmarkMembershipUnknown}
	strategy, err := searchfilter.ResolveBookmarkStrategy(string(request.Strategy), filter.Membership, filter.Active())
	if err != nil {
		return outcome, bookmarkSearchError(err)
	}
	filter.Strategy = strategy
	hasRange := filter.Active()

	err = execute(ctx, func(ctx context.Context, client *pixiv.Client) (bool, error) {
		if !hasRange {
			page, err := searchArtworkPages(ctx, client, request.Word, request.Query, request.Plan, request.Include)
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
			func(ctx context.Context, cursor sdk.Cursor) ([]legacysdk.Artwork, sdk.Cursor, error) {
				candidateQuery.Cursor = cursor
				page, err := client.SearchArtworks(ctx, request.Word, candidateQuery)
				if err != nil {
					return nil, sdk.Cursor{}, err
				}
				items := make([]legacysdk.Artwork, len(page.Items))
				for i, item := range page.Items {
					items[i], err = record.ArtworkOutput(item, false)
					if err != nil {
						return nil, sdk.Cursor{}, err
					}
				}
				return items, page.Cursor, nil
			},
			func(item legacysdk.Artwork) (bool, error) {
				if item.TotalBookmarks < 0 {
					return false, &sdk.Error{Operation: "pixiv:SearchArtworks", Reason: sdk.MalformedUpstreamResponse, Message: "artwork bookmark count is negative"}
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
				return client.CheckpointSearchArtworks(ctx, request.Word, checkpointQuery, consumed)
			},
		)
		if err != nil {
			return false, err
		}
		filtered := filter.Outcome(pageResult.HasMore || !next.IsZero())
		outcome = artworkSearchOutcome{
			Page:   sdk.Page[legacysdk.Artwork]{Items: items, Cursor: next},
			Filter: &filtered,
		}
		return false, nil
	})
	if err != nil {
		return artworkSearchOutcome{}, err
	}
	return outcome, nil
}

// bookmarkSearchError 把共享收藏校验错误映射成公开 SDK 的错误类别，并保留既有消息。
func bookmarkSearchError(err error) error {
	kind := sdk.InvalidArgument
	switch {
	case errors.Is(err, searchfilter.ErrBookmarkServer):
		kind = sdk.UpstreamUnavailable
	case errors.Is(err, searchfilter.ErrBookmarkNonPremium):
		kind = sdk.Forbidden
	}
	return &sdk.Error{Operation: "pixiv:SearchArtworks", Reason: kind, Message: err.Error(), Err: err}
}

func searchArtworkPages(ctx context.Context, client *pixiv.Client, word string, query pixiv.ArtworkSearchOptions, plan pagination.PagePlan, include func(legacysdk.Artwork) (bool, error)) (sdk.Page[legacysdk.Artwork], error) {
	if include == nil {
		include = func(legacysdk.Artwork) (bool, error) { return true, nil }
	}
	items, next, _, err := pagination.CollectFilteredPagesFrom(ctx, plan, query.Cursor, func(ctx context.Context, cursor sdk.Cursor) ([]legacysdk.Artwork, sdk.Cursor, error) {
		fetchQuery := query
		fetchQuery.Cursor = cursor
		page, err := client.SearchArtworks(ctx, word, fetchQuery)
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		items := make([]legacysdk.Artwork, len(page.Items))
		for i, item := range page.Items {
			items[i], err = record.ArtworkOutput(item, false)
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
		}
		return items, page.Cursor, nil
	}, include, func(cursor sdk.Cursor, consumed int) (sdk.Cursor, error) {
		checkpointQuery := query
		checkpointQuery.Cursor = cursor
		return client.CheckpointSearchArtworks(ctx, word, checkpointQuery, consumed)
	})
	if err != nil {
		return sdk.Page[legacysdk.Artwork]{}, err
	}
	return sdk.Page[legacysdk.Artwork]{Items: items, Cursor: next}, nil
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
