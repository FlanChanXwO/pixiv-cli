package series

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"

	endpointcontinuation "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/continuation"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/novel"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

type Transport interface {
	GetJSON(context.Context, string, url.Values, any) error
}

type Request struct {
	SeriesID  int64
	LastOrder int64
}

type Result struct {
	Series        novel.Series
	Items         []novel.Novel
	NextLastOrder int64
	HasNext       bool
}

type Client struct{ transport Transport }

func New(transport Transport) *Client { return &Client{transport: transport} }

func (c *Client) List(ctx context.Context, request Request) (Result, error) {
	if c == nil || c.transport == nil {
		return Result{}, errors.New("novel series transport is not configured")
	}
	query := url.Values{"series_id": {strconv.FormatInt(request.SeriesID, 10)}}
	if request.LastOrder > 0 {
		query.Set("last_order", strconv.FormatInt(request.LastOrder, 10))
	}
	var raw responseDTO
	if err := c.transport.GetJSON(ctx, protocol.AppNovelSeries, query, &raw); err != nil {
		return Result{}, err
	}
	if raw.Detail == nil || raw.Detail.ID <= 0 || raw.Detail.User.ID <= 0 || !raw.Novels.Present || !raw.Novels.Valid {
		return Result{}, protocol.MalformedResponse()
	}
	items := make([]novel.Novel, len(raw.Novels.Items))
	for index, value := range raw.Novels.Items {
		if value.ID <= 0 || value.User.ID <= 0 {
			return Result{}, protocol.MalformedResponse()
		}
		items[index] = mapNovel(value)
	}
	result := Result{Series: novel.Series{ID: raw.Detail.ID, Title: raw.Detail.Title, Caption: raw.Detail.Caption, User: mapUser(raw.Detail.User), IsConcluded: raw.Detail.IsConcluded}, Items: items}
	if raw.NextURL != nil {
		if *raw.NextURL == "" {
			return Result{}, protocol.MalformedResponse()
		}
		next, err := continuation(*raw.NextURL)
		if err != nil {
			return Result{}, err
		}
		result.NextLastOrder, result.HasNext = next, true
	}
	return result, nil
}

type responseDTO struct {
	Detail  *seriesDetailDTO       `json:"novel_series_detail"`
	Novels  requiredList[novelDTO] `json:"novels"`
	NextURL *string                `json:"next_url"`
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

type seriesDetailDTO struct {
	ID          int64   `json:"id"`
	Title       string  `json:"title"`
	Caption     string  `json:"caption"`
	User        userDTO `json:"user"`
	IsConcluded bool    `json:"is_concluded"`
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
	return novel.Novel{ID: value.ID, Title: value.Title, Caption: value.Caption, XRestrict: intValue(value.XRestrict), TextLength: intValue(value.TextLength), IsOriginal: boolValue(value.IsOriginal), User: mapUser(value.User), Tags: mapTags(value.Tags), ImageURLs: mapImageURLs(value.ImageURLs), CreateDate: value.CreateDate, TotalBookmarks: value.TotalBookmarks, TotalView: value.TotalView}
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

func continuation(rawURL string) (int64, error) {
	_, value, err := endpointcontinuation.Parse(rawURL, endpointcontinuation.Spec{
		Path:             protocol.AppNovelSeries,
		Keys:             []string{"last_order"},
		AllowedQueryKeys: []string{"series_id"},
	})
	if err != nil || value <= 0 {
		return 0, protocol.MalformedResponse()
	}
	return value, nil
}
