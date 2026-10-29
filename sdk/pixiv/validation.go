package pixiv

import (
	"net/url"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// validateBookmarkRange checks an inclusive range at the public SDK boundary and
// adds no hidden upper limit.
func validateBookmarkRange(operation string, minimum, maximum *int) error {
	if minimum != nil && *minimum < 0 {
		return newError(operation, sdk.InvalidArgument, "bookmark minimum must be non-negative")
	}
	if maximum != nil && *maximum < 0 {
		return newError(operation, sdk.InvalidArgument, "bookmark maximum must be non-negative")
	}
	if minimum != nil && maximum != nil && *minimum > *maximum {
		return newError(operation, sdk.InvalidArgument, "bookmark minimum must not exceed maximum")
	}
	return nil
}

func validateRestrict(operation string, value Restrict) error {
	switch value {
	case "", RestrictPublic, RestrictPrivate:
		return nil
	default:
		return newError(operation, sdk.InvalidArgument, "restrict is unsupported")
	}
}

func normalizeFollowingRestrict(operation string, value Restrict) (Restrict, error) {
	if value == "" {
		value = RestrictPublic
	}
	if err := validateRestrict(operation, value); err != nil {
		return "", err
	}
	return value, nil
}

// normalizeLatestArtworkContentType resolves an empty public value to the frozen
// illustration feed and rejects the composite and ugoira subtypes the SDK does
// not yet support there.
func normalizeLatestArtworkContentType(operation string, value SearchContentType) (string, error) {
	switch value {
	case "", SearchContentTypeIllust:
		return string(SearchContentTypeIllust), nil
	case SearchContentTypeManga:
		return string(SearchContentTypeManga), nil
	default:
		return "", newError(operation, sdk.InvalidArgument, "content type is unsupported for latest artworks")
	}
}

// normalizeUserArtworkKind maps an empty public value and illust to the same query
// digest, so a valid cursor still resumes.
func normalizeUserArtworkKind(operation string, value ArtworkKind) (string, error) {
	switch value {
	case "", ArtworkKind("illust"):
		return "illust", nil
	case ArtworkKindManga, ArtworkKindUgoira:
		return string(value), nil
	default:
		return "", newError(operation, sdk.InvalidArgument, "artwork kind is unsupported")
	}
}

// applyResolutionFilter translates a public resolution band into App API width
// and height bounds.
func applyResolutionFilter(query url.Values, resolution SearchResolution) {
	switch resolution {
	case SearchResolutionHigh:
		query.Set(api.ParamWidthMin, "3000")
		query.Set(api.ParamHeightMin, "3000")
	case SearchResolutionMedium:
		query.Set(api.ParamWidthMin, "1000")
		query.Set(api.ParamWidthMax, "2999")
		query.Set(api.ParamHeightMin, "1000")
		query.Set(api.ParamHeightMax, "2999")
	case SearchResolutionLow:
		query.Set(api.ParamWidthMax, "999")
		query.Set(api.ParamHeightMax, "999")
	}
}

func validateSearchWord(operation, value string) error {
	if strings.TrimSpace(value) == "" {
		return newError(operation, sdk.InvalidArgument, "search word is required")
	}
	return nil
}

func validateSearchArtworksRequest(operation string, request SearchArtworksRequest) error {
	if err := validateSearchTarget(operation, request.Target); err != nil {
		return err
	}
	if err := validateSortMode(operation, request.Sort); err != nil {
		return err
	}
	if err := validateDuration(operation, request.Duration); err != nil {
		return err
	}
	if err := validateSearchContentType(operation, request.ContentType); err != nil {
		return err
	}
	if err := validateSearchAIMode(operation, request.AIMode); err != nil {
		return err
	}
	if err := validateSearchAspectRatio(operation, request.AspectRatio); err != nil {
		return err
	}
	if err := validateSearchResolution(operation, request.Resolution); err != nil {
		return err
	}
	return validateDateRange(operation, request.StartDate, request.EndDate)
}

func validateSearchTarget(operation string, value SearchTarget) error {
	switch value {
	case "", SearchTargetPartialMatchForTags, SearchTargetExactMatchForTags, SearchTargetTitleAndCaption, SearchTargetKeyword:
		return nil
	default:
		return newError(operation, sdk.InvalidArgument, "search target is unsupported")
	}
}

func validateSortMode(operation string, value SortMode) error {
	switch value {
	case "", SortModeDateDesc, SortModeDateAsc, SortModePopularDesc:
		return nil
	default:
		return newError(operation, sdk.InvalidArgument, "sort mode is unsupported")
	}
}

func validateDuration(operation string, value DurationFilter) error {
	switch value {
	case "", DurationLastDay, DurationLastWeek, DurationLastMonth:
		return nil
	default:
		return newError(operation, sdk.InvalidArgument, "duration is unsupported")
	}
}

func validateSearchContentType(operation string, value SearchContentType) error {
	switch value {
	case "", SearchContentTypeAll, SearchContentTypeIllustAndUgoira, SearchContentTypeIllust, SearchContentTypeManga, SearchContentTypeUgoira:
		return nil
	default:
		return newError(operation, sdk.InvalidArgument, "content type is unsupported")
	}
}

func validateSearchAIMode(operation string, value SearchAIMode) error {
	switch value {
	case "", SearchAIModeAll, SearchAIModeExclude, SearchAIModeOnly:
		return nil
	default:
		return newError(operation, sdk.InvalidArgument, "AI mode is unsupported")
	}
}

func validateSearchAspectRatio(operation string, value SearchAspectRatio) error {
	switch value {
	case "", SearchAspectRatioAll, SearchAspectRatioLandscape, SearchAspectRatioPortrait, SearchAspectRatioSquare:
		return nil
	default:
		return newError(operation, sdk.InvalidArgument, "aspect ratio is unsupported")
	}
}

func validateSearchResolution(operation string, value SearchResolution) error {
	switch value {
	case "", SearchResolutionAll, SearchResolutionHigh, SearchResolutionMedium, SearchResolutionLow:
		return nil
	default:
		return newError(operation, sdk.InvalidArgument, "resolution is unsupported")
	}
}

func validateRankingMode(operation string, value RankingMode) error {
	switch value {
	case RankingModeDay, RankingModeDayMale, RankingModeDayFemale, RankingModeWeek,
		RankingModeWeekOriginal, RankingModeWeekRookie, RankingModeMonth,
		RankingModeDayManga, RankingModeWeekManga, RankingModeMonthManga,
		RankingModeWeekRookieManga, RankingModeDayR18, RankingModeDayMaleR18,
		RankingModeDayFemaleR18, RankingModeWeekR18, RankingModeWeekR18G:
		return nil
	default:
		return newError(operation, sdk.InvalidArgument, "ranking mode is unsupported")
	}
}

func validateDateRange(operation, startDate, endDate string) error {
	if err := validateDate(operation, "start date", startDate); err != nil {
		return err
	}
	if err := validateDate(operation, "end date", endDate); err != nil {
		return err
	}
	if startDate != "" && endDate != "" && startDate > endDate {
		return newError(operation, sdk.InvalidArgument, "start date must not be later than end date")
	}
	return nil
}

func validateDate(operation, field, value string) error {
	if value == "" {
		return nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil || parsed.Format("2006-01-02") != value {
		return newError(operation, sdk.InvalidArgument, field+" must use YYYY-MM-DD")
	}
	return nil
}
