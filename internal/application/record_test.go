package application_test

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordFromArtworkPreservesSDKFieldsAndNormalizesID(t *testing.T) {
	artwork := pixiv.Artwork{
		ID:             9_007_199_254_740_993,
		Title:          "作品标题",
		Caption:        "说明",
		Kind:           pixiv.ArtworkKindUgoira,
		PageCount:      2,
		TotalBookmarks: 8,
		TotalViews:     42,
		User:           pixiv.User{ID: 99, Name: "作者"},
		Tags:           []pixiv.Tag{{Name: "tag-a", TranslatedName: "标签 A"}},
	}

	record, err := application.RecordFromArtwork(artwork)
	require.NoError(t, err)

	got, err := json.Marshal(record)
	require.NoError(t, err)
	want, err := application.MarshalRecordValue(artwork)
	require.NoError(t, err)

	var wantObject map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(want, &wantObject))
	wantObject["id"] = json.RawMessage(`"9007199254740993"`)
	wantObject["type"] = json.RawMessage(`"ugoira"`)
	wantObject["url"] = json.RawMessage(`"https://www.pixiv.net/artworks/9007199254740993"`)
	want, err = json.Marshal(wantObject)
	require.NoError(t, err)

	assert.JSONEq(t, string(want), string(got))
	assert.Equal(t, "9007199254740993", record.ID())
	assert.Equal(t, "ugoira", record.Type())
	assert.Equal(t, "https://www.pixiv.net/artworks/9007199254740993", record.URL())
	assert.NotContains(t, string(got), `"schema"`)
	assert.NotContains(t, string(got), `"version"`)
}

func TestRecordFromNovelAndUserPreview(t *testing.T) {
	novel := pixiv.Novel{
		ID:             88,
		Title:          "小说标题",
		Caption:        "小说说明",
		TextLength:     1234,
		IsOriginal:     true,
		TotalViews:     55,
		Tags:           []pixiv.Tag{{Name: "novel-tag"}},
		XRestrict:      0,
		User:           pixiv.User{ID: 77, Name: "小说作者"},
		TotalBookmarks: 3,
	}
	userPreview := pixiv.UserPreview{User: pixiv.User{
		ID:         9_007_199_254_740_993,
		Name:       "用户",
		Account:    "user-account",
		Comment:    "用户简介",
		IsFollowed: true,
	}}

	novelRecord, err := application.RecordFromNovel(novel)
	require.NoError(t, err)
	assertRecordMatchesSource(t, novel, novelRecord, "88", "novel", "https://www.pixiv.net/novel/show.php?id=88")

	userRecord, err := application.RecordFromUserPreview(userPreview)
	require.NoError(t, err)
	assertRecordMatchesSource(t, userPreview, userRecord, "9007199254740993", "user", "https://www.pixiv.net/users/9007199254740993")
}

func TestRecordFromUserPreviewPreservesCompleteEnvelope(t *testing.T) {
	preview := pixiv.UserPreview{
		User: pixiv.User{ID: 9_007_199_254_740_993, Name: "推荐作者", Account: "recommended-author"},
		Illusts: []pixiv.Artwork{{
			ID: 101, Title: "预览插画", Kind: pixiv.ArtworkKindIllustration,
		}},
		Novels: []pixiv.Novel{{
			ID: 202, Title: "预览小说",
		}},
	}

	record, err := application.RecordFromUserPreview(preview)
	require.NoError(t, err)
	assertRecordMatchesSource(t, preview, record, "9007199254740993", "user", "https://www.pixiv.net/users/9007199254740993")

	got, err := json.Marshal(record)
	require.NoError(t, err)
	var object struct {
		User struct {
			ID json.RawMessage `json:"id"`
		} `json:"user"`
		Illusts []struct {
			Title string `json:"title"`
		} `json:"illusts"`
		Novels []struct {
			Title string `json:"title"`
		} `json:"novels"`
	}
	require.NoError(t, json.Unmarshal(got, &object))
	assert.Equal(t, "9007199254740993", string(object.User.ID))
	require.Len(t, object.Illusts, 1)
	assert.Equal(t, "预览插画", object.Illusts[0].Title)
	require.Len(t, object.Novels, 1)
	assert.Equal(t, "预览小说", object.Novels[0].Title)
}

func TestRecordFromUserDetailPreservesCompleteEnvelope(t *testing.T) {
	detail := pixiv.UserDetail{
		User:             pixiv.User{ID: 321, Name: "画师", Account: "artist"},
		Profile:          pixiv.UserProfile{Webpage: "https://example.test/artist", Region: "Tokyo", TotalIllusts: 9},
		ProfilePublicity: pixiv.UserProfilePublicity{Region: true},
		Workspace:        pixiv.UserWorkspace{PC: "desktop", Tool: "pen", WorkspaceImageURL: "https://example.test/workspace.png"},
	}

	record, err := application.RecordFromUserDetail(detail)
	require.NoError(t, err)
	assertRecordMatchesSource(t, detail, record, "321", "user", "https://www.pixiv.net/users/321")
}

