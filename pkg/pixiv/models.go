package pixiv

// IllustDetail 是作品详情响应；保留 Pixiv App API 的 illust envelope。
type IllustDetail struct {
	Illust Illust `json:"illust"`
}

// Illust 是供调用方稳定使用的规范化作品模型。
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

// User 是作品作者的规范化摘要。
type User struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Account    string `json:"account"`
	Comment    string `json:"comment"`
	IsFollowed bool   `json:"is_followed"`
}

// UserPreview 是用户列表中的稳定 envelope。
type UserPreview struct {
	User User `json:"user"`
}

// Cursor 是版本化、不透明且绑定查询的分页游标；零值表示没有下一批。
type Cursor string

type SearchTarget string

const (
	SearchTargetPartialMatchForTags SearchTarget = "partial_match_for_tags"
	SearchTargetExactMatchForTags   SearchTarget = "exact_match_for_tags"
	SearchTargetTitleAndCaption     SearchTarget = "title_and_caption"
)

type SortMode string

const (
	SortModeDateDesc SortMode = "date_desc"
	SortModeDateAsc  SortMode = "date_asc"
)

type RankingMode string

const (
	RankingModeDay   RankingMode = "day"
	RankingModeWeek  RankingMode = "week"
	RankingModeMonth RankingMode = "month"
)

type Restrict string

const (
	RestrictPublic  Restrict = "public"
	RestrictPrivate Restrict = "private"
)

type IllustType string

const (
	IllustTypeIllust IllustType = "illust"
	IllustTypeManga  IllustType = "manga"
	IllustTypeUgoira IllustType = "ugoira"
)

type SearchIllustRequest struct {
	Word     string       `json:"word"`
	Target   SearchTarget `json:"search_target,omitempty"`
	Sort     SortMode     `json:"sort,omitempty"`
	Duration string       `json:"duration,omitempty"`
	Cursor   Cursor       `json:"cursor,omitempty"`
}
type IllustRankingRequest struct {
	Mode   RankingMode `json:"mode,omitempty"`
	Date   string      `json:"date,omitempty"`
	Cursor Cursor      `json:"cursor,omitempty"`
}
type IllustRecommendedRequest struct {
	Cursor Cursor `json:"cursor,omitempty"`
}
type FollowingIllustsRequest struct {
	Restrict Restrict `json:"restrict,omitempty"`
	Cursor   Cursor   `json:"cursor,omitempty"`
}
type SearchUserRequest struct {
	Word   string `json:"word"`
	Cursor Cursor `json:"cursor,omitempty"`
}
type UserDetailRequest struct {
	UserID int64 `json:"user_id"`
}
type UserArtworksRequest struct {
	UserID int64      `json:"user_id"`
	Type   IllustType `json:"type,omitempty"`
	Cursor Cursor     `json:"cursor,omitempty"`
}
type UserBookmarksRequest struct {
	UserID   int64    `json:"user_id"`
	Restrict Restrict `json:"restrict,omitempty"`
	Tag      string   `json:"tag,omitempty"`
	Cursor   Cursor   `json:"cursor,omitempty"`
}
type UserFollowingRequest struct {
	UserID   int64    `json:"user_id"`
	Restrict Restrict `json:"restrict,omitempty"`
	Cursor   Cursor   `json:"cursor,omitempty"`
}

type IllustListResult struct {
	Illusts    []Illust `json:"illusts"`
	NextCursor Cursor   `json:"next_cursor,omitempty"`
}

type UserListResult struct {
	UserPreviews []UserPreview `json:"user_previews"`
	NextCursor   Cursor        `json:"next_cursor,omitempty"`
}

type UserDetailResult struct {
	User User `json:"user"`
}

// Tag 是作品标签及其翻译。
type Tag struct {
	Name           string `json:"name"`
	TranslatedName string `json:"translated_name"`
}

// ImageURLs 汇集同一页面的标准图片尺寸 URL。
type ImageURLs struct {
	SquareMedium string `json:"square_medium"`
	Medium       string `json:"medium"`
	Large        string `json:"large"`
	Original     string `json:"original"`
}

// SinglePage 保留 App API 单页作品的原图字段。
type SinglePage struct {
	OriginalImageURL string `json:"original_image_url"`
}

// MetaPage 是按作品顺序规范化的完整页面元数据。
type MetaPage struct {
	PageIndex int       `json:"page_index"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	Extension string    `json:"extension"`
	ImageURLs ImageURLs `json:"image_urls"`
}
