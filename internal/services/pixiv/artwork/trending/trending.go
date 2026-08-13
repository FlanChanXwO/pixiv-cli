package trending

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/artwork"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

type Transport interface {
	GetJSON(context.Context, string, url.Values, any) error
}

type Client struct{ transport Transport }

func New(transport Transport) *Client { return &Client{transport: transport} }

func (c *Client) List(ctx context.Context) ([]artwork.TrendingTag, error) {
	if c == nil || c.transport == nil {
		return nil, errors.New("artwork trending transport is not configured")
	}
	var raw responseDTO
	if err := c.transport.GetJSON(ctx, protocol.AppTrendingTagsIllust, nil, &raw); err != nil {
		return nil, err
	}
	if !raw.TrendTags.Present || !raw.TrendTags.Valid {
		return nil, protocol.MalformedResponse()
	}
	result := make([]artwork.TrendingTag, len(raw.TrendTags.Items))
	for index, item := range raw.TrendTags.Items {
		if item.Tag == "" || !item.Illust.Present || !item.Illust.Valid || item.Illust.Value.ID <= 0 {
			return nil, protocol.MalformedResponse()
		}
		result[index] = artwork.TrendingTag{
			Tag:            item.Tag,
			TranslatedName: item.TranslatedName,
			Artwork:        mapArtwork(item.Illust.Value),
		}
	}
	return result, nil
}

type responseDTO struct {
	TrendTags requiredList[trendTagDTO] `json:"trend_tags"`
}

type trendTagDTO struct {
	Tag            string                    `json:"tag"`
	TranslatedName string                    `json:"translated_name"`
	Illust         requiredObject[illustDTO] `json:"illust"`
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

type requiredObject[T any] struct {
	Value   T
	Present bool
	Valid   bool
}

func (o *requiredObject[T]) UnmarshalJSON(data []byte) error {
	*o = requiredObject[T]{Present: true}
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		return nil
	}
	if len(data) == 0 || data[0] != '{' {
		return json.Unmarshal(data, &o.Value)
	}
	if err := json.Unmarshal(data, &o.Value); err != nil {
		return err
	}
	o.Valid = true
	return nil
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
