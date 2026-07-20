package pixiv_test

import (
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
