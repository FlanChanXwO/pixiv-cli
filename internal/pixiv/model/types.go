package model

import "encoding/json"

type IllustList struct {
	Illusts            []Illust `json:"illusts"`
	NextOffset         int      `json:"-"`
	NextMaxBookmarkID  int64    `json:"-"`
	ContinuationExists bool     `json:"-"`
}

type SearchIllustOptions struct {
	Tools []string `json:"tools"`
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

// Novel 是推荐接口所需的规范化小说字段，刻意不传递上游的 UI 或策略字段。
type Novel struct {
	ID             int64     `json:"id"`
	Title          string    `json:"title"`
	Caption        string    `json:"caption"`
	User           User      `json:"user"`
	Tags           []Tag     `json:"tags"`
	ImageURLs      ImageURLs `json:"image_urls"`
	CreateDate     string    `json:"create_date"`
	TotalBookmarks int       `json:"total_bookmarks"`
	TotalView      int       `json:"total_view"`
}

type NovelList struct {
	Novels             []Novel `json:"novels"`
	NextOffset         int     `json:"-"`
	ContinuationExists bool    `json:"-"`
}

type RecommendedUserPreview struct {
	User    User     `json:"user"`
	Illusts []Illust `json:"illusts"`
	Novels  []Novel  `json:"novels"`
}

type RecommendedUserList struct {
	UserPreviews       []RecommendedUserPreview `json:"user_previews"`
	NextOffset         int                      `json:"-"`
	ContinuationExists bool                     `json:"-"`
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
	ID               int64            `json:"id"`
	Name             string           `json:"name"`
	Account          string           `json:"account"`
	Comment          string           `json:"comment"`
	IsFollowed       bool             `json:"is_followed"`
	ProfileImageURLs ProfileImageURLs `json:"profile_image_urls"`
}

// ProfileImageURLs 是用户头像的可选 URL 集合，不与作品 ImageURLs 混用。
type ProfileImageURLs struct {
	Medium *string `json:"medium,omitempty"`
}

// UserDetail 是 App user/detail 经 adapter 规范化后的完整固定 envelope。
type UserDetail struct {
	User             User             `json:"user"`
	Profile          Profile          `json:"profile"`
	ProfilePublicity ProfilePublicity `json:"profile_publicity"`
	Workspace        Workspace        `json:"workspace"`
}

type Profile struct {
	Webpage                    *string `json:"webpage,omitempty"`
	Gender                     string  `json:"gender"`
	Birth                      string  `json:"birth"`
	BirthDay                   string  `json:"birth_day"`
	BirthYear                  int     `json:"birth_year"`
	Region                     string  `json:"region"`
	AddressID                  int64   `json:"address_id"`
	CountryCode                string  `json:"country_code"`
	Job                        string  `json:"job"`
	JobID                      int64   `json:"job_id"`
	TotalFollowUsers           int     `json:"total_follow_users"`
	TotalMyPixivUsers          int     `json:"total_mypixiv_users"`
	TotalIllusts               int     `json:"total_illusts"`
	TotalManga                 int     `json:"total_manga"`
	TotalNovels                int     `json:"total_novels"`
	TotalIllustBookmarksPublic int     `json:"total_illust_bookmarks_public"`
	TotalIllustSeries          int     `json:"total_illust_series"`
	TotalNovelSeries           int     `json:"total_novel_series"`
	BackgroundImageURL         *string `json:"background_image_url,omitempty"`
	TwitterAccount             string  `json:"twitter_account"`
	TwitterURL                 *string `json:"twitter_url,omitempty"`
	PawooURL                   *string `json:"pawoo_url,omitempty"`
	IsPremium                  bool    `json:"is_premium"`
	IsUsingCustomProfileImage  bool    `json:"is_using_custom_profile_image"`
}

type ProfilePublicity struct {
	Gender    bool `json:"gender"`
	Region    bool `json:"region"`
	BirthDay  bool `json:"birth_day"`
	BirthYear bool `json:"birth_year"`
	Job       bool `json:"job"`
	Pawoo     bool `json:"pawoo"`
}

type Workspace struct {
	PC                string  `json:"pc"`
	Monitor           string  `json:"monitor"`
	Tool              string  `json:"tool"`
	Scanner           string  `json:"scanner"`
	Tablet            string  `json:"tablet"`
	Mouse             string  `json:"mouse"`
	Printer           string  `json:"printer"`
	Desktop           string  `json:"desktop"`
	Music             string  `json:"music"`
	Desk              string  `json:"desk"`
	Chair             string  `json:"chair"`
	Comment           string  `json:"comment"`
	WorkspaceImageURL *string `json:"workspace_image_url,omitempty"`
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
