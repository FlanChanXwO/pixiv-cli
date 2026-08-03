package cli

import (
	"encoding/json"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

// TestRecordMarshalStableFields 固定管道记录协议：无论 SDK 模型如何变化，
// record 顶层始终携带 id/type/url，且作品 type 使用 illustration 常量。
func TestRecordMarshalStableFields(t *testing.T) {
	artwork := pixiv.Artwork{ID: 123, Title: "work", Kind: pixiv.ArtworkKindIllustration, User: pixiv.User{Name: "artist"}}
	record, err := application.RecordFromArtwork(artwork)
	require.NoError(t, err)
	body, err := json.Marshal(record)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(body, &decoded))
	require.Equal(t, "123", decoded["id"])
	require.Equal(t, "illustration", decoded["type"])
	require.Equal(t, "https://www.pixiv.net/artworks/123", decoded["url"])

	novel := pixiv.Novel{ID: 9, Title: "novel", User: pixiv.User{Name: "writer"}}
	record, err = application.RecordFromNovel(novel)
	require.NoError(t, err)
	body, err = json.Marshal(record)
	require.NoError(t, err)
	decoded = nil
	require.NoError(t, json.Unmarshal(body, &decoded))
	require.Equal(t, "9", decoded["id"])
	require.Equal(t, "novel", decoded["type"])
	require.Equal(t, "https://www.pixiv.net/novel/show.php?id=9", decoded["url"])
}

// TestRecordMarshalArtworkJSON 确认公开 SDK 模型无 json tag 时仍可被 record 协议
// 以 snake_case 字段输出，且不缺失 id/title 等关键字段。
func TestRecordMarshalArtworkJSON(t *testing.T) {
	artwork := pixiv.Artwork{ID: 7, Title: "title", TotalViews: 10, Kind: pixiv.ArtworkKindIllustration}
	record, err := application.RecordFromArtwork(artwork)
	require.NoError(t, err)
	body, err := json.Marshal(record)
	require.NoError(t, err)
	require.Contains(t, string(body), `"id":"7"`)
	require.Contains(t, string(body), `"title":"title"`)
	require.Contains(t, string(body), `"total_views":10`)
}
