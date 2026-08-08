package pixiv

import "time"

const tokyoOffsetSeconds = 9 * 60 * 60

// SearchQuickDateRange 将官方 App 的长日期快捷项规范化为包含边界的日本日期。
// App 搜索接口以 start_date/end_date 表示半年和一年，而非可依赖的 duration 枚举。
func SearchQuickDateRange(value string, now time.Time) (startDate, endDate string, ok bool) {
	today := now.In(time.FixedZone("Asia/Tokyo", tokyoOffsetSeconds))
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())

	var start time.Time
	switch value {
	case "within_half_year":
		start = addMonthsClamped(today, -6)
	case "within_year":
		start = addMonthsClamped(today, -12)
	default:
		return "", "", false
	}
	return start.Format("2006-01-02"), today.Format("2006-01-02"), true
}

// addMonthsClamped 对齐 LocalDate.minusMonths/minusYears：目标月份没有同一天时取该月末日。
func addMonthsClamped(date time.Time, months int) time.Time {
	monthIndex := int(date.Month()) - 1 + months
	year := date.Year() + monthIndex/12
	monthIndex %= 12
	if monthIndex < 0 {
		monthIndex += 12
		year--
	}
	month := time.Month(monthIndex + 1)
	lastDay := time.Date(year, month+1, 0, 0, 0, 0, 0, date.Location()).Day()
	day := min(date.Day(), lastDay)
	return time.Date(year, month, day, 0, 0, 0, 0, date.Location())
}
