package record_test

import (
	"math"
	"testing"

	recordpkg "github.com/FlanChanXwO/pixiv-cli/internal/shared/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordMatchesFiltersForObjectAndStringTags(t *testing.T) {
	objectTags, err := recordpkg.ParseRecordJSON([]byte(`{
		"id":"7",
		"type":"illust",
		"url":"https://www.pixiv.net/artworks/7",
		"tags":[{"name":"tag-a"},{"name":"tag-b","translated_name":"标签 B"}],
		"total_view":5000,
		"page_count":2
	}`))
	require.NoError(t, err)
	stringTags, err := recordpkg.ParseRecordJSON([]byte(`{
		"id":"8",
		"type":"novel",
		"url":"https://www.pixiv.net/novel/show.php?id=8",
		"tags":["tag-a","tag-b"],
		"views":100,
		"page_count":1
	}`))
	require.NoError(t, err)

	filter := recordpkg.RecordFilter{
		ID:           "7",
		Type:         "illust",
		Tags:         []string{"tag-a", "tag-b"},
		MinViews:     int64Ptr(5000),
		MinPageCount: int64Ptr(2),
	}
	assert.True(t, objectTags.Matches(filter))
	assert.False(t, objectTags.Matches(recordpkg.RecordFilter{Tags: []string{"tag-a", "missing"}}))
	assert.True(t, stringTags.Matches(recordpkg.RecordFilter{Tags: []string{"tag-a", "tag-b"}, MinViews: int64Ptr(100), MinPageCount: int64Ptr(1)}))
	assert.False(t, stringTags.Matches(recordpkg.RecordFilter{MinViews: int64Ptr(101)}))
	assert.False(t, stringTags.Matches(recordpkg.RecordFilter{MinPageCount: int64Ptr(2)}))
}

func TestRecordMatchesLeavesUnfilteredRecordsUnchangedAndRejectsMissingMetrics(t *testing.T) {
	parsed, err := recordpkg.ParseRecordJSON([]byte(`{
		"id":"user-1",
		"type":"user",
		"url":"https://www.pixiv.net/users/1",
		"custom":true
	}`))
	require.NoError(t, err)

	assert.True(t, parsed.Matches(recordpkg.RecordFilter{}))
	assert.True(t, parsed.Matches(recordpkg.RecordFilter{ID: "user-1", Type: "user"}))
	assert.False(t, parsed.Matches(recordpkg.RecordFilter{ID: "other"}))
	assert.False(t, parsed.Matches(recordpkg.RecordFilter{Type: "illust"}))
	assert.False(t, parsed.Matches(recordpkg.RecordFilter{Tags: []string{"tag-a"}}))
	assert.False(t, parsed.Matches(recordpkg.RecordFilter{MinViews: int64Ptr(0)}))
	assert.False(t, parsed.Matches(recordpkg.RecordFilter{MinPageCount: int64Ptr(0)}))
}

func TestRecordMatchesTreatsNullAndWrongTypesAsMissingFields(t *testing.T) {
	nullTags, err := recordpkg.ParseRecordJSON([]byte(`{
		"id":"tags-null","type":"illust","url":"https://www.pixiv.net/artworks/1","tags":[null]
	}`))
	require.NoError(t, err)
	nullViews, err := recordpkg.ParseRecordJSON([]byte(`{
		"id":"views-null","type":"illust","url":"https://www.pixiv.net/artworks/2","total_view":null,"views":7
	}`))
	require.NoError(t, err)
	wrongTypeViews, err := recordpkg.ParseRecordJSON([]byte(`{
		"id":"views-wrong","type":"illust","url":"https://www.pixiv.net/artworks/3","total_view":"unknown","views":7
	}`))
	require.NoError(t, err)
	nullPages, err := recordpkg.ParseRecordJSON([]byte(`{
		"id":"pages-null","type":"illust","url":"https://www.pixiv.net/artworks/4","page_count":null
	}`))
	require.NoError(t, err)
	wrongTypePages, err := recordpkg.ParseRecordJSON([]byte(`{
		"id":"pages-wrong","type":"illust","url":"https://www.pixiv.net/artworks/5","page_count":"one"
	}`))
	require.NoError(t, err)

	assert.False(t, nullTags.Matches(recordpkg.RecordFilter{Tags: []string{""}}))
	assert.True(t, nullViews.Matches(recordpkg.RecordFilter{MinViews: int64Ptr(7)}))
	assert.True(t, wrongTypeViews.Matches(recordpkg.RecordFilter{MinViews: int64Ptr(7)}))
	assert.False(t, nullPages.Matches(recordpkg.RecordFilter{MinPageCount: int64Ptr(0)}))
	assert.False(t, wrongTypePages.Matches(recordpkg.RecordFilter{MinPageCount: int64Ptr(0)}))
}

func TestRecordMatchesSupportsInt64MetricThresholds(t *testing.T) {
	parsed, err := recordpkg.ParseRecordJSON([]byte(`{
		"id":"max","type":"illust","url":"https://www.pixiv.net/artworks/6",
		"total_view":9223372036854775807,"page_count":9223372036854775807
	}`))
	require.NoError(t, err)
	below, err := recordpkg.ParseRecordJSON([]byte(`{
		"id":"below","type":"illust","url":"https://www.pixiv.net/artworks/7",
		"total_view":9223372036854775806,"page_count":9223372036854775806
	}`))
	require.NoError(t, err)
	max := int64(math.MaxInt64)

	filter := recordpkg.RecordFilter{MinViews: &max, MinPageCount: &max}
	assert.True(t, parsed.Matches(filter))
	assert.False(t, below.Matches(filter))
}

func int64Ptr(value int64) *int64 {
	return &value
}