func TestRecordMappersRejectNonPositiveSDKID(t *testing.T) {
	for _, test := range []struct {
		name string
		make func() (application.Record, error)
	}{
		{name: "artwork", make: func() (application.Record, error) {
			return application.RecordFromArtwork(pixiv.Artwork{ID: 0, Kind: pixiv.ArtworkKindIllustration})
		}},
		{name: "novel", make: func() (application.Record, error) {
			return application.RecordFromNovel(pixiv.Novel{ID: -1})
		}},
		{name: "user", make: func() (application.Record, error) {
			return application.RecordFromUserPreview(pixiv.UserPreview{User: pixiv.User{ID: 0}})
		}},
		{name: "user detail", make: func() (application.Record, error) {
			return application.RecordFromUserDetail(pixiv.UserDetail{User: pixiv.User{ID: 0}})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.make()
			require.ErrorContains(t, err, "record id must be positive")
		})
	}
}

func TestParseRecordJSONNormalizesNumericIDAndPreservesUnknownFields(t *testing.T) {
	record, err := application.ParseRecordJSON([]byte(`{
		"id":9007199254740993,
		"type":"illust",
		"url":"https://www.pixiv.net/artworks/9007199254740993",
		"custom":{"enabled":true,"score":1.25},
		"tags":["tag-a"]
	}`))
	require.NoError(t, err)

	got, err := json.Marshal(record)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"id":"9007199254740993",
		"type":"illust",
		"url":"https://www.pixiv.net/artworks/9007199254740993",
		"custom":{"enabled":true,"score":1.25},
		"tags":["tag-a"]
	}`, string(got))
	assert.Equal(t, "9007199254740993", record.ID())
}

func TestRecordUnmarshalJSONUsesPipelineValidationAndNormalization(t *testing.T) {
	var record application.Record
	err := json.Unmarshal([]byte(`{
		"id":9007199254740993,
		"type":"illust",
		"url":"https://www.pixiv.net/artworks/9007199254740993",
		"version":"must-not-survive",
		"unknown":{"value":true}
	}`), &record)
	require.NoError(t, err)
	assert.Equal(t, "9007199254740993", record.ID())
	encoded, err := json.Marshal(record)
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"9007199254740993","type":"illust","url":"https://www.pixiv.net/artworks/9007199254740993","unknown":{"value":true}}`, string(encoded))
}

func TestParseRecordJSONRemovesVersionMetadataRecursively(t *testing.T) {
	record, err := application.ParseRecordJSON([]byte(`{
		"id":"1",
		"type":"illust",
		"url":"https://www.pixiv.net/artworks/1",
		"version":"top-version",
		"schema":{"name":"old"},
		"api_version":"api-version",
		"apiVersion":"api-version-camel",
		"schema_version":"schema-version",
		"schemaVersion":"schema-version-camel",
		"protocol_version":"protocol-version",
		"format_version":"format-version",
		"record_version":"record-version",
		"sdk_version":"sdk-version",
		"mcp_version":"mcp-version",
		"cli_version":"cli-version",
		"version_info":{"name":"old"},
		"conversion":"must-stay",
		"custom":{"keep":true,"version":"nested-version","schemaVersion":"nested-schema","conversion":"nested-conversion"},
		"items":[
			{"keep":"first","apiVersion":"nested-api","protocol_version":"nested-protocol"},
			{"conversion":"second-conversion","nested":[{"format_version":"nested-format","keep":"deep"}]}
		]
	}`))
	require.NoError(t, err)

	got, err := json.Marshal(record)
	require.NoError(t, err)
	var object any
	require.NoError(t, json.Unmarshal(got, &object))
	assertNoVersionMetadata(t, object)
	assert.JSONEq(t, `{
		"id":"1",
		"type":"illust",
		"url":"https://www.pixiv.net/artworks/1",
		"conversion":"must-stay",
		"custom":{"keep":true,"conversion":"nested-conversion"},
		"items":[
			{"keep":"first"},
			{"conversion":"second-conversion","nested":[{"keep":"deep"}]}
		]
	}`, string(got))
}

