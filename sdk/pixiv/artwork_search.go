package pixiv

import (
	"context"
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// SearchArtworks searches artworks. Repeat the original request fields when
// continuing with a non-zero Cursor.
func (c *Client) SearchArtworks(ctx context.Context, request SearchArtworksRequest) (sdk.Page[Artwork], error) {
	ctx = logging.WithOperation(ctx, "SearchArtworks")
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
	if err := c.app.GetJSON(ctx, api.PathSearchIllust, searchQuery, &raw); err != nil {
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
	mapped, err := c.mapArtworks(items)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	page := sdk.Page[Artwork]{Items: mapped}
	if hasNext {
		page.Next, err = c.buildCursor("SearchArtworks", query, api.ParamOffset, int64(nextOffset), true)
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
	query.Set("word", request.Word)
	query.Set("search_target", string(request.Target))
	query.Set("sort", string(request.Sort))
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
		// Bind only the initial window; the effective upstream offset comes from
		// the current cursor or the first request.
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
	// Accumulate the position within the batch so a second truncation does not
	// replay the consumed prefix. Guard the addition against overflow.
	if consumed > int(^uint(0)>>1)-state.Consumed {
		return sdk.Cursor{}, newError("SearchArtworks", sdk.InvalidArgument, "consumed position overflows")
	}
	state.Consumed += consumed
	return c.buildContinuationCursor("SearchArtworks", query, state)
}

func (c *Client) searchArtworksContinuation(query url.Values, cursor sdk.Cursor, initialOffset int) (continuationEnvelope, error) {
	if cursor.IsZero() {
		// The first read and an in-batch checkpoint must address the same upstream
		// batch.
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

// searchArtworksWireQuery translates a public search request into App API search
// parameters. AIMode=only is filtered locally, so the wire still sends all; the
// accepted values of the remaining enums are collected here.
func searchArtworksWireQuery(request SearchArtworksRequest, offset int) url.Values {
	query := url.Values{
		api.ParamWord:         {request.Word},
		api.ParamSearchTarget: {string(request.Target)},
		api.ParamSort:         {string(request.Sort)},
	}
	setOptionalParam(query, api.ParamDuration, string(request.Duration))
	setOptionalParam(query, api.ParamStartDate, request.StartDate)
	setOptionalParam(query, api.ParamEndDate, request.EndDate)
	query.Set(api.ParamSearchAIType, "0")
	if request.AIMode == SearchAIModeExclude {
		query.Set(api.ParamSearchAIType, "1")
	}
	switch request.AspectRatio {
	case SearchAspectRatioLandscape, SearchAspectRatioPortrait, SearchAspectRatioSquare:
		query.Set(api.ParamRatioPattern, string(request.AspectRatio))
	}
	switch request.ContentType {
	case SearchContentTypeIllustAndUgoira:
		query.Set(api.ParamContentType, "illust_and_ugoira")
	case SearchContentTypeIllust, SearchContentTypeManga, SearchContentTypeUgoira:
		query.Set(api.ParamContentType, string(request.ContentType))
	}
	setOptionalParam(query, api.ParamTool, request.Tool)
	if request.BookmarkMin != nil {
		query.Set(api.ParamBookmarkNumMin, strconv.Itoa(*request.BookmarkMin))
	}
	if request.BookmarkMax != nil {
		query.Set(api.ParamBookmarkNumMax, strconv.Itoa(*request.BookmarkMax))
	}
	applyResolutionFilter(query, request.Resolution)
	if offset > 0 {
		query.Set(api.ParamOffset, strconv.Itoa(offset))
	}
	return query
}

func setOptionalParam(query url.Values, key, value string) {
	if value != "" {
		query.Set(key, value)
	}
}

// SearchArtworksRequest searches artworks. Repeat the original fields when
// continuing with Cursor.
type SearchArtworksRequest struct {
	Word     string
	Target   SearchTarget
	Sort     SortMode
	Duration DurationFilter
	// StartDate and EndDate are inclusive YYYY-MM-DD bounds; empty leaves the
	// bound to upstream.
	StartDate   string
	EndDate     string
	ContentType SearchContentType
	AIMode      SearchAIMode
	AspectRatio SearchAspectRatio
	Resolution  SearchResolution
	Tool        string
	BookmarkMin *int
	BookmarkMax *int
	Cursor      sdk.Cursor
	// CursorContext binds caller-side filtering semantics; it is hashed into
	// the cursor binding and is never sent to Pixiv. Repeat it when resuming.
	CursorContext string
	// Offset is the starting position in the raw upstream result stream. Repeat
	// it when continuing with Cursor.
	Offset int
}

var searchContinuation = continuationSpec{
	path: api.PathSearchIllust,
	keys: []string{api.ParamOffset},
	query: []string{
		api.ParamWord, api.ParamSearchTarget, api.ParamSort,
		api.ParamDuration, api.ParamStartDate, api.ParamEndDate,
		api.ParamSearchAIType, api.ParamRatioPattern, api.ParamContentType,
		api.ParamTool, api.ParamBookmarkNumMin, api.ParamBookmarkNumMax,
		api.ParamWidthMin, api.ParamWidthMax, api.ParamHeightMin, api.ParamHeightMax,
	},
}
