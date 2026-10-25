package searchfilter_test

import (
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/searchfilter"
	"github.com/stretchr/testify/require"
)

func TestNormalizeFilterCanonicalizesAndRejectsUnknownValues(t *testing.T) {
	tests := []struct {
		name        string
		rating      string
		contentType string
		wantRating  searchfilter.Rating
		wantContent searchfilter.ContentType
		wantError   string
	}{
		{name: "empty values are unrestricted", wantRating: searchfilter.RatingAll, wantContent: searchfilter.ContentTypeAll},
		{name: "trim and case fold", rating: " R18 ", contentType: " ILLUST ", wantRating: searchfilter.RatingR18, wantContent: searchfilter.ContentTypeIllust},
		{name: "aggregate content type", contentType: "illust-and-ugoira", wantRating: searchfilter.RatingAll, wantContent: searchfilter.ContentTypeIllustAndUgoira},
		{name: "mature rating", rating: "mature", wantRating: searchfilter.RatingMature, wantContent: searchfilter.ContentTypeAll},
		{name: "unknown rating", rating: "explicit", wantError: "rating must be one of"},
		{name: "unknown content type", contentType: "animation", wantError: "content-type must be one of"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := searchfilter.NormalizeFilter(test.rating, test.contentType)
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.wantRating, got.Rating)
			require.Equal(t, test.wantContent, got.ContentType)
		})
	}
}

func TestFilterMatchesLocalRatingAndContentType(t *testing.T) {
	tests := []struct {
		name        string
		rating      string
		contentType string
		xRestrict   int
		kind        string
		wantMatch   bool
	}{
		{name: "mature r18 illustration", rating: "mature", contentType: "illust-and-ugoira", xRestrict: 1, kind: "illustration", wantMatch: true},
		{name: "mature r18g ugoira", rating: "mature", contentType: "illust-and-ugoira", xRestrict: 2, kind: "ugoira", wantMatch: true},
		{name: "safe rejected by mature", rating: "mature", contentType: "illust-and-ugoira", xRestrict: 0, kind: "illust"},
		{name: "manga excluded by aggregate", rating: "mature", contentType: "illust-and-ugoira", xRestrict: 1, kind: "manga"},
		{name: "sfw matches zero", rating: "sfw", xRestrict: 0, kind: "illust", wantMatch: true},
		{name: "sfw rejects r18", rating: "sfw", xRestrict: 1, kind: "illust"},
		{name: "r18 matches one", rating: "r18", xRestrict: 1, kind: "illust", wantMatch: true},
		{name: "r18 rejects r18g", rating: "r18", xRestrict: 2, kind: "illust"},
		{name: "r18g matches two", rating: "r18g", xRestrict: 2, kind: "illust", wantMatch: true},
		{name: "mature rejects unknown class", rating: "mature", xRestrict: 3, kind: "illust"},
		{name: "all preserves unknown class", rating: "all", xRestrict: 3, kind: "illust", wantMatch: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contentType := test.contentType
			if contentType == "" {
				contentType = "all"
			}
			filter, err := searchfilter.NormalizeFilter(test.rating, contentType)
			require.NoError(t, err)
			require.Equal(t, test.wantMatch, filter.Matches(test.xRestrict, test.kind))
		})
	}
}

func TestFilterContextBindsCanonicalLocalSemantics(t *testing.T) {
	empty, err := searchfilter.NormalizeFilter("", "")
	require.NoError(t, err)
	explicit, err := searchfilter.NormalizeFilter("all", "all")
	require.NoError(t, err)
	illustration, err := searchfilter.NormalizeFilter("r18", "illust")
	require.NoError(t, err)

	require.Equal(t, empty.CursorContext(), explicit.CursorContext())
	require.NotEqual(t, empty.CursorContext(), illustration.CursorContext())
	require.Len(t, empty.CursorContext(), 64)
	require.NotContains(t, illustration.CursorContext(), "r18")
	require.NotContains(t, illustration.CursorContext(), "illust")

	trimmed, err := searchfilter.NormalizeFilter(" R18 ", " ILLUST ")
	require.NoError(t, err)
	require.Equal(t, illustration.CursorContext(), trimmed.CursorContext())
	require.False(t, strings.Contains(trimmed.CursorContext(), "x_restrict"))
}
