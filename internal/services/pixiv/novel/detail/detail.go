package detail

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/novel"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

type Transport interface {
	GetJSON(context.Context, string, url.Values, any) error
	GetRaw(context.Context, string, url.Values) ([]byte, error)
}

type Client struct{ transport Transport }

func New(transport Transport) *Client { return &Client{transport: transport} }

type Result struct {
	Novel        novel.Novel
	SeriesNextID int64
	SeriesPrevID int64
	SeriesTitle  string
}

func (c *Client) Detail(ctx context.Context, novelID int64) (Result, error) {
	if c == nil || c.transport == nil {
		return Result{}, errors.New("novel detail transport is not configured")
	}
	var raw detailResponseDTO
	if err := c.transport.GetJSON(ctx, protocol.AppNovelDetail, url.Values{"novel_id": {strconv.FormatInt(novelID, 10)}}, &raw); err != nil {
		return Result{}, err
	}
	if raw.Novel == nil || raw.Novel.ID <= 0 || raw.Novel.User.ID <= 0 {
		return Result{}, protocol.MalformedResponse()
	}
	result := Result{Novel: mapNovel(*raw.Novel)}
	if raw.SeriesNext != nil {
		if raw.SeriesNext.ID <= 0 {
			return Result{}, protocol.MalformedResponse()
		}
		result.SeriesNextID = raw.SeriesNext.ID
		result.SeriesTitle = raw.SeriesNext.Title
	}
	if raw.SeriesPrev != nil {
		if raw.SeriesPrev.ID <= 0 {
			return Result{}, protocol.MalformedResponse()
		}
		result.SeriesPrevID = raw.SeriesPrev.ID
	}
	return result, nil
}

func (c *Client) Content(ctx context.Context, novelID int64) ([]byte, error) {
	if c == nil || c.transport == nil {
		return nil, errors.New("novel detail transport is not configured")
	}
	body, err := c.transport.GetRaw(ctx, protocol.AppNovelContent, url.Values{"novel_id": {strconv.FormatInt(novelID, 10)}})
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, protocol.MalformedResponse()
	}
	return append([]byte(nil), body...), nil
}

type detailResponseDTO struct {
	Novel      *novelDTO          `json:"novel"`
	SeriesNext *novelSeriesRefDTO `json:"series_next"`
	SeriesPrev *novelSeriesRefDTO `json:"series_prev"`
}

type novelSeriesRefDTO struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
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

func mapNovel(value novelDTO) novel.Novel {
	return novel.Novel{
		ID: value.ID, Title: value.Title, Caption: value.Caption,
		XRestrict: intValue(value.XRestrict), TextLength: intValue(value.TextLength), IsOriginal: boolValue(value.IsOriginal),
		User: mapUser(value.User), Tags: mapTags(value.Tags), ImageURLs: mapImageURLs(value.ImageURLs),
		CreateDate: value.CreateDate, TotalBookmarks: value.TotalBookmarks, TotalView: value.TotalView,
	}
}

func mapUser(value userDTO) novel.UserSummary {
	return novel.UserSummary{ID: value.ID, Name: value.Name, Account: value.Account, Comment: value.Comment, IsFollowed: value.IsFollowed, ProfileImageURLs: novel.ProfileImageURLs{Medium: cloneString(value.ProfileImageURLs.Medium)}}
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
