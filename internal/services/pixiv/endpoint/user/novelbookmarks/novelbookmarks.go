package novelbookmarks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	endpointcontinuation "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/continuation"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/novel"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

// Transport 是小说收藏 family 所需的最小 App API 读写传输能力。
type Transport interface {
	GetJSON(context.Context, string, url.Values, any) error
	PostForm(context.Context, string, url.Values) error
}

type Request struct {
	UserID        int64
	Restrict      string
	Tag           string
	MaxBookmarkID int64
}

type Result struct {
	Items             []novel.Novel
	NextMaxBookmarkID int64
	HasNext           bool
}

type Client struct{ transport Transport }

func New(transport Transport) *Client { return &Client{transport: transport} }

// List 读取用户小说收藏，并只把 next_url 中的 max_bookmark_id 作为续传值。
func (c *Client) List(ctx context.Context, request Request) (Result, error) {
	if c == nil || c.transport == nil {
		return Result{}, errors.New("novel bookmark transport is not configured")
	}
	query := url.Values{
		"user_id":  {strconv.FormatInt(request.UserID, 10)},
		"restrict": {request.Restrict},
	}
	if request.Tag != "" {
		query.Set("tag", request.Tag)
	}
	if request.MaxBookmarkID > 0 {
		query.Set("max_bookmark_id", strconv.FormatInt(request.MaxBookmarkID, 10))
	}
	var raw responseDTO
	if err := c.transport.GetJSON(ctx, protocol.AppUserNovelBookmarks, query, &raw); err != nil {
		return Result{}, err
	}
	if !raw.Novels.Present || !raw.Novels.Valid {
		return Result{}, protocol.MalformedResponse()
	}
	items := make([]novel.Novel, len(raw.Novels.Items))
	for index, value := range raw.Novels.Items {
		if value.ID <= 0 || value.User.ID <= 0 {
			return Result{}, protocol.MalformedResponse()
		}
		items[index] = mapNovel(value)
	}
	result := Result{Items: items}
	if raw.NextURL != nil {
		if *raw.NextURL == "" {
			return Result{}, protocol.MalformedResponse()
		}
		next, err := continuation(*raw.NextURL)
		if err != nil {
			return Result{}, err
		}
		result.NextMaxBookmarkID, result.HasNext = next, true
	}
	return result, nil
}

// Tags 是 novel bookmark tags 的内部 candidate adapter。
// 当前 snapshot 不声明续页语义；非 null next_url 必须显式失败，避免丢失数据。
type TagsRequest struct {
	UserID   int64
	Restrict string
}

type BookmarkTag struct {
	Name  string
	Count int
}

type TagsResult struct {
	Items []BookmarkTag
}

func (c *Client) Tags(ctx context.Context, request TagsRequest) (TagsResult, error) {
	if c == nil || c.transport == nil {
		return TagsResult{}, errors.New("novel bookmark transport is not configured")
	}
	var raw tagsResponseDTO
	if err := c.transport.GetJSON(ctx, protocol.AppUserNovelBookmarkTags, url.Values{
		"user_id":  {strconv.FormatInt(request.UserID, 10)},
		"restrict": {request.Restrict},
	}, &raw); err != nil {
		return TagsResult{}, err
	}
	if !raw.Tags.Present || !raw.Tags.Valid || raw.NextURL != nil {
		return TagsResult{}, protocol.MalformedResponse()
	}
	items := make([]BookmarkTag, len(raw.Tags.Items))
	for index, value := range raw.Tags.Items {
		if value.Name == "" {
			return TagsResult{}, protocol.MalformedResponse()
		}
		items[index] = BookmarkTag{Name: value.Name, Count: value.Count}
	}
	return TagsResult{Items: items}, nil
}

// BookmarkDetail 是 novel bookmark detail candidate 的 normalized 状态。
// 该 candidate 尚未通过 live wire/SDK gate，不在此处形成 public operation。
type BookmarkDetail struct {
	Restrict string
	Tags     []string
}

func (c *Client) Detail(ctx context.Context, novelID int64) (BookmarkDetail, error) {
	if c == nil || c.transport == nil {
		return BookmarkDetail{}, errors.New("novel bookmark transport is not configured")
	}
	var raw detailResponseDTO
	if err := c.transport.GetJSON(ctx, protocol.AppNovelBookmarkDetail, url.Values{
		"novel_id": {strconv.FormatInt(novelID, 10)},
	}, &raw); err != nil {
		var failure protocol.Failure
		if errors.As(err, &failure) && failure.Kind == protocol.FailureHTTPStatus && failure.StatusCode == http.StatusNotFound {
			return BookmarkDetail{Tags: []string{}}, nil
		}
		return BookmarkDetail{}, err
	}
	if raw.Detail == nil {
		return BookmarkDetail{Tags: []string{}}, nil
	}
	if raw.Detail.IsBookmarked != nil && !*raw.Detail.IsBookmarked {
		if raw.Detail.Restrict != "" || len(raw.Detail.Tags) != 0 {
			return BookmarkDetail{}, protocol.MalformedResponse()
		}
		return BookmarkDetail{Tags: []string{}}, nil
	}
	tags := []string{}
	for _, tag := range raw.Detail.Tags {
		tags = append(tags, tag.Name)
	}
	return BookmarkDetail{Restrict: raw.Detail.Restrict, Tags: tags}, nil
}

// AddRequest 描述 novel bookmark add candidate 的请求参数。
// 该请求仍未通过 live wire/SDK gate，不构成 public operation。
type AddRequest struct {
	NovelID  int64
	Restrict string
	Tags     []string
}

