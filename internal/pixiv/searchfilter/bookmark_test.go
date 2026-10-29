package searchfilter

import (
	"errors"
	"testing"
)

func TestBookmarkFilterContract(t *testing.T) {
	min, max := 5, 10
	negative := -1
	lo, hi := 10, 5
	cases := []struct {
		name     string
		min, max *int
		wantErr  error
		matches  []int
		rejects  []int
	}{
		{"open", nil, nil, nil, []int{0, 100}, nil},
		{"bounded", &min, &max, nil, []int{5, 7, 10}, []int{4, 11}},
		{"negative", &negative, nil, ErrBookmarkMinimum, nil, nil},
		{"inverted", &lo, &hi, ErrBookmarkRangeOrder, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateBookmarkRange(tc.min, tc.max)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				return
			}
			filter := BookmarkRange{Min: tc.min, Max: tc.max}
			for _, count := range tc.matches {
				if !filter.Matches(count) {
					t.Fatalf("count %d rejected", count)
				}
			}
			for _, count := range tc.rejects {
				if filter.Matches(count) {
					t.Fatalf("count %d accepted", count)
				}
			}
		})
	}

	strategies := []struct {
		requested  string
		membership BookmarkMembership
		hasRange   bool
		want       BookmarkStrategy
		wantErr    error
	}{
		{"", BookmarkMembershipUnknown, true, BookmarkStrategyLocal, nil},
		{"", BookmarkMembershipUnknown, false, BookmarkStrategyAuto, nil},
		{"local", BookmarkMembershipUnknown, true, BookmarkStrategyLocal, nil},
		{"best_effort", BookmarkMembershipUnknown, true, BookmarkStrategyBestEffort, nil},
		{"server", BookmarkMembershipNonPremium, true, "", ErrBookmarkNonPremium},
		{"server", BookmarkMembershipUnknown, true, "", ErrBookmarkServer},
		{"bogus", BookmarkMembershipUnknown, true, "", ErrBookmarkStrategy},
	}
	for _, tc := range strategies {
		got, err := ResolveBookmarkStrategy(tc.requested, tc.membership, tc.hasRange)
		if !errors.Is(err, tc.wantErr) || got != tc.want {
			t.Fatalf("%q/%v/%v = %q, %v; want %q, %v", tc.requested, tc.membership, tc.hasRange, got, err, tc.want, tc.wantErr)
		}
	}

	if got := (BookmarkRange{Strategy: BookmarkStrategyLocal}).CursorContext(); got == "" {
		t.Fatal("empty cursor context")
	}
	if got := (BookmarkRange{Strategy: BookmarkStrategyAuto}).Completeness(false); got != BookmarkCompletenessCompleteForSource {
		t.Fatalf("completeness = %q", got)
	}
	if got := (BookmarkRange{Strategy: BookmarkStrategyBestEffort}).Completeness(false); got != BookmarkCompletenessPartial {
		t.Fatalf("best_effort completeness = %q", got)
	}
}
