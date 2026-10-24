// bookmark 领域：作品与小说的收藏读取、收藏列表和收藏标签。
// 收藏写操作在 mutation.go；请求类型与 continuation Spec 都在本文件。

package pixiv

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixivapi"
	usernovelbookmarks "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/novelbookmarks"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

type bookmarkTagWire struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type bookmarkTagsWire struct {
	Tags    pixivapi.RequiredList[bookmarkTagWire] `json:"bookmark_tags"`
	NextURL *string                                `json:"next_url"`
}

type bookmarkDetailWire struct {
	Detail *struct {
		IsBookmarked *bool  `json:"is_bookmarked"`
		Restrict     string `json:"restrict"`
		Tags         []struct {
			Name         string `json:"name"`
			IsRegistered bool   `json:"is_registered"`
		} `json:"tags"`
	} `json:"bookmark_detail"`
}

// UserArtworkBookmarks lists one user's bookmarked artworks.
func (c *Client) UserArtworkBookmarks(ctx context.Context, request UserArtworkBookmarksRequest) (sdk.Page[Artwork], error) {
	if request.UserID <= 0 {
		return sdk.Page[Artwork]{}, newError("UserArtworkBookmarks", sdk.InvalidArgument, "user ID must be positive")
	}
	if err := validateRestrict("UserArtworkBookmarks", request.Restrict); err != nil {
		return sdk.Page[Artwork]{}, err
	}
	base := url.Values{
		pixivapi.ParamUserID:   {itoa(request.UserID)},
		pixivapi.ParamRestrict: {string(request.Restrict)},
	}
	if request.Tag != "" {
		base.Set(pixivapi.ParamTag, request.Tag)
	}
	maxID, err := c.continuationPositiveValue("UserArtworkBookmarks", base, request.Cursor, pixivapi.ParamMaxBookmarkID)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	query := cloneValues(base)
	if maxID > 0 {
		query.Set(pixivapi.ParamMaxBookmarkID, itoa(maxID))
	}

	var raw artworkListWire
	if err := c.app.GetJSON(ctx, pixivapi.PathUserBookmarksIllust, query, &raw); err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "UserArtworkBookmarks")
	}
	items, err := artworkItems("UserArtworkBookmarks", raw.Illusts, false)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	next, hasNext, err := bookmarkArtworksContinuation.value(raw.NextURL)
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "UserArtworkBookmarks")
	}
	return c.artworkPage("UserArtworkBookmarks", base, pixivapi.ParamMaxBookmarkID, items, next, hasNext)
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
	base := url.Values{
		pixivapi.ParamUserID:   {itoa(request.UserID)},
		pixivapi.ParamRestrict: {string(request.Restrict)},
	}
	offset, err := c.continuationPositiveOffset("UserArtworkBookmarkTags", base, request.Cursor)
	if err != nil {
		return sdk.Page[BookmarkTag]{}, err
	}
	query := cloneValues(base)
	if offset > 0 {
		query.Set(pixivapi.ParamOffset, itoa(int64(offset)))
	}

	var raw bookmarkTagsWire
	if err := c.app.GetJSON(ctx, pixivapi.PathUserBookmarkTags, query, &raw); err != nil {
		return sdk.Page[BookmarkTag]{}, classifyAppError(err, "UserArtworkBookmarkTags")
	}
	if !raw.Tags.Present || !raw.Tags.Valid {
		return sdk.Page[BookmarkTag]{}, newError("UserArtworkBookmarkTags", sdk.MalformedUpstreamResponse, "")
	}
	items := make([]BookmarkTag, len(raw.Tags.Items))
	for index, value := range raw.Tags.Items {
		if value.Name == "" {
			return sdk.Page[BookmarkTag]{}, newError("UserArtworkBookmarkTags", sdk.MalformedUpstreamResponse, "")
		}
		items[index] = BookmarkTag{Name: value.Name, Count: value.Count}
	}
	next, hasNext, err := bookmarkTagsContinuation.offset(raw.NextURL)
	if err != nil {
		return sdk.Page[BookmarkTag]{}, classifyAppError(err, "UserArtworkBookmarkTags")
	}
	cursor, err := c.buildCursor("UserArtworkBookmarkTags", base, pixivapi.ParamOffset, int64(next), hasNext)
	if err != nil {
		return sdk.Page[BookmarkTag]{}, err
	}
	return sdk.Page[BookmarkTag]{Items: items, Next: cursor}, nil
}

