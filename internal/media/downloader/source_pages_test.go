package downloader_test

import (
	"testing"

	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/stretchr/testify/require"
)

func TestParsePageSpecParsesRangesDedupsAndSorts(t *testing.T) {
	pages, err := downloader.ParsePageSpec("3,1,2-4,1")
	require.NoError(t, err)
	require.Equal(t, []int{1, 2, 3, 4}, pages)
}

func TestParsePageSpecEmptyMeansAll(t *testing.T) {
	pages, err := downloader.ParsePageSpec("  ")
	require.NoError(t, err)
	require.Nil(t, pages)
}

func TestParsePageSpecRejectsInvalid(t *testing.T) {
	for _, spec := range []string{"0", "1-", "-2", "a", "2-1", "1,,2", "1-2-3"} {
		_, err := downloader.ParsePageSpec(spec)
		require.Error(t, err, spec)
	}
}

func TestValidateDownloadQuality(t *testing.T) {
	for _, q := range []downloader.DownloadQuality{
		downloader.DownloadQualityOriginal,
		downloader.DownloadQualityRegular,
		downloader.DownloadQualitySmall,
		downloader.DownloadQualityThumb,
		downloader.DownloadQualityMini,
	} {
		require.NoError(t, downloader.ValidateDownloadQuality(q))
	}
	require.Error(t, downloader.ValidateDownloadQuality("huge"))
}
