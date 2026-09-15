package record_test

import (
	"encoding/json"
	"testing"

	recordpkg "github.com/FlanChanXwO/pixiv-cli/internal/shared/record"
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

	record, err := recordpkg.RecordFromArtworkDTO(pixiv.ToArtworkDTO(artwork))
	require.NoError(t, err)

	got, err := json.Marshal(record)
	require.NoError(t, err)
	want, err := json.Marshal(pixiv.ToArtworkDTO(artwork))
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

func TestRecordFromArtworkNormalizesIllustrationType(t *testing.T) {
	artwork := pixiv.Artwork{ID: 101, Kind: pixiv.ArtworkKindIllustration}

	record, err := recordpkg.RecordFromArtworkDTO(pixiv.ToArtworkDTO(artwork))
	require.NoError(t, err)

	assert.Equal(t, "illust", record.Type())
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

	novelRecord, err := recordpkg.RecordFromNovelDTO(pixiv.ToNovelDTO(novel))
	require.NoError(t, err)
	assertRecordMatchesSource(t, novel, novelRecord, "88", "novel", "https://www.pixiv.net/novel/show.php?id=88")

	userRecord, err := recordpkg.RecordFromUserPreviewDTO(pixiv.ToUserPreviewDTO(userPreview))
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

	record, err := recordpkg.RecordFromUserPreviewDTO(pixiv.ToUserPreviewDTO(preview))
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

	record, err := recordpkg.RecordFromUserDetailDTO(pixiv.ToUserDetailDTO(detail))
	require.NoError(t, err)
	assertRecordMatchesSource(t, detail, record, "321", "user", "https://www.pixiv.net/users/321")
}

func TestRecordFromNovelContentPreservesCanonicalIdentityAndStructuredFields(t *testing.T) {
	content := pixiv.NovelContent{
		NovelID: 808,
		Title:   "正文标题",
		Caption: "正文说明",
		Blocks: []pixiv.NovelBlock{
			{
				Kind:  pixiv.NovelBlockParagraph,
				Text:  "第一段",
				Marks: []pixiv.NovelMark{{Kind: pixiv.NovelMarkStrong, Text: "重点"}},
			},
			{Kind: pixiv.NovelBlockHeader, Text: "章节一"},
			{
				Kind: pixiv.NovelBlockUnknown,
				Unknown: &pixiv.NovelUnknownBlock{
					RawType: "quote",
					Payload: map[string]string{"label": "引用"},
				},
			},
		},
	}

	gotRecord, err := recordpkg.RecordFromNovelContentDTO(pixiv.ToNovelContentDTO(content))
	require.NoError(t, err)
	body, err := json.Marshal(gotRecord)
	require.NoError(t, err)

	var object map[string]any
	require.NoError(t, json.Unmarshal(body, &object))
	assert.Equal(t, "808", object["id"])
	assert.Equal(t, "novel", object["type"])
	assert.Equal(t, "https://www.pixiv.net/novel/show.php?id=808", object["url"])
	assert.Equal(t, float64(808), object["novel_id"])
	assert.Equal(t, "正文标题", object["title"])
	assert.Equal(t, "正文说明", object["caption"])

	blocks, ok := object["blocks"].([]any)
	require.True(t, ok, "blocks must remain a structured array")
	require.Len(t, blocks, 3)
	firstBlock, ok := blocks[0].(map[string]any)
	require.True(t, ok, "first block must remain a structured object")
	assert.Equal(t, "paragraph", firstBlock["kind"])
	assert.Equal(t, "第一段", firstBlock["text"])
	marks, ok := firstBlock["marks"].([]any)
	require.True(t, ok, "inline marks must remain structured")
	require.Len(t, marks, 1)
	mark, ok := marks[0].(map[string]any)
	require.True(t, ok, "inline mark must remain a structured object")
	assert.Equal(t, "strong", mark["kind"])
	assert.Equal(t, "重点", mark["text"])

	thirdBlock, ok := blocks[2].(map[string]any)
	require.True(t, ok, "unknown block must remain a structured object")
	unknown, ok := thirdBlock["unknown"].(map[string]any)
	require.True(t, ok, "unknown block payload must remain available")
	assert.Equal(t, "quote", unknown["raw_type"])
	payload, ok := unknown["payload"].(map[string]any)
	require.True(t, ok, "unknown block payload must remain structured")
	assert.Equal(t, "引用", payload["label"])
}

func TestRecordMappersRejectNonPositiveSDKID(t *testing.T) {
	for _, test := range []struct {
		name string
		make func() (recordpkg.Record, error)
	}{
		{name: "artwork", make: func() (recordpkg.Record, error) {
			return recordpkg.RecordFromArtworkDTO(pixiv.ToArtworkDTO(pixiv.Artwork{ID: 0, Kind: pixiv.ArtworkKindIllustration}))
		}},
		{name: "novel", make: func() (recordpkg.Record, error) {
			return recordpkg.RecordFromNovelDTO(pixiv.ToNovelDTO(pixiv.Novel{ID: -1}))
		}},
		{name: "novel content", make: func() (recordpkg.Record, error) {
			return recordpkg.RecordFromNovelContentDTO(pixiv.ToNovelContentDTO(pixiv.NovelContent{NovelID: -1}))
		}},
		{name: "user", make: func() (recordpkg.Record, error) {
			return recordpkg.RecordFromUserPreviewDTO(pixiv.ToUserPreviewDTO(pixiv.UserPreview{User: pixiv.User{ID: 0}}))
		}},
		{name: "user detail", make: func() (recordpkg.Record, error) {
			return recordpkg.RecordFromUserDetailDTO(pixiv.ToUserDetailDTO(pixiv.UserDetail{User: pixiv.User{ID: 0}}))
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.make()
			require.ErrorContains(t, err, "record id must be positive")
		})
	}
}

func assertRecordMatchesSource(t *testing.T, source any, record recordpkg.Record, id, recordType, url string) {
	t.Helper()
	got, err := json.Marshal(record)
	require.NoError(t, err)
	want, err := json.Marshal(recordSourceDTO(source))
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

func recordSourceDTO(source any) any {
	switch value := source.(type) {
	case pixiv.Artwork:
		return pixiv.ToArtworkDTO(value)
	case pixiv.Novel:
		return pixiv.ToNovelDTO(value)
	case pixiv.UserPreview:
		return pixiv.ToUserPreviewDTO(value)
	case pixiv.UserDetail:
		return pixiv.ToUserDetailDTO(value)
	default:
		return source
	}
}