// ArtworkBookmark reads the current user's bookmark detail for one artwork.
func (c *Client) ArtworkBookmark(ctx context.Context, request ArtworkBookmarkRequest) (ArtworkBookmarkDetail, error) {
	if request.ArtworkID <= 0 {
		return ArtworkBookmarkDetail{}, newError("ArtworkBookmark", sdk.InvalidArgument, "artwork ID must be positive")
	}
	var raw bookmarkDetailWire
	query := url.Values{pixivapi.ParamIllustID: {itoa(request.ArtworkID)}}
	if err := c.app.GetJSON(ctx, pixivapi.PathIllustBookmarkDetail, query, &raw); err != nil {
		// v2 对“未收藏”作品可能返回 404；SDK 的公开契约把它统一表示为
		// 空状态，因此只在该端点上转换 404，其他错误仍原样暴露。
		var failure pixivapi.Failure
		if !errors.As(err, &failure) || failure.Kind != pixivapi.FailureHTTPStatus || failure.StatusCode != http.StatusNotFound {
			return ArtworkBookmarkDetail{}, classifyAppError(err, "ArtworkBookmark")
		}
		return ArtworkBookmarkDetail{Tags: []string{}}, nil
	}
	if raw.Detail == nil || raw.Detail.IsBookmarked != nil && !*raw.Detail.IsBookmarked {
		// 未收藏响应中的 tags 可能是作品自身标签，不是收藏标签；统一归一为空状态。
		return ArtworkBookmarkDetail{Tags: []string{}}, nil
	}
	tags := []string{}
	for _, tag := range raw.Detail.Tags {
		if tag.IsRegistered {
			tags = append(tags, tag.Name)
		}
	}
	return ArtworkBookmarkDetail{Restrict: Restrict(raw.Detail.Restrict), Tags: tags}, nil
}

// UserNovelBookmarks lists one user's bookmarked novels.
func (c *Client) UserNovelBookmarks(ctx context.Context, request UserNovelBookmarksRequest) (sdk.Page[Novel], error) {
	if request.UserID <= 0 {
		return sdk.Page[Novel]{}, newError("UserNovelBookmarks", sdk.InvalidArgument, "user ID must be positive")
	}
	if err := validateRestrict("UserNovelBookmarks", request.Restrict); err != nil {
		return sdk.Page[Novel]{}, err
	}
	query := url.Values{"user_id": {itoa(request.UserID)}, "restrict": {string(request.Restrict)}}
	if request.Tag != "" {
		query.Set("tag", request.Tag)
	}
	maxID, err := c.continuationPositiveValue("UserNovelBookmarks", query, request.Cursor, "max_bookmark_id")
	if err != nil {
		return sdk.Page[Novel]{}, err
	}
	list, err := c.userNovelBookmarks.List(ctx, usernovelbookmarks.Request{
		UserID: request.UserID, Restrict: string(request.Restrict), Tag: request.Tag, MaxBookmarkID: maxID,
	})
	if err != nil {
		return sdk.Page[Novel]{}, classifyAppError(err, "UserNovelBookmarks")
	}
	return c.novelEntityPage("UserNovelBookmarks", query, "max_bookmark_id", list.Items, list.NextMaxBookmarkID, list.HasNext)
}

