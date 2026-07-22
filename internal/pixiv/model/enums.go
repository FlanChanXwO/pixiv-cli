package model

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

// SearchIllustFilters 是 facade 与协议 adapter 之间的规范化搜索筛选。
type SearchIllustFilters struct {
	Rating      string
	ContentType string
	AIMode      string
	AspectRatio string
	Resolution  string
	Tool        string
}
