package pagination_test

import (
	"errors"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
)

func TestParsePlanKeepsCLIAndMCPRules(t *testing.T) {
	zero, one, three := 0, 1, 3
	for _, tc := range []struct {
		name              string
		limit, page       int
		limitSet, pageSet bool
		want              pagination.PagePlan
		wantErr           error
	}{
		{"omitted limit reads one batch", 0, 0, false, false, pagination.PagePlan{Page: 1, OneBatch: true}, nil},
		{"explicit zero limit reads all", 0, 0, true, false, pagination.PagePlan{Page: 1}, nil},
		{"page shifts the window", 10, 3, true, true, pagination.PagePlan{Page: 3, Limit: 10, Skip: 20}, nil},
		{"negative limit", -1, 0, true, false, pagination.PagePlan{}, pagination.ErrLimitNegative},
		{"explicit zero page", 10, 0, true, true, pagination.PagePlan{}, pagination.ErrPageNotPositive},
		{"page without limit", 0, 2, false, true, pagination.PagePlan{}, pagination.ErrPageRequiresLimit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := pagination.ParsePlan(tc.limit, tc.page, tc.limitSet, tc.pageSet)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("plan = %+v, err = %v, want %+v", got, err, tc.want)
			}
		})
	}
	for _, tc := range []struct {
		limit, page *int
		want        pagination.PagePlan
	}{
		{nil, nil, pagination.PagePlan{Page: 1, OneBatch: true}},
		{&zero, nil, pagination.PagePlan{Page: 1}},
		{&three, &one, pagination.PagePlan{Page: 1, Limit: 3}},
	} {
		got, err := pagination.ParseInput(tc.page, tc.limit)
		if err != nil || got != tc.want {
			t.Fatalf("ParseInput(%v, %v) = %+v, %v; want %+v", tc.page, tc.limit, got, err, tc.want)
		}
	}
}
