package bookmark

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

type Transport interface {
	GetJSON(context.Context, string, url.Values, any) error
	PostForm(context.Context, string, url.Values) error
}

type Client struct{ transport Transport }

func New(transport Transport) *Client { return &Client{transport: transport} }

type ArtworksRequest struct {
	UserID        int64
	Restrict      string
	Tag           string
	MaxBookmarkID int64
}

type ArtworksResult struct {
	Items             []artwork.Artwork
	NextMaxBookmarkID int64
	HasNext           bool
}

func (c *Client) Artworks(ctx context.Context, request ArtworksRequest) (ArtworksResult, error) {
	if c == nil || c.transport == nil {
		return ArtworksResult{}, errors.New("artwork bookmark transport is not configured")
	}
	query := url.Values{"user_id": {strconv.FormatInt(request.UserID, 10)}, "restrict": {request.Restrict}}
	if request.Tag != "" {
		query.Set("tag", request.Tag)
	}
	if request.MaxBookmarkID > 0 {
		query.Set("max_bookmark_id", strconv.FormatInt(request.MaxBookmarkID, 10))
	}
	var raw artworksResponseDTO
	if err := c.transport.GetJSON(ctx, protocol.AppUserBookmarks, query, &raw); err != nil {
		return ArtworksResult{}, err
	}
	if !raw.Illusts.Present || !raw.Illusts.Valid {
		return ArtworksResult{}, protocol.MalformedResponse()
	}
	items := make([]artwork.Artwork, len(raw.Illusts.Items))
	for index, value := range raw.Illusts.Items {
		if value.ID <= 0 {
			return ArtworksResult{}, protocol.MalformedResponse()
		}
		items[index] = mapArtwork(value)
	}
	next, hasNext, err := continuation(raw.NextURL, "max_bookmark_id", false)
	if err != nil {
		return ArtworksResult{}, err
	}
	return ArtworksResult{Items: items, NextMaxBookmarkID: next, HasNext: hasNext}, nil
}

type TagsRequest struct {
	UserID   int64
	Restrict string
	Offset   int
}

type TagsResult struct {
	Items      []artwork.BookmarkTag
	NextOffset int
	HasNext    bool
}

func (c *Client) Tags(ctx context.Context, request TagsRequest) (TagsResult, error) {
	if c == nil || c.transport == nil {
		return TagsResult{}, errors.New("artwork bookmark transport is not configured")
	}
	query := url.Values{"user_id": {strconv.FormatInt(request.UserID, 10)}, "restrict": {request.Restrict}}
	if request.Offset > 0 {
		query.Set("offset", strconv.Itoa(request.Offset))
	}
	var raw tagsResponseDTO
	if err := c.transport.GetJSON(ctx, protocol.AppUserBookmarkTags, query, &raw); err != nil {
		return TagsResult{}, err
	}
	items := make([]artwork.BookmarkTag, len(raw.Tags))
	for index, value := range raw.Tags {
		if value.Name == "" {
			return TagsResult{}, protocol.MalformedResponse()
		}
		items[index] = artwork.BookmarkTag{Name: value.Name, Count: value.Count}
	}
	next, hasNext, err := continuation(raw.NextURL, "offset", true)
	if err != nil {
		return TagsResult{}, err
	}
	return TagsResult{Items: items, NextOffset: int(next), HasNext: hasNext}, nil
}

func (c *Client) Detail(ctx context.Context, artworkID int64) (artwork.BookmarkDetail, error) {
	if c == nil || c.transport == nil {
		return artwork.BookmarkDetail{}, errors.New("artwork bookmark transport is not configured")
	}
	var raw detailResponseDTO
	if err := c.transport.GetJSON(ctx, protocol.AppIllustBookmarkDetail, url.Values{
		"illust_id": {strconv.FormatInt(artworkID, 10)},
	}, &raw); err != nil {
		var failure protocol.Failure
		// v2 对“未收藏”作品可能返回 404；SDK 的公开契约把它统一表示为
		// 空状态，因此只在该端点上转换 404，其他错误仍原样暴露。
		if errors.As(err, &failure) && failure.Kind == protocol.FailureHTTPStatus && failure.StatusCode == http.StatusNotFound {
			return artwork.BookmarkDetail{Tags: []string{}}, nil
		}
		return artwork.BookmarkDetail{}, err
	}
	if raw.Detail == nil {
		return artwork.BookmarkDetail{Tags: []string{}}, nil
	}
	if raw.Detail.IsBookmarked != nil && !*raw.Detail.IsBookmarked {
		return artwork.BookmarkDetail{Tags: []string{}}, nil
	}
	tags := []string{}
	if raw.Detail.Tags != nil {
		for _, tag := range raw.Detail.Tags {
			tags = append(tags, tag.Name)
		}
	}
	return artwork.BookmarkDetail{Restrict: raw.Detail.Restrict, Tags: tags}, nil
}

type AddRequest struct {
	ArtworkID int64
	Restrict  string
	Tags      []string
}

