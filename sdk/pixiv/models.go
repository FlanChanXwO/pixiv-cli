package pixiv

import (
	"encoding/json"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// Restrict selects the visibility scope of bookmarks, following, or
// recommendation sources.
type Restrict string

// Restrict values select the bookmark or following visibility scope.
const (
	RestrictPublic  Restrict = "public"
	RestrictPrivate Restrict = "private"
)

// ArtworkKind distinguishes the artwork types unified in Artwork.
type ArtworkKind string

// ArtworkKind values distinguish the artwork types unified in Artwork.
const (
	ArtworkKindIllustration ArtworkKind = "illustration"
	ArtworkKindManga        ArtworkKind = "manga"
	ArtworkKindUgoira       ArtworkKind = "ugoira"
	// ArtworkKindUnknown preserves an upstream kind the SDK does not yet
	// recognize; Artwork.RawKind carries the original identifier.
	ArtworkKindUnknown ArtworkKind = "unknown"
)

// Tag is a single Pixiv tag. TranslatedName is empty when upstream provided no
// translation.
type Tag struct {
	Name           string `json:"name"`
	TranslatedName string `json:"translated_name"`
}

// ImageResource is a first-party image exposed through the shared resource
// contract. Variant records the upstream size role (for example "original",
// "medium", or "square_medium") when upstream provided one. Width and Height
// are zero when upstream did not provide them.
type ImageResource struct {
	Resource sdk.Resource `json:"resource"`
	Variant  string       `json:"variant"`
	Width    int          `json:"width"`
	Height   int          `json:"height"`
}

// Stamp is a Pixiv comment stamp. Image is a first-party resource identified
// by the stable stamp ID; upstream does not provide image dimensions here.
type Stamp struct {
	ID    int64         `json:"id"`
	Image ImageResource `json:"image"`
}

// ArtworkPage is one page of a multi-page artwork. PageIndex is zero-based and
// follows the upstream display order. Image is a usable image resource.
type ArtworkPage struct {
	PageIndex int           `json:"page_index"`
	Image     ImageResource `json:"image"`
	Width     int           `json:"width"`
	Height    int           `json:"height"`
}

// ArtworkSeriesSummary 是作品响应中的系列标识摘要。
type ArtworkSeriesSummary struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

// Artwork unifies illustrations, manga, and ugoira. It is addressable by its
// stable upstream ID. PublishedAt is always the UTC publication time; UpdatedAt
// is set only when upstream provides a modification time, and is nil otherwise.
//
// List operations populate the cover, identity, title, tags, user, and counters
// but leave Pages empty; Artwork detail populates Pages with every page. The
// completeness of a given field is part of the model contract and never faked.
type Artwork struct {
	ID             int64         `json:"id"`
	Title          string        `json:"title"`
	Caption        string        `json:"caption"`
	Kind           ArtworkKind   `json:"kind"`
	RawKind        string        `json:"raw_kind"`
	Tags           []Tag         `json:"tags"`
	User           User          `json:"user"`
	PublishedAt    time.Time     `json:"published_at"`
	UpdatedAt      *time.Time    `json:"updated_at,omitempty"`
	TotalBookmarks int           `json:"total_bookmarks"`
	TotalViews     int           `json:"total_views"`
	Width          int           `json:"width"`
	Height         int           `json:"height"`
	PageCount      int           `json:"page_count"`
	XRestrict      int           `json:"x_restrict"`
	AIType         int           `json:"ai_type"`
	Tools          []string      `json:"tools,omitempty"`
	Cover          ImageResource `json:"cover"`
	Pages          []ArtworkPage `json:"pages,omitempty"`

	// Viewer 与 safety 字段为 nil 表示当前 endpoint 未提供该值。
	IsBookmarked          *bool                 `json:"is_bookmarked,omitempty"`
	IsMuted               *bool                 `json:"is_muted,omitempty"`
	Visible               *bool                 `json:"visible,omitempty"`
	SanityLevel           *int                  `json:"sanity_level,omitempty"`
	RestrictionAttributes *[]string             `json:"restriction_attributes,omitempty"`
	Series                *ArtworkSeriesSummary `json:"series,omitempty"`
	TotalComments         *int                  `json:"total_comments,omitempty"`
}

// Novel is an addressable novel entry. List operations populate identity,
// title, user, tags, counters, and cover; Novel detail adds the caption and
// full text-length fields. PublishedAt is the UTC publication time and
// UpdatedAt is set only when upstream provides one.
type Novel struct {
	ID             int64         `json:"id"`
	Title          string        `json:"title"`
	Caption        string        `json:"caption"`
	User           User          `json:"user"`
	Tags           []Tag         `json:"tags"`
	PublishedAt    time.Time     `json:"published_at"`
	UpdatedAt      *time.Time    `json:"updated_at"`
	XRestrict      int           `json:"x_restrict"`
	TextLength     int           `json:"text_length"`
	IsOriginal     bool          `json:"is_original"`
	TotalBookmarks int           `json:"total_bookmarks"`
	TotalViews     int           `json:"total_views"`
	Cover          ImageResource `json:"cover"`
}

// User is an addressable Pixiv user. ProfileImage is the profile image
// resource when upstream provides one.
type User struct {
	ID           int64         `json:"id"`
	Name         string        `json:"name"`
	Account      string        `json:"account"`
	Comment      string        `json:"comment"`
	IsFollowed   bool          `json:"is_followed"`
	ProfileImage ImageResource `json:"profile_image"`
}

// UserDetail adds profile and workspace information beyond the User identity.
// Optional values remain zero when upstream does not provide them.
type UserDetail struct {
	User             User                 `json:"user"`
	Profile          UserProfile          `json:"profile"`
	ProfilePublicity UserProfilePublicity `json:"profile_publicity"`
	Workspace        UserWorkspace        `json:"workspace"`
}

// UserProfile carries a user's public profile counters and links.
type UserProfile struct {
	Webpage                   string `json:"webpage"`
	Gender                    string `json:"gender"`
	BirthDay                  string `json:"birth_day"`
	BirthYear                 int    `json:"birth_year"`
	Region                    string `json:"region"`
	CountryCode               string `json:"country_code"`
	Job                       string `json:"job"`
	TotalFollowUsers          int    `json:"total_follow_users"`
	TotalMyPixivUsers         int    `json:"total_my_pixiv_users"`
	TotalIllusts              int    `json:"total_illusts"`
	TotalManga                int    `json:"total_manga"`
	TotalNovels               int    `json:"total_novels"`
	TotalIllustBookmarks      int    `json:"total_illust_bookmarks"`
	TotalIllustSeries         int    `json:"total_illust_series"`
	TotalNovelSeries          int    `json:"total_novel_series"`
	BackgroundImageURL        string `json:"background_image_url"`
	TwitterAccount            string `json:"twitter_account"`
	TwitterURL                string `json:"twitter_url"`
	PawooURL                  string `json:"pawoo_url"`
	IsPremium                 bool   `json:"is_premium"`
	IsUsingCustomProfileImage bool   `json:"is_using_custom_profile_image"`
}

// UserProfilePublicity records which profile fields are publicly visible.
type UserProfilePublicity struct {
	Gender    bool `json:"gender"`
	Region    bool `json:"region"`
	BirthDay  bool `json:"birth_day"`
	BirthYear bool `json:"birth_year"`
	Job       bool `json:"job"`
	Pawoo     bool `json:"pawoo"`
}

// UserWorkspace describes a user's declared workspace and tools.
type UserWorkspace struct {
	PC                string `json:"pc"`
	Monitor           string `json:"monitor"`
	Tool              string `json:"tool"`
	Scanner           string `json:"scanner"`
	Tablet            string `json:"tablet"`
	Mouse             string `json:"mouse"`
	Printer           string `json:"printer"`
	Desktop           string `json:"desktop"`
	Music             string `json:"music"`
	Desk              string `json:"desk"`
	Chair             string `json:"chair"`
	Comment           string `json:"comment"`
	WorkspaceImageURL string `json:"workspace_image_url"`
}

// UserPreview couples a user with sample works used by recommendation and
// follower list operations.
type UserPreview struct {
	User    User      `json:"user"`
	Illusts []Artwork `json:"illusts"`
	Novels  []Novel   `json:"novels"`
}

// Comment is a single comment on an artwork or novel.
type Comment struct {
	ID            int64     `json:"id"`
	User          User      `json:"user"`
	Comment       string    `json:"comment"`
	CreatedAt     time.Time `json:"created_at"`
	ParentComment *Comment  `json:"parent_comment"`
}

// CommentMutationResult reports the comment ID returned by an upstream
// create or reply response. The ID does not prove that a later read-back
// succeeded; callers must perform that verification separately.
type CommentMutationResult struct {
	CommentID int64
}

// CommentAccessControl records the upstream access-control state for a comment
// page when upstream provided it.
type CommentAccessControl struct {
	CanComment bool `json:"can_comment"`
	IsLocked   bool `json:"is_locked"`
	// NumericValue preserves the current App API scalar without assigning it
	// business meaning. CanComment and IsLocked are meaningful only when the
	// object form was supplied by upstream.
	NumericValue *int64 `json:"-"`
}

// MarshalJSON emits the current scalar form when upstream supplied it, and the
// object form otherwise.
func (value CommentAccessControl) MarshalJSON() ([]byte, error) {
	if value.NumericValue != nil {
		return json.Marshal(struct {
			NumericValue int64 `json:"comment_access_control"`
		}{NumericValue: *value.NumericValue})
	}
	return json.Marshal(struct {
		CanComment bool `json:"can_comment"`
		IsLocked   bool `json:"is_locked"`
	}{CanComment: value.CanComment, IsLocked: value.IsLocked})
}

// CommentPage wraps a paged list of comments together with optional upstream
// total and access-control metadata. Total and AccessControl are non-nil only
// when upstream explicitly provided them; unknown values are never faked to
// zero.
type CommentPage struct {
	Page          sdk.Page[Comment]     `json:"page"`
	Total         *int64                `json:"total"`
	AccessControl *CommentAccessControl `json:"access_control"`
}

// NovelSeries is an addressable novel series.
type NovelSeries struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Caption     string `json:"caption"`
	User        User   `json:"user"`
	IsConcluded bool   `json:"is_concluded"`
}

