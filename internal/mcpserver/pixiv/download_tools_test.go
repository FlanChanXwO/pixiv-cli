package pixiv_test

import (
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/download"
	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/stretchr/testify/require"
)

func TestParseDownloadSelectionAcceptsExplicitUgoiraMode(t *testing.T) {
	pages, quality, format, err := download.ParseDownloadSelection("1,3", "regular", "apng")
	require.NoError(t, err)
	require.Equal(t, []int{1, 3}, pages)
	require.Equal(t, downloader.DownloadQualityRegular, quality)
	require.Equal(t, downloader.UgoiraFormatAPNG, format)
}

func TestParseDownloadSelectionDefaultsUgoiraModeToGIF(t *testing.T) {
	_, _, format, err := download.ParseDownloadSelection("", "", "")
	require.NoError(t, err)
	require.Equal(t, downloader.UgoiraFormatGIF, format)
}

func TestParseDownloadOptionsUsesClosedPageRangesAndValidatesQuality(t *testing.T) {
	pages, quality, format, err := download.ParseDownloadOptions("1,3-5", "regular", "apng")
	require.NoError(t, err)
	require.Equal(t, []int{1, 3, 4, 5}, pages)
	require.Equal(t, downloader.DownloadQualityRegular, quality)
	require.Equal(t, downloader.UgoiraFormatAPNG, format)
}

func TestParseDownloadOptionsRejectsOpenRangeAndUnknownUgoiraMode(t *testing.T) {
	if _, _, _, err := download.ParseDownloadOptions("3-", "regular", "gif"); err == nil {
		t.Fatal("open page range must be rejected")
	}
	if _, _, _, err := download.ParseDownloadOptions("1", "regular", "frames"); err == nil {
		t.Fatal("frames ugoira mode must be rejected")
	}
}
