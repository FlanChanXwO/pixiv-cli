package webapi

import (
	"fmt"
	"net/url"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/model"
)

func webSearchOrder(sort string) string {
	switch sort {
	case "", string(model.SortModeDateDesc):
		return "date_d"
	case string(model.SortModeDateAsc):
		return "date"
	default:
		return sort
	}
}

func webSearchMode(target string) string {
	switch target {
	case "", string(model.SearchTargetPartialMatchForTags):
		return "s_tag"
	case string(model.SearchTargetExactMatchForTags):
		return "s_tag_full"
	case string(model.SearchTargetTitleAndCaption):
		return "s_tc"
	default:
		return target
	}
}

func webRankingMode(mode string) string {
	switch mode {
	case "", string(model.RankingModeDay):
		return "daily"
	case string(model.RankingModeDayMale):
		return "male"
	case string(model.RankingModeDayFemale):
		return "female"
	case string(model.RankingModeWeek):
		return "weekly"
	case string(model.RankingModeWeekOriginal):
		return "original"
	case string(model.RankingModeWeekRookie):
		return "rookie"
	case string(model.RankingModeMonth):
		return "monthly"
	default:
		return mode
	}
}

func setDuration(q url.Values, duration string) error {
	if duration == "" {
		return nil
	}
	now := time.Now()
	var start time.Time
	switch duration {
	case "within_last_day":
		start = now.AddDate(0, 0, -1)
	case "within_last_week":
		start = now.AddDate(0, 0, -7)
	case "within_last_month":
		start = now.AddDate(0, -1, 0)
	default:
		return fmt.Errorf("pixiv web fallback does not support duration %q", duration)
	}
	q.Set("scd", start.Format("2006-01-02"))
	q.Set("ecd", now.Format("2006-01-02"))
	return nil
}