// NovelSeriesResult carries a novel series with its paged novels.
type NovelSeriesResult struct {
	Series NovelSeries     `json:"series"`
	Novels sdk.Page[Novel] `json:"novels"`
}

// BookmarkTag is one tag used by a user's artwork or novel bookmarks. Count
// is the number of bookmarks carrying the tag when upstream provided it,
// otherwise zero.
type BookmarkTag struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// TrendingTag is a currently trending artwork tag paired with a sample artwork.
type TrendingTag struct {
	Tag            string  `json:"tag"`
	TranslatedName string  `json:"translated_name"`
	Artwork        Artwork `json:"artwork"`
}

// ArtworkBookmarkDetail is the current user's bookmark state for one artwork.
// A zero-value Restrict means the artwork is not bookmarked.
type ArtworkBookmarkDetail struct {
	Restrict Restrict `json:"restrict"`
	Tags     []string `json:"tags"`
}

// MarshalJSON keeps the established output: an absent or empty tag list is
// null, while the in-memory Tags slice stays non-nil for callers.
func (value ArtworkBookmarkDetail) MarshalJSON() ([]byte, error) {
	return marshalBookmarkDetail(value.Restrict, value.Tags)
}

// NovelBookmarkDetail is the current user's bookmark state for one novel.
// A zero-value Restrict means the novel is not bookmarked.
type NovelBookmarkDetail struct {
	Restrict Restrict `json:"restrict"`
	Tags     []string `json:"tags"`
}

