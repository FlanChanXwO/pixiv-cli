package timeline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork"
	endpointcontinuation "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/continuation"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

type Transport interface {
	GetJSON(context.Context, string, url.Values, any) error
}

type Client struct{ transport Transport }

func New(transport Transport) *Client { return &Client{transport: transport} }

type Kind string

const (
	Following    Kind = "following"
	Latest       Kind = "latest"
	MyPixiv      Kind = "mypixiv"
	UserArtworks Kind = "user_artworks"
)

type Request struct {
	Kind        Kind
	Restrict    string
	ContentType string
	UserID      int64
	ArtworkType string
	Offset      int
	MaxIllustID int64
}

type Result struct {
	Items      []artwork.Artwork
	NextOffset int
	NextKey    string
	NextValue  int64
	HasNext    bool
}

func (c *Client) List(ctx context.Context, request Request) (Result, error) {
	if c == nil || c.transport == nil {
		return Result{}, errors.New("artwork timeline transport is not configured")
	}
	path, query, err := requestValues(request)
	if err != nil {
		return Result{}, err
	}
	var raw responseDTO
	if err := c.transport.GetJSON(ctx, path, query, &raw); err != nil {
		return Result{}, err
	}
	if !raw.Illusts.Present || !raw.Illusts.Valid {
		return Result{}, protocol.MalformedResponse()
	}
	items := make([]artwork.Artwork, len(raw.Illusts.Items))
	for index, value := range raw.Illusts.Items {
		if value.ID <= 0 || (request.Kind == MyPixiv && value.User.ID <= 0) {
			return Result{}, protocol.MalformedResponse()
		}
		items[index] = mapArtwork(value)
	}
	continuationKeys := []string{"offset"}
	if request.Kind == Latest {
		continuationKeys = []string{"max_illust_id", "offset"}
	}
	nextKey, nextValue, hasNext, err := continuation(raw.NextURL, path, continuationKeys)
	if err != nil {
		return Result{}, err
	}
	result := Result{Items: items, NextKey: nextKey, NextValue: nextValue, HasNext: hasNext}
	if nextKey == "offset" {
		result.NextOffset = int(nextValue)
	}
	return result, nil
}

func requestValues(request Request) (string, url.Values, error) {
	switch request.Kind {
	case Following:
		query := url.Values{"restrict": {request.Restrict}}
		setOffset(query, request.Offset)
		return protocol.AppIllustFollow, query, nil
	case Latest:
		// 最新作品的新续页目标是 max_illust_id；拒绝 offset 输入，避免把旧
		// continuation 静默降级为首页请求并造成重复数据。响应解析仍保留
		// offset 兼容分支，供后续兼容层明确处理历史响应。
		if request.Offset != 0 {
			return "", nil, errors.New("latest artwork continuation must use max_illust_id")
		}
		if request.MaxIllustID < 0 {
			return "", nil, errors.New("max illust ID must be non-negative")
		}
		contentType, err := normalizeLatestContentType(request.ContentType)
		if err != nil {
			return "", nil, err
		}
		query := url.Values{"content_type": {contentType}, "filter": {"for_android"}}
		if request.MaxIllustID > 0 {
			query.Set("max_illust_id", strconv.FormatInt(request.MaxIllustID, 10))
		}
		return protocol.AppIllustNew, query, nil
	case MyPixiv:
		query := url.Values{}
		setOffset(query, request.Offset)
		return protocol.AppIllustMyPixiv, query, nil
	case UserArtworks:
		if request.UserID <= 0 {
			return "", nil, errors.New("user artwork user ID must be positive")
		}
		artworkType, err := normalizeUserArtworkType(request.ArtworkType)
		if err != nil {
			return "", nil, err
		}
		query := url.Values{"user_id": {strconv.FormatInt(request.UserID, 10)}, "type": {artworkType}}
		setOffset(query, request.Offset)
		return protocol.AppUserIllusts, query, nil
	default:
		return "", nil, errors.New("unsupported artwork timeline kind")
	}
}

func normalizeLatestContentType(value string) (string, error) {
	// 目前只有 illust 具备该 endpoint 的确认两页证据；manga 是既有 CLI/MCP
	// 兼容输入，继续保留，但在 ugoira 或 compound subtype 的独立证据完成前拒绝它们，
	// 避免把候选能力误当成目标 contract。
	switch value {
	case "", "illust":
		return "illust", nil
	case "manga":
		return value, nil
	default:
		return "", errors.New("unsupported latest artwork content type")
	}
}

func normalizeUserArtworkType(value string) (string, error) {
	switch value {
	case "", "illustration", "illust":
		// 空值和 ArtworkKindIllustration 的既有拼写都使用默认的 illust。
		return "illust", nil
	case "manga", "ugoira":
		return value, nil
	default:
		return "", errors.New("unsupported user artwork subtype")
	}
}

func setOffset(query url.Values, offset int) {
	if offset > 0 {
		query.Set("offset", strconv.Itoa(offset))
	}
}

type responseDTO struct {
	Illusts requiredList[illustDTO] `json:"illusts"`
	NextURL *string                 `json:"next_url"`
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
	PageIndex int          `json:"page_index"`
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

func continuation(rawURL *string, path string, keys []string) (string, int64, bool, error) {
	if rawURL == nil {
		return "", 0, false, nil
	}
	key, value, err := endpointcontinuation.Parse(*rawURL, endpointcontinuation.Spec{
		Path:             path,
		Keys:             keys,
		AllowedQueryKeys: allowedContinuationQueryKeys(path),
	})
	if err != nil || value <= 0 || (key == "offset" && int64(int(value)) != value) {
		return "", 0, false, protocol.MalformedResponse()
	}
	return key, value, true, nil
}

func allowedContinuationQueryKeys(path string) []string {
	switch path {
	case protocol.AppIllustFollow:
		return []string{"restrict"}
	case protocol.AppIllustNew:
		return []string{"content_type", "filter"}
	case protocol.AppUserIllusts:
		return []string{"user_id", "type"}
	default:
		return nil
	}
}

func mapArtwork(dto illustDTO) artwork.Artwork {
	tags := make([]artwork.Tag, len(dto.Tags))
	for index, tag := range dto.Tags {
		tags[index] = artwork.Tag{Name: tag.Name, TranslatedName: tag.TranslatedName}
	}
	pages := make([]artwork.MetaPage, len(dto.MetaPages))
	for index, page := range dto.MetaPages {
		pages[index] = artwork.MetaPage{PageIndex: page.PageIndex, Width: page.Width, Height: page.Height, Extension: page.Extension, ImageURLs: mapImageURLs(page.ImageURLs)}
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