// UserNovelBookmarkTags lists the bookmark tags of one user's bookmarked
// novels. The candidate upstream endpoint currently has no continuation
// contract, so a non-zero cursor is rejected instead of being guessed.
func (c *Client) UserNovelBookmarkTags(ctx context.Context, request UserNovelBookmarkTagsRequest) (sdk.Page[BookmarkTag], error) {
	if request.UserID <= 0 {
		return sdk.Page[BookmarkTag]{}, newError("UserNovelBookmarkTags", sdk.InvalidArgument, "user ID must be positive")
	}
	if err := validateRestrict("UserNovelBookmarkTags", request.Restrict); err != nil {
		return sdk.Page[BookmarkTag]{}, err
	}
	if !request.Cursor.IsZero() {
		return sdk.Page[BookmarkTag]{}, newError("UserNovelBookmarkTags", sdk.InvalidCursor, "novel bookmark tags continuation is not supported")
	}
	result, err := c.userNovelBookmarks.Tags(ctx, usernovelbookmarks.TagsRequest{
		UserID: request.UserID, Restrict: string(request.Restrict),
	})
	if err != nil {
		return sdk.Page[BookmarkTag]{}, classifyAppError(err, "UserNovelBookmarkTags")
	}
	items := make([]BookmarkTag, 0, len(result.Items))
	for _, tag := range result.Items {
		items = append(items, BookmarkTag{Name: tag.Name, Count: tag.Count})
	}
	return sdk.Page[BookmarkTag]{Items: items}, nil
}

// NovelBookmark reads the current user's bookmark detail for one novel.
// The upstream route is a candidate and is kept endpoint-oriented; it does
// not expose novel metadata or mutation outcome through this method.
func (c *Client) NovelBookmark(ctx context.Context, request NovelBookmarkRequest) (NovelBookmarkDetail, error) {
	if request.NovelID <= 0 {
		return NovelBookmarkDetail{}, newError("NovelBookmark", sdk.InvalidArgument, "novel ID must be positive")
	}
	detail, err := c.userNovelBookmarks.Detail(ctx, request.NovelID)
	if err != nil {
		return NovelBookmarkDetail{}, classifyAppError(err, "NovelBookmark")
	}
	return NovelBookmarkDetail{Restrict: Restrict(detail.Restrict), Tags: append([]string{}, detail.Tags...)}, nil
}

// UserArtworkBookmarksRequest lists one user's bookmarked artworks.
type UserArtworkBookmarksRequest struct {
	UserID   int64
	Restrict Restrict
	Tag      string
	Cursor   sdk.Cursor
}

// UserArtworkBookmarkTagsRequest lists the bookmark tags of one user's
// bookmarked artworks.
type UserArtworkBookmarkTagsRequest struct {
	UserID   int64
	Restrict Restrict
	Cursor   sdk.Cursor
}

// ArtworkBookmarkRequest reads the current user's bookmark detail for one
// artwork.
type ArtworkBookmarkRequest struct {
	ArtworkID int64
}

// UserNovelBookmarksRequest lists one user's bookmarked novels.
type UserNovelBookmarksRequest struct {
	UserID   int64
	Restrict Restrict
	Tag      string
	Cursor   sdk.Cursor
}

// UserNovelBookmarkTagsRequest lists the bookmark tags of one user's
// bookmarked novels. The candidate upstream endpoint currently has no
// continuation contract; Cursor is retained so the public page shape can be
// extended without replacing this request type when that contract is frozen.
type UserNovelBookmarkTagsRequest struct {
	UserID   int64
	Restrict Restrict
	Cursor   sdk.Cursor
}

// NovelBookmarkRequest reads the current user's bookmark detail for one
// novel.
type NovelBookmarkRequest struct {
	NovelID int64
}

var bookmarkArtworksContinuation = continuationSpec{
	path:  pixivapi.PathUserBookmarksIllust,
	keys:  []string{pixivapi.ParamMaxBookmarkID},
	query: []string{pixivapi.ParamUserID, pixivapi.ParamRestrict, pixivapi.ParamTag},
}

var bookmarkTagsContinuation = continuationSpec{
	path:  pixivapi.PathUserBookmarkTags,
	keys:  []string{pixivapi.ParamOffset},
	query: []string{pixivapi.ParamUserID, pixivapi.ParamRestrict},
}
