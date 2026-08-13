package pixiv_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	pipeline "github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestTask12MCPRecordSerializationOmitsResourceTransport(t *testing.T) {
	ref, err := sdk.NewResourceRef("pixiv", []byte(`{"kind":"artwork-cover","id":1}`))
	if err != nil {
		t.Fatal(err)
	}
	expiresAt := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	artwork := pixiv.Artwork{
		ID:    1,
		Kind:  pixiv.ArtworkKindIllustration,
		Title: "safe artwork",
		Cover: pixiv.ImageResource{Resource: sdk.Resource{
			Ref:            ref,
			URL:            "https://signed.example/private?signature=secret",
			RequestHeaders: map[string]string{"Cookie": "session-secret", "Referer": "https://www.pixiv.net/"},
			ExpiresAt:      &expiresAt,
		}},
		User: pixiv.User{ID: 9, ProfileImage: pixiv.ImageResource{Resource: sdk.Resource{
			Ref: ref,
			URL: "https://signed.example/profile?signature=secret",
		}}},
	}

	artworkRecords, err := records.FromArtworks([]pixiv.Artwork{artwork})
	if err != nil {
		t.Fatal(err)
	}
	assertMCPResourceTransportAbsent(t, artworkRecords, ref.String())

	novels, err := records.FromNovels([]pixiv.Novel{{ID: 2, Cover: pixiv.ImageResource{Resource: artwork.Cover.Resource}}})
	if err != nil {
		t.Fatal(err)
	}
	assertMCPResourceTransportAbsent(t, novels, ref.String())

	previews, err := records.FromUserPreviews([]pixiv.UserPreview{{User: artwork.User, Illusts: []pixiv.Artwork{artwork}, Novels: []pixiv.Novel{{ID: 2, Cover: artwork.Cover}}}})
	if err != nil {
		t.Fatal(err)
	}
	assertMCPResourceTransportAbsent(t, previews, ref.String())

	detailRecord, err := records.FromUserDetail(pixiv.UserDetail{User: artwork.User})
	if err != nil {
		t.Fatal(err)
	}
	assertMCPResourceTransportAbsent(t, []pipeline.Record{detailRecord}, ref.String())

	trending := pixiv.ToTrendingTagDTO(pixiv.TrendingTag{Tag: "safe-tag", TranslatedName: "safe", Artwork: artwork})
	rawTrending, err := json.Marshal(trending)
	if err != nil {
		t.Fatal(err)
	}
	assertSensitiveMCPValuesAbsent(t, rawTrending)
	if !strings.Contains(string(rawTrending), ref.String()) {
		t.Fatalf("trending output lost opaque resource reference: %s", rawTrending)
	}
}

func assertMCPResourceTransportAbsent(t *testing.T, value any, refString string) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	assertSensitiveMCPValuesAbsent(t, raw)
	if !strings.Contains(string(raw), refString) {
		t.Fatalf("MCP output lost opaque resource reference: %s", raw)
	}
}

func assertSensitiveMCPValuesAbsent(t *testing.T, raw []byte) {
	t.Helper()
	text := strings.ToLower(string(raw))
	for _, forbidden := range []string{"signed.example", "signature=secret", "session-secret", "request_headers", "expires_at"} {
		if strings.Contains(text, strings.ToLower(forbidden)) {
			t.Fatalf("MCP output contains sensitive resource transport value %q: %s", forbidden, raw)
		}
	}
}
