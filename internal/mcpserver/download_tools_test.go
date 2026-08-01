package mcpserver

import (
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/stretchr/testify/require"
)

func TestParseDownloadSelectionAcceptsExplicitUgoiraMode(t *testing.T) {
	pages, quality, format, err := parseDownloadSelection("1,3", "regular", "apng")
	require.NoError(t, err)
	require.Equal(t, []int{1, 3}, pages)
	require.Equal(t, application.DownloadQualityRegular, quality)
	require.Equal(t, application.UgoiraFormatAPNG, format)
}

func TestParseDownloadSelectionDefaultsUgoiraModeToGIF(t *testing.T) {
	_, _, format, err := parseDownloadSelection("", "", "")
	require.NoError(t, err)
	require.Equal(t, application.UgoiraFormatGIF, format)
}

func TestParseDownloadOptionsSupportsOpenPageRange(t *testing.T) {
	selection, quality, mode, err := parseDownloadOptions("3-", "regular", "frames")
	require.NoError(t, err)
	pages, err := selection.Resolve(5)
	require.NoError(t, err)
	require.Equal(t, []int{3, 4, 5}, pages)
	require.Equal(t, sdk.DownloadQualityRegular, quality)
	require.Equal(t, sdk.UgoiraModeFrames, mode)
}
