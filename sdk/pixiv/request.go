package pixiv

import "github.com/FlanChanXwO/pixiv-cli/sdk"

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

// RankingMode selects an artwork ranking category.
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

// SearchArtworksRequest searches artworks. Repeat the original fields when
// continuing with Cursor.
type SearchArtworksRequest struct {
	Word        string
	Target      SearchTarget
	Sort        SortMode
	Duration    DurationFilter
	StartDate   string
	EndDate     string
	ContentType SearchContentType
	AIMode      SearchAIMode
	AspectRatio SearchAspectRatio
	Resolution  SearchResolution
	Tool        string
	BookmarkMin *int
	BookmarkMax *int
	Cursor      sdk.Cursor
}

// ArtworkRequest selects one artwork by its stable ID.
type ArtworkRequest struct {
	ArtworkID int64
}

// ArtworkPagesRequest selects the image pages of one artwork.
type ArtworkPagesRequest struct {
	ArtworkID int64
}

// RelatedArtworksRequest lists artworks related to one artwork.
type RelatedArtworksRequest struct {
	ArtworkID int64
	Cursor    sdk.Cursor
}

// ArtworkSeriesRequest lists artworks within one illustration series.
type ArtworkSeriesRequest struct {
	SeriesID int64
	Cursor   sdk.Cursor
}

// ArtworkRankingRequest lists the current artwork ranking.
type ArtworkRankingRequest struct {
	Mode   RankingMode
	Date   string
	Cursor sdk.Cursor
}

// RecommendedArtworksRequest lists recommended artworks.
type RecommendedArtworksRequest struct {
	Cursor sdk.Cursor
}

// FollowingArtworksRequest lists artworks by followed users.
type FollowingArtworksRequest struct {
	Restrict Restrict
	Cursor   sdk.Cursor
}

// LatestArtworksRequest lists the newest artworks.
type LatestArtworksRequest struct {
	ContentType SearchContentType
	Cursor      sdk.Cursor
}

// UserArtworksRequest lists one user's artworks.
type UserArtworksRequest struct {
	UserID int64
	Kind   ArtworkKind
	Cursor sdk.Cursor
}

// UserArtworkBookmarksRequest lists one user's bookmarked artworks.
type UserArtworkBookmarksRequest struct {
	UserID   int64
	Restrict Restrict
	Tag      string
	Cursor   sdk.Cursor
}

// UserArtworkBookmarkTagsRequest lists the bookmark tags of one user's
// bookmarked artworks.
type UserArtworkBookmarkTagsRequest struct {
	UserID   int64
	Restrict Restrict
	Cursor   sdk.Cursor
}

// MyPixivArtworksRequest lists artworks from the current user's MyPixiv feed.
type MyPixivArtworksRequest struct {
	Cursor sdk.Cursor
}

// TrendingArtworkTagsRequest lists currently trending artwork tags.
type TrendingArtworkTagsRequest struct{}

// UgoiraMetadataRequest selects the ugoira metadata of one artwork. The
// artwork must be a ugoira.
type UgoiraMetadataRequest struct {
	ArtworkID int64
}

// ArtworkCommentsRequest lists comments on one artwork.
type ArtworkCommentsRequest struct {
	ArtworkID int64
	Cursor    sdk.Cursor
}

// ArtworkBookmarkRequest reads the current user's bookmark detail for one
// artwork.
type ArtworkBookmarkRequest struct {
	ArtworkID int64
}

// SearchNovelsRequest searches novels. Repeat the original fields when
// continuing with Cursor.
type SearchNovelsRequest struct {
	Word     string
	Target   SearchTarget
	Sort     SortMode
	Duration DurationFilter
	Cursor   sdk.Cursor
}

// NovelRequest selects one novel by its stable ID.
type NovelRequest struct {
	NovelID int64
}

// NovelSeriesRequest lists novels within one novel series.
type NovelSeriesRequest struct {
	SeriesID int64
	Cursor   sdk.Cursor
}

// NovelContentRequest reads the structured content of one novel.
type NovelContentRequest struct {
	NovelID int64
}

// NovelCommentsRequest lists comments on one novel.
type NovelCommentsRequest struct {
	NovelID int64
	Cursor  sdk.Cursor
}

// RecommendedNovelsRequest lists recommended novels.
type RecommendedNovelsRequest struct {
	Cursor sdk.Cursor
}

// FollowingNovelsRequest lists novels by followed users.
type FollowingNovelsRequest struct {
	Restrict Restrict
	Cursor   sdk.Cursor
}

// LatestNovelsRequest lists the newest novels.
type LatestNovelsRequest struct {
	Cursor sdk.Cursor
}

// UserNovelsRequest lists one user's novels.
type UserNovelsRequest struct {
	UserID int64
	Cursor sdk.Cursor
}

// UserNovelBookmarksRequest lists one user's bookmarked novels.
type UserNovelBookmarksRequest struct {
	UserID   int64
	Restrict Restrict
	Tag      string
	Cursor   sdk.Cursor
}

// MyPixivNovelsRequest lists novels from the current user's MyPixiv feed.
type MyPixivNovelsRequest struct {
	Cursor sdk.Cursor
}

// SearchUsersRequest searches users.
type SearchUsersRequest struct {
	Word   string
	Cursor sdk.Cursor
}

// UserRequest selects one user by their stable ID.
type UserRequest struct {
	UserID int64
}

// RecommendedUsersRequest lists recommended users.
type RecommendedUsersRequest struct {
	Cursor sdk.Cursor
}

// RelatedUsersRequest lists users related to one user.
type RelatedUsersRequest struct {
	UserID int64
	Cursor sdk.Cursor
}

// UserFollowingRequest lists the users one user follows.
type UserFollowingRequest struct {
	UserID   int64
	Restrict Restrict
	Cursor   sdk.Cursor
}

// UserFollowersRequest lists the users following one user.
type UserFollowersRequest struct {
	UserID   int64
	Restrict Restrict
	Cursor   sdk.Cursor
}

// UserBlockedUsersRequest lists the users one user has blocked.
type UserBlockedUsersRequest struct {
	UserID int64
	Cursor sdk.Cursor
}

// MyPixivUsersRequest lists the current user's MyPixiv feed users.
type MyPixivUsersRequest struct {
	Cursor sdk.Cursor
}

// CurrentUserRequest reads the current authenticated user's detail.
type CurrentUserRequest struct{}

// AddBookmarkRequest bookmarks one artwork. Tags are applied as bookmark tags
// when non-empty.
type AddBookmarkRequest struct {
	ArtworkID int64
	Restrict  Restrict
	Tags      []string
}

// RemoveBookmarkRequest removes the current user's bookmark from one artwork.
type RemoveBookmarkRequest struct {
	ArtworkID int64
}

// FollowUserRequest follows one user.
type FollowUserRequest struct {
	UserID   int64
	Restrict Restrict
}

// UnfollowUserRequest unfollows one user.
type UnfollowUserRequest struct {
	UserID int64
}

// SetAIArtworkVisibilityRequest sets whether AI-generated artworks are shown
// in the current user's feeds.
type SetAIArtworkVisibilityRequest struct {
	Visible bool
}
