package pixiv

import (
	"context"
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixivapi"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// SearchArtworks searches artworks. Repeat the original request fields when
// continuing with a non-zero Cursor.
func (c *Client) SearchArtworks(ctx context.Context, request SearchArtworksRequest) (sdk.Page[Artwork], error) {
	request, query, err := searchArtworksQuery(request)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	state, err := c.searchArtworksContinuation(query, request.Cursor, request.Offset)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	searchQuery := searchArtworksWireQuery(request, int(state.Value))
	var raw artworkListWire
	if err := c.app.GetJSON(ctx, pixivapi.PathSearchIllust, searchQuery, &raw); err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "SearchArtworks")
	}
	items, err := artworkItems("SearchArtworks", raw.Illusts, false)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	nextOffset, hasNext, err := searchContinuation.offset(raw.NextURL)
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "SearchArtworks")
	}
	page := sdk.Page[Artwork]{Items: make([]Artwork, len(items))}
	for index, item := range items {
		page.Items[index], err = c.mapArtwork(item)
		if err != nil {
			return sdk.Page[Artwork]{}, err
		}
	}
	if hasNext {
		page.Next, err = c.buildCursor("SearchArtworks", query, pixivapi.ParamOffset, int64(nextOffset), true)
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
	if request.Offset < 0 {
		return request, nil, newError("SearchArtworks", sdk.InvalidArgument, "offset must be non-negative")
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
	if request.Offset > 0 {
		// 只绑定初始窗口；上游 offset 由当前 cursor 或首次请求决定。
		query.Set("initial_offset", itoa(int64(request.Offset)))
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
	state, err := c.searchArtworksContinuation(query, request.Cursor, request.Offset)
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

func (c *Client) searchArtworksContinuation(query url.Values, cursor sdk.Cursor, initialOffset int) (continuationEnvelope, error) {
	if cursor.IsZero() {
		// 首次读取与批内 checkpoint 必须指向同一原始批次。
		return continuationEnvelope{Key: "offset", Value: int64(initialOffset)}, nil
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

// searchArtworksWireQuery 把公开搜索请求翻译成 App search 参数。AIMode=only
// 由 SDK 在本地过滤，wire 仍发送 all；其余 enum 的候选值在此汇总。
func searchArtworksWireQuery(request SearchArtworksRequest, offset int) url.Values {
	query := url.Values{
		pixivapi.ParamWord:         {request.Word},
		pixivapi.ParamSearchTarget: {string(request.Target)},
		pixivapi.ParamSort:         {string(request.Sort)},
	}
	setOptionalParam(query, pixivapi.ParamDuration, string(request.Duration))
	setOptionalParam(query, pixivapi.ParamStartDate, request.StartDate)
	setOptionalParam(query, pixivapi.ParamEndDate, request.EndDate)
	query.Set(pixivapi.ParamSearchAIType, "0")
	if request.AIMode == SearchAIModeExclude {
		query.Set(pixivapi.ParamSearchAIType, "1")
	}
	switch request.AspectRatio {
	case SearchAspectRatioLandscape, SearchAspectRatioPortrait, SearchAspectRatioSquare:
		query.Set(pixivapi.ParamRatioPattern, string(request.AspectRatio))
	}
	switch request.ContentType {
	case SearchContentTypeIllustAndUgoira:
		query.Set(pixivapi.ParamContentType, "illust_and_ugoira")
	case SearchContentTypeIllust, SearchContentTypeManga, SearchContentTypeUgoira:
		query.Set(pixivapi.ParamContentType, string(request.ContentType))
	}
	setOptionalParam(query, pixivapi.ParamTool, request.Tool)
	if request.BookmarkMin != nil {
		query.Set(pixivapi.ParamBookmarkNumMin, strconv.Itoa(*request.BookmarkMin))
	}
	if request.BookmarkMax != nil {
		query.Set(pixivapi.ParamBookmarkNumMax, strconv.Itoa(*request.BookmarkMax))
	}
	applyResolutionFilter(query, request.Resolution)
	if offset > 0 {
		query.Set(pixivapi.ParamOffset, strconv.Itoa(offset))
	}
	return query
}

func setOptionalParam(query url.Values, key, value string) {
	if value != "" {
		query.Set(key, value)
	}
}
