package downloader

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	downloadapp "github.com/FlanChanXwO/pixiv-cli/internal/application/download"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestDownloadPagesSelectionIsOneBasedAndErrorsOnMissing(t *testing.T) {
	dir := t.TempDir()
	client := &fakePixivClient{
		details: map[int64]pixiv.Artwork{
			7: {
				ID: 7, Title: "multi", PageCount: 3, Kind: pixiv.ArtworkKindIllustration, User: pixiv.User{Name: "author"},
				Pages: []pixiv.ArtworkPage{
					artworkPage("https://i.example/7_p0.png", 0),
					artworkPage("https://i.example/7_p1.png", 1),
					artworkPage("https://i.example/7_p2.png", 2),
				},
			},
		},
		downloads: map[string][]byte{
			"https://i.example/7_p0.png": []byte("p0"),
			"https://i.example/7_p1.png": []byte("p1"),
			"https://i.example/7_p2.png": []byte("p2"),
		},
	}
	m := NewManager(client, dir, "{id}")
	got, err := m.Download(context.Background(), downloadapp.DownloadRequest{
		IllustIDs: []int64{7},
		Pages:     []int{1, 3},
		Quality:   downloadapp.DownloadQualityOriginal,
	})
	if err != nil {
		t.Fatalf("Download error: %v", err)
	}
	if len(got) != 1 || len(got[0].Files) != 2 {
		t.Fatalf("files=%+v", got)
	}
	if got[0].Files[0].Page != 1 || got[0].Files[1].Page != 3 {
		t.Fatalf("pages=%+v", got[0].Files)
	}
	if filepath.Base(got[0].Files[0].Path) != "7_p0.png" || filepath.Base(got[0].Files[1].Path) != "7_p2.png" {
		t.Fatalf("paths=%q %q", got[0].Files[0].Path, got[0].Files[1].Path)
	}

	_, err = m.Download(context.Background(), downloadapp.DownloadRequest{
		IllustIDs: []int64{7},
		Pages:     []int{4},
		Quality:   downloadapp.DownloadQualityOriginal,
	})
	if err == nil || !strings.Contains(err.Error(), "page 4 does not exist") {
		t.Fatalf("missing page error=%v", err)
	}
}

func TestDownloadQualityValidatesButDownloadsSinglePageURL(t *testing.T) {
	dir := t.TempDir()
	rawURL := "https://i.example/original.png"
	client := &fakePixivClient{
		details: map[int64]pixiv.Artwork{
			1: {
				ID: 1, Title: "q", PageCount: 1, Kind: pixiv.ArtworkKindIllustration, User: pixiv.User{Name: "a"},
				Pages: []pixiv.ArtworkPage{artworkPage(rawURL, 0)},
			},
		},
		downloads: map[string][]byte{rawURL: []byte("original")},
	}
	m := NewManager(client, dir, "{id}")
	for _, quality := range []downloadapp.DownloadQuality{
		downloadapp.DownloadQualityOriginal,
		downloadapp.DownloadQualityRegular,
		downloadapp.DownloadQualitySmall,
		downloadapp.DownloadQualityThumb,
		downloadapp.DownloadQualityMini,
	} {
		got, err := m.Download(context.Background(), downloadapp.DownloadRequest{
			IllustIDs: []int64{1},
			Quality:   quality,
		})
		if err != nil {
			t.Fatalf("quality %s: %v", quality, err)
		}
		body, err := os.ReadFile(got[0].Files[0].Path)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != "original" {
			t.Fatalf("quality %s body=%q want %q", quality, body, "original")
		}
	}
}

func TestDownloadUgoiraRejectsQualityAndPages(t *testing.T) {
	dir := t.TempDir()
	client := &fakePixivClient{
		details: map[int64]pixiv.Artwork{
			9: {ID: 9, Title: "u", Kind: pixiv.ArtworkKindUgoira, PageCount: 1, User: pixiv.User{Name: "a"}},
		},
	}
	m := NewManager(client, dir, "{id}")
	_, err := m.Download(context.Background(), downloadapp.DownloadRequest{
		IllustIDs: []int64{9},
		Quality:   downloadapp.DownloadQualityRegular,
	})
	if err == nil || !strings.Contains(err.Error(), "ugoira quality") {
		t.Fatalf("quality error=%v", err)
	}
	_, err = m.Download(context.Background(), downloadapp.DownloadRequest{
		IllustIDs: []int64{9},
		Pages:     []int{1},
		Quality:   downloadapp.DownloadQualityOriginal,
	})
	if err == nil || !strings.Contains(err.Error(), "page selection is unsupported") {
		t.Fatalf("pages error=%v", err)
	}
}
