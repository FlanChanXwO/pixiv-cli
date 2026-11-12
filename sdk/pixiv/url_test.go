package pixiv_test

import (
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

func TestNovelAndSeriesURLs(t *testing.T) {
	c := &pixiv.Client{}
	for _, test := range []struct {
		url, want string
		ref       pixiv.Reference
	}{
		{"https://pixiv.net/novel/show.php?id=23&tracking=1#view", "https://www.pixiv.net/novel/show.php?id=23", c.Novel(23)},
		{"https://www.pixiv.net/zh-CN/novel/show.php?id=23", "https://www.pixiv.net/novel/show.php?id=23", c.Novel(23)},
		{"https://pixiv.net/novel/series/7", "https://www.pixiv.net/novel/series/7", c.NovelSeries(7)},
		{"https://pixiv.net/en/novel/series/7/", "https://www.pixiv.net/novel/series/7", c.NovelSeries(7)},
		{"https://pixiv.net/user/11/series/7", "https://www.pixiv.net/user/11/series/7", c.ArtworkSeries(7)},
		{"https://www.pixiv.net/en/user/11/series/7?tracking=1", "https://www.pixiv.net/user/11/series/7", c.ArtworkSeries(7)},
	} {
		t.Run(test.url, func(t *testing.T) {
			ref, err := c.ParseURL(test.url)
			require.NoError(t, err)
			require.IsType(t, test.ref, ref)
			canonical, err := ref.CanonicalURL()
			require.NoError(t, err)
			require.Equal(t, test.want, canonical)
			if series, ok := ref.(*pixiv.ArtworkSeriesRef); ok {
				require.EqualValues(t, 11, series.OwnerUserID)
			}
		})
	}
}

func TestMalformedNovelAndSeriesURLs(t *testing.T) {
	c := &pixiv.Client{}
	for _, raw := range []string{
		"https://pixiv.net/novel/show.php", "https://pixiv.net/novel/show.php?id=23&id=24",
		"https://pixiv.net/novel/show.php?id=0", "https://pixiv.net/novel/show.php?id=23&id=%zz",
		"https://pixiv.net/novel/series/0", "https://pixiv.net/user/0/series/7",
		"https://pixiv.net/user/11/series/x", "https://pixiv.net/novel/series/9223372036854775808",
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := c.ParseURL(raw)
			require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
		})
	}
}

func TestNovelAndSeriesCanonicalValidation(t *testing.T) {
	c := &pixiv.Client{}
	for _, ref := range []pixiv.Reference{
		(*pixiv.NovelRef)(nil), c.Novel(0), (*pixiv.ArtworkSeriesRef)(nil), c.ArtworkSeries(7),
		&pixiv.ArtworkSeriesRef{ID: 0, OwnerUserID: 11}, (*pixiv.NovelSeriesRef)(nil), c.NovelSeries(-1),
	} {
		_, err := ref.CanonicalURL()
		require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	}
	url, err := c.Novel(23).CanonicalURL()
	require.NoError(t, err)
	require.Equal(t, "https://www.pixiv.net/novel/show.php?id=23", url)
	url, err = c.NovelSeries(7).CanonicalURL()
	require.NoError(t, err)
	require.Equal(t, "https://www.pixiv.net/novel/series/7", url)
}
