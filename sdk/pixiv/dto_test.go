package pixiv_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestArtworkDTOIsExplicitAndOpaque(t *testing.T) {
	ref, err := sdk.NewResourceRef("pixiv", []byte("artwork:42:original"))
	if err != nil {
		t.Fatalf("NewResourceRef: %v", err)
	}
	expires := time.Now().Add(time.Hour)
	artwork := pixiv.Artwork{
		ID:          42,
		Title:       "title",
		Kind:        pixiv.ArtworkKindIllustration,
		RawKind:     "illust",
		Tags:        []pixiv.Tag{{Name: "tag", TranslatedName: "translated"}},
		User:        pixiv.User{ID: 7, Name: "artist"},
		PublishedAt: time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC),
		UpdatedAt:   &expires,
		Cover: pixiv.ImageResource{Resource: sdk.Resource{
			Ref:            ref,
			URL:            "https://signed.example/secret?token=do-not-leak",
			RequestHeaders: map[string]string{"Cookie": "secret-cookie"},
			ExpiresAt:      &expires,
		}},
		Pages: []pixiv.ArtworkPage{{
			PageIndex: 0,
			Image:     pixiv.ImageResource{Resource: sdk.Resource{Ref: ref}},
		}},
	}

	dto := pixiv.ToArtworkDTO(artwork)
	if dto.ID != artwork.ID || dto.Kind != artwork.Kind || len(dto.Pages) != 1 {
		t.Fatalf("artwork DTO lost fields: %+v", dto)
	}
	if dto.Cover.Resource == nil || dto.Cover.Resource.Ref != ref.String() {
		t.Fatalf("artwork DTO lost resource ref: %+v", dto.Cover.Resource)
	}

	data, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	encoded := string(data)
	for _, forbidden := range []string{"signed.example", "do-not-leak", "Cookie", "secret-cookie", "expires_at", "request_headers"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("artwork DTO leaked %q: %s", forbidden, encoded)
		}
	}
	for _, required := range []string{`"total_bookmarks"`, `"published_at"`, `"translated_name"`} {
		if !strings.Contains(encoded, required) {
			t.Fatalf("artwork DTO omitted %q: %s", required, encoded)
		}
	}

	dto.Tags[0].Name = "changed"
	if artwork.Tags[0].Name != "tag" {
		t.Fatal("DTO shares the source tag slice")
	}
}

func TestNovelContentDTOPreservesUnknownBlockSafely(t *testing.T) {
	content := pixiv.NovelContent{
		NovelID: 9,
		Title:   "novel",
		Blocks: []pixiv.NovelBlock{{
			Kind: pixiv.NovelBlockUnknown,
			Unknown: &pixiv.NovelUnknownBlock{
				RawType: "future_block",
				Payload: map[string]string{"label": "keep"},
			},
		}},
	}

	dto := pixiv.ToNovelContentDTO(content)
	if len(dto.Blocks) != 1 || dto.Blocks[0].Unknown == nil {
		t.Fatalf("unknown block was not preserved: %+v", dto)
	}
	dto.Blocks[0].Unknown.Payload["label"] = "changed"
	if content.Blocks[0].Unknown.Payload["label"] != "keep" {
		t.Fatal("DTO shares the source unknown payload map")
	}
}

func TestArtworkDTOOmitsAbsentOptionalFields(t *testing.T) {
	artwork := pixiv.Artwork{
		ID:          1,
		Title:       "no-optional-fields",
		Kind:        pixiv.ArtworkKindIllustration,
		RawKind:     "illust",
		User:        pixiv.User{ID: 2, Name: "artist"},
		PublishedAt: time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC),
		Tags:        []pixiv.Tag{},
	}
	raw, err := json.Marshal(pixiv.ToArtworkDTO(artwork))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, key := range []string{`"updated_at"`, `"tools"`, `"pages"`} {
		if strings.Contains(text, key) {
			t.Fatalf("ArtworkDTO with absent optional fields still emits %s: %s", key, text)
		}
	}
	for _, key := range []string{`"id"`, `"title"`, `"kind"`, `"raw_kind"`, `"tags"`, `"published_at"`} {
		if !strings.Contains(text, key) {
			t.Fatalf("ArtworkDTO missing required key %s: %s", key, text)
		}
	}
}

func TestArtworkDTOEmitsOptionalFieldsWhenPresent(t *testing.T) {
	updated := time.Date(2026, time.February, 2, 3, 4, 5, 0, time.UTC)
	artwork := pixiv.Artwork{
		ID:          1,
		Title:       "with-optional-fields",
		Kind:        pixiv.ArtworkKindIllustration,
		RawKind:     "illust",
		User:        pixiv.User{ID: 2, Name: "artist"},
		PublishedAt: time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC),
		UpdatedAt:   &updated,
		Tools:       []string{"CLIP STUDIO PAINT"},
		Tags:        []pixiv.Tag{},
		Pages:       []pixiv.ArtworkPage{{PageIndex: 0, Width: 100, Height: 100}},
	}
	raw, err := json.Marshal(pixiv.ToArtworkDTO(artwork))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, key := range []string{`"updated_at"`, `"tools"`, `"pages"`} {
		if !strings.Contains(text, key) {
			t.Fatalf("ArtworkDTO with present optional fields missing %s: %s", key, text)
		}
	}
}
