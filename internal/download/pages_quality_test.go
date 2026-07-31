package download

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	pixiv "github.com/FlanChanXwO/pixiv-cli/pixiv"
)

func TestDownloadPagesSelectionIsOneBasedAndErrorsOnMissing(t *testing.T) {
	dir := t.TempDir()
	client := &fakePixivClient{
		details: map[int64]pixiv.Illust{
			7: {
				ID: 7, Title: "multi", PageCount: 3, Type: "illust", User: pixiv.User{Name: "author"},
				MetaPages: []pixiv.MetaPage{
					{ImageURLs: pixiv.ImageURLs{Original: "https://i.example/7_p0.png"}},
					{ImageURLs: pixiv.ImageURLs{Original: "https://i.example/7_p1.png"}},
					{ImageURLs: pixiv.ImageURLs{Original: "https://i.example/7_p2.png"}},
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
	got, err := m.Download(context.Background(), application.DownloadRequest{
		IllustIDs: []int64{7},
		Pages:     []int{1, 3},
		Quality:   application.DownloadQualityOriginal,
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

	_, err = m.Download(context.Background(), application.DownloadRequest{
		IllustIDs: []int64{7},
		Pages:     []int{4},
		Quality:   application.DownloadQualityOriginal,
	})
	if err == nil || !strings.Contains(err.Error(), "page 4 does not exist") {
		t.Fatalf("missing page error=%v", err)
	}
}

func TestDownloadQualitySelectsMappedURLs(t *testing.T) {
	dir := t.TempDir()
	client := &fakePixivClient{
		details: map[int64]pixiv.Illust{
			1: {
				ID: 1, Title: "q", PageCount: 1, Type: "illust", User: pixiv.User{Name: "a"},
				ImageURLs: pixiv.ImageURLs{
					SquareMedium: "https://i.example/mini.jpg",
					Medium:       "https://i.example/small.jpg",
					Large:        "https://i.example/regular.jpg",
					Original:     "https://i.example/original.png",
				},
				MetaSinglePage: pixiv.SinglePage{OriginalImageURL: "https://i.example/original.png"},
			},
		},
		downloads: map[string][]byte{
			"https://i.example/mini.jpg":     []byte("mini"),
			"https://i.example/small.jpg":    []byte("small"),
			"https://i.example/regular.jpg":  []byte("regular"),
			"https://i.example/original.png": []byte("original"),
		},
	}
	m := NewManager(client, dir, "{id}")
	cases := []struct {
		quality application.DownloadQuality
		want    string
	}{
		{application.DownloadQualityOriginal, "original"},
		{application.DownloadQualityRegular, "regular"},
		{application.DownloadQualitySmall, "small"},
		{application.DownloadQualityThumb, "mini"},
		{application.DownloadQualityMini, "mini"},
	}
	for _, tc := range cases {
		got, err := m.Download(context.Background(), application.DownloadRequest{
			IllustIDs: []int64{1},
			Quality:   tc.quality,
		})
		if err != nil {
			t.Fatalf("quality %s: %v", tc.quality, err)
		}
		body, err := os.ReadFile(got[0].Files[0].Path)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != tc.want {
			t.Fatalf("quality %s body=%q want %q", tc.quality, body, tc.want)
		}
	}
}

func TestDownloadUgoiraRejectsQualityAndPages(t *testing.T) {
	dir := t.TempDir()
	client := &fakePixivClient{
		details: map[int64]pixiv.Illust{
			9: {ID: 9, Title: "u", Type: string(pixiv.IllustTypeUgoira), PageCount: 1, User: pixiv.User{Name: "a"}},
		},
	}
	m := NewManager(client, dir, "{id}")
	_, err := m.Download(context.Background(), application.DownloadRequest{
		IllustIDs: []int64{9},
		Quality:   application.DownloadQualityRegular,
	})
	if err == nil || !strings.Contains(err.Error(), "ugoira quality") {
		t.Fatalf("quality error=%v", err)
	}
	_, err = m.Download(context.Background(), application.DownloadRequest{
		IllustIDs: []int64{9},
		Pages:     []int{1},
		Quality:   application.DownloadQualityOriginal,
	})
	if err == nil || !strings.Contains(err.Error(), "page selection is unsupported") {
		t.Fatalf("pages error=%v", err)
	}
}
