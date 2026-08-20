// Package date 提供不携带产品协议语义的日期计算 helper。
package date

import "time"

// AddMonthsClamped 按月份移动日期；目标月份没有原日期的日号时取该月末日。
//
// 时间的时区、时刻和其余字段由调用方决定，函数只负责日历上的月份移动。
func AddMonthsClamped(value time.Time, months int) time.Time {
	monthIndex := int(value.Month()) - 1 + months
	year := value.Year() + monthIndex/12
	monthIndex %= 12
	if monthIndex < 0 {
		monthIndex += 12
		year--
	}
	month := time.Month(monthIndex + 1)
	lastDay := time.Date(year, month+1, 0, 0, 0, 0, 0, value.Location()).Day()
	day := min(value.Day(), lastDay)
	return time.Date(year, month, day, value.Hour(), value.Minute(), value.Second(), value.Nanosecond(), value.Location())
}
