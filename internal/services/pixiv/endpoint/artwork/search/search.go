package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

// Transport 是 artwork search 所需的最窄 App API 请求能力。
type Transport interface {
	GetJSON(context.Context, string, url.Values, any) error
}

// Filters 表达 search endpoint 已验证的 App 参数；不包含 CLI strategy 或
// logical pagination 语义。
type Filters struct {
	ContentType string
	AIMode      string
	AspectRatio string
	Resolution  string
	Tool        string
	BookmarkMin *int
	BookmarkMax *int
}

// Request 是 artwork search 的 endpoint 请求。
type Request struct {
	Word      string
	Target    string
	Sort      string
	Duration  string
	StartDate string
	EndDate   string
	Offset    int
	Filters   Filters
}

// Result 是一次 search endpoint 批次。next_url 只被解析为 opaque
// continuation state，不向调用方泄露为可请求 URL。
type Result struct {
	Items      []artwork.Artwork
	NextOffset int
	HasNext    bool
}

// Client 只组合 search endpoint 所需的 transport。
type Client struct {
	transport Transport
}

// New 建立 artwork search endpoint client。
func New(transport Transport) *Client {
	return &Client{transport: transport}
}

// Search 请求并规范化一个 artwork search 批次。
func (c *Client) Search(ctx context.Context, request Request) (Result, error) {
	if c == nil || c.transport == nil {
		return Result{}, fmt.Errorf("pixiv artwork search transport is not configured")
	}
	query := url.Values{
		"word":          {request.Word},
		"search_target": {request.Target},
		"sort":          {request.Sort},
	}
	setOptional(query, "duration", request.Duration)
	setOptional(query, "start_date", request.StartDate)
	setOptional(query, "end_date", request.EndDate)
	setFilters(query, request.Filters)
	if request.Offset > 0 {
		query.Set("offset", strconv.Itoa(request.Offset))
	}

	var raw responseDTO
	if err := c.transport.GetJSON(ctx, protocol.AppSearchIllust, query, &raw); err != nil {
		return Result{}, err
	}
	if !raw.Illusts.Present || !raw.Illusts.Valid {
		return Result{}, protocol.MalformedResponse()
	}
	items := make([]artwork.Artwork, len(raw.Illusts.Items))
	for index, item := range raw.Illusts.Items {
		if item.ID <= 0 {
			return Result{}, protocol.MalformedResponse()
		}
		items[index] = mapArtwork(item)
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
		result.NextOffset = offset
		result.HasNext = true
	}
	return result, nil
}

func setOptional(query url.Values, key, value string) {
	if value != "" {
		query.Set(key, value)
	}
}

func setFilters(query url.Values, filters Filters) {
	// App search requires an explicit AI mode. The public facade performs the
	// local "only" filtering; the endpoint only sends the verified upstream
	// all/exclude values.
	query.Set("search_ai_type", "0")
	if filters.AIMode == "exclude" {
		query.Set("search_ai_type", "1")
	}
	switch filters.AspectRatio {
	case "landscape", "portrait", "square":
		query.Set("ratio_pattern", filters.AspectRatio)
	}
	switch filters.ContentType {
	case "illust-and-ugoira":
		query.Set("content_type", "illust_and_ugoira")
	case "illust", "manga", "ugoira":
		query.Set("content_type", filters.ContentType)
	}
	setOptional(query, "tool", filters.Tool)
	if filters.BookmarkMin != nil {
		query.Set("bookmark_num_min", strconv.Itoa(*filters.BookmarkMin))
	}
	if filters.BookmarkMax != nil {
		query.Set("bookmark_num_max", strconv.Itoa(*filters.BookmarkMax))
	}
	switch filters.Resolution {
	case "high":
		query.Set("width_min", "3000")
		query.Set("height_min", "3000")
	case "medium":
		query.Set("width_min", "1000")
		query.Set("width_max", "2999")
		query.Set("height_min", "1000")
		query.Set("height_max", "2999")
	case "low":
		query.Set("width_max", "999")
		query.Set("height_max", "999")
	}
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

func mapArtwork(value illustDTO) artwork.Artwork {
	return artwork.Artwork{
		ID:             value.ID,
		Title:          value.Title,
		Caption:        value.Caption,
		Type:           value.Type,
		PageCount:      value.PageCount,
		TotalBookmarks: value.TotalBookmarks,
		TotalView:      value.TotalView,
		XRestrict:      value.XRestrict,
		User:           mapUser(value.User),
		Tags:           mapTags(value.Tags),
		ImageURLs:      mapImageURLs(value.ImageURLs),
		MetaSinglePage: artwork.SinglePage{OriginalImageURL: value.MetaSinglePage.OriginalImageURL},
		MetaPages:      mapMetaPages(value.MetaPages),
		AIType:         value.AIType,
		CreateDate:     value.CreateDate,
		Width:          value.Width,
		Height:         value.Height,
		Tools:          append([]string(nil), value.Tools...),
	}
}

func mapUser(value userDTO) artwork.UserSummary {
	return artwork.UserSummary{
		ID:         value.ID,
		Name:       value.Name,
		Account:    value.Account,
		Comment:    value.Comment,
		IsFollowed: value.IsFollowed,
		ProfileImageURLs: artwork.ProfileImageURLs{
			Medium: cloneString(value.ProfileImageURLs.Medium),
		},
	}
}

func mapTags(values []tagDTO) []artwork.Tag {
	if values == nil {
		return nil
	}
	result := make([]artwork.Tag, len(values))
	for index, value := range values {
		result[index] = artwork.Tag{Name: value.Name, TranslatedName: value.TranslatedName}
	}
	return result
}

func mapImageURLs(value imageURLsDTO) artwork.ImageURLs {
	return artwork.ImageURLs{SquareMedium: value.SquareMedium, Medium: value.Medium, Large: value.Large, Original: value.Original}
}

func mapMetaPages(values []metaPageDTO) []artwork.MetaPage {
	if values == nil {
		return nil
	}
	result := make([]artwork.MetaPage, len(values))
	for index, value := range values {
		result[index] = artwork.MetaPage{
			PageIndex: index,
			Width:     value.Width,
			Height:    value.Height,
			Extension: value.Extension,
			ImageURLs: mapImageURLs(value.ImageURLs),
		}
	}
	return result
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