// MarshalJSON keeps the established output: an absent or empty tag list is
// null, while the in-memory Tags slice stays non-nil for callers.
func (value NovelBookmarkDetail) MarshalJSON() ([]byte, error) {
	return marshalBookmarkDetail(value.Restrict, value.Tags)
}

func marshalBookmarkDetail(restrict Restrict, tags []string) ([]byte, error) {
	if len(tags) == 0 {
		tags = nil
	}
	return json.Marshal(struct {
		Restrict Restrict `json:"restrict"`
		Tags     []string `json:"tags"`
	}{restrict, tags})
}

// SearchTarget selects which novel or artwork fields a search query matches.
type SearchTarget string

// SearchTarget values select which fields a search query matches.
const (
	SearchTargetPartialMatchForTags SearchTarget = "partial_match_for_tags"
	SearchTargetExactMatchForTags   SearchTarget = "exact_match_for_tags"
	SearchTargetTitleAndCaption     SearchTarget = "title_and_caption"
	SearchTargetKeyword             SearchTarget = "keyword"
)

// SortMode orders search results.
type SortMode string

// SortMode values define the supported SortMode filesystem.
const (
	SortModeDateDesc    SortMode = "date_desc"
	SortModeDateAsc     SortMode = "date_asc"
	SortModePopularDesc SortMode = "popular_desc"
)

// DurationFilter narrows search results to a recent publication window.
type DurationFilter string

// DurationFilter values define the supported DurationFilter filesystem.
const (
	DurationLastDay   DurationFilter = "within_last_day"
	DurationLastWeek  DurationFilter = "within_last_week"
	DurationLastMonth DurationFilter = "within_last_month"
)

// RankingMode selects an artwork or novel ranking category.
type RankingMode string

// RankingMode values define the supported RankingMode filesystem.
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

// SearchContentType filters artwork search results by kind.
type SearchContentType string

// SearchContentType values define the supported SearchContentType filesystem.
const (
	SearchContentTypeAll             SearchContentType = "all"
	SearchContentTypeIllustAndUgoira SearchContentType = "illust-and-ugoira"
	SearchContentTypeIllust          SearchContentType = "illust"
	SearchContentTypeManga           SearchContentType = "manga"
	SearchContentTypeUgoira          SearchContentType = "ugoira"
)

// SearchAIMode filters artwork search results by AI-generated content.
type SearchAIMode string

// SearchAIMode values define the supported SearchAIMode filesystem.
const (
	SearchAIModeAll     SearchAIMode = "all"
	SearchAIModeExclude SearchAIMode = "exclude"
	SearchAIModeOnly    SearchAIMode = "only"
)

// SearchAspectRatio filters artwork search results by aspect ratio.
type SearchAspectRatio string

// SearchAspectRatio values define the supported SearchAspectRatio filesystem.
const (
	SearchAspectRatioAll       SearchAspectRatio = "all"
	SearchAspectRatioLandscape SearchAspectRatio = "landscape"
	SearchAspectRatioPortrait  SearchAspectRatio = "portrait"
	SearchAspectRatioSquare    SearchAspectRatio = "square"
)

// SearchResolution filters artwork search results by resolution band.
type SearchResolution string

// SearchResolution values define the supported SearchResolution filesystem.
const (
	SearchResolutionAll    SearchResolution = "all"
	SearchResolutionHigh   SearchResolution = "high"
	SearchResolutionMedium SearchResolution = "medium"
	SearchResolutionLow    SearchResolution = "low"
)
