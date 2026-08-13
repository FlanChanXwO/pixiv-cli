package pixiv

import (
	"context"
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/artwork/bookmark"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/artwork/comments"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/artwork/ranking"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/artwork/recommended"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/artwork/related"
	artworksearch "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/artwork/search"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/artwork/series"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/artwork/timeline"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// SearchArtworks searches artworks. Repeat the original request fields when
// continuing with a non-zero Cursor.
func (c *Client) SearchArtworks(ctx context.Context, request SearchArtworksRequest) (sdk.Page[Artwork], error) {
	if err := validateSearchWord("SearchArtworks", request.Word); err != nil {
		return sdk.Page[Artwork]{}, err
	}
	if err := validateSearchArtworksRequest("SearchArtworks", request); err != nil {
		return sdk.Page[Artwork]{}, err
	}
	if err := validateBookmarkRange("SearchArtworks", request.BookmarkMin, request.BookmarkMax); err != nil {
		return sdk.Page[Artwork]{}, err
	}
	if request.Target == "" {
		request.Target = SearchTargetPartialMatchForTags
	}
	if request.Sort == "" {
		request.Sort = SortModeDateDesc
	}
	query := url.Values{}
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
	offset, err := c.continuationOffset("SearchArtworks", query, request.Cursor)
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
		Offset:    offset,
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
	return page, nil
}

// Artwork returns one artwork by its stable ID, including every image page.
func (c *Client) Artwork(ctx context.Context, request ArtworkRequest) (Artwork, error) {
	if request.ArtworkID <= 0 {
		return Artwork{}, newError("Artwork", sdk.InvalidArgument, "artwork ID must be positive")
	}
	detail, err := c.artworkDetail.Artwork(ctx, request.ArtworkID)
	if err != nil {
		return Artwork{}, classifyAppError(err, "Artwork")
	}
	return c.mapArtworkEntityDetail(detail.Artwork)
}

// ArtworkPages returns every image page of one artwork as usable resources.
func (c *Client) ArtworkPages(ctx context.Context, request ArtworkPagesRequest) ([]ArtworkPage, error) {
	if request.ArtworkID <= 0 {
		return nil, newError("ArtworkPages", sdk.InvalidArgument, "artwork ID must be positive")
	}
	detail, err := c.artworkDetail.Artwork(ctx, request.ArtworkID)
	if err != nil {
		return nil, classifyAppError(err, "ArtworkPages")
	}
	return c.mapArtworkEntityPages(detail.Artwork)
}

// RelatedArtworks lists artworks related to one artwork.
func (c *Client) RelatedArtworks(ctx context.Context, request RelatedArtworksRequest) (sdk.Page[Artwork], error) {
	if request.ArtworkID <= 0 {
		return sdk.Page[Artwork]{}, newError("RelatedArtworks", sdk.InvalidArgument, "artwork ID must be positive")
	}
	query := url.Values{"illust_id": {itoa(request.ArtworkID)}}
	offset, err := c.continuationOffset("RelatedArtworks", query, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	list, err := c.artworkRelated.List(ctx, related.Request{ArtworkID: request.ArtworkID, Offset: offset})
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "RelatedArtworks")
	}
	return c.artworkPage("RelatedArtworks", query, "offset", list.Items, int64(list.NextOffset), list.HasNext)
}

// ArtworkSeries lists artworks within one illustration series.
func (c *Client) ArtworkSeries(ctx context.Context, request ArtworkSeriesRequest) (sdk.Page[Artwork], error) {
	if request.SeriesID <= 0 {
		return sdk.Page[Artwork]{}, newError("ArtworkSeries", sdk.InvalidArgument, "series ID must be positive")
	}
	query := url.Values{"illust_series_id": {itoa(request.SeriesID)}}
	lastOrder, err := c.continuationValue("ArtworkSeries", query, request.Cursor, "last_order")
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	list, err := c.artworkSeries.List(ctx, series.Request{SeriesID: request.SeriesID, LastOrder: lastOrder})
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "ArtworkSeries")
	}
	return c.artworkPage("ArtworkSeries", query, "last_order", list.Items, list.NextLastOrder, list.HasNext)
}

// ArtworkRanking lists the current artwork ranking.
func (c *Client) ArtworkRanking(ctx context.Context, request ArtworkRankingRequest) (sdk.Page[Artwork], error) {
	if request.Mode == "" {
		request.Mode = RankingModeDay
	}
	if err := validateRankingMode("ArtworkRanking", request.Mode); err != nil {
		return sdk.Page[Artwork]{}, err
	}
	if err := validateDate("ArtworkRanking", "date", request.Date); err != nil {
		return sdk.Page[Artwork]{}, err
	}
	query := url.Values{"mode": {string(request.Mode)}}
	if request.Date != "" {
		query.Set("date", request.Date)
	}
	offset, err := c.continuationOffset("ArtworkRanking", query, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	list, err := c.artworkRanking.List(ctx, ranking.Request{Mode: string(request.Mode), Date: request.Date, Offset: offset})
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "ArtworkRanking")
	}
	return c.artworkPage("ArtworkRanking", query, "offset", list.Items, int64(list.NextOffset), list.HasNext)
}

