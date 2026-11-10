package pixiv

import "github.com/FlanChanXwO/pixiv-cli/sdk"

// RankingMode selects an artwork or novel ranking category.
type RankingMode string

const (
	// RankingModeDay selects the daily ranking.
	RankingModeDay RankingMode = "day"
	// RankingModeDayMale selects the daily ranking among male readers.
	RankingModeDayMale RankingMode = "day_male"
	// RankingModeDayFemale selects the daily ranking among female readers.
	RankingModeDayFemale RankingMode = "day_female"
	// RankingModeWeek selects the weekly ranking.
	RankingModeWeek RankingMode = "week"
	// RankingModeWeekOriginal selects the weekly original-work ranking.
	RankingModeWeekOriginal RankingMode = "week_original"
	// RankingModeWeekRookie selects the weekly newcomer ranking.
	RankingModeWeekRookie RankingMode = "week_rookie"
	// RankingModeMonth selects the monthly ranking.
	RankingModeMonth RankingMode = "month"
	// RankingModeDayManga selects the daily manga ranking.
	RankingModeDayManga RankingMode = "day_manga"
	// RankingModeWeekManga selects the weekly manga ranking.
	RankingModeWeekManga RankingMode = "week_manga"
	// RankingModeMonthManga selects the monthly manga ranking.
	RankingModeMonthManga RankingMode = "month_manga"
	// RankingModeWeekRookieManga selects the weekly newcomer manga ranking.
	RankingModeWeekRookieManga RankingMode = "week_rookie_manga"
	// RankingModeDayR18 selects the daily R-18 ranking.
	RankingModeDayR18 RankingMode = "day_r18"
	// RankingModeDayMaleR18 selects the daily R-18 ranking among male readers.
	RankingModeDayMaleR18 RankingMode = "day_male_r18"
	// RankingModeDayFemaleR18 selects the daily R-18 ranking among female readers.
	RankingModeDayFemaleR18 RankingMode = "day_female_r18"
	// RankingModeWeekR18 selects the weekly R-18 ranking.
	RankingModeWeekR18 RankingMode = "week_r18"
	// RankingModeWeekR18G selects the weekly R-18G ranking.
	RankingModeWeekR18G RankingMode = "week_r18g"
)

func rankingMode(mode RankingMode) (RankingMode, error) {
	if mode == "" {
		mode = RankingModeDay
	}
	switch mode {
	case RankingModeDay, RankingModeDayMale, RankingModeDayFemale, RankingModeWeek,
		RankingModeWeekOriginal, RankingModeWeekRookie, RankingModeMonth,
		RankingModeDayManga, RankingModeWeekManga, RankingModeMonthManga,
		RankingModeWeekRookieManga, RankingModeDayR18, RankingModeDayMaleR18,
		RankingModeDayFemaleR18, RankingModeWeekR18, RankingModeWeekR18G:
		return mode, nil
	default:
		return "", newError("Ranking", sdk.InvalidArgument, "unsupported ranking mode", nil)
	}
}
