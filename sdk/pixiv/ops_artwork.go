package pixiv

import (
	"context"
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/model"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// SearchArtworks searches artworks. Repeat the original request fields when
// continuing with a non-zero Cursor.
func (c *Client) SearchArtworks(ctx context.Context, request SearchArtworksRequest) (sdk.Page[Artwork], error) {
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
	filters := model.SearchIllustFilters{
		AIMode:      string(request.AIMode),
		ContentType: string(request.ContentType),
		AspectRatio: string(request.AspectRatio),
		Resolution:  string(request.Resolution),
		Tool:        request.Tool,
		BookmarkMin: request.BookmarkMin,
		BookmarkMax: request.BookmarkMax,
	}
	list, err := c.app.SearchIllust(ctx, request.Word, string(request.Target), string(request.Sort), string(request.Duration), request.StartDate, request.EndDate, offset, filters)
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "SearchArtworks")
	}
	return c.illustPage("SearchArtworks", query, list)
}

// Artwork returns one artwork by its stable ID, including every image page.
func (c *Client) Artwork(ctx context.Context, request ArtworkRequest) (Artwork, error) {
	if request.ArtworkID <= 0 {
		return Artwork{}, newError("Artwork", sdk.CodeInvalidArgument, "artwork ID must be positive")
	}
	detail, err := c.app.IllustDetail(ctx, request.ArtworkID)
	if err != nil {
		return Artwork{}, classifyAppError(err, "Artwork")
	}
	return c.mapArtworkDetail(detail.Illust)
}

// ArtworkPages returns every image page of one artwork as usable resources.
func (c *Client) ArtworkPages(ctx context.Context, request ArtworkPagesRequest) ([]ArtworkPage, error) {
	if request.ArtworkID <= 0 {
		return nil, newError("ArtworkPages", sdk.CodeInvalidArgument, "artwork ID must be positive")
	}
	detail, err := c.app.IllustDetail(ctx, request.ArtworkID)
	if err != nil {
		return nil, classifyAppError(err, "ArtworkPages")
	}
	return c.mapArtworkPages(detail.Illust)
}

// RelatedArtworks lists artworks related to one artwork.
func (c *Client) RelatedArtworks(ctx context.Context, request RelatedArtworksRequest) (sdk.Page[Artwork], error) {
	if request.ArtworkID <= 0 {
		return sdk.Page[Artwork]{}, newError("RelatedArtworks", sdk.CodeInvalidArgument, "artwork ID must be positive")
	}
	query := url.Values{"illust_id": {itoa(request.ArtworkID)}}
	offset, err := c.continuationOffset("RelatedArtworks", query, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	list, err := c.app.IllustRelated(ctx, request.ArtworkID, offset)
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "RelatedArtworks")
	}
	return c.illustPage("RelatedArtworks", query, list)
}

// ArtworkSeries lists artworks within one illustration series.
func (c *Client) ArtworkSeries(ctx context.Context, request ArtworkSeriesRequest) (sdk.Page[Artwork], error) {
	if request.SeriesID <= 0 {
		return sdk.Page[Artwork]{}, newError("ArtworkSeries", sdk.CodeInvalidArgument, "series ID must be positive")
	}
	query := url.Values{"illust_series_id": {itoa(request.SeriesID)}}
	lastOrder, err := c.continuationValue("ArtworkSeries", query, request.Cursor, "last_order")
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	list, err := c.app.IllustSeries(ctx, request.SeriesID, lastOrder)
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "ArtworkSeries")
	}
	return c.illustPage("ArtworkSeries", query, list)
}

// ArtworkRanking lists the current artwork ranking.
func (c *Client) ArtworkRanking(ctx context.Context, request ArtworkRankingRequest) (sdk.Page[Artwork], error) {
	query := url.Values{"mode": {string(request.Mode)}}
	if request.Date != "" {
		query.Set("date", request.Date)
	}
	offset, err := c.continuationOffset("ArtworkRanking", query, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	list, err := c.app.IllustRanking(ctx, string(request.Mode), request.Date, offset)
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "ArtworkRanking")
	}
	return c.illustPage("ArtworkRanking", query, list)
}