// RecommendedArtworks lists recommended artworks.
func (c *Client) RecommendedArtworks(ctx context.Context, request RecommendedArtworksRequest) (sdk.Page[Artwork], error) {
	query := url.Values{}
	offset, contExists, err := c.continuationOffsetExists("RecommendedArtworks", query, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	list, err := c.artworkRecommended.List(ctx, recommended.Request{Offset: offset, ContinuationExists: contExists})
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "RecommendedArtworks")
	}
	return c.artworkPage("RecommendedArtworks", query, "offset", list.Items, int64(list.NextOffset), list.HasNext)
}

// FollowingArtworks lists artworks by followed users.
func (c *Client) FollowingArtworks(ctx context.Context, request FollowingArtworksRequest) (sdk.Page[Artwork], error) {
	query := url.Values{"restrict": {string(request.Restrict)}}
	offset, err := c.continuationOffset("FollowingArtworks", query, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	list, err := c.artworkTimeline.List(ctx, timeline.Request{Kind: timeline.Following, Restrict: string(request.Restrict), Offset: offset})
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "FollowingArtworks")
	}
	return c.artworkPage("FollowingArtworks", query, "offset", list.Items, int64(list.NextOffset), list.HasNext)
}

// LatestArtworks lists the newest artworks.
func (c *Client) LatestArtworks(ctx context.Context, request LatestArtworksRequest) (sdk.Page[Artwork], error) {
	query := url.Values{}
	contentType := string(request.ContentType)
	if contentType == "" {
		contentType = "illust"
	}
	offset, err := c.continuationOffset("LatestArtworks", query, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	list, err := c.artworkTimeline.List(ctx, timeline.Request{Kind: timeline.Latest, ContentType: contentType, Offset: offset})
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "LatestArtworks")
	}
	return c.artworkPage("LatestArtworks", query, "offset", list.Items, int64(list.NextOffset), list.HasNext)
}

// UserArtworks lists one user's artworks.
func (c *Client) UserArtworks(ctx context.Context, request UserArtworksRequest) (sdk.Page[Artwork], error) {
	if request.UserID <= 0 {
		return sdk.Page[Artwork]{}, newError("UserArtworks", sdk.InvalidArgument, "user ID must be positive")
	}
	kind := string(request.Kind)
	query := url.Values{"user_id": {itoa(request.UserID)}, "type": {kind}}
	offset, err := c.continuationOffset("UserArtworks", query, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	list, err := c.artworkTimeline.List(ctx, timeline.Request{Kind: timeline.UserArtworks, UserID: request.UserID, ArtworkType: kind, Offset: offset})
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "UserArtworks")
	}
	return c.artworkPage("UserArtworks", query, "offset", list.Items, int64(list.NextOffset), list.HasNext)
}

// UserArtworkBookmarks lists one user's bookmarked artworks.
func (c *Client) UserArtworkBookmarks(ctx context.Context, request UserArtworkBookmarksRequest) (sdk.Page[Artwork], error) {
	if request.UserID <= 0 {
		return sdk.Page[Artwork]{}, newError("UserArtworkBookmarks", sdk.InvalidArgument, "user ID must be positive")
	}
	if err := validateRestrict("UserArtworkBookmarks", request.Restrict); err != nil {
		return sdk.Page[Artwork]{}, err
	}
	query := url.Values{"user_id": {itoa(request.UserID)}, "restrict": {string(request.Restrict)}}
	if request.Tag != "" {
		query.Set("tag", request.Tag)
	}
	maxID, err := c.continuationValue("UserArtworkBookmarks", query, request.Cursor, "max_bookmark_id")
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	list, err := c.artworkBookmark.Artworks(ctx, bookmark.ArtworksRequest{UserID: request.UserID, Restrict: string(request.Restrict), Tag: request.Tag, MaxBookmarkID: maxID})
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "UserArtworkBookmarks")
	}
	return c.artworkPage("UserArtworkBookmarks", query, "max_bookmark_id", list.Items, list.NextMaxBookmarkID, list.HasNext)
}

