package pixiv

import (
	"context"
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/model"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// SearchNovels searches novels. Repeat the original request fields when
// continuing with a non-zero Cursor.
func (c *Client) SearchNovels(ctx context.Context, request SearchNovelsRequest) (sdk.Page[Novel], error) {
	if request.Word == "" {
		return sdk.Page[Novel]{}, newError("SearchNovels", sdk.InvalidArgument, "search word is required")
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
	offset, err := c.continuationOffset("SearchNovels", query, request.Cursor)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	list, err := c.app.SearchNovel(ctx, request.Word, string(request.Target), string(request.Sort), string(request.Duration), offset)
	if err != nil {
		return sdk.Page[Novel]{}, classifyAppError(err, "SearchNovels")
	}
	return c.novelPage("SearchNovels", query, "offset", list)
}

// Novel returns one novel by its stable ID.
func (c *Client) Novel(ctx context.Context, request NovelRequest) (Novel, error) {
	if request.NovelID <= 0 {
		return Novel{}, newError("Novel", sdk.InvalidArgument, "novel ID must be positive")
	}
	detail, err := c.app.NovelDetail(ctx, request.NovelID)
	if err != nil {
		return Novel{}, classifyAppError(err, "Novel")
	}
	return c.mapNovel(detail.Novel)
}

// NovelSeries returns a novel series with its paged novels.
func (c *Client) NovelSeries(ctx context.Context, request NovelSeriesRequest) (NovelSeriesResult, error) {
	if request.SeriesID <= 0 {
		return NovelSeriesResult{}, newError("NovelSeries", sdk.InvalidArgument, "series ID must be positive")
	}
	query := url.Values{"series_id": {itoa(request.SeriesID)}}
	lastOrder, err := c.continuationValue("NovelSeries", query, request.Cursor, "last_order")
	if err != nil {
		return NovelSeriesResult{}, err
	}
	result, err := c.app.NovelSeries(ctx, request.SeriesID, lastOrder)
	if err != nil {
		return NovelSeriesResult{}, classifyAppError(err, "NovelSeries")
	}
	items := make([]Novel, 0, len(result.Novels))
	for _, m := range result.Novels {
		novel, err := c.mapNovel(m)
		if err != nil {
			return NovelSeriesResult{}, err
		}
		items = append(items, novel)
	}
	next, err := c.buildCursor("NovelSeries", query, "last_order", result.NextValue, result.ContinuationExists)
	if err != nil {
		return NovelSeriesResult{}, err
	}
	return NovelSeriesResult{
		Series: NovelSeries{
			ID:          result.Series.ID,
			Title:       result.Series.Title,
			Caption:     result.Series.Caption,
			User:        c.mapUser(result.Series.User),
			IsConcluded: result.Series.IsConcluded,
		},
		Novels: sdk.Page[Novel]{Items: items, Next: next},
	}, nil
}

// NovelContent reads the structured body of one novel.
func (c *Client) NovelContent(ctx context.Context, request NovelContentRequest) (NovelContent, error) {
	if request.NovelID <= 0 {
		return NovelContent{}, newError("NovelContent", sdk.InvalidArgument, "novel ID must be positive")
	}
	html, err := c.app.NovelContent(ctx, request.NovelID)
	if err != nil {
		return NovelContent{}, classifyAppError(err, "NovelContent")
	}
	return c.parseNovelContent(request.NovelID, html)
}

// NovelComments lists comments on one novel.
func (c *Client) NovelComments(ctx context.Context, request NovelCommentsRequest) (CommentPage, error) {
	if request.NovelID <= 0 {
		return CommentPage{}, newError("NovelComments", sdk.InvalidArgument, "novel ID must be positive")
	}
	query := url.Values{"novel_id": {itoa(request.NovelID)}}
	offset, err := c.continuationOffset("NovelComments", query, request.Cursor)
	if err != nil {
		return CommentPage{}, err
	}
	list, err := c.app.NovelComments(ctx, request.NovelID, offset)
	if err != nil {
		return CommentPage{}, classifyAppError(err, "NovelComments")
	}
	return c.commentPage("NovelComments", query, list)
}

// RecommendedNovels lists recommended novels.
func (c *Client) RecommendedNovels(ctx context.Context, request RecommendedNovelsRequest) (sdk.Page[Novel], error) {
	query := url.Values{}
	offset, contExists, err := c.continuationOffsetExists("RecommendedNovels", query, request.Cursor)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	list, err := c.app.NovelRecommended(ctx, offset, contExists)
	if err != nil {
		return sdk.Page[Novel]{}, classifyAppError(err, "RecommendedNovels")
	}
	return c.novelPage("RecommendedNovels", query, "offset", list)
}

// FollowingNovels lists novels by followed users.
func (c *Client) FollowingNovels(ctx context.Context, request FollowingNovelsRequest) (sdk.Page[Novel], error) {
	query := url.Values{"restrict": {string(request.Restrict)}}
	offset, err := c.continuationOffset("FollowingNovels", query, request.Cursor)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	list, err := c.app.NovelFollow(ctx, string(request.Restrict), offset)
	if err != nil {
		return sdk.Page[Novel]{}, classifyAppError(err, "FollowingNovels")
	}
	return c.novelPage("FollowingNovels", query, "offset", list)
}

// LatestNovels lists the newest novels.
func (c *Client) LatestNovels(ctx context.Context, request LatestNovelsRequest) (sdk.Page[Novel], error) {
	query := url.Values{}
	offset, err := c.continuationOffset("LatestNovels", query, request.Cursor)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	list, err := c.app.NovelNew(ctx, offset)
	if err != nil {
		return sdk.Page[Novel]{}, classifyAppError(err, "LatestNovels")
	}
	return c.novelPage("LatestNovels", query, "offset", list)
}

// UserNovels lists one user's novels.
func (c *Client) UserNovels(ctx context.Context, request UserNovelsRequest) (sdk.Page[Novel], error) {
	if request.UserID <= 0 {
		return sdk.Page[Novel]{}, newError("UserNovels", sdk.InvalidArgument, "user ID must be positive")
	}
	query := url.Values{"user_id": {itoa(request.UserID)}}
	offset, err := c.continuationOffset("UserNovels", query, request.Cursor)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	list, err := c.app.UserNovels(ctx, request.UserID, offset)
	if err != nil {
		return sdk.Page[Novel]{}, classifyAppError(err, "UserNovels")
	}
	return c.novelPage("UserNovels", query, "offset", list)
}

// UserNovelBookmarks lists one user's bookmarked novels.
func (c *Client) UserNovelBookmarks(ctx context.Context, request UserNovelBookmarksRequest) (sdk.Page[Novel], error) {
	if request.UserID <= 0 {
		return sdk.Page[Novel]{}, newError("UserNovelBookmarks", sdk.InvalidArgument, "user ID must be positive")
	}
	query := url.Values{"user_id": {itoa(request.UserID)}, "restrict": {string(request.Restrict)}}
	if request.Tag != "" {
		query.Set("tag", request.Tag)
	}
	maxID, err := c.continuationValue("UserNovelBookmarks", query, request.Cursor, "max_bookmark_id")
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	list, err := c.app.UserNovelBookmarks(ctx, request.UserID, string(request.Restrict), request.Tag, maxID)
	if err != nil {
		return sdk.Page[Novel]{}, classifyAppError(err, "UserNovelBookmarks")
	}
	return c.novelPage("UserNovelBookmarks", query, "max_bookmark_id", list)
}

// MyPixivNovels lists novels from the current user's MyPixiv feed.
func (c *Client) MyPixivNovels(ctx context.Context, request MyPixivNovelsRequest) (sdk.Page[Novel], error) {
	query := url.Values{}
	offset, err := c.continuationOffset("MyPixivNovels", query, request.Cursor)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	list, err := c.app.NovelMyPixiv(ctx, offset)
	if err != nil {
		return sdk.Page[Novel]{}, classifyAppError(err, "MyPixivNovels")
	}
	return c.novelPage("MyPixivNovels", query, "offset", list)
}

func (c *Client) novelPage(op string, query url.Values, key string, list *model.NovelList) (sdk.Page[Novel], error) {
	items := make([]Novel, 0, len(list.Novels))
	for _, m := range list.Novels {
		novel, err := c.mapNovel(m)
		if err != nil {
			return sdk.Page[Novel]{}, err
		}
		items = append(items, novel)
	}
	value := int64(list.NextOffset)
	if key == "max_bookmark_id" {
		value = list.NextMaxBookmarkID
	}
	next, err := c.buildCursor(op, query, key, value, list.ContinuationExists)
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	return sdk.Page[Novel]{Items: items, Next: next}, nil
}
