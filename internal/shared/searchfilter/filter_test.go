package searchfilter_test

import (
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/shared/searchfilter"
	"github.com/stretchr/testify/require"
)

func TestNormalizeFilterCanonicalizesRatingAndContentType(t *testing.T) {
	tests := []struct {
		name        string
		rating      string
		contentType string
		wantRating  searchfilter.Rating
		wantContent searchfilter.ContentType
	}{
		{name: "empty values are unrestricted", wantRating: searchfilter.RatingAll, wantContent: searchfilter.ContentTypeAll},
		{name: "trim and case fold", rating: " R18 ", contentType: " ILLUSTRATION ", wantRating: searchfilter.RatingR18, wantContent: searchfilter.ContentTypeIllust},
		{name: "illustration alias", contentType: "illustration", wantRating: searchfilter.RatingAll, wantContent: searchfilter.ContentTypeIllust},
		{name: "aggregate content type", contentType: "illust-and-ugoira", wantRating: searchfilter.RatingAll, wantContent: searchfilter.ContentTypeIllustAndUgoira},
		{name: "mature rating", rating: "mature", wantRating: searchfilter.RatingMature, wantContent: searchfilter.ContentTypeAll},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := searchfilter.NormalizeFilter(test.rating, test.contentType)
			require.NoError(t, err)
			require.Equal(t, test.wantRating, got.Rating)
			require.Equal(t, test.wantContent, got.ContentType)
		})
	}
}

func TestNormalizeFilterRejectsUnknownValues(t *testing.T) {
	for _, test := range []struct {
		name        string
		rating      string
		contentType string
		wantMessage string
	}{
		{name: "rating", rating: "explicit", wantMessage: "rating must be one of"},
		{name: "content type", contentType: "animation", wantMessage: "content-type must be one of"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := searchfilter.NormalizeFilter(test.rating, test.contentType)
			require.Error(t, err)
			require.ErrorContains(t, err, test.wantMessage)
		})
	}
}

func TestFilterMatchesLocalRatingAndContentType(t *testing.T) {
	filter, err := searchfilter.NormalizeFilter("mature", "illust-and-ugoira")
	require.NoError(t, err)

	for _, test := range []struct {
		name      string
		xRestrict int
		kind      string
		wantMatch bool
	}{
		{name: "r18 illustration", xRestrict: 1, kind: "illustration", wantMatch: true},
		{name: "r18g ugoira", xRestrict: 2, kind: "ugoira", wantMatch: true},
		{name: "safe illustration", xRestrict: 0, kind: "illust", wantMatch: false},
		{name: "manga excluded by aggregate", xRestrict: 1, kind: "manga", wantMatch: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.wantMatch, filter.Matches(test.xRestrict, test.kind))
		})
	}
}

func TestRatingMatchesOnlyItsKnownXRestrictClass(t *testing.T) {
	for _, test := range []struct {
		name      string
		rating    string
		xRestrict int
		wantMatch bool
	}{
		{name: "sfw matches zero", rating: "sfw", xRestrict: 0, wantMatch: true},
		{name: "sfw rejects r18", rating: "sfw", xRestrict: 1, wantMatch: false},
		{name: "r18 matches one", rating: "r18", xRestrict: 1, wantMatch: true},
		{name: "r18 rejects r18g", rating: "r18", xRestrict: 2, wantMatch: false},
		{name: "r18g matches two", rating: "r18g", xRestrict: 2, wantMatch: true},
		{name: "mature rejects unknown class", rating: "mature", xRestrict: 3, wantMatch: false},
		{name: "all preserves unknown class", rating: "all", xRestrict: 3, wantMatch: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			filter, err := searchfilter.NormalizeFilter(test.rating, "all")
			require.NoError(t, err)
			require.Equal(t, test.wantMatch, filter.Matches(test.xRestrict, "illust"))
		})
	}
}

func TestFilterContextBindsCanonicalLocalSemantics(t *testing.T) {
	empty, err := searchfilter.NormalizeFilter("", "")
	require.NoError(t, err)
	explicit, err := searchfilter.NormalizeFilter("all", "all")
	require.NoError(t, err)
	illustration, err := searchfilter.NormalizeFilter("r18", "illustration")
	require.NoError(t, err)

	require.Equal(t, empty.CursorContext(), explicit.CursorContext())
	require.NotEqual(t, empty.CursorContext(), illustration.CursorContext())
	require.Len(t, empty.CursorContext(), 64)
	require.NotContains(t, illustration.CursorContext(), "r18")
	require.NotContains(t, illustration.CursorContext(), "illustration")

	context, err := searchfilter.FilterContext(" R18 ", " ILLUSTRATION ")
	require.NoError(t, err)
	require.Equal(t, illustration.CursorContext(), context)
	require.False(t, strings.Contains(context, "x_restrict"))
}
