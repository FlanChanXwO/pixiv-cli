package appapi

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/model"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

// v1_ext.go 承载后续分批加入的 App API 适配：评论、收藏详情、收藏标签、
// 小说详情/系列/正文、用户相关/关注者/屏蔽、AI 可见性与当前用户。

// ---- 评论 ----

type commentsDTO struct {
	Comments      []commentDTO             `json:"comments"`
	NextURL       *string                  `json:"next_url"`
	TotalComments *int64                   `json:"total_comments"`
	AccessControl *commentAccessControlDTO `json:"access_control"`
}

type commentDTO struct {
	ID            int64       `json:"id"`
	User          userDTO     `json:"user"`
	Comment       string      `json:"comment"`
	Caption       string      `json:"caption"`
	CreateDate    string      `json:"created_at"`
	ParentComment *commentDTO `json:"parent_comment"`
}

type commentAccessControlDTO struct {
	CanComment bool `json:"can_comment"`
	IsLocked   bool `json:"is_locked"`
}

// ArtworkComments 返回一个插画评论批次。next_url 只抽取 offset 数值，绝不跟随 URL。
func (c *Client) ArtworkComments(ctx context.Context, illustID int64, offset int) (*model.CommentList, error) {
	q := url.Values{"illust_id": {fmt.Sprint(illustID)}}
	setOffset(q, offset)
	return c.getComments(ctx, protocol.AppIllustComments, q)
}

// NovelComments 返回一个小说评论批次。
func (c *Client) NovelComments(ctx context.Context, novelID int64, offset int) (*model.CommentList, error) {
	q := url.Values{"novel_id": {fmt.Sprint(novelID)}}
	setOffset(q, offset)
	return c.getComments(ctx, protocol.AppNovelComments, q)
}

func (c *Client) getComments(ctx context.Context, path string, query url.Values) (*model.CommentList, error) {
	var raw commentsDTO
	if err := c.getJSONWithRetry(ctx, path, query, &raw); err != nil {
		return nil, err
	}
	for _, item := range raw.Comments {
		if !validCommentChain(item) {
			return nil, protocol.MalformedResponse()
		}
	}
	out := &model.CommentList{}
	if raw.Comments != nil {
		out.Comments = make([]model.Comment, len(raw.Comments))
	}
	for i, item := range raw.Comments {
		out.Comments[i] = mapComment(item)
	}
	if raw.TotalComments != nil {
		value := *raw.TotalComments
		out.Total = &value
	}
	if raw.AccessControl != nil {
		out.AccessControl = &model.CommentAccessControl{
			CanComment: raw.AccessControl.CanComment,
			IsLocked:   raw.AccessControl.IsLocked,
		}
	}
	if raw.NextURL != nil {
		if *raw.NextURL == "" {
			return nil, protocol.MalformedResponse()
		}
		value, err := continuationValue(*raw.NextURL, "offset")
		if err != nil {
			return nil, err
		}
		out.NextOffset, out.ContinuationExists = int(value), true
	}
	return out, nil
}

// validCommentChain 沿 parent_comment 链校验所有评论 ID 均为正数。
func validCommentChain(d commentDTO) bool {
	for {
		if d.ID <= 0 {
			return false
		}
		if d.ParentComment == nil {
			return true
		}
		d = *d.ParentComment
	}
}

// mapComment 递归映射评论及其父评论；comment 字段缺失时回退 caption。
func mapComment(d commentDTO) model.Comment {
	out := model.Comment{
		ID:         d.ID,
		User:       mapUser(d.User),
		Comment:    commentText(d),
		CreateDate: d.CreateDate,
	}
	if d.ParentComment != nil {
		parent := mapComment(*d.ParentComment)
		out.ParentComment = &parent
	}
	return out
}

func commentText(d commentDTO) string {
	if d.Comment != "" {
		return d.Comment
	}
	return d.Caption
}

// ---- 收藏详情 ----

type illustBookmarkDetailDTO struct {
	BookmarkDetail *bookmarkDetailDTO `json:"bookmark_detail"`
}

type bookmarkDetailDTO struct {
	Restrict string   `json:"restrict"`
	Tags     []string `json:"tags"`
}

