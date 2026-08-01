package pixiv_test

import (
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pixiv"
)

func TestCompileIllustFilterMatchesTypedIllustFields(t *testing.T) {
	filter, err := pixiv.CompileIllustFilter(`bookmarkCount >= 5000 and xRestrict == 0 and any(tags, # in ["miku", "vocaloid"])`)
	if err != nil {
		t.Fatal(err)
	}

	matched, err := filter.Match(pixiv.Illust{
		ID:             42,
		Title:          "Miku",
		Type:           "illust",
		TotalBookmarks: 5000,
		XRestrict:      0,
		Tags:           []pixiv.Tag{{Name: "miku"}},
		User:           pixiv.User{ID: 7, Name: "artist"},
	})
	if err != nil || !matched {
		t.Fatalf("Match() = %v, %v; want true, nil", matched, err)
	}

	matched, err = filter.Match(pixiv.Illust{TotalBookmarks: 6000, XRestrict: 1, Tags: []pixiv.Tag{{Name: "miku"}}})
	if err != nil || matched {
		t.Fatalf("Match() = %v, %v; want false, nil", matched, err)
	}
}

func TestCompileIllustFilterRejectsUnsafeAndUnknownSyntax(t *testing.T) {
	for _, expression := range []string{
		`bookmarkCount + 1 > 2`,
		`bookmarkCount > 1 && xRestrict == 0`,
		`!false`,
		`title matches ".*"`,
		`let x = id; x == 1`,
		`unknown == 1`,
		`userId.String() == "1"`,
	} {
		t.Run(expression, func(t *testing.T) {
			_, err := pixiv.CompileIllustFilter(expression)
			if err == nil {
				t.Fatal("CompileIllustFilter() error = nil")
			}
			if !strings.Contains(err.Error(), "filter") {
				t.Fatalf("error %q does not identify the filter", err)
			}
		})
	}
}

func TestCompileIllustFilterResolutionMatchesSearchBounds(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		width   int
		height  int
		matched bool
	}{
		{"high", `resolution == "high"`, 3000, 3000, true},
		{"wide but not high", `resolution == "high"`, 5000, 1000, false},
		{"medium", `resolution == "medium"`, 1000, 2999, true},
		{"medium max boundary", `resolution == "medium"`, 2999, 2999, true},
		{"low", `resolution == "low"`, 999, 999, true},
		{"unknown dimensions", `resolution == "all"`, 0, 999, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filter, err := pixiv.CompileIllustFilter(test.expr)
			if err != nil {
				t.Fatal(err)
			}
			matched, err := filter.Match(pixiv.Illust{Width: test.width, Height: test.height})
			if err != nil || matched != test.matched {
				t.Fatalf("Match() = %v, %v; want %v", matched, err, test.matched)
			}
		})
	}
}
