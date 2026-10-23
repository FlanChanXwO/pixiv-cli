package pixiv_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func secretResource(t *testing.T, payload string) sdk.Resource {
	t.Helper()
	ref, err := sdk.NewResourceRef("pixiv", []byte(payload))
	if err != nil {
		t.Fatalf("NewResourceRef: %v", err)
	}
	expires := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	return sdk.Resource{
		Ref:                 ref,
		URL:                 "https://signed.example/secret?token=do-not-leak",
		RequestHeaders:      map[string]string{"Cookie": "secret-cookie"},
		ExpiresAt:           &expires,
		RequiresCredentials: true,
	}
}

func assertOpaque(t *testing.T, encoded string) {
	t.Helper()
	for _, forbidden := range []string{"signed.example", "do-not-leak", "secret-cookie", "Cookie", "expires_at", "request_headers", `"url"`} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("output leaked runtime resource state %q: %s", forbidden, encoded)
		}
	}
}

func marshal(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(raw)
}

func TestArtworkJSONIsExplicitAndOpaque(t *testing.T) {
	artwork := pixiv.Artwork{
		ID:          42,
		Title:       "title",
		Kind:        pixiv.ArtworkKindIllustration,
		RawKind:     "illust",
		Tags:        []pixiv.Tag{{Name: "tag", TranslatedName: "translated"}},
		User:        pixiv.User{ID: 7, Name: "artist", ProfileImage: pixiv.ImageResource{Resource: secretResource(t, "user:7")}},
		PublishedAt: time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC),
		Cover:       pixiv.ImageResource{Resource: secretResource(t, "artwork:42:original"), Variant: "original"},
		Pages: []pixiv.ArtworkPage{{
			PageIndex: 0,
			Image:     pixiv.ImageResource{Resource: secretResource(t, "artwork:42:page:0")},
		}},
	}
	encoded := marshal(t, artwork)
	assertOpaque(t, encoded)
	for _, required := range []string{`"total_bookmarks"`, `"published_at"`, `"translated_name"`, `"requires_credentials":true`} {
		if !strings.Contains(encoded, required) {
			t.Fatalf("artwork JSON omitted %q: %s", required, encoded)
		}
	}
}

func TestNovelJSONKeepsContentBlocksOpaque(t *testing.T) {
	content := pixiv.NovelContent{
		NovelID: 9,
		Title:   "novel",
		Blocks: []pixiv.NovelBlock{
			{Kind: pixiv.NovelBlockImage, Image: &pixiv.NovelImageBlock{Resource: secretResource(t, "novel:9:image:0")}},
			{Kind: pixiv.NovelBlockFile, File: &pixiv.NovelFileBlock{Resource: secretResource(t, "novel:9:file:0")}},
			{Kind: pixiv.NovelBlockUnknown, Unknown: &pixiv.NovelUnknownBlock{RawType: "future_block", Payload: map[string]string{"label": "keep"}}},
		},
	}
	encoded := marshal(t, content)
	assertOpaque(t, encoded)
	if !strings.Contains(encoded, `"label":"keep"`) {
		t.Fatalf("unknown block payload was dropped: %s", encoded)
	}
}

func TestUgoiraMetadataJSONIsOpaque(t *testing.T) {
	metadata := pixiv.UgoiraMetadata{
		ArtworkID: 7,
		Archives:  []pixiv.UgoiraArchive{{Quality: pixiv.UgoiraQualityOriginal, Resource: secretResource(t, "ugoira:7")}},
		Frames:    []pixiv.UgoiraFrame{{Filename: "000001.jpg", DelayMilliseconds: 40}},
	}
	assertOpaque(t, marshal(t, metadata))
}

func TestArtworkJSONOmitsAbsentOptionalFields(t *testing.T) {
	artwork := pixiv.Artwork{
		ID:          1,
		Title:       "no-optional-fields",
		Kind:        pixiv.ArtworkKindIllustration,
		RawKind:     "illust",
		User:        pixiv.User{ID: 2, Name: "artist"},
		PublishedAt: time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC),
		Tags:        []pixiv.Tag{},
	}
	text := marshal(t, artwork)
	for _, key := range []string{`"updated_at"`, `"tools"`, `"pages"`, `"series"`, `"total_comments"`, `"is_bookmarked"`, `"is_muted"`, `"visible"`, `"sanity_level"`, `"restriction_attributes"`} {
		if strings.Contains(text, key) {
			t.Fatalf("artwork with absent optional fields still emits %s: %s", key, text)
		}
	}
	for _, key := range []string{`"id"`, `"title"`, `"kind"`, `"raw_kind"`, `"tags"`, `"published_at"`} {
		if !strings.Contains(text, key) {
			t.Fatalf("artwork JSON missing required key %s: %s", key, text)
		}
	}
}

func TestArtworkJSONPreservesViewerStateAndSeries(t *testing.T) {
	comments := 0
	artwork := pixiv.Artwork{
		ID: 42, Kind: pixiv.ArtworkKindIllustration,
		IsBookmarked: ptrTo(true), IsMuted: ptrTo(false), Visible: ptrTo(true), SanityLevel: ptrTo(2),
		RestrictionAttributes: ptrTo([]string{"restricted_mode"}),
		Series:                &pixiv.ArtworkSeriesSummary{ID: 123, Title: "chapter"},
		TotalComments:         &comments,
	}
	raw := marshal(t, artwork)
	for _, want := range []string{`"is_bookmarked":true`, `"is_muted":false`, `"visible":true`, `"sanity_level":2`, `"restriction_attributes":["restricted_mode"]`, `"series":{"id":123,"title":"chapter"}`, `"total_comments":0`} {
		if !strings.Contains(raw, want) {
			t.Fatalf("JSON %s missing %s", raw, want)
		}
	}
}

func TestPageJSONEmitsCursorTextAndEmptyLastPage(t *testing.T) {
	cursor, err := buildCursorForTest()
	if err != nil {
		t.Fatalf("cursor: %v", err)
	}
	encoded := marshal(t, sdk.Page[pixiv.Tag]{Items: []pixiv.Tag{{Name: "tag"}}, Next: cursor})
	if !strings.Contains(encoded, `"next":"`+cursor.String()+`"`) {
		t.Fatalf("page JSON lost cursor text: %s", encoded)
	}
	last := marshal(t, sdk.Page[pixiv.Tag]{Items: []pixiv.Tag{}})
	if last != `{"items":[],"next":""}` {
		t.Fatalf("last page JSON = %s", last)
	}
}

func buildCursorForTest() (sdk.Cursor, error) {
	return sdk.NewCursor("pixiv", "Test", 1, "hash", []byte("state"))
}

func ptrTo[T any](value T) *T {
	return &value
}
