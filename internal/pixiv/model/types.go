package model

type IllustList struct {
	Illusts []Illust `json:"illusts"`
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
	ImageURLs ImageURLs `json:"image_urls"`
}

type UserPreviewList struct {
	UserPreviews []UserPreview `json:"user_previews"`
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
	ZipURLs struct {
		Medium string `json:"medium"`
	} `json:"zip_urls"`
	Frames []UgoiraFrame `json:"frames"`
}

type UgoiraFrame struct {
	File  string `json:"file"`
	Delay int    `json:"delay"`
}
