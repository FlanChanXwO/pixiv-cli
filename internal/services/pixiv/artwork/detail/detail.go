package detail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/artwork"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

// Transport 是 detail family 所需的最小 App API transport。
type Transport interface {
	GetJSON(context.Context, string, url.Values, any) error
}

type Client struct{ transport Transport }

func New(transport Transport) *Client { return &Client{transport: transport} }

type ArtworkResult struct {
	Artwork artwork.Artwork
}

type UgoiraResult struct {
	Metadata artwork.UgoiraMetadata
}

// Artwork 读取作品详情。pages 所需的 meta_single_page/meta_pages 与详情同属
// 一个上游响应，因此不另造一个会重复请求的 pages client。
func (c *Client) Artwork(ctx context.Context, artworkID int64) (ArtworkResult, error) {
	if c == nil || c.transport == nil {
		return ArtworkResult{}, errors.New("artwork detail transport is not configured")
	}
	var raw artworkResponseDTO
	if err := c.transport.GetJSON(ctx, protocol.AppIllustDetail, url.Values{
		"illust_id": {strconv.FormatInt(artworkID, 10)},
	}, &raw); err != nil {
		return ArtworkResult{}, err
	}
	if raw.Illust == nil || raw.Illust.ID <= 0 {
		return ArtworkResult{}, protocol.MalformedResponse()
	}
	return ArtworkResult{Artwork: mapArtwork(*raw.Illust)}, nil
}

// UgoiraMetadata 读取可播放动图的压缩包与帧信息。
func (c *Client) UgoiraMetadata(ctx context.Context, artworkID int64) (UgoiraResult, error) {
	if c == nil || c.transport == nil {
		return UgoiraResult{}, errors.New("artwork detail transport is not configured")
	}
	var raw ugoiraResponseDTO
	if err := c.transport.GetJSON(ctx, protocol.AppUgoiraMetadata, url.Values{
		"illust_id": {strconv.FormatInt(artworkID, 10)},
	}, &raw); err != nil {
		return UgoiraResult{}, err
	}
	if !raw.Metadata.Present || !raw.Metadata.Valid {
		return UgoiraResult{}, protocol.MalformedResponse()
	}
	metadata := raw.Metadata.Value
	if !metadata.ZipURLs.Present || !metadata.ZipURLs.Valid ||
		(metadata.ZipURLs.Value.Medium == "" && metadata.ZipURLs.Value.Original == "") ||
		!metadata.Frames.Present || !metadata.Frames.Valid || len(metadata.Frames.Items) == 0 {
		return UgoiraResult{}, protocol.MalformedResponse()
	}
	for _, frame := range metadata.Frames.Items {
		if frame.File == "" {
			return UgoiraResult{}, protocol.MalformedResponse()
		}
	}
	return UgoiraResult{Metadata: artwork.UgoiraMetadata{
		ZipURLs: artwork.UgoiraZipURLs{
			Medium:   metadata.ZipURLs.Value.Medium,
			Original: metadata.ZipURLs.Value.Original,
		},
		Frames: mapFrames(metadata.Frames.Items),
	}}, nil
}

type artworkResponseDTO struct {
	Illust *illustDTO `json:"illust"`
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

type ugoiraResponseDTO struct {
	Metadata requiredObject[ugoiraMetadataDTO] `json:"ugoira_metadata"`
}

type ugoiraMetadataDTO struct {
	ZipURLs requiredObject[ugoiraZipURLsDTO] `json:"zip_urls"`
	Frames  requiredList[ugoiraFrameDTO]     `json:"frames"`
}

type ugoiraZipURLsDTO struct {
	Medium   string `json:"medium"`
	Original string `json:"original"`
}

type ugoiraFrameDTO struct {
	File  string `json:"file"`
	Delay int    `json:"delay"`
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
		pages[index] = artwork.MetaPage{
			PageIndex: page.PageIndex,
			Width:     page.Width,
			Height:    page.Height,
			Extension: page.Extension,
			ImageURLs: mapImageURLs(page.ImageURLs),
		}
	}
	return artwork.Artwork{
		ID:             dto.ID,
		Title:          dto.Title,
		Caption:        dto.Caption,
		Type:           dto.Type,
		PageCount:      dto.PageCount,
		TotalBookmarks: dto.TotalBookmarks,
		TotalView:      dto.TotalView,
		XRestrict:      dto.XRestrict,
		User:           mapUser(dto.User),
		Tags:           tags,
		ImageURLs:      mapImageURLs(dto.ImageURLs),
		MetaSinglePage: artwork.SinglePage{OriginalImageURL: dto.MetaSinglePage.OriginalImageURL},
		MetaPages:      pages,
		AIType:         dto.AIType,
		CreateDate:     dto.CreateDate,
		Width:          dto.Width,
		Height:         dto.Height,
		Tools:          append([]string(nil), dto.Tools...),
	}
}

func mapUser(dto userDTO) artwork.UserSummary {
	return artwork.UserSummary{
		ID:               dto.ID,
		Name:             dto.Name,
		Account:          dto.Account,
		Comment:          dto.Comment,
		IsFollowed:       dto.IsFollowed,
		ProfileImageURLs: artwork.ProfileImageURLs{Medium: dto.ProfileImageURLs.Medium},
	}
}

func mapImageURLs(dto imageURLsDTO) artwork.ImageURLs {
	return artwork.ImageURLs{SquareMedium: dto.SquareMedium, Medium: dto.Medium, Large: dto.Large, Original: dto.Original}
}

func mapFrames(values []ugoiraFrameDTO) []artwork.UgoiraFrame {
	frames := make([]artwork.UgoiraFrame, len(values))
	for index, value := range values {
		frames[index] = artwork.UgoiraFrame{File: value.File, Delay: value.Delay}
	}
	return frames
}