// RecommendedArtworks lists recommended artworks.
func (c *Client) RecommendedArtworks(ctx context.Context, request RecommendedArtworksRequest) (sdk.Page[Artwork], error) {
	query := url.Values{}
	offset, contExists, err := c.continuationOffsetExists("RecommendedArtworks", query, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	list, err := c.app.IllustRecommended(ctx, offset, contExists)
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "RecommendedArtworks")
	}
	return c.illustPage("RecommendedArtworks", query, list)
}

// FollowingArtworks lists artworks by followed users.
func (c *Client) FollowingArtworks(ctx context.Context, request FollowingArtworksRequest) (sdk.Page[Artwork], error) {
	query := url.Values{"restrict": {string(request.Restrict)}}
	offset, err := c.continuationOffset("FollowingArtworks", query, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	list, err := c.app.IllustFollow(ctx, string(request.Restrict), offset)
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "FollowingArtworks")
	}
	return c.illustPage("FollowingArtworks", query, list)
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
	list, err := c.app.IllustNew(ctx, contentType, offset)
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "LatestArtworks")
	}
	return c.illustPage("LatestArtworks", query, list)
}

// UserArtworks lists one user's artworks.
func (c *Client) UserArtworks(ctx context.Context, request UserArtworksRequest) (sdk.Page[Artwork], error) {
	if request.UserID <= 0 {
		return sdk.Page[Artwork]{}, newError("UserArtworks", sdk.CodeInvalidArgument, "user ID must be positive")
	}
	kind := string(request.Kind)
	query := url.Values{"user_id": {itoa(request.UserID)}, "type": {kind}}
	offset, err := c.continuationOffset("UserArtworks", query, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	list, err := c.app.UserArtworks(ctx, request.UserID, kind, offset)
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "UserArtworks")
	}
	return c.illustPage("UserArtworks", query, list)
}

// UserArtworkBookmarks lists one user's bookmarked artworks.
func (c *Client) UserArtworkBookmarks(ctx context.Context, request UserArtworkBookmarksRequest) (sdk.Page[Artwork], error) {
	if request.UserID <= 0 {
		return sdk.Page[Artwork]{}, newError("UserArtworkBookmarks", sdk.CodeInvalidArgument, "user ID must be positive")
	}
	query := url.Values{"user_id": {itoa(request.UserID)}, "restrict": {string(request.Restrict)}}
	if request.Tag != "" {
		query.Set("tag", request.Tag)
	}
	maxID, err := c.continuationValue("UserArtworkBookmarks", query, request.Cursor, "max_bookmark_id")
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	list, err := c.app.UserBookmarks(ctx, request.UserID, string(request.Restrict), request.Tag, maxID)
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "UserArtworkBookmarks")
	}
	return c.illustPage("UserArtworkBookmarks", query, list)
}

// UserArtworkBookmarkTags lists the bookmark tags of one user's bookmarked
// artworks.
func (c *Client) UserArtworkBookmarkTags(ctx context.Context, request UserArtworkBookmarkTagsRequest) (sdk.Page[BookmarkTag], error) {
	if request.UserID <= 0 {
		return sdk.Page[BookmarkTag]{}, newError("UserArtworkBookmarkTags", sdk.CodeInvalidArgument, "user ID must be positive")
	}
	query := url.Values{"user_id": {itoa(request.UserID)}, "restrict": {string(request.Restrict)}}
	offset, err := c.continuationOffset("UserArtworkBookmarkTags", query, request.Cursor)
	if err != nil {
		return sdk.Page[BookmarkTag]{}, err
	}
	list, err := c.app.UserArtworkBookmarkTags(ctx, request.UserID, string(request.Restrict), offset)
	if err != nil {
		return sdk.Page[BookmarkTag]{}, classifyAppError(err, "UserArtworkBookmarkTags")
	}
	items := make([]BookmarkTag, 0, len(list.Tags))
	for _, tag := range list.Tags {
		items = append(items, BookmarkTag{Name: tag.Name, Count: tag.Count})
	}
	next, err := c.buildCursor("UserArtworkBookmarkTags", query, "offset", int64(list.NextOffset), list.ContinuationExists)
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
	list, err := c.app.IllustMyPixiv(ctx, offset)
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "MyPixivArtworks")
	}
	return c.illustPage("MyPixivArtworks", query, list)
}

