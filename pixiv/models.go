package pixiv

import "time"

// IllustDetail 是作品详情响应；保留 Pixiv App API 的 illust envelope。
type IllustDetail struct {
	Illust Illust `json:"illust"`
}

// Illust 是供调用方稳定使用的规范化作品模型。
type Illust struct {
	URL            string     `json:"url"`
	ID             int64      `json:"id"`
	Title          string     `json:"title"`
	Caption        string     `json:"caption,omitempty"`
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
	Tools          []string   `json:"tools"`
}

// User 是作品作者的规范化摘要。
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

// UserPreview 是用户列表中的稳定 envelope。
type UserPreview struct {
	User User `json:"user"`
}

// UserSearchSource 表示用户列表的上游语义，避免将匿名相关作者误解为官方用户名搜索。
type UserSearchSource string

const (
	UserSearchSourceApp                  UserSearchSource = "app_search"
	UserSearchSourceRelatedIllustAuthors UserSearchSource = "related_illust_authors"
)

// Cursor 是版本化、不透明且绑定查询的分页游标；零值表示没有下一批。
type Cursor string

type SearchTarget string

const (
	SearchTargetPartialMatchForTags SearchTarget = "partial_match_for_tags"
	SearchTargetExactMatchForTags   SearchTarget = "exact_match_for_tags"
	SearchTargetTitleAndCaption     SearchTarget = "title_and_caption"
	SearchTargetKeyword             SearchTarget = "keyword"
)

type SortMode string

const (
	SortModeDateDesc SortMode = "date_desc"
	SortModeDateAsc  SortMode = "date_asc"
)

type RankingMode string

