package pixiv

import (
	"testing"

	downloadapp "github.com/FlanChanXwO/pixiv-cli/internal/application/download"
	"github.com/stretchr/testify/require"
)

func TestParseDownloadSelectionAcceptsExplicitUgoiraMode(t *testing.T) {
	pages, quality, format, err := parseDownloadSelection("1,3", "regular", "apng")
	require.NoError(t, err)
	require.Equal(t, []int{1, 3}, pages)
	require.Equal(t, downloadapp.DownloadQualityRegular, quality)
	require.Equal(t, downloadapp.UgoiraFormatAPNG, format)
}

func TestParseDownloadSelectionDefaultsUgoiraModeToGIF(t *testing.T) {
	_, _, format, err := parseDownloadSelection("", "", "")
	require.NoError(t, err)
	require.Equal(t, downloadapp.UgoiraFormatGIF, format)
}

func TestParseDownloadOptionsUsesClosedPageRangesAndValidatesQuality(t *testing.T) {
	pages, quality, format, err := parseDownloadOptions("1,3-5", "regular", "apng")
	require.NoError(t, err)
	require.Equal(t, []int{1, 3, 4, 5}, pages)
	require.Equal(t, downloadapp.DownloadQualityRegular, quality)
	require.Equal(t, downloadapp.UgoiraFormatAPNG, format)
}

func TestParseDownloadOptionsRejectsOpenRangeAndUnknownUgoiraMode(t *testing.T) {
	if _, _, _, err := parseDownloadOptions("3-", "regular", "gif"); err == nil {
		t.Fatal("open page range must be rejected")
	}
	if _, _, _, err := parseDownloadOptions("1", "regular", "frames"); err == nil {
		t.Fatal("frames ugoira mode must be rejected")
	}
}