func TestParseRecordJSONPreservesUnknownNumbersWithoutFloatConversion(t *testing.T) {
	record, err := application.ParseRecordJSON([]byte(`{
		"id":"1",
		"type":"illust",
		"url":"https://www.pixiv.net/artworks/1",
		"custom_integer":9007199254740993123456789,
		"custom_decimal":0.12345678901234567890123456789,
		"nested":{"integer":9223372036854775807,"decimal":1.0000000000000000001}
	}`))
	require.NoError(t, err)

	got, err := json.Marshal(record)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"id":"1",
		"type":"illust",
		"url":"https://www.pixiv.net/artworks/1",
		"custom_integer":9007199254740993123456789,
		"custom_decimal":0.12345678901234567890123456789,
		"nested":{"integer":9223372036854775807,"decimal":1.0000000000000000001}
	}`, string(got))
	assert.Contains(t, string(got), `"custom_integer":9007199254740993123456789`)
	assert.Contains(t, string(got), `"custom_decimal":0.12345678901234567890123456789`)
}

func TestParseRecordJSONRejectsInvalidRequiredFields(t *testing.T) {
	for _, test := range []struct {
		name     string
		line     string
		contains string
	}{
		{name: "invalid JSON", line: `{`, contains: "invalid record JSON object"},
		{name: "array", line: `[]`, contains: "invalid record JSON object"},
		{name: "missing id", line: `{"type":"illust","url":"https://www.pixiv.net/artworks/1"}`, contains: "record id is required"},
		{name: "empty id", line: `{"id":"","type":"illust","url":"https://www.pixiv.net/artworks/1"}`, contains: "record id must be a non-empty string"},
		{name: "null id", line: `{"id":null,"type":"illust","url":"https://www.pixiv.net/artworks/1"}`, contains: "record id must be a non-empty string or positive integer"},
		{name: "float id", line: `{"id":1.5,"type":"illust","url":"https://www.pixiv.net/artworks/1"}`, contains: "record id must be a non-empty string or positive integer"},
		{name: "negative id", line: `{"id":-1,"type":"illust","url":"https://www.pixiv.net/artworks/1"}`, contains: "record id must be a non-empty string or positive integer"},
		{name: "zero id", line: `{"id":0,"type":"illust","url":"https://www.pixiv.net/artworks/1"}`, contains: "record id must be a non-empty string or positive integer"},
		{name: "missing type", line: `{"id":"1","url":"https://www.pixiv.net/artworks/1"}`, contains: "record type is required"},
		{name: "wrong type type", line: `{"id":"1","type":1,"url":"https://www.pixiv.net/artworks/1"}`, contains: "record type must be a string"},
		{name: "null type", line: `{"id":"1","type":null,"url":"https://www.pixiv.net/artworks/1"}`, contains: "record type must be a string"},
		{name: "empty url", line: `{"id":"1","type":"illust","url":""}`, contains: "record url must be a non-empty string"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := application.ParseRecordJSON([]byte(test.line))
			require.Error(t, err)
			assert.ErrorContains(t, err, test.contains)
		})
	}
}

func TestRecordMatchesFiltersForObjectAndStringTags(t *testing.T) {
	objectTags, err := application.ParseRecordJSON([]byte(`{
		"id":"7",
		"type":"illust",
		"url":"https://www.pixiv.net/artworks/7",
		"tags":[{"name":"tag-a"},{"name":"tag-b","translated_name":"标签 B"}],
		"total_view":5000,
		"page_count":2
	}`))
	require.NoError(t, err)
	stringTags, err := application.ParseRecordJSON([]byte(`{
		"id":"8",
		"type":"novel",
		"url":"https://www.pixiv.net/novel/show.php?id=8",
		"tags":["tag-a","tag-b"],
		"views":100,
		"page_count":1
	}`))
	require.NoError(t, err)

	filter := application.RecordFilter{
		ID:           "7",
		Type:         "illust",
		Tags:         []string{"tag-a", "tag-b"},
		MinViews:     int64Ptr(5000),
		MinPageCount: int64Ptr(2),
	}
	assert.True(t, objectTags.Matches(filter))
	assert.False(t, objectTags.Matches(application.RecordFilter{Tags: []string{"tag-a", "missing"}}))
	assert.True(t, stringTags.Matches(application.RecordFilter{Tags: []string{"tag-a", "tag-b"}, MinViews: int64Ptr(100), MinPageCount: int64Ptr(1)}))
	assert.False(t, stringTags.Matches(application.RecordFilter{MinViews: int64Ptr(101)}))
	assert.False(t, stringTags.Matches(application.RecordFilter{MinPageCount: int64Ptr(2)}))
}

func TestRecordMatchesLeavesUnfilteredRecordsUnchangedAndRejectsMissingMetrics(t *testing.T) {
	record, err := application.ParseRecordJSON([]byte(`{
		"id":"user-1",
		"type":"user",
		"url":"https://www.pixiv.net/users/1",
		"custom":true
	}`))
	require.NoError(t, err)

	assert.True(t, record.Matches(application.RecordFilter{}))
	assert.True(t, record.Matches(application.RecordFilter{ID: "user-1", Type: "user"}))
	assert.False(t, record.Matches(application.RecordFilter{ID: "other"}))
	assert.False(t, record.Matches(application.RecordFilter{Type: "illust"}))
	assert.False(t, record.Matches(application.RecordFilter{Tags: []string{"tag-a"}}))
	assert.False(t, record.Matches(application.RecordFilter{MinViews: int64Ptr(0)}))
	assert.False(t, record.Matches(application.RecordFilter{MinPageCount: int64Ptr(0)}))
}

func TestRecordMatchesTreatsNullAndWrongTypesAsMissingFields(t *testing.T) {
	nullTags, err := application.ParseRecordJSON([]byte(`{
		"id":"tags-null","type":"illust","url":"https://www.pixiv.net/artworks/1","tags":[null]
	}`))
	require.NoError(t, err)
	nullViews, err := application.ParseRecordJSON([]byte(`{
		"id":"views-null","type":"illust","url":"https://www.pixiv.net/artworks/2","total_view":null,"views":7
	}`))
	require.NoError(t, err)
	wrongTypeViews, err := application.ParseRecordJSON([]byte(`{
		"id":"views-wrong","type":"illust","url":"https://www.pixiv.net/artworks/3","total_view":"unknown","views":7
	}`))
	require.NoError(t, err)
	nullPages, err := application.ParseRecordJSON([]byte(`{
		"id":"pages-null","type":"illust","url":"https://www.pixiv.net/artworks/4","page_count":null
	}`))
	require.NoError(t, err)
	wrongTypePages, err := application.ParseRecordJSON([]byte(`{
		"id":"pages-wrong","type":"illust","url":"https://www.pixiv.net/artworks/5","page_count":"one"
	}`))
	require.NoError(t, err)

	assert.False(t, nullTags.Matches(application.RecordFilter{Tags: []string{""}}))
	assert.True(t, nullViews.Matches(application.RecordFilter{MinViews: int64Ptr(7)}))
	assert.True(t, wrongTypeViews.Matches(application.RecordFilter{MinViews: int64Ptr(7)}))
	assert.False(t, nullPages.Matches(application.RecordFilter{MinPageCount: int64Ptr(0)}))
	assert.False(t, wrongTypePages.Matches(application.RecordFilter{MinPageCount: int64Ptr(0)}))
}

func TestRecordMatchesSupportsInt64MetricThresholds(t *testing.T) {
	record, err := application.ParseRecordJSON([]byte(`{
		"id":"max","type":"illust","url":"https://www.pixiv.net/artworks/6",
		"total_view":9223372036854775807,"page_count":9223372036854775807
	}`))
	require.NoError(t, err)
	below, err := application.ParseRecordJSON([]byte(`{
		"id":"below","type":"illust","url":"https://www.pixiv.net/artworks/7",
		"total_view":9223372036854775806,"page_count":9223372036854775806
	}`))
	require.NoError(t, err)
	max := int64(math.MaxInt64)

	filter := application.RecordFilter{MinViews: &max, MinPageCount: &max}
	assert.True(t, record.Matches(filter))
	assert.False(t, below.Matches(filter))
}

func assertRecordMatchesSource(t *testing.T, source any, record application.Record, id, recordType, url string) {
	t.Helper()
	got, err := json.Marshal(record)
	require.NoError(t, err)
	want, err := application.MarshalRecordValue(source)
	require.NoError(t, err)

	var wantObject map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(want, &wantObject))
	wantObject["id"], _ = json.Marshal(id)
	wantObject["type"], _ = json.Marshal(recordType)
	wantObject["url"], _ = json.Marshal(url)
	want, err = json.Marshal(wantObject)
	require.NoError(t, err)

	assert.JSONEq(t, string(want), string(got))
	assert.Equal(t, id, record.ID())
	assert.Equal(t, recordType, record.Type())
	assert.Equal(t, url, record.URL())
}

func assertNoVersionMetadata(t *testing.T, value any) {
	t.Helper()
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			assert.NotContains(t, testVersionMetadataKeys, normalizeTestMetadataKey(key), "version metadata key %q must be absent", key)
			assertNoVersionMetadata(t, child)
		}
	case []any:
		for _, child := range current {
			assertNoVersionMetadata(t, child)
		}
	}
}

var testVersionMetadataKeys = map[string]struct{}{
	"version": {}, "schema": {}, "apiversion": {}, "schemaversion": {}, "protocolversion": {},
	"formatversion": {}, "recordversion": {}, "sdkversion": {}, "mcpversion": {}, "cliversion": {}, "versioninfo": {},
}

func normalizeTestMetadataKey(key string) string {
	key = strings.ToLower(key)
	key = strings.ReplaceAll(key, "_", "")
	return strings.ReplaceAll(key, "-", "")
}

func ptr[T any](value T) *T {
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}
