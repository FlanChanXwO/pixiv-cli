package pixiv

import (
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/model"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

func TestMapArtwork(t *testing.T) {
	client, _ := New("token")
	m := model.Illust{
		ID:         9001,
		Title:      "test art",
		Type:       "manga",
		PageCount:  2,
		CreateDate: "2024-05-01T10:00:00+09:00",
		Width:      1000,
		Height:     800,
		XRestrict:  1,
		ImageURLs:  model.ImageURLs{Original: "https://i.pximg.net/img/original/1.png"},
		MetaPages: []model.MetaPage{
			{PageIndex: 0, Width: 1000, Height: 800, ImageURLs: model.ImageURLs{Original: "https://i.pximg.net/img/p0.png"}},
			{PageIndex: 1, Width: 1000, Height: 800, ImageURLs: model.ImageURLs{Original: "https://i.pximg.net/img/p1.png"}},
		},
	}
	artwork, err := client.mapArtworkDetail(m)
	if err != nil {
		t.Fatalf("mapArtworkDetail: %v", err)
	}
	if artwork.Kind != ArtworkKindManga || artwork.ID != 9001 {
		t.Fatalf("artwork = %+v", artwork)
	}
	expected, _ := time.Parse(time.RFC3339, "2024-05-01T10:00:00+09:00")
	if !artwork.PublishedAt.Equal(expected.UTC()) {
		t.Fatalf("published = %v, want %v", artwork.PublishedAt, expected.UTC())
	}
	if artwork.Cover.Resource.URL == "" {
		t.Fatal("cover resource missing")
	}
	if len(artwork.Pages) != 2 {
		t.Fatalf("pages = %d, want 2", len(artwork.Pages))
	}
	if artwork.Pages[1].PageIndex != 1 || artwork.Pages[1].Image.Resource.URL == "" {
		t.Fatalf("page 1 = %+v", artwork.Pages[1])
	}
}

func TestMapArtworkMissingPublishTimeFails(t *testing.T) {
	client, _ := New("token")
	m := model.Illust{ID: 1, Type: "illust", ImageURLs: model.ImageURLs{Original: "https://i.pximg.net/img/1.png"}}
	if _, err := client.mapArtwork(m); sdk.CodeOf(err) != sdk.CodeMalformedUpstreamResponse {
		t.Fatalf("expected CodeMalformedUpstreamResponse, got %v", err)
	}
}

func TestMapUgoiraMetadata(t *testing.T) {
	client, _ := New("token")
	result := &model.UgoiraMetadataResult{UgoiraMetadata: model.UgoiraMetadata{
		ZipURLs: model.UgoiraZipURLs{Medium: "https://i.pximg.net/zip/m.zip", Original: "https://i.pximg.net/zip/o.zip"},
		Frames: []model.UgoiraFrame{
			{File: "0.jpg", Delay: 100},
			{File: "1.jpg", Delay: 200},
		},
	}}
	meta, err := client.mapUgoiraMetadata(777, result)
	if err != nil {
		t.Fatalf("mapUgoiraMetadata: %v", err)
	}
	if len(meta.Archives) != 2 {
		t.Fatalf("archives = %d, want 2", len(meta.Archives))
	}
	if meta.Archives[0].Quality != UgoiraQualityMedium || meta.Archives[1].Quality != UgoiraQualityOriginal {
		t.Fatalf("archive qualities = %+v", meta.Archives)
	}
	if len(meta.Frames) != 2 || meta.Frames[1].DelayMilliseconds != 200 {
		t.Fatalf("frames = %+v", meta.Frames)
	}
}

func TestMapUgoiraMetadataUnsafeFilename(t *testing.T) {
	client, _ := New("token")
	result := &model.UgoiraMetadataResult{UgoiraMetadata: model.UgoiraMetadata{
		ZipURLs: model.UgoiraZipURLs{Original: "https://i.pximg.net/zip/o.zip"},
		Frames: []model.UgoiraFrame{
			{File: "../evil.jpg", Delay: 100},
		},
	}}
	if _, err := client.mapUgoiraMetadata(1, result); sdk.CodeOf(err) != sdk.CodeMalformedUpstreamResponse {
		t.Fatalf("expected CodeMalformedUpstreamResponse for traversal filename, got %v", err)
	}
}

func TestMapUgoiraRejectsMissingArchive(t *testing.T) {
	client, _ := New("token")
	result := &model.UgoiraMetadataResult{UgoiraMetadata: model.UgoiraMetadata{
		Frames: []model.UgoiraFrame{{File: "0.jpg", Delay: 100}},
	}}
	if _, err := client.mapUgoiraMetadata(1, result); sdk.CodeOf(err) != sdk.CodeMalformedUpstreamResponse {
		t.Fatalf("expected CodeMalformedUpstreamResponse for missing archive, got %v", err)
	}
}

func TestSafeArchiveFilename(t *testing.T) {
	valid := []string{"0.jpg", "frames/01.png", "a-b_c.jpg"}
	for _, name := range valid {
		if !safeArchiveFilename(name) {
			t.Fatalf("%q should be safe", name)
		}
	}
	invalid := []string{"", "../x", "a/../b", "/abs", `win\path`, ".", ".."}
	for _, name := range invalid {
		if safeArchiveFilename(name) {
			t.Fatalf("%q should be unsafe", name)
		}
	}
}

func TestArtworkPagesWithoutMetaPagesUsesSingle(t *testing.T) {
	client, _ := New("token")
	m := model.Illust{
		ID:             5,
		Type:           "illust",
		CreateDate:     "2024-01-01T00:00:00Z",
		MetaSinglePage: model.SinglePage{OriginalImageURL: "https://i.pximg.net/img/5.png"},
	}
	pages, err := client.mapArtworkPages(m)
	if err != nil {
		t.Fatalf("mapArtworkPages: %v", err)
	}
	if len(pages) != 1 || pages[0].PageIndex != 0 {
		t.Fatalf("pages = %+v", pages)
	}
	if !strings.Contains(pages[0].Image.Resource.URL, "5.png") {
		t.Fatalf("page url = %q", pages[0].Image.Resource.URL)
	}
}
