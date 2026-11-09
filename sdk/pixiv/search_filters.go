package pixiv

import (
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// SearchTarget selects the fields matched by an artwork or novel search.
type SearchTarget string

const (
	// SearchTargetPartialMatchForTags matches tags partially; this is the default.
	SearchTargetPartialMatchForTags SearchTarget = "partial_match_for_tags"
	// SearchTargetExactMatchForTags matches complete tags.
	SearchTargetExactMatchForTags SearchTarget = "exact_match_for_tags"
	// SearchTargetTitleAndCaption matches titles and captions.
	SearchTargetTitleAndCaption SearchTarget = "title_and_caption"
	// SearchTargetKeyword matches free keywords.
	SearchTargetKeyword SearchTarget = "keyword"
)

// SortMode orders search results by publication date or popularity.
type SortMode string

const (
	// SortModeDateDesc orders newest first; this is the default.
	SortModeDateDesc SortMode = "date_desc"
	// SortModeDateAsc orders oldest first.
	SortModeDateAsc SortMode = "date_asc"
	// SortModePopularDesc orders most popular first.
	SortModePopularDesc SortMode = "popular_desc"
)

// DurationFilter narrows search to a recent publication window; zero leaves it unrestricted.
type DurationFilter string

const (
	// DurationLastDay selects the preceding day.
	DurationLastDay DurationFilter = "within_last_day"
	// DurationLastWeek selects the preceding week.
	DurationLastWeek DurationFilter = "within_last_week"
	// DurationLastMonth selects the preceding month.
	DurationLastMonth DurationFilter = "within_last_month"
)

// ArtworkCategory selects categories for artwork search and latest feeds.
type ArtworkCategory string

const (
	// ArtworkCategoryAll leaves categories unrestricted, like the zero value.
	ArtworkCategoryAll ArtworkCategory = "all"
	// ArtworkCategoryIllustAndUgoira includes illustrations and animations.
	ArtworkCategoryIllustAndUgoira ArtworkCategory = "illust-and-ugoira"
	// ArtworkCategoryIllust selects illustrations.
	ArtworkCategoryIllust ArtworkCategory = "illust"
	// ArtworkCategoryManga selects manga.
	ArtworkCategoryManga ArtworkCategory = "manga"
	// ArtworkCategoryUgoira selects animations.
	ArtworkCategoryUgoira ArtworkCategory = "ugoira"
)

// SearchAIMode selects how artwork search handles AI-generated works.
type SearchAIMode string

const (
	// SearchAIModeAll includes all works, like the zero value.
	SearchAIModeAll SearchAIMode = "all"
	// SearchAIModeExclude asks upstream to exclude AI-generated works.
	SearchAIModeExclude SearchAIMode = "exclude"
	// SearchAIModeOnly locally retains works with upstream AI classification 2.
	SearchAIModeOnly SearchAIMode = "only"
)

// SearchAspectRatio narrows artwork search by shape.
type SearchAspectRatio string

const (
	// SearchAspectRatioAll leaves shapes unrestricted, like the zero value.
	SearchAspectRatioAll SearchAspectRatio = "all"
	// SearchAspectRatioLandscape selects horizontal works.
	SearchAspectRatioLandscape SearchAspectRatio = "landscape"
	// SearchAspectRatioPortrait selects vertical works.
	SearchAspectRatioPortrait SearchAspectRatio = "portrait"
	// SearchAspectRatioSquare selects square works.
	SearchAspectRatioSquare SearchAspectRatio = "square"
)

// SearchResolution narrows artwork search by upstream pixel dimensions.
type SearchResolution string

const (
	// SearchResolutionAll leaves dimensions unrestricted, like the zero value.
	SearchResolutionAll SearchResolution = "all"
	// SearchResolutionHigh requires both dimensions to be at least 3000 pixels.
	SearchResolutionHigh SearchResolution = "high"
	// SearchResolutionMedium requires both dimensions to be between 1000 and 2999 pixels.
	SearchResolutionMedium SearchResolution = "medium"
	// SearchResolutionLow requires both dimensions to be at most 999 pixels.
	SearchResolutionLow SearchResolution = "low"
)

const (
	searchAIInclude               = "0"
	searchAIExclude               = "1"
	searchCombinedCategory        = "illust_and_ugoira"
	searchHighResolutionMinimum   = 3000
	searchMediumResolutionMinimum = 1000
)

func searchQuery(word string, target SearchTarget, sort SortMode, duration DurationFilter) (url.Values, error) {
	if strings.TrimSpace(word) == "" {
		return nil, newError("Search", sdk.InvalidArgument, "search word is required", nil)
	}
	if target == "" {
		target = SearchTargetPartialMatchForTags
	}
	if sort == "" {
		sort = SortModeDateDesc
	}
	if !slices.Contains([]SearchTarget{SearchTargetPartialMatchForTags, SearchTargetExactMatchForTags, SearchTargetTitleAndCaption, SearchTargetKeyword}, target) {
		return nil, newError("Search", sdk.InvalidArgument, "unsupported search target", nil)
	}
	if !slices.Contains([]SortMode{SortModeDateDesc, SortModeDateAsc, SortModePopularDesc}, sort) {
		return nil, newError("Search", sdk.InvalidArgument, "unsupported search ordering", nil)
	}
	if !slices.Contains([]DurationFilter{"", DurationLastDay, DurationLastWeek, DurationLastMonth}, duration) {
		return nil, newError("Search", sdk.InvalidArgument, "unsupported search duration", nil)
	}
	query := url.Values{api.ParamWord: {word}, api.ParamSearchTarget: {string(target)}, api.ParamSort: {string(sort)}}
	if duration != "" {
		query.Set(api.ParamDuration, string(duration))
	}
	return query, nil
}

func artworkSearchQuery(word string, options ArtworkSearchOptions) (url.Values, error) {
	// 1. Normalize defaults and validate public filter values before credential I/O.
	query, err := searchQuery(word, options.Target, options.Sort, options.Duration)
	if err != nil {
		return nil, err
	}
	if options.Offset < 0 {
		return nil, newError("SearchArtworks", sdk.InvalidArgument, "offset must be nonnegative", nil)
	}
	if !validDate(options.StartDate) || !validDate(options.EndDate) {
		return nil, newError("SearchArtworks", sdk.InvalidArgument, "invalid inclusive YYYY-MM-DD date bounds", nil)
	}
	if options.StartDate != "" && options.EndDate != "" && options.StartDate > options.EndDate {
		return nil, newError("SearchArtworks", sdk.InvalidArgument, "start date must not be later than end date", nil)
	}
	if options.BookmarkMin != nil && *options.BookmarkMin < 0 || options.BookmarkMax != nil && *options.BookmarkMax < 0 {
		return nil, newError("SearchArtworks", sdk.InvalidArgument, "bookmark count bounds must be nonnegative", nil)
	}
	if options.BookmarkMin != nil && options.BookmarkMax != nil && *options.BookmarkMin > *options.BookmarkMax {
		return nil, newError("SearchArtworks", sdk.InvalidArgument, "bookmark minimum must not exceed maximum", nil)
	}
	if !slices.Contains([]ArtworkCategory{"", ArtworkCategoryAll, ArtworkCategoryIllustAndUgoira, ArtworkCategoryIllust, ArtworkCategoryManga, ArtworkCategoryUgoira}, options.Category) {
		return nil, newError("SearchArtworks", sdk.InvalidArgument, "unsupported artwork category", nil)
	}
	if !slices.Contains([]SearchAIMode{"", SearchAIModeAll, SearchAIModeExclude, SearchAIModeOnly}, options.AIMode) {
		return nil, newError("SearchArtworks", sdk.InvalidArgument, "unsupported AI mode", nil)
	}
	if !slices.Contains([]SearchAspectRatio{"", SearchAspectRatioAll, SearchAspectRatioLandscape, SearchAspectRatioPortrait, SearchAspectRatioSquare}, options.AspectRatio) {
		return nil, newError("SearchArtworks", sdk.InvalidArgument, "unsupported aspect ratio", nil)
	}
	if !slices.Contains([]SearchResolution{"", SearchResolutionAll, SearchResolutionHigh, SearchResolutionMedium, SearchResolutionLow}, options.Resolution) {
		return nil, newError("SearchArtworks", sdk.InvalidArgument, "unsupported resolution", nil)
	}
	// 2. Bind both upstream filters and local selection semantics into the original query.
	for key, value := range map[string]string{api.ParamStartDate: options.StartDate, api.ParamEndDate: options.EndDate, api.ParamTool: options.Tool, searchContextKey: options.CursorContext} {
		if value != "" {
			query.Set(key, value)
		}
	}
	if options.Category != "" && options.Category != ArtworkCategoryAll {
		category := string(options.Category)
		if options.Category == ArtworkCategoryIllustAndUgoira {
			category = searchCombinedCategory
		}
		query.Set(api.ParamContentType, category)
	}
	query.Set(api.ParamSearchAIType, searchAIInclude)
	if options.AIMode == SearchAIModeExclude {
		query.Set(api.ParamSearchAIType, searchAIExclude)
	}
	if options.AIMode == SearchAIModeOnly {
		query.Set(searchAIModeKey, string(options.AIMode))
	}
	if options.AspectRatio != "" && options.AspectRatio != SearchAspectRatioAll {
		query.Set(api.ParamRatioPattern, string(options.AspectRatio))
	}
	if options.BookmarkMin != nil {
		query.Set(api.ParamBookmarkNumMin, strconv.Itoa(*options.BookmarkMin))
	}
	if options.BookmarkMax != nil {
		query.Set(api.ParamBookmarkNumMax, strconv.Itoa(*options.BookmarkMax))
	}
	if options.Offset > 0 {
		query.Set(searchInitialOffsetKey, strconv.Itoa(options.Offset))
	}
	switch options.Resolution {
	case SearchResolutionHigh:
		query.Set(api.ParamWidthMin, strconv.Itoa(searchHighResolutionMinimum))
		query.Set(api.ParamHeightMin, strconv.Itoa(searchHighResolutionMinimum))
	case SearchResolutionMedium:
		query.Set(api.ParamWidthMin, strconv.Itoa(searchMediumResolutionMinimum))
		query.Set(api.ParamHeightMin, strconv.Itoa(searchMediumResolutionMinimum))
		query.Set(api.ParamWidthMax, strconv.Itoa(searchHighResolutionMinimum-1))
		query.Set(api.ParamHeightMax, strconv.Itoa(searchHighResolutionMinimum-1))
	case SearchResolutionLow:
		query.Set(api.ParamWidthMax, strconv.Itoa(searchMediumResolutionMinimum-1))
		query.Set(api.ParamHeightMax, strconv.Itoa(searchMediumResolutionMinimum-1))
	}
	return query, nil
}

func validDate(value string) bool {
	if value == "" {
		return true
	}
	parsed, err := time.Parse(time.DateOnly, value)
	return err == nil && parsed.Format(time.DateOnly) == value
}