// ArtworkBookmarkDetail 返回当前账号对单个作品的收藏状态。bookmark_detail 为
// null/missing 表示未收藏，返回空收藏状态而非错误。
func (c *Client) ArtworkBookmarkDetail(ctx context.Context, illustID int64) (*model.IllustBookmarkDetail, error) {
	var raw illustBookmarkDetailDTO
	if err := c.getJSONWithRetry(ctx, protocol.AppIllustBookmarkDetail, url.Values{"illust_id": {fmt.Sprint(illustID)}}, &raw); err != nil {
		return nil, err
	}
	out := &model.IllustBookmarkDetail{Tags: []string{}}
	if raw.BookmarkDetail != nil {
		out.Restrict = raw.BookmarkDetail.Restrict
		if raw.BookmarkDetail.Tags != nil {
			out.Tags = append([]string(nil), raw.BookmarkDetail.Tags...)
		}
	}
	return out, nil
}

// ---- 用户作品收藏标签 ----

type bookmarkTagsDTO struct {
	BookmarkTags []bookmarkTagDTO `json:"bookmark_tags"`
	NextURL      *string          `json:"next_url"`
}

type bookmarkTagDTO struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// UserArtworkBookmarkTags 返回一个用户作品收藏标签批次。
func (c *Client) UserArtworkBookmarkTags(ctx context.Context, userID int64, restrict string, offset int) (*model.BookmarkTagList, error) {
	q := url.Values{"user_id": {fmt.Sprint(userID)}, "restrict": {restrict}}
	setOffset(q, offset)
	var raw bookmarkTagsDTO
	if err := c.getJSONWithRetry(ctx, protocol.AppUserBookmarkTags, q, &raw); err != nil {
		return nil, err
	}
	out := &model.BookmarkTagList{}
	if raw.BookmarkTags != nil {
		out.Tags = make([]model.BookmarkTag, len(raw.BookmarkTags))
	}
	for i, item := range raw.BookmarkTags {
		if item.Name == "" {
			return nil, protocol.MalformedResponse()
		}
		out.Tags[i] = model.BookmarkTag{Name: item.Name, Count: item.Count}
	}
	if raw.NextURL != nil {
		if *raw.NextURL == "" {
			return nil, protocol.MalformedResponse()
		}
		value, err := continuationValue(*raw.NextURL, "offset")
		if err != nil {
			return nil, err
		}
		out.NextOffset, out.ContinuationExists = int(value), true
	}
	return out, nil
}

// ---- 小说详情 ----

type novelDetailDTO struct {
	Novel      *novelDTO          `json:"novel"`
	SeriesNext *novelSeriesRefDTO `json:"series_next"`
	SeriesPrev *novelSeriesRefDTO `json:"series_prev"`
}

type novelSeriesRefDTO struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

// NovelDetail 返回小说详情及其可选的前后系列引用。
func (c *Client) NovelDetail(ctx context.Context, novelID int64) (*model.NovelDetail, error) {
	var raw novelDetailDTO
	if err := c.getJSONWithRetry(ctx, protocol.AppNovelDetail, url.Values{"novel_id": {fmt.Sprint(novelID)}}, &raw); err != nil {
		return nil, err
	}
	if raw.Novel == nil || raw.Novel.ID <= 0 || raw.Novel.User.ID <= 0 {
		return nil, protocol.MalformedResponse()
	}
	out := &model.NovelDetail{Novel: mapNovel(*raw.Novel)}
	if raw.SeriesNext != nil {
		if raw.SeriesNext.ID <= 0 {
			return nil, protocol.MalformedResponse()
		}
		out.SeriesNextID = raw.SeriesNext.ID
		out.SeriesTitle = raw.SeriesNext.Title
	}
	if raw.SeriesPrev != nil {
		if raw.SeriesPrev.ID <= 0 {
			return nil, protocol.MalformedResponse()
		}
		out.SeriesPrevID = raw.SeriesPrev.ID
	}
	return out, nil
}

// ---- 小说系列 ----

type novelSeriesDTO struct {
	NovelSeriesDetail *novelSeriesDetailDTO `json:"novel_series_detail"`
	Novels            []novelDTO            `json:"novels"`
	NextURL           *string               `json:"next_url"`
}