const (
	RankingModeDay             RankingMode = "day"
	RankingModeDayMale         RankingMode = "day_male"
	RankingModeDayFemale       RankingMode = "day_female"
	RankingModeWeek            RankingMode = "week"
	RankingModeWeekOriginal    RankingMode = "week_original"
	RankingModeWeekRookie      RankingMode = "week_rookie"
	RankingModeMonth           RankingMode = "month"
	RankingModeDayManga        RankingMode = "day_manga"
	RankingModeWeekManga       RankingMode = "week_manga"
	RankingModeMonthManga      RankingMode = "month_manga"
	RankingModeWeekRookieManga RankingMode = "week_rookie_manga"
	RankingModeDayR18          RankingMode = "day_r18"
	RankingModeDayMaleR18      RankingMode = "day_male_r18"
	RankingModeDayFemaleR18    RankingMode = "day_female_r18"
	RankingModeWeekR18         RankingMode = "week_r18"
	RankingModeWeekR18G        RankingMode = "week_r18g"
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

// SearchResolution 是插画搜索使用的稳定分辨率档位。
type SearchResolution string

const (
	SearchResolutionAll    SearchResolution = "all"
	SearchResolutionHigh   SearchResolution = "high"
	SearchResolutionMedium SearchResolution = "medium"
	SearchResolutionLow    SearchResolution = "low"
)

type SearchRating string

const (
	SearchRatingAll    SearchRating = "all"
	SearchRatingSFW    SearchRating = "sfw"
	SearchRatingR18    SearchRating = "r18"
	SearchRatingR18G   SearchRating = "r18g"
	SearchRatingMature SearchRating = "mature"
)

type SearchContentType string

const (
	SearchContentTypeAll             SearchContentType = "all"
	SearchContentTypeIllustAndUgoira SearchContentType = "illust-and-ugoira"
	SearchContentTypeIllust          SearchContentType = "illust"
	SearchContentTypeManga           SearchContentType = "manga"
	SearchContentTypeUgoira          SearchContentType = "ugoira"
)

type SearchAIMode string

const (
	SearchAIModeAll     SearchAIMode = "all"
	SearchAIModeExclude SearchAIMode = "exclude"
	SearchAIModeOnly    SearchAIMode = "only"
)

type SearchAspectRatio string

const (
	SearchAspectRatioAll       SearchAspectRatio = "all"
	SearchAspectRatioLandscape SearchAspectRatio = "landscape"
	SearchAspectRatioPortrait  SearchAspectRatio = "portrait"
	SearchAspectRatioSquare    SearchAspectRatio = "square"
)

// SearchIllustFilters 是独立于 App/Web wire 参数的稳定搜索筛选契约。
type SearchIllustFilters struct {
	Rating      SearchRating      `json:"rating,omitempty"`
	ContentType SearchContentType `json:"content_type,omitempty"`
	AIMode      SearchAIMode      `json:"ai_mode,omitempty"`
	AspectRatio SearchAspectRatio `json:"aspect_ratio,omitempty"`
	Resolution  SearchResolution  `json:"resolution,omitempty"`
	Tool        string            `json:"tool,omitempty"`
	BookmarkMin *int              `json:"bookmark_min,omitempty"`
	BookmarkMax *int              `json:"bookmark_max,omitempty"`
}

type SearchIllustRequest struct {
	Word      string              `json:"word"`
	Target    SearchTarget        `json:"search_target,omitempty"`
	Sort      SortMode            `json:"sort,omitempty"`
	Duration  string              `json:"duration,omitempty"`
	StartDate string              `json:"start_date,omitempty"`
	EndDate   string              `json:"end_date,omitempty"`
	Cursor    Cursor              `json:"cursor,omitempty"`
	Filters   SearchIllustFilters `json:"filters,omitempty"`
}

// NovelSearchFilters 是小说搜索可由稳定结果字段验证的本地筛选条件。
type NovelSearchFilters struct {
	Rating        SearchRating `json:"rating,omitempty"`
	MinTextLength int          `json:"min_text_length,omitempty"`
	MaxTextLength int          `json:"max_text_length,omitempty"`
	OriginalOnly  bool         `json:"original_only,omitempty"`
}

type SearchNovelRequest struct {
	Word     string             `json:"word"`
	Target   SearchTarget       `json:"search_target,omitempty"`
	Sort     SortMode           `json:"sort,omitempty"`
	Duration string             `json:"duration,omitempty"`
	Cursor   Cursor             `json:"cursor,omitempty"`
	Filters  NovelSearchFilters `json:"filters,omitempty"`
}
type IllustRankingRequest struct {
	Mode   RankingMode `json:"mode,omitempty"`
	Date   string      `json:"date,omitempty"`
	Cursor Cursor      `json:"cursor,omitempty"`
}
type IllustRecommendedRequest struct {
	Cursor Cursor `json:"cursor,omitempty"`
}

// NovelRecommendedRequest 仅包含本操作的 opaque continuation。
type NovelRecommendedRequest struct {
	Cursor Cursor `json:"cursor,omitempty"`
}

// UserRecommendedRequest 仅包含本操作的 opaque continuation。
type UserRecommendedRequest struct {
	Cursor Cursor `json:"cursor,omitempty"`
}
type FollowingIllustsRequest struct {
	Restrict Restrict `json:"restrict,omitempty"`
	Cursor   Cursor   `json:"cursor,omitempty"`
}

// FollowingNovelsRequest 指定当前账号关注用户的小说新作查询。
type FollowingNovelsRequest struct {
	Restrict Restrict `json:"restrict,omitempty"`
	Cursor   Cursor   `json:"cursor,omitempty"`
}

// LatestIllustsRequest 指定全站最新插画或漫画查询；Type 必须是 illust 或 manga。
type LatestIllustsRequest struct {
	Type   IllustType `json:"type"`
	Cursor Cursor     `json:"cursor,omitempty"`
}

// LatestNovelsRequest 指定全站最新小说查询。
type LatestNovelsRequest struct {
	Cursor Cursor `json:"cursor,omitempty"`
}

// MyPixivUsersRequest 指定要读取 MyPixiv 用户列表的账号。
type MyPixivUsersRequest struct {
	UserID int64  `json:"user_id"`
	Cursor Cursor `json:"cursor,omitempty"`
}

// MyPixivIllustsRequest 指定当前认证账号的 MyPixiv 插画聚合查询。
type MyPixivIllustsRequest struct {
	Cursor Cursor `json:"cursor,omitempty"`
}

// MyPixivNovelsRequest 指定当前认证账号的 MyPixiv 小说聚合查询。
type MyPixivNovelsRequest struct {
	Cursor Cursor `json:"cursor,omitempty"`
}

// UserNovelsRequest 指定用户小说查询。
type UserNovelsRequest struct {
	UserID int64  `json:"user_id"`
	Cursor Cursor `json:"cursor,omitempty"`
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

// AddBookmarkRequest 指定要收藏的作品、可见范围与标签。
type AddBookmarkRequest struct {
	IllustID int64    `json:"illust_id"`
	Restrict Restrict `json:"restrict,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

// RemoveBookmarkRequest 指定要取消收藏的作品。
type RemoveBookmarkRequest struct {
	IllustID int64 `json:"illust_id"`
}

// FollowUserRequest 指定要关注的用户与可见范围。
type FollowUserRequest struct {
	UserID   int64    `json:"user_id"`
	Restrict Restrict `json:"restrict,omitempty"`
}

// UnfollowUserRequest 指定要取消关注的用户。
type UnfollowUserRequest struct {
	UserID int64 `json:"user_id"`
}

type IllustRelatedRequest struct {
	IllustID int64  `json:"illust_id"`
	Cursor   Cursor `json:"cursor,omitempty"`
}

// IllustSeriesRequest 指定插画系列及其不透明续页游标。UserID 是页面 URL 中的
// 作者归属断言，防止把格式正确但作者不匹配的 series URL 静默展开为别人的作品。
type IllustSeriesRequest struct {
	SeriesID int64  `json:"series_id"`
	UserID   int64  `json:"user_id"`
	Cursor   Cursor `json:"cursor,omitempty"`
}

type IllustListResult struct {
	Illusts    []Illust `json:"illusts"`
	NextCursor Cursor   `json:"next_cursor,omitempty"`
}

// Novel 是小说列表结果中的稳定必要字段。
type Novel struct {
	URL            string    `json:"url"`
	ID             int64     `json:"id"`
	Title          string    `json:"title"`
	Caption        string    `json:"caption"`
	XRestrict      int       `json:"x_restrict"`
	TextLength     int       `json:"text_length"`
	IsOriginal     bool      `json:"is_original"`
	User           User      `json:"user"`
	Tags           []Tag     `json:"tags"`
	ImageURLs      ImageURLs `json:"image_urls"`
	CreateDate     string    `json:"create_date"`
	TotalBookmarks int       `json:"total_bookmarks"`
	TotalView      int       `json:"total_view"`
}

// NovelListResult 包含小说批次与仅能续用到该操作的 opaque cursor。
type NovelListResult struct {
	Novels     []Novel `json:"novels"`
	NextCursor Cursor  `json:"next_cursor,omitempty"`
}

// RecommendedUserPreview 只公开用户与可用作品预览，不传递上游排名或 UI 策略字段。
type RecommendedUserPreview struct {
	User    User     `json:"user"`
	Illusts []Illust `json:"illusts"`
	Novels  []Novel  `json:"novels"`
}

// UserRecommendedResult 包含作者推荐批次与仅能续用到该操作的 opaque cursor。
type UserRecommendedResult struct {
	UserPreviews []RecommendedUserPreview `json:"user_previews"`
	NextCursor   Cursor                   `json:"next_cursor,omitempty"`
}

type UserListResult struct {
	UserPreviews []UserPreview    `json:"user_previews"`
	NextCursor   Cursor           `json:"next_cursor,omitempty"`
	Source       UserSearchSource `json:"source,omitempty"`
}

// UserDetailResult 是用户详情的稳定完整 envelope。
type UserDetailResult struct {
	User             User             `json:"user"`
	Profile          Profile          `json:"profile"`
	ProfilePublicity ProfilePublicity `json:"profile_publicity"`
	Workspace        Workspace        `json:"workspace"`
}

// Profile 是用户公开档案的稳定字段集合；上游未公开数据保持零值。
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

// ProfilePublicity 表示档案字段的公开状态。
type ProfilePublicity struct {
	Gender    bool `json:"gender"`
	Region    bool `json:"region"`
	BirthDay  bool `json:"birth_day"`
	BirthYear bool `json:"birth_year"`
	Job       bool `json:"job"`
	Pawoo     bool `json:"pawoo"`
}

// Workspace 是用户公开工作环境信息；图片 URL 可选。
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

type TrendingTagsIllustResult struct {
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
	ZipURLs         UgoiraZipURLs    `json:"zip_urls"`
	Frames          []UgoiraFrame    `json:"frames"`
	DownloadURL     string           `json:"download_url"`
	DownloadQuality UgoiraZipQuality `json:"download_quality"`
}

type UgoiraZipURLs struct {
	Medium   string `json:"medium"`
	Original string `json:"original,omitempty"`
}

// UgoiraZipQuality 标识 SDK 为动图选择的可下载 ZIP 质量。
type UgoiraZipQuality string

const (
	UgoiraZipQualityMedium   UgoiraZipQuality = "medium"
	UgoiraZipQualityOriginal UgoiraZipQuality = "original"
)

type UgoiraFrame struct {
	File  string `json:"file"`
	Delay int    `json:"delay"`
}

// Account 是不含 refresh token 的本地账号摘要。
type Account struct {
	UserID                 int64      `json:"user_id"`
	Username               string     `json:"username,omitempty"`
	Default                bool       `json:"default"`
	HasToken               bool       `json:"has_token"`
	PremiumStatus          *bool      `json:"premium_status,omitempty"`
	PremiumStatusCheckedAt *time.Time `json:"premium_status_checked_at,omitempty"`
}

// PremiumStatus 是当前已认证账号由 App profile 返回的会员资格快照。
// CheckedAt 为零时表示状态尚未查询；缓存时效由 OpenDefault 的 runtime config 决定。
type PremiumStatus struct {
	IsPremium bool      `json:"is_premium"`
	CheckedAt time.Time `json:"checked_at"`
}

// AccountsResult 包含当前默认 UID 与不含凭据的账号列表。
type AccountsResult struct {
	DefaultUserID int64     `json:"default_user_id,omitempty"`
	Accounts      []Account `json:"accounts"`
}

// LoginOptions 控制成功登录后是否选择该账号为默认账号。
type LoginOptions struct {
	UseAsDefault bool
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
