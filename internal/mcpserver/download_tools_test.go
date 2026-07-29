package mcpserver

import (
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/stretchr/testify/require"
)

func TestParseDownloadSelectionAcceptsExplicitUgoiraFormat(t *testing.T) {
	pages, quality, format, err := parseDownloadSelection("1,3", "regular", "apng")
	require.NoError(t, err)
	require.Equal(t, []int{1, 3}, pages)
	require.Equal(t, application.DownloadQualityRegular, quality)
	require.Equal(t, application.UgoiraFormatAPNG, format)
}

func TestParseDownloadSelectionDefaultsUgoiraFormatToGIF(t *testing.T) {
	_, _, format, err := parseDownloadSelection("", "", "")
	require.NoError(t, err)
	require.Equal(t, application.UgoiraFormatGIF, format)
}
