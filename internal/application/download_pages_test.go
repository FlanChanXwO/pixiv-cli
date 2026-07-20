package application_test

import (
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/stretchr/testify/require"
)

func TestParsePageSpecParsesRangesDedupsAndSorts(t *testing.T) {
	pages, err := application.ParsePageSpec("3,1,2-4,1")
	require.NoError(t, err)
	require.Equal(t, []int{1, 2, 3, 4}, pages)
}

func TestParsePageSpecEmptyMeansAll(t *testing.T) {
	pages, err := application.ParsePageSpec("  ")
	require.NoError(t, err)
	require.Nil(t, pages)
}

func TestParsePageSpecRejectsInvalid(t *testing.T) {
	for _, spec := range []string{"0", "1-", "-2", "a", "2-1", "1,,2", "1-2-3"} {
		_, err := application.ParsePageSpec(spec)
		require.Error(t, err, spec)
	}
}

func TestValidateDownloadQuality(t *testing.T) {
	for _, q := range []application.DownloadQuality{
		application.DownloadQualityOriginal,
		application.DownloadQualityRegular,
		application.DownloadQualitySmall,
		application.DownloadQualityThumb,
		application.DownloadQualityMini,
	} {
		require.NoError(t, application.ValidateDownloadQuality(q))
	}
	require.Error(t, application.ValidateDownloadQuality("huge"))
}