// UserArtworkBookmarkTags lists the bookmark tags of one user's bookmarked
// artworks.
func (c *Client) UserArtworkBookmarkTags(ctx context.Context, request UserArtworkBookmarkTagsRequest) (sdk.Page[BookmarkTag], error) {
	if request.UserID <= 0 {
		return sdk.Page[BookmarkTag]{}, newError("UserArtworkBookmarkTags", sdk.InvalidArgument, "user ID must be positive")
	}
	if err := validateRestrict("UserArtworkBookmarkTags", request.Restrict); err != nil {
		return sdk.Page[BookmarkTag]{}, err
	}
	query := url.Values{"user_id": {itoa(request.UserID)}, "restrict": {string(request.Restrict)}}
	offset, err := c.continuationOffset("UserArtworkBookmarkTags", query, request.Cursor)
	if err != nil {
		return sdk.Page[BookmarkTag]{}, err
	}
	list, err := c.artworkBookmark.Tags(ctx, bookmark.TagsRequest{UserID: request.UserID, Restrict: string(request.Restrict), Offset: offset})
	if err != nil {
		return sdk.Page[BookmarkTag]{}, classifyAppError(err, "UserArtworkBookmarkTags")
	}
	items := make([]BookmarkTag, 0, len(list.Items))
	for _, tag := range list.Items {
		items = append(items, BookmarkTag{Name: tag.Name, Count: tag.Count})
	}
	next, err := c.buildCursor("UserArtworkBookmarkTags", query, "offset", int64(list.NextOffset), list.HasNext)
	if err != nil {
		return sdk.Page[BookmarkTag]{}, err
	}
	return sdk.Page[BookmarkTag]{Items: items, Next: next}, nil
}

// MyPixivArtworks lists artworks from the current user's MyPixiv feed.
func (c *Client) MyPixivArtworks(ctx context.Context, request MyPixivArtworksRequest) (sdk.Page[Artwork], error) {
	query := url.Values{}
	offset, err := c.continuationOffset("MyPixivArtworks", query, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	list, err := c.artworkTimeline.List(ctx, timeline.Request{Kind: timeline.MyPixiv, Offset: offset})
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "MyPixivArtworks")
	}
	return c.artworkPage("MyPixivArtworks", query, "offset", list.Items, int64(list.NextOffset), list.HasNext)
}

// TrendingArtworkTags lists currently trending artwork tags.
func (c *Client) TrendingArtworkTags(ctx context.Context, request TrendingArtworkTagsRequest) ([]TrendingTag, error) {
	result, err := c.artworkTrending.List(ctx)
	if err != nil {
		return nil, classifyAppError(err, "TrendingArtworkTags")
	}
	items := make([]TrendingTag, 0, len(result))
	for _, tag := range result {
		mapped, err := c.mapArtworkEntity(tag.Artwork)
		if err != nil {
			return nil, err
		}
		items = append(items, TrendingTag{Tag: tag.Tag, TranslatedName: tag.TranslatedName, Artwork: mapped})
	}
	return items, nil
}

// UgoiraMetadata returns the playable metadata of a ugoira artwork. The artwork
// must be a ugoira; other kinds return InvalidArgument.
func (c *Client) UgoiraMetadata(ctx context.Context, request UgoiraMetadataRequest) (UgoiraMetadata, error) {
	if request.ArtworkID <= 0 {
		return UgoiraMetadata{}, newError("UgoiraMetadata", sdk.InvalidArgument, "artwork ID must be positive")
	}
	result, err := c.artworkDetail.UgoiraMetadata(ctx, request.ArtworkID)
	if err != nil {
		return UgoiraMetadata{}, classifyAppError(err, "UgoiraMetadata")
	}
	return c.mapUgoiraMetadataEntity(request.ArtworkID, result.Metadata)
}

// ArtworkComments lists comments on one artwork.
func (c *Client) ArtworkComments(ctx context.Context, request ArtworkCommentsRequest) (CommentPage, error) {
	if request.ArtworkID <= 0 {
		return CommentPage{}, newError("ArtworkComments", sdk.InvalidArgument, "artwork ID must be positive")
	}
	query := url.Values{"illust_id": {itoa(request.ArtworkID)}}
	offset, err := c.continuationOffset("ArtworkComments", query, request.Cursor)
	if err != nil {
		return CommentPage{}, err
	}
	list, err := c.artworkComments.List(ctx, comments.Request{ArtworkID: request.ArtworkID, Offset: offset})
	if err != nil {
		return CommentPage{}, classifyAppError(err, "ArtworkComments")
	}
	return c.mapArtworkCommentPage("ArtworkComments", query, list.Items, list.NextOffset, list.HasNext, list.Total, list.AccessControl)
}

// ArtworkBookmark reads the current user's bookmark detail for one artwork.
func (c *Client) ArtworkBookmark(ctx context.Context, request ArtworkBookmarkRequest) (ArtworkBookmarkDetail, error) {
	if request.ArtworkID <= 0 {
		return ArtworkBookmarkDetail{}, newError("ArtworkBookmark", sdk.InvalidArgument, "artwork ID must be positive")
	}
	detail, err := c.artworkBookmark.Detail(ctx, request.ArtworkID)
	if err != nil {
		return ArtworkBookmarkDetail{}, classifyAppError(err, "ArtworkBookmark")
	}
	return ArtworkBookmarkDetail{Restrict: Restrict(detail.Restrict), Tags: detail.Tags}, nil
}
