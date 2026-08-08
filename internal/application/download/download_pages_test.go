package download_test

import (
	"testing"

	downloadapp "github.com/FlanChanXwO/pixiv-cli/internal/application/download"
	"github.com/stretchr/testify/require"
)

func TestParsePageSpecParsesRangesDedupsAndSorts(t *testing.T) {
	pages, err := downloadapp.ParsePageSpec("3,1,2-4,1")
	require.NoError(t, err)
	require.Equal(t, []int{1, 2, 3, 4}, pages)
}

func TestParsePageSpecEmptyMeansAll(t *testing.T) {
	pages, err := downloadapp.ParsePageSpec("  ")
	require.NoError(t, err)
	require.Nil(t, pages)
}

func TestParsePageSpecRejectsInvalid(t *testing.T) {
	for _, spec := range []string{"0", "1-", "-2", "a", "2-1", "1,,2", "1-2-3"} {
		_, err := downloadapp.ParsePageSpec(spec)
		require.Error(t, err, spec)
	}
}

func TestValidateDownloadQuality(t *testing.T) {
	for _, q := range []downloadapp.DownloadQuality{
		downloadapp.DownloadQualityOriginal,
		downloadapp.DownloadQualityRegular,
		downloadapp.DownloadQualitySmall,
		downloadapp.DownloadQualityThumb,
		downloadapp.DownloadQualityMini,
	} {
		require.NoError(t, downloadapp.ValidateDownloadQuality(q))
	}
	require.Error(t, downloadapp.ValidateDownloadQuality("huge"))
}
