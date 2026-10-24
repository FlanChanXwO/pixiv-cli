package downloader_test

import (
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/stretchr/testify/require"
)

func TestParsePageSpec(t *testing.T) {
	for _, test := range []struct {
		name      string
		spec      string
		wantPages []int
		wantNil   bool
		wantError bool
	}{
		{name: "ranges dedup and sort", spec: "3,1,2-4,1", wantPages: []int{1, 2, 3, 4}},
		{name: "empty means all", spec: "  ", wantNil: true},
		{name: "zero", spec: "0", wantError: true},
		{name: "open range", spec: "1-", wantError: true},
		{name: "negative range", spec: "-2", wantError: true},
		{name: "non integer", spec: "a", wantError: true},
		{name: "reversed range", spec: "2-1", wantError: true},
		{name: "empty item", spec: "1,,2", wantError: true},
		{name: "multiple ranges", spec: "1-2-3", wantError: true},
		{name: "unbounded expansion", spec: "1-100002", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			pages, err := downloader.ParsePageSpec(test.spec)
			if test.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if test.wantNil {
				require.Nil(t, pages)
				return
			}
			require.Equal(t, test.wantPages, pages)
		})
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
