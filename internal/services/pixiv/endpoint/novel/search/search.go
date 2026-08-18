package search

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/novel"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

// Transport 是 novel search 所需的最小 App API 能力。
type Transport interface {
	GetJSON(context.Context, string, url.Values, any) error
}

type Request struct {
	Word     string
	Target   string
	Sort     string
	Duration string
	Offset   int
}

type Result struct {
	Items      []novel.Novel
	NextOffset int
	HasNext    bool
}

type Client struct{ transport Transport }

func New(transport Transport) *Client { return &Client{transport: transport} }

func (c *Client) Search(ctx context.Context, request Request) (Result, error) {
	if c == nil || c.transport == nil {
		return Result{}, errors.New("novel search transport is not configured")
	}
	query := url.Values{
		"word":          {request.Word},
		"search_target": {request.Target},
		"sort":          {request.Sort},
	}
	if request.Duration != "" {
		query.Set("duration", request.Duration)
	}
	if request.Offset > 0 {
		query.Set("offset", strconv.Itoa(request.Offset))
	}
	var raw responseDTO
	if err := c.transport.GetJSON(ctx, protocol.AppSearchNovel, query, &raw); err != nil {
		return Result{}, err
	}
	if !raw.Novels.Present || !raw.Novels.Valid {
		return Result{}, protocol.MalformedResponse()
	}
	items := make([]novel.Novel, len(raw.Novels.Items))
	for index, value := range raw.Novels.Items {
		if value.ID <= 0 || value.User.ID <= 0 || value.TextLength == nil || value.XRestrict == nil || value.IsOriginal == nil {
			return Result{}, protocol.MalformedResponse()
		}
		items[index] = mapNovel(value)
	}
	result := Result{Items: items}
	if raw.NextURL != nil {
		if *raw.NextURL == "" {
			return Result{}, protocol.MalformedResponse()
		}
		offset, err := continuationOffset(*raw.NextURL)
		if err != nil {
			return Result{}, err
		}
		result.NextOffset, result.HasNext = offset, true
	}
	return result, nil
}

type responseDTO struct {
	Novels  requiredList[novelDTO] `json:"novels"`
	NextURL *string                `json:"next_url"`
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
		ID:         value.ID,
		Name:       value.Name,
		Account:    value.Account,
		Comment:    value.Comment,
		IsFollowed: value.IsFollowed,
		ProfileImageURLs: novel.ProfileImageURLs{
			Medium: cloneString(value.ProfileImageURLs.Medium),
		},
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

func boolValue(value *bool) bool {
	return value != nil && *value
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func continuationOffset(rawURL string) (int, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return 0, protocol.MalformedResponse()
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(values["offset"]) != 1 {
		return 0, protocol.MalformedResponse()
	}
	offset, err := strconv.ParseInt(values.Get("offset"), 10, 64)
	if err != nil || offset <= 0 || int64(int(offset)) != offset {
		return 0, protocol.MalformedResponse()
	}
	return int(offset), nil
}
