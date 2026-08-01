package pixiv_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pixiv"
)

func TestParsePageSpecAndQuality(t *testing.T) {
	t.Parallel()
	pages, err := pixiv.ParsePageSpec("3,1,2-4,1")
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 4 || pages[0] != 1 || pages[3] != 4 {
		t.Fatalf("pages=%v", pages)
	}
	if err := pixiv.ValidateDownloadQuality(pixiv.DownloadQualityThumb); err != nil {
		t.Fatal(err)
	}
	if err := pixiv.ValidateDownloadQuality("nope"); err == nil {
		t.Fatal("expected invalid quality")
	}
}

func TestParsePageSelectionAcceptsOpenEndedRange(t *testing.T) {
	selection, err := pixiv.ParsePageSelection("3-")
	if err != nil {
		t.Fatal(err)
	}
	pages, err := selection.Resolve(5)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := fmt.Sprint(pages), "[3 4 5]"; got != want {
		t.Fatalf("Resolve() = %s, want %s", got, want)
	}
}

func TestPageSelectionClosedPagesDoesNotExposeOpenRange(t *testing.T) {
	closed, err := pixiv.ParsePageSelection("1,3-4")
	if err != nil {
		t.Fatal(err)
	}
	pages, ok := closed.ClosedPages()
	if !ok || fmt.Sprint(pages) != "[1 3 4]" {
		t.Fatalf("ClosedPages() = %v, %v", pages, ok)
	}
	pages[0] = 99
	again, _ := closed.ClosedPages()
	if again[0] != 1 {
		t.Fatalf("ClosedPages() leaked its backing array: %v", again)
	}
	open, err := pixiv.ParsePageSelection("3-")
	if err != nil {
		t.Fatal(err)
	}
	if pages, ok := open.ClosedPages(); ok || pages != nil {
		t.Fatalf("open ClosedPages() = %v, %v", pages, ok)
	}
}

func TestDownloadOptionsExposeArchiveAndUgoiraModes(t *testing.T) {
	for _, mode := range []pixiv.UgoiraMode{pixiv.UgoiraModeGIF, pixiv.UgoiraModeAPNG, pixiv.UgoiraModeZIP, pixiv.UgoiraModeFrames} {
		if err := pixiv.ValidateUgoiraMode(mode); err != nil {
			t.Fatalf("ValidateUgoiraMode(%q): %v", mode, err)
		}
	}
}

func TestValidateUgoiraFormat(t *testing.T) {
	t.Parallel()
	for _, format := range []pixiv.UgoiraFormat{
		pixiv.UgoiraFormatGIF,
		pixiv.UgoiraFormatAPNG,
	} {
		if err := pixiv.ValidateUgoiraFormat(format); err != nil {
			t.Fatalf("format %q: %v", format, err)
		}
	}
	if err := pixiv.ValidateUgoiraFormat("webp"); err == nil {
		t.Fatal("expected invalid ugoira format")
	}
}

func TestDownloadDefaultsAreDocumentedStableValues(t *testing.T) {
	if pixiv.DefaultDownloadPath != "./downloads" {
		t.Fatalf("download path = %q", pixiv.DefaultDownloadPath)
	}
	if pixiv.DefaultFilenameTemplate != "{author} - {title}_{id}" {
		t.Fatalf("filename template = %q", pixiv.DefaultFilenameTemplate)
	}
}

func TestClientOptionTypesKeepConstructorBoundaries(t *testing.T) {
	direct := reflect.TypeOf(pixiv.NewClientOptions{})
	for _, forbidden := range []string{"AuthFilePath", "ConfigFilePath", "OAuthBaseURL", "RefreshToken", "UserID"} {
		if _, found := direct.FieldByName(forbidden); found {
			t.Fatalf("NewClientOptions unexpectedly contains %s", forbidden)
		}
	}
	defaults := reflect.TypeOf(pixiv.OpenDefaultOptions{})
	for _, required := range []string{"AuthFilePath", "ConfigFilePath", "OAuthBaseURL", "RefreshToken", "UserID"} {
		if _, found := defaults.FieldByName(required); !found {
			t.Fatalf("OpenDefaultOptions is missing %s", required)
		}
	}
}
