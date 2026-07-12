package model

import "encoding/json"

type IllustList struct {
	Illusts            []Illust `json:"illusts"`
	NextOffset         int      `json:"-"`
	NextMaxBookmarkID  int64    `json:"-"`
	ContinuationExists bool     `json:"-"`
}

// MarshalJSON 将合法空列表稳定编码为 []，避免内部消费者产生 wire 上的 null。
func (l IllustList) MarshalJSON() ([]byte, error) {
	items := l.Illusts
	if items == nil {
		items = []Illust{}
	}
	return json.Marshal(struct {
		Illusts []Illust `json:"illusts"`
	}{Illusts: items})
}

type IllustDetail struct {
	Illust Illust `json:"illust"`
}

type Illust struct {
	ID             int64      `json:"id"`
	Title          string     `json:"title"`
	Type           string     `json:"type"`
	PageCount      int        `json:"page_count"`
	TotalBookmarks int        `json:"total_bookmarks"`
	TotalView      int        `json:"total_view"`
	XRestrict      int        `json:"x_restrict"`
	User           User       `json:"user"`
	Tags           []Tag      `json:"tags"`
	ImageURLs      ImageURLs  `json:"image_urls"`
	MetaSinglePage SinglePage `json:"meta_single_page"`
	MetaPages      []MetaPage `json:"meta_pages"`
	AIType         int        `json:"ai_type"`
	CreateDate     string     `json:"create_date"`
	Width          int        `json:"width"`
	Height         int        `json:"height"`
}

type User struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Account    string `json:"account"`
	Comment    string `json:"comment"`
	IsFollowed bool   `json:"is_followed"`
}

type Tag struct {
	Name           string `json:"name"`
	TranslatedName string `json:"translated_name"`
}

type ImageURLs struct {
	SquareMedium string `json:"square_medium"`
	Medium       string `json:"medium"`
	Large        string `json:"large"`
	Original     string `json:"original"`
}

type SinglePage struct {
	OriginalImageURL string `json:"original_image_url"`
}

type MetaPage struct {
	PageIndex int       `json:"page_index"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	Extension string    `json:"extension"`
	ImageURLs ImageURLs `json:"image_urls"`
}

type UserPreviewList struct {
	UserPreviews       []UserPreview `json:"user_previews"`
	NextOffset         int           `json:"-"`
	ContinuationExists bool          `json:"-"`
}

func (l UserPreviewList) MarshalJSON() ([]byte, error) {
	items := l.UserPreviews
	if items == nil {
		items = []UserPreview{}
	}
	return json.Marshal(struct {
		UserPreviews []UserPreview `json:"user_previews"`
	}{UserPreviews: items})
}

type UserPreview struct {
	User User `json:"user"`
}

type TrendTags struct {
	TrendTags []TrendTag `json:"trend_tags"`
}

type TrendTag struct {
	Tag            string `json:"tag"`
	TranslatedName string `json:"translated_name"`
	Illust         Illust `json:"illust"`
}

type UgoiraMetadataResult struct {
	UgoiraMetadata UgoiraMetadata `json:"ugoira_metadata"`
}

type UgoiraMetadata struct {
	ZipURLs UgoiraZipURLs `json:"zip_urls"`
	Frames  []UgoiraFrame `json:"frames"`
}

type UgoiraZipURLs struct {
	Medium   string `json:"medium"`
	Original string `json:"original"`
}

type UgoiraFrame struct {
	File  string `json:"file"`
	Delay int    `json:"delay"`
}
