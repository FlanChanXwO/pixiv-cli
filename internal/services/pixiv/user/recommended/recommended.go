package recommended

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/artwork"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/novel"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/user"
)

type Transport interface {
	GetJSON(context.Context, string, url.Values, any) error
}

type Request struct {
	Offset             int
	ContinuationExists bool
}

type Result struct {
	Items      []user.Preview
	NextOffset int
	HasNext    bool
}

type Client struct{ transport Transport }

func New(transport Transport) *Client { return &Client{transport: transport} }

func (c *Client) List(ctx context.Context, request Request) (Result, error) {
	if c == nil || c.transport == nil {
		return Result{}, errors.New("user recommended transport is not configured")
	}
	query := url.Values{}
	if request.ContinuationExists {
		query.Set("offset", strconv.Itoa(request.Offset))
	} else if request.Offset > 0 {
		query.Set("offset", strconv.Itoa(request.Offset))
	}
	var raw responseDTO
	if err := c.transport.GetJSON(ctx, protocol.AppUserRecommended, query, &raw); err != nil {
		return Result{}, err
	}
	if !raw.Users.Present || !raw.Users.Valid {
		return Result{}, protocol.MalformedResponse()
	}
	items := make([]user.Preview, len(raw.Users.Items))
	for index, value := range raw.Users.Items {
		if value.User.ID <= 0 {
			return Result{}, protocol.MalformedResponse()
		}
		preview := user.Preview{User: mapUser(value.User)}
		preview.Illusts = make([]artwork.Artwork, len(value.Illusts))
		for nestedIndex, illust := range value.Illusts {
			if illust.ID <= 0 || illust.User.ID <= 0 {
				return Result{}, protocol.MalformedResponse()
			}
			preview.Illusts[nestedIndex] = mapArtwork(illust)
		}
		preview.Novels = make([]novel.Novel, len(value.Novels))
		for nestedIndex, item := range value.Novels {
			if item.ID <= 0 || item.User.ID <= 0 {
				return Result{}, protocol.MalformedResponse()
			}
			preview.Novels[nestedIndex] = mapNovel(item)
		}
		items[index] = preview
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
		result.NextOffset, result.HasNext = next, true
	}
	return result, nil
}

type responseDTO struct {
	Users   requiredList[previewDTO] `json:"user_previews"`
	NextURL *string                  `json:"next_url"`
}
type previewDTO struct {
	User    userDTO     `json:"user"`
	Illusts []illustDTO `json:"illusts"`
	Novels  []novelDTO  `json:"novels"`
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
	if aux.IllustAIType != nil {
		d.AIType = *aux.IllustAIType
	} else if aux.LegacyAIType != nil {
		d.AIType = *aux.LegacyAIType
	}
	return nil
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
func mapUser(value userDTO) user.User {
	return user.User{ID: value.ID, Name: value.Name, Account: value.Account, Comment: value.Comment, IsFollowed: value.IsFollowed, ProfileImageURLs: user.ProfileImageURLs{Medium: cloneString(value.ProfileImageURLs.Medium)}}
}
func mapArtwork(value illustDTO) artwork.Artwork {
	return artwork.Artwork{ID: value.ID, Title: value.Title, Caption: value.Caption, Type: value.Type, PageCount: value.PageCount, TotalBookmarks: value.TotalBookmarks, TotalView: value.TotalView, XRestrict: value.XRestrict, User: mapArtworkUser(value.User), Tags: mapArtworkTags(value.Tags), ImageURLs: mapImageURLs(value.ImageURLs), MetaSinglePage: artwork.SinglePage{OriginalImageURL: value.MetaSinglePage.OriginalImageURL}, MetaPages: mapMetaPages(value.MetaPages), AIType: value.AIType, CreateDate: value.CreateDate, Width: value.Width, Height: value.Height, Tools: append([]string(nil), value.Tools...)}
}
func mapArtworkUser(value userDTO) artwork.UserSummary {
	return artwork.UserSummary{ID: value.ID, Name: value.Name, Account: value.Account, Comment: value.Comment, IsFollowed: value.IsFollowed, ProfileImageURLs: artwork.ProfileImageURLs{Medium: cloneString(value.ProfileImageURLs.Medium)}}
}
func mapArtworkTags(values []tagDTO) []artwork.Tag {
	if values == nil {
		return nil
	}
	result := make([]artwork.Tag, len(values))
	for index, value := range values {
		result[index] = artwork.Tag{Name: value.Name, TranslatedName: value.TranslatedName}
	}
	return result
}
func mapMetaPages(values []metaPageDTO) []artwork.MetaPage {
	if values == nil {
		return nil
	}
	result := make([]artwork.MetaPage, len(values))
	for index, value := range values {
		result[index] = artwork.MetaPage{PageIndex: value.PageIndex, Width: value.Width, Height: value.Height, Extension: value.Extension, ImageURLs: mapImageURLs(value.ImageURLs)}
	}
	return result
}
func mapImageURLs(value imageURLsDTO) artwork.ImageURLs {
	return artwork.ImageURLs{SquareMedium: value.SquareMedium, Medium: value.Medium, Large: value.Large, Original: value.Original}
}
func mapNovel(value novelDTO) novel.Novel {
	return novel.Novel{ID: value.ID, Title: value.Title, Caption: value.Caption, XRestrict: intValue(value.XRestrict), TextLength: intValue(value.TextLength), IsOriginal: boolValue(value.IsOriginal), User: mapNovelUser(value.User), Tags: mapNovelTags(value.Tags), ImageURLs: mapNovelImageURLs(value.ImageURLs), CreateDate: value.CreateDate, TotalBookmarks: value.TotalBookmarks, TotalView: value.TotalView}
}
func mapNovelUser(value userDTO) novel.UserSummary {
	return novel.UserSummary{ID: value.ID, Name: value.Name, Account: value.Account, Comment: value.Comment, IsFollowed: value.IsFollowed, ProfileImageURLs: novel.ProfileImageURLs{Medium: cloneString(value.ProfileImageURLs.Medium)}}
}
func mapNovelTags(values []tagDTO) []novel.Tag {
	if values == nil {
		return nil
	}
	result := make([]novel.Tag, len(values))
	for index, value := range values {
		result[index] = novel.Tag{Name: value.Name, TranslatedName: value.TranslatedName}
	}
	return result
}
func mapNovelImageURLs(value imageURLsDTO) novel.ImageURLs {
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
func continuation(rawURL string) (int, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return 0, protocol.MalformedResponse()
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(values["offset"]) != 1 {
		return 0, protocol.MalformedResponse()
	}
	value, err := strconv.ParseInt(values.Get("offset"), 10, 64)
	if err != nil || value < 0 || int64(int(value)) != value {
		return 0, protocol.MalformedResponse()
	}
	return int(value), nil
}
