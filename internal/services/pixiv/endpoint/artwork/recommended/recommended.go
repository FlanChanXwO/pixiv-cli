package recommended

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork"
	endpointcontinuation "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/continuation"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

type Transport interface {
	GetJSON(context.Context, string, url.Values, any) error
}

type Client struct{ transport Transport }

func New(transport Transport) *Client { return &Client{transport: transport} }

type Request struct {
	ContentType string
	// ContinuationParams 非空表示续页：整体回放上游 next_url 给出的多参数
	// 集合（offset、bookmark 游标、viewed 下标数组等）；nil 表示首页。
	ContinuationParams url.Values
}

type Result struct {
	Items []artwork.Artwork
	// NextParams 是上游 next_url 的完整查询参数集，HasNext 为 true 时非空。
	NextParams url.Values
	HasNext    bool
}

func (c *Client) List(ctx context.Context, request Request) (Result, error) {
	if c == nil || c.transport == nil {
		return Result{}, errors.New("artwork recommended transport is not configured")
	}
	if err := validateRequest(request); err != nil {
		return Result{}, err
	}
	var raw responseDTO
	if err := c.transport.GetJSON(ctx, protocol.AppIllustRecommended, request.ContinuationParams, &raw); err != nil {
		return Result{}, err
	}
	if !raw.Illusts.Present || !raw.Illusts.Valid {
		return Result{}, protocol.MalformedResponse()
	}
	items := make([]artwork.Artwork, len(raw.Illusts.Items))
	for index, value := range raw.Illusts.Items {
		if value.ID <= 0 {
			return Result{}, protocol.MalformedResponse()
		}
		items[index] = mapArtwork(value)
	}
	nextParams, hasNext, err := continuation(raw.NextURL)
	if err != nil {
		return Result{}, err
	}
	return Result{Items: items, NextParams: nextParams, HasNext: hasNext}, nil
}

func validateRequest(request Request) error {
	// recommended 的 subtype 仍是 candidate；未有独立两页证据前，不把它透传成已支持能力。
	if request.ContentType != "" {
		return errors.New("artwork recommended content type is unsupported")
	}
	// 续页参数只能整体来自上游 next_url 的回放（由 SDK 从 cursor 解出），
	// 不接受调用方自拼的部分参数；空集合与首页等价，由 nil 表示首页。
	if request.ContinuationParams != nil && len(request.ContinuationParams) == 0 {
		return errors.New("artwork recommended continuation params are invalid")
	}
	return nil
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

func continuation(rawURL *string) (url.Values, bool, error) {
	if rawURL == nil {
		return nil, false, nil
	}
	// live 证据（G1-T28）：next_url 中的 viewed[] 是上游会话参数，回放会被
	// 400 拒绝；提取续页参数集时剔除，其余参数（offset/bookmark 游标/include
	// flags）原样回放。
	params, hasNext, err := endpointcontinuation.ParseParams(*rawURL, endpointcontinuation.Spec{
		Path:               protocol.AppIllustRecommended,
		Keys:               []string{"offset", "min_bookmark_id_for_recent_illust", "max_bookmark_id_for_recommend", "include_ranking_illusts", "include_privacy_policy"},
		AllowZero:          true,
		IgnoredKeyPrefixes: []string{"viewed["},
	})
	if err != nil || !hasNext {
		return nil, false, protocol.MalformedResponse()
	}
	return params, true, nil
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
