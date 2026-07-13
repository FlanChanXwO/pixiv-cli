package appapi

import (
	"bytes"
	"encoding/json"
)

// 以下 DTO 只表达 App API wire shape；normalized model 的所有权仍在 internal/pixiv/model。
type illustListDTO struct {
	Illusts requiredList[illustDTO] `json:"illusts"`
	NextURL *string                 `json:"next_url"`
}
type illustDetailDTO struct {
	Illust *illustDTO `json:"illust"`
}
type illustDTO struct {
	ID             int64         `json:"id"`
	Title          string        `json:"title"`
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
	AIType         int           `json:"ai_type"`
	CreateDate     string        `json:"create_date"`
	Width          int           `json:"width"`
	Height         int           `json:"height"`
}
type userDTO struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Account    string `json:"account"`
	Comment    string `json:"comment"`
	IsFollowed bool   `json:"is_followed"`
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
type userPreviewListDTO struct {
	UserPreviews requiredList[userPreviewDTO] `json:"user_previews"`
	NextURL      *string                      `json:"next_url"`
}
type userPreviewDTO struct {
	User userDTO `json:"user"`
}
type trendTagsDTO struct {
	TrendTags requiredList[trendTagDTO] `json:"trend_tags"`
}
type trendTagDTO struct {
	Tag            string                    `json:"tag"`
	TranslatedName string                    `json:"translated_name"`
	Illust         requiredObject[illustDTO] `json:"illust"`
}
type ugoiraMetadataResultDTO struct {
	UgoiraMetadata requiredObject[ugoiraMetadataDTO] `json:"ugoira_metadata"`
}
type ugoiraMetadataDTO struct {
	ZipURLs requiredObject[ugoiraZipURLsDTO] `json:"zip_urls"`
	Frames  requiredList[ugoiraFrameDTO]     `json:"frames"`
}
type ugoiraZipURLsDTO struct {
	Medium string `json:"medium"`
}
type ugoiraFrameDTO struct {
	File  string `json:"file"`
	Delay int    `json:"delay"`
}
type userDetailDTO struct {
	User *userDTO `json:"user"`
}

// requiredList 区分 wire 的显式空数组与缺失/null；只有前者是合法空批次。
type requiredList[T any] struct {
	Items   []T
	Present bool
	Valid   bool
}

func (l *requiredList[T]) UnmarshalJSON(data []byte) error {
	*l = requiredList[T]{}
	l.Present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil
	}
	if err := json.Unmarshal(data, &l.Items); err != nil {
		return err
	}
	l.Valid = true
	return nil
}

// requiredObject 每次反序列化都先清空 Value，确保重复 object key 遵循 JSON 最后值而非字段合并。
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
