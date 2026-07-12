package appapi

// 以下 DTO 只表达 App API wire shape；normalized model 的所有权仍在 internal/pixiv/model。
type illustListDTO struct {
	Illusts []illustDTO `json:"illusts"`
}
type illustDetailDTO struct {
	Illust illustDTO `json:"illust"`
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
	UserPreviews []userPreviewDTO `json:"user_previews"`
}
type userPreviewDTO struct {
	User userDTO `json:"user"`
}
type trendTagsDTO struct {
	TrendTags []trendTagDTO `json:"trend_tags"`
}
type trendTagDTO struct {
	Tag            string    `json:"tag"`
	TranslatedName string    `json:"translated_name"`
	Illust         illustDTO `json:"illust"`
}
type ugoiraMetadataResultDTO struct {
	UgoiraMetadata ugoiraMetadataDTO `json:"ugoira_metadata"`
}
type ugoiraMetadataDTO struct {
	ZipURLs struct {
		Medium string `json:"medium"`
	} `json:"zip_urls"`
	Frames []ugoiraFrameDTO `json:"frames"`
}
type ugoiraFrameDTO struct {
	File  string `json:"file"`
	Delay int    `json:"delay"`
}
type userDetailDTO struct {
	User userDTO `json:"user"`
}