type novelSeriesDetailDTO struct {
	ID          int64   `json:"id"`
	Title       string  `json:"title"`
	Caption     string  `json:"caption"`
	User        userDTO `json:"user"`
	IsConcluded bool    `json:"is_concluded"`
}

// NovelSeries 读取小说系列的一个批次。next_url 只抽取 last_order 数值，绝不将
// 上游 URL 作为下一跳请求目标。
func (c *Client) NovelSeries(ctx context.Context, seriesID int64, lastOrder int64) (*model.NovelSeriesResult, error) {
	q := url.Values{"series_id": {fmt.Sprint(seriesID)}}
	if lastOrder > 0 {
		q.Set("last_order", fmt.Sprint(lastOrder))
	}
	var raw novelSeriesDTO
	if err := c.getJSONWithRetry(ctx, protocol.AppNovelSeries, q, &raw); err != nil {
		return nil, err
	}
	if raw.NovelSeriesDetail == nil || raw.NovelSeriesDetail.ID <= 0 || raw.NovelSeriesDetail.User.ID <= 0 {
		return nil, protocol.MalformedResponse()
	}
	out := &model.NovelSeriesResult{
		Series: model.NovelSeries{
			ID:          raw.NovelSeriesDetail.ID,
			Title:       raw.NovelSeriesDetail.Title,
			Caption:     raw.NovelSeriesDetail.Caption,
			User:        mapUser(raw.NovelSeriesDetail.User),
			IsConcluded: raw.NovelSeriesDetail.IsConcluded,
		},
	}
	if raw.Novels != nil {
		out.Novels = make([]model.Novel, len(raw.Novels))
	}
	for i, novel := range raw.Novels {
		if novel.ID <= 0 || novel.User.ID <= 0 {
			return nil, protocol.MalformedResponse()
		}
		out.Novels[i] = mapNovel(novel)
	}
	if raw.NextURL != nil {
		if *raw.NextURL == "" {
			return nil, protocol.MalformedResponse()
		}
		value, err := continuationValueWithPolicy(*raw.NextURL, "last_order", positiveContinuation)
		if err != nil {
			return nil, err
		}
		out.NextValue, out.ContinuationExists = value, true
	}
	return out, nil
}

// ---- 小说正文 ----

// NovelContent 返回小说的原始 HTML 正文。需要 WithUserID 注入的 X-User-Id 头。
func (c *Client) NovelContent(ctx context.Context, novelID int64) ([]byte, error) {
	body, err := c.getRawWithRetry(ctx, protocol.AppNovelContent, url.Values{"novel_id": {fmt.Sprint(novelID)}})
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, protocol.MalformedResponse()
	}
	return body, nil
}

// getRawWithRetry 与 getJSONWithRetry 共享同一认证刷新与限流重试契约，但返回
// 原始响应体而不做 JSON 解码。
func (c *Client) getRawWithRetry(ctx context.Context, path string, query url.Values) ([]byte, error) {
	body, err := c.getRawWithAuthRetry(ctx, path, query)
	if c.disableRetryAfterRetry {
		return body, err
	}
	retryAfter, shouldRetry := retryAfterForRateLimit(err)
	if !shouldRetry {
		return body, err
	}
	if err := waitForRetryAfter(ctx, retryAfter); err != nil {
		return nil, protocol.Transport(err)
	}
	return c.getRawWithAuthRetry(ctx, path, query)
}

func (c *Client) getRawWithAuthRetry(ctx context.Context, path string, query url.Values) ([]byte, error) {
	body, err := c.getRaw(ctx, path, query)
	if !isAuthAPIResponse(err) {
		return body, err
	}
	if c.session == nil {
		return body, err
	}
	if refreshErr := c.session.Refresh(ctx); refreshErr != nil {
		return body, err
	}
	return c.getRaw(ctx, path, query)
}

func (c *Client) getRaw(ctx context.Context, path string, query url.Values) ([]byte, error) {
	req := c.restyClient.R().SetContext(ctx)
	req.SetHeaders(c.apiHeaders())
	if len(query) > 0 {
		req.SetQueryParamsFromValues(query)
	}
	resp, err := req.Execute(http.MethodGet, c.apiBase+path)
	if err != nil {
		return nil, protocol.Transport(err)
	}
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		retryAfter, present := parseRetryAfter(resp.Header().Get("Retry-After"), time.Now())
		return nil, protocol.HTTPStatusWithRetryAfter(resp.StatusCode(), retryAfter, present)
	}
	return resp.Body(), nil
}