// TrendingArtworkTags lists currently trending artwork tags.
func (c *Client) TrendingArtworkTags(ctx context.Context, request TrendingArtworkTagsRequest) ([]TrendingTag, error) {
	result, err := c.app.TrendingTagsIllust(ctx)
	if err != nil {
		return nil, classifyAppError(err, "TrendingArtworkTags")
	}
	items := make([]TrendingTag, 0, len(result.TrendTags))
	for _, tag := range result.TrendTags {
		artwork, err := c.mapArtwork(tag.Illust)
		if err != nil {
			return nil, err
		}
		items = append(items, TrendingTag{Tag: tag.Tag, TranslatedName: tag.TranslatedName, Artwork: artwork})
	}
	return items, nil
}

// UgoiraMetadata returns the playable metadata of a ugoira artwork. The artwork
// must be a ugoira; other kinds return CodeInvalidArgument.
func (c *Client) UgoiraMetadata(ctx context.Context, request UgoiraMetadataRequest) (UgoiraMetadata, error) {
	if request.ArtworkID <= 0 {
		return UgoiraMetadata{}, newError("UgoiraMetadata", sdk.CodeInvalidArgument, "artwork ID must be positive")
	}
	result, err := c.app.UgoiraMetadata(ctx, request.ArtworkID)
	if err != nil {
		return UgoiraMetadata{}, classifyAppError(err, "UgoiraMetadata")
	}
	return c.mapUgoiraMetadata(request.ArtworkID, result)
}

// ArtworkComments lists comments on one artwork.
func (c *Client) ArtworkComments(ctx context.Context, request ArtworkCommentsRequest) (CommentPage, error) {
	if request.ArtworkID <= 0 {
		return CommentPage{}, newError("ArtworkComments", sdk.CodeInvalidArgument, "artwork ID must be positive")
	}
	query := url.Values{"illust_id": {itoa(request.ArtworkID)}}
	offset, err := c.continuationOffset("ArtworkComments", query, request.Cursor)
	if err != nil {
		return CommentPage{}, err
	}
	list, err := c.app.ArtworkComments(ctx, request.ArtworkID, offset)
	if err != nil {
		return CommentPage{}, classifyAppError(err, "ArtworkComments")
	}
	return c.commentPage("ArtworkComments", query, list)
}

// ArtworkBookmark reads the current user's bookmark detail for one artwork.
func (c *Client) ArtworkBookmark(ctx context.Context, request ArtworkBookmarkRequest) (ArtworkBookmarkDetail, error) {
	if request.ArtworkID <= 0 {
		return ArtworkBookmarkDetail{}, newError("ArtworkBookmark", sdk.CodeInvalidArgument, "artwork ID must be positive")
	}
	detail, err := c.app.ArtworkBookmarkDetail(ctx, request.ArtworkID)
	if err != nil {
		return ArtworkBookmarkDetail{}, classifyAppError(err, "ArtworkBookmark")
	}
	return ArtworkBookmarkDetail{Restrict: Restrict(detail.Restrict), Tags: detail.Tags}, nil
}

// illustPage maps an adapter illust list into a public artwork page, encoding
// the offset continuation into an opaque cursor.
func (c *Client) illustPage(op string, query url.Values, list *model.IllustList) (sdk.Page[Artwork], error) {
	items := make([]Artwork, 0, len(list.Illusts))
	for _, m := range list.Illusts {
		artwork, err := c.mapArtwork(m)
		if err != nil {
			return sdk.Page[Artwork]{}, err
		}
		items = append(items, artwork)
	}
	next, err := c.buildCursor(op, query, "offset", int64(list.NextOffset), list.ContinuationExists)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	return sdk.Page[Artwork]{Items: items, Next: next}, nil
}
