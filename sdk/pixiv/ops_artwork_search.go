package pixiv

import (
	"context"
	"net/url"

	artworksearch "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork/search"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// SearchArtworks searches artworks. Repeat the original request fields when
// continuing with a non-zero Cursor.
func (c *Client) SearchArtworks(ctx context.Context, request SearchArtworksRequest) (sdk.Page[Artwork], error) {
	request, query, err := searchArtworksQuery(request)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	state, err := c.searchArtworksContinuation(query, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	filters := artworksearch.Filters{
		AIMode:      string(request.AIMode),
		ContentType: string(request.ContentType),
		AspectRatio: string(request.AspectRatio),
		Resolution:  string(request.Resolution),
		Tool:        request.Tool,
		BookmarkMin: request.BookmarkMin,
		BookmarkMax: request.BookmarkMax,
	}
	result, err := c.artworkSearch.Search(ctx, artworksearch.Request{
		Word:      request.Word,
		Target:    string(request.Target),
		Sort:      string(request.Sort),
		Duration:  string(request.Duration),
		StartDate: request.StartDate,
		EndDate:   request.EndDate,
		Offset:    int(state.Value),
		Filters:   filters,
	})
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "SearchArtworks")
	}
	items := make([]Artwork, len(result.Items))
	for index, item := range result.Items {
		items[index], err = c.mapArtworkEntity(item)
		if err != nil {
			return sdk.Page[Artwork]{}, err
		}
	}
	page := sdk.Page[Artwork]{Items: items}
	if result.HasNext {
		page.Next, err = c.buildCursor("SearchArtworks", query, "offset", int64(result.NextOffset), true)
		if err != nil {
			return sdk.Page[Artwork]{}, err
		}
	}
	if request.AIMode == SearchAIModeOnly {
		filtered := make([]Artwork, 0, len(page.Items))
		for _, artwork := range page.Items {
			if artwork.AIType == 2 {
				filtered = append(filtered, artwork)
			}
		}
		page.Items = filtered
	}
	if state.Consumed > len(page.Items) {
		return sdk.Page[Artwork]{}, newError("SearchArtworks", sdk.InvalidCursor, "checkpoint exceeds the current batch; restart pagination")
	}
	page.Items = page.Items[state.Consumed:]
	return page, nil
}

func searchArtworksQuery(request SearchArtworksRequest) (SearchArtworksRequest, url.Values, error) {
	if err := validateSearchWord("SearchArtworks", request.Word); err != nil {
		return request, nil, err
	}
	if err := validateSearchArtworksRequest("SearchArtworks", request); err != nil {
		return request, nil, err
	}
	if err := validateBookmarkRange("SearchArtworks", request.BookmarkMin, request.BookmarkMax); err != nil {
		return request, nil, err
	}
	if request.Target == "" {
		request.Target = SearchTargetPartialMatchForTags
	}
	if request.Sort == "" {
		request.Sort = SortModeDateDesc
	}
	query := url.Values{}
	if request.CursorContext != "" {
		query.Set("cursor_context", request.CursorContext)
	}
	if request.Word != "" {
		query.Set("word", request.Word)
	}
	if request.Target != "" {
		query.Set("search_target", string(request.Target))
	}
	if request.Sort != "" {
		query.Set("sort", string(request.Sort))
	}
	if request.Duration != "" {
		query.Set("duration", string(request.Duration))
	}
	if request.StartDate != "" {
		query.Set("start_date", request.StartDate)
	}
	if request.EndDate != "" {
		query.Set("end_date", request.EndDate)
	}
	if request.ContentType != "" && request.ContentType != SearchContentTypeAll {
		query.Set("content_type", string(request.ContentType))
	}
	if request.AIMode != "" && request.AIMode != SearchAIModeAll {
		// The App adapter uses search_ai_type=0 for both all and only. Keep the
		// public mode in the cursor digest so changing local filtering cannot
		// reuse a continuation produced for another result set.
		query.Set("ai_mode", string(request.AIMode))
	}
	if request.AspectRatio != "" && request.AspectRatio != SearchAspectRatioAll {
		query.Set("ratio_pattern", string(request.AspectRatio))
	}
	if request.Resolution != "" && request.Resolution != SearchResolutionAll {
		query.Set("resolution", string(request.Resolution))
	}
	if request.Tool != "" {
		query.Set("tool", request.Tool)
	}
	if request.BookmarkMin != nil {
		query.Set("bookmark_num_min", itoa(int64(*request.BookmarkMin)))
	}
	if request.BookmarkMax != nil {
		query.Set("bookmark_num_max", itoa(int64(*request.BookmarkMax)))
	}
	return request, query, nil
}

// CheckpointSearchArtworks records consumption within the batch fetched by request.
// consumed counts normalized SDK items, after AI filtering, including items the
// caller filtered or skipped. Resuming re-fetches that batch, not a snapshot.
func (c *Client) CheckpointSearchArtworks(request SearchArtworksRequest, consumed int) (sdk.Cursor, error) {
	_, query, err := searchArtworksQuery(request)
	if err != nil {
		return sdk.Cursor{}, err
	}
	if consumed <= 0 {
		return sdk.Cursor{}, newError("SearchArtworks", sdk.InvalidArgument, "consumed must be positive")
	}
	state, err := c.searchArtworksContinuation(query, request.Cursor)
	if err != nil {
		return sdk.Cursor{}, err
	}
	// 累计批内位置，避免第二次截断把已消费前缀重新返回；检查整数溢出。
	if consumed > int(^uint(0)>>1)-state.Consumed {
		return sdk.Cursor{}, newError("SearchArtworks", sdk.InvalidArgument, "consumed position overflows")
	}
	state.Consumed += consumed
	return c.buildContinuationCursor("SearchArtworks", query, state)
}

func (c *Client) searchArtworksContinuation(query url.Values, cursor sdk.Cursor) (continuationEnvelope, error) {
	if cursor.IsZero() {
		return continuationEnvelope{Key: "offset"}, nil
	}
	state, err := c.continuationState("SearchArtworks", query, cursor)
	if err != nil {
		return continuationEnvelope{}, err
	}
	if state.Key != "offset" || state.Value > int64(int(^uint(0)>>1)) {
		return continuationEnvelope{}, newError("SearchArtworks", sdk.InvalidCursor, "cursor continuation kind or position mismatch")
	}
	return state, nil
}