// ---- 用户小说收藏 ----

// UserNovelBookmarks 返回一个用户小说收藏批次。next_url 只抽取 max_bookmark_id
// 数值作为续传值，绝不跟随 URL。
func (c *Client) UserNovelBookmarks(ctx context.Context, userID int64, restrict, tag string, maxBookmarkID int64) (*model.NovelList, error) {
	q := url.Values{"user_id": {fmt.Sprint(userID)}, "restrict": {restrict}}
	setOptional(q, "tag", tag)
	if maxBookmarkID > 0 {
		q.Set("max_bookmark_id", fmt.Sprint(maxBookmarkID))
	}
	var raw novelListDTO
	if err := c.getJSONWithRetry(ctx, protocol.AppUserNovelBookmarks, q, &raw); err != nil {
		return nil, err
	}
	if !raw.Novels.Present || !raw.Novels.Valid {
		return nil, protocol.MalformedResponse()
	}
	for _, novel := range raw.Novels.Items {
		if novel.ID <= 0 || novel.User.ID <= 0 {
			return nil, protocol.MalformedResponse()
		}
	}
	out := mapNovelList(raw)
	if raw.NextURL != nil {
		if *raw.NextURL == "" {
			return nil, protocol.MalformedResponse()
		}
		value, err := continuationValue(*raw.NextURL, "max_bookmark_id")
		if err != nil {
			return nil, err
		}
		out.ContinuationExists = true
		out.NextMaxBookmarkID = value
	}
	return &out, nil
}

// ---- 用户相关/关注者/屏蔽 ----

// RelatedUsers 返回与 seed 用户相关的用户预览批次。
func (c *Client) RelatedUsers(ctx context.Context, seedUserID int64, offset int) (*model.UserPreviewList, error) {
	q := url.Values{"seed_user_id": {fmt.Sprint(seedUserID)}}
	setOffset(q, offset)
	return c.getUserPreviewList(ctx, protocol.AppUserRelated, q)
}

// UserFollowers 返回关注指定用户的用户预览批次。
func (c *Client) UserFollowers(ctx context.Context, userID int64, restrict string, offset int) (*model.UserPreviewList, error) {
	q := url.Values{"user_id": {fmt.Sprint(userID)}, "restrict": {restrict}}
	setOffset(q, offset)
	return c.getUserPreviewList(ctx, protocol.AppUserFollower, q)
}

// UserBlockedUsers 返回指定用户屏蔽的用户预览批次。
func (c *Client) UserBlockedUsers(ctx context.Context, userID int64, offset int) (*model.UserPreviewList, error) {
	q := url.Values{"user_id": {fmt.Sprint(userID)}}
	setOffset(q, offset)
	return c.getUserPreviewList(ctx, protocol.AppUserList, q)
}

// ---- AI 作品可见性 ----

// SetAIArtworkVisibility 设置当前账号是否显示 AI 生成作品。
func (c *Client) SetAIArtworkVisibility(ctx context.Context, aiShow bool) error {
	value := "0"
	if aiShow {
		value = "1"
	}
	return c.postFormWithRetry(ctx, protocol.AppEditAIShowSettings, url.Values{"ai_show": {value}})
}

// ---- 当前用户 ----

// CurrentUser 返回当前认证账号的完整详情，envelope 与 user/detail 一致。
func (c *Client) CurrentUser(ctx context.Context) (*model.UserDetail, error) {
	var raw userDetailDTO
	if err := c.getJSONWithRetry(ctx, protocol.AppUserMe, nil, &raw); err != nil {
		return nil, err
	}
	if !raw.User.Present || !raw.User.Valid || raw.User.Value.ID <= 0 ||
		!raw.Profile.Present || !raw.Profile.Valid ||
		!raw.ProfilePublicity.Present || !raw.ProfilePublicity.Valid || !raw.ProfilePublicity.Value.valid() ||
		!raw.Workspace.Present || !raw.Workspace.Valid {
		return nil, protocol.MalformedResponse()
	}
	out := mapUserDetail(raw)
	return &out, nil
}
