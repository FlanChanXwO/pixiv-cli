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
	RankingModeDay          RankingMode = "day"
	RankingModeDayMale      RankingMode = "day_male"
	RankingModeDayFemale    RankingMode = "day_female"
	RankingModeWeek         RankingMode = "week"
	RankingModeWeekOriginal RankingMode = "week_original"
	RankingModeWeekRookie   RankingMode = "week_rookie"
	RankingModeMonth        RankingMode = "month"
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