func (c *Client) Add(ctx context.Context, request AddRequest) error {
	if c == nil || c.transport == nil {
		return errors.New("artwork bookmark transport is not configured")
	}
	form := url.Values{"illust_id": {strconv.FormatInt(request.ArtworkID, 10)}, "restrict": {request.Restrict}}
	for _, tag := range request.Tags {
		form.Add("tags[]", tag)
	}
	return c.transport.PostForm(ctx, protocol.AppBookmarkAdd, form)
}

func (c *Client) Remove(ctx context.Context, artworkID int64) error {
	if c == nil || c.transport == nil {
		return errors.New("artwork bookmark transport is not configured")
	}
	return c.transport.PostForm(ctx, protocol.AppBookmarkDelete, url.Values{
		"illust_id": {strconv.FormatInt(artworkID, 10)},
	})
}

type artworksResponseDTO struct {
	Illusts requiredList[illustDTO] `json:"illusts"`
	NextURL *string                 `json:"next_url"`
}

type tagsResponseDTO struct {
	Tags    []bookmarkTagDTO `json:"bookmark_tags"`
	NextURL *string          `json:"next_url"`
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

type illustDTO struct {
	ID             int64         `json:"id"`
	Title          string        `json:"title"`
	Caption        string        `json:"caption"`
	Type           string        `json:"type"`
	PageCount      int           `json:"page_count"`
	TotalBookmarks int           `json:"total_bookmarks"`
	TotalView      int           `json:"total_view"`
	XRestrict      int           `json:"x_restrict"`
	User           userDTO       `json:"user"`
	Tags           []tagDTO      `json:"tags"`
	ImageURLs      imageURLsDTO  `json:"image_urls"`
	MetaSinglePage singlePageDTO `json:"meta_single_page"`
	MetaPages      []metaPageDTO `json:"meta_pages"`
	AIType         int           `json:"-"`
	CreateDate     string        `json:"create_date"`
	Width          int           `json:"width"`
	Height         int           `json:"height"`
	Tools          []string      `json:"tools"`
}

func (d *illustDTO) UnmarshalJSON(data []byte) error {
	type wire illustDTO
	aux := struct {
		*wire
		IllustAIType *int `json:"illust_ai_type"`
		LegacyAIType *int `json:"ai_type"`
	}{wire: (*wire)(d)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	switch {
	case aux.IllustAIType != nil:
		d.AIType = *aux.IllustAIType
	case aux.LegacyAIType != nil:
		d.AIType = *aux.LegacyAIType
	default:
		d.AIType = 0
	}
	return nil
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

type singlePageDTO struct {
	OriginalImageURL string `json:"original_image_url"`
}

type metaPageDTO struct {
	Width     int          `json:"width"`
	Height    int          `json:"height"`
	Extension string       `json:"extension"`
	ImageURLs imageURLsDTO `json:"image_urls"`
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

func continuation(rawURL *string, key string, requireInt bool) (int64, bool, error) {
	if rawURL == nil {
		return 0, false, nil
	}
	if *rawURL == "" {
		return 0, false, protocol.MalformedResponse()
	}
	parsed, err := url.Parse(*rawURL)
	if err != nil {
		return 0, false, protocol.MalformedResponse()
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(values[key]) != 1 {
		return 0, false, protocol.MalformedResponse()
	}
	value, err := strconv.ParseInt(values.Get(key), 10, 64)
	if err != nil || value <= 0 || requireInt && int64(int(value)) != value {
		return 0, false, protocol.MalformedResponse()
	}
	return value, true, nil
}

func mapArtwork(dto illustDTO) artwork.Artwork {
	tags := make([]artwork.Tag, len(dto.Tags))
	for index, tag := range dto.Tags {
		tags[index] = artwork.Tag{Name: tag.Name, TranslatedName: tag.TranslatedName}
	}
	pages := make([]artwork.MetaPage, len(dto.MetaPages))
	for index, page := range dto.MetaPages {
		pages[index] = artwork.MetaPage{PageIndex: index, Width: page.Width, Height: page.Height, Extension: page.Extension, ImageURLs: mapImageURLs(page.ImageURLs)}
	}
	return artwork.Artwork{ID: dto.ID, Title: dto.Title, Caption: dto.Caption, Type: dto.Type,
		PageCount: dto.PageCount, TotalBookmarks: dto.TotalBookmarks, TotalView: dto.TotalView, XRestrict: dto.XRestrict,
		User: mapUser(dto.User), Tags: tags, ImageURLs: mapImageURLs(dto.ImageURLs),
		MetaSinglePage: artwork.SinglePage{OriginalImageURL: dto.MetaSinglePage.OriginalImageURL}, MetaPages: pages,
		AIType: dto.AIType, CreateDate: dto.CreateDate, Width: dto.Width, Height: dto.Height, Tools: append([]string(nil), dto.Tools...)}
}

func mapUser(dto userDTO) artwork.UserSummary {
	return artwork.UserSummary{ID: dto.ID, Name: dto.Name, Account: dto.Account, Comment: dto.Comment, IsFollowed: dto.IsFollowed,
		ProfileImageURLs: artwork.ProfileImageURLs{Medium: dto.ProfileImageURLs.Medium}}
}

func mapImageURLs(dto imageURLsDTO) artwork.ImageURLs {
	return artwork.ImageURLs{SquareMedium: dto.SquareMedium, Medium: dto.Medium, Large: dto.Large, Original: dto.Original}
}