// Add 写入 novel bookmark add candidate，并原样传播传输层错误。
// 2xx/空响应只代表 status-only transport 成功，不能作为收藏状态已改变的证明。
func (c *Client) Add(ctx context.Context, request AddRequest) error {
	if c == nil || c.transport == nil {
		return errors.New("novel bookmark transport is not configured")
	}
	if request.NovelID <= 0 {
		return errors.New("bookmark novel ID must be positive")
	}
	if request.Restrict != "public" && request.Restrict != "private" {
		return errors.New("bookmark restrict must be public or private")
	}
	form := url.Values{"novel_id": {strconv.FormatInt(request.NovelID, 10)}, "restrict": {request.Restrict}}
	for _, tag := range request.Tags {
		form.Add("tags[]", tag)
	}
	return c.transport.PostForm(ctx, protocol.AppNovelBookmarkAdd, form)
}

// Remove 写入 novel bookmark delete candidate，并原样传播传输层错误。
// 删除后的 detail/list/tags 读回与状态恢复仍由后续验证任务负责。
func (c *Client) Remove(ctx context.Context, novelID int64) error {
	if c == nil || c.transport == nil {
		return errors.New("novel bookmark transport is not configured")
	}
	if novelID <= 0 {
		return errors.New("bookmark novel ID must be positive")
	}
	return c.transport.PostForm(ctx, protocol.AppNovelBookmarkDelete, url.Values{
		"novel_id": {strconv.FormatInt(novelID, 10)},
	})
}

type responseDTO struct {
	Novels  requiredList[novelDTO] `json:"novels"`
	NextURL *string                `json:"next_url"`
}

type tagsResponseDTO struct {
	Tags    requiredList[bookmarkTagDTO] `json:"bookmark_tags"`
	NextURL *string                      `json:"next_url"`
}

type bookmarkTagDTO struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type detailResponseDTO struct {
	Detail *bookmarkDetailDTO `json:"bookmark_detail"`
}

type bookmarkDetailDTO struct {
	IsBookmarked *bool                  `json:"is_bookmarked"`
	Restrict     string                 `json:"restrict"`
	Tags         []bookmarkDetailTagDTO `json:"tags"`
}

type bookmarkDetailTagDTO struct {
	Name         string `json:"name"`
	IsRegistered bool   `json:"is_registered"`
}

type novelDTO struct {
	ID             int64        `json:"id"`
	Title          string       `json:"title"`
	Caption        string       `json:"caption"`
	XRestrict      *int         `json:"x_restrict"`
	TextLength     *int         `json:"text_length"`
	IsOriginal     *bool        `json:"is_original"`
	User           userDTO      `json:"user"`
	Tags           []tagDTO     `json:"tags"`
	ImageURLs      imageURLsDTO `json:"image_urls"`
	CreateDate     string       `json:"create_date"`
	TotalBookmarks int          `json:"total_bookmarks"`
	TotalView      int          `json:"total_view"`
}

type userDTO struct {
	ID               int64               `json:"id"`
	Name             string              `json:"name"`
	Account          string              `json:"account"`
	Comment          string              `json:"comment"`
	IsFollowed       bool                `json:"is_followed"`
	ProfileImageURLs profileImageURLsDTO `json:"profile_image_urls"`
}

type profileImageURLsDTO struct {
	Medium *string `json:"medium"`
}

type tagDTO struct {
	Name           string `json:"name"`
	TranslatedName string `json:"translated_name"`
}

type imageURLsDTO struct {
	SquareMedium string `json:"square_medium"`
	Medium       string `json:"medium"`
	Large        string `json:"large"`
	Original     string `json:"original"`
}

type requiredList[T any] struct {
	Items   []T
	Present bool
	Valid   bool
}

func (l *requiredList[T]) UnmarshalJSON(data []byte) error {
	*l = requiredList[T]{Present: true}
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil
	}
	if err := json.Unmarshal(data, &l.Items); err != nil {
		return err
	}
	l.Valid = true
	return nil
}

func mapNovel(value novelDTO) novel.Novel {
	return novel.Novel{
		ID:             value.ID,
		Title:          value.Title,
		Caption:        value.Caption,
		XRestrict:      intValue(value.XRestrict),
		TextLength:     intValue(value.TextLength),
		IsOriginal:     boolValue(value.IsOriginal),
		User:           mapUser(value.User),
		Tags:           mapTags(value.Tags),
		ImageURLs:      mapImageURLs(value.ImageURLs),
		CreateDate:     value.CreateDate,
		TotalBookmarks: value.TotalBookmarks,
		TotalView:      value.TotalView,
	}
}

func mapUser(value userDTO) novel.UserSummary {
	return novel.UserSummary{
		ID:               value.ID,
		Name:             value.Name,
		Account:          value.Account,
		Comment:          value.Comment,
		IsFollowed:       value.IsFollowed,
		ProfileImageURLs: novel.ProfileImageURLs{Medium: cloneString(value.ProfileImageURLs.Medium)},
	}
}

func mapTags(values []tagDTO) []novel.Tag {
	if values == nil {
		return nil
	}
	result := make([]novel.Tag, len(values))
	for index, value := range values {
		result[index] = novel.Tag{Name: value.Name, TranslatedName: value.TranslatedName}
	}
	return result
}

func mapImageURLs(value imageURLsDTO) novel.ImageURLs {
	return novel.ImageURLs{SquareMedium: value.SquareMedium, Medium: value.Medium, Large: value.Large, Original: value.Original}
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func boolValue(value *bool) bool { return value != nil && *value }

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func continuation(rawURL string) (int64, error) {
	_, value, err := endpointcontinuation.Parse(rawURL, endpointcontinuation.Spec{
		Path:             protocol.AppUserNovelBookmarks,
		Keys:             []string{"max_bookmark_id"},
		AllowedQueryKeys: []string{"user_id", "restrict", "tag"},
	})
	if err != nil {
		return 0, err
	}
	return value, nil
}
