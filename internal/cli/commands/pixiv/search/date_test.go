package search

import (
	"testing"
	"time"
)

func TestQuickDateRangeUsesTokyoDateAndClampsMonthEnd(t *testing.T) {
	location := time.FixedZone("UTC-8", -8*60*60)
	now := time.Date(2026, time.August, 31, 23, 0, 0, 0, location)
	start, end, ok := quickDateRange("within_half_year", now)
	if !ok || start != "2026-03-01" || end != "2026-09-01" {
		t.Fatalf("half year range = %q..%q ok=%v", start, end, ok)
	}
}

func TestQuickDateRangeClampsLeapDayForYear(t *testing.T) {
	now := time.Date(2024, time.February, 29, 12, 0, 0, 0, time.FixedZone("Asia/Tokyo", 9*60*60))
	start, end, ok := quickDateRange("within_year", now)
	if !ok || start != "2023-02-28" || end != "2024-02-29" {
		t.Fatalf("year range = %q..%q ok=%v", start, end, ok)
	}
}

func TestQuickDateRangeRejectsUnknownValue(t *testing.T) {
	if _, _, ok := quickDateRange("within_last_month", time.Now()); ok {
		t.Fatal("short duration unexpectedly resolved to an explicit date range")
	}
}
