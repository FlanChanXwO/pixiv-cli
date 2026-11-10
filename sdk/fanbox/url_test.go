package fanbox_test

import (
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/stretchr/testify/require"
)

func TestFanboxURLs(t *testing.T) {
	client, err := fanbox.New("session-fixture", fanbox.Options{})
	require.NoError(t, err)
	for _, tc := range []struct {
		raw, canonical string
		ref            any
	}{
		{"https://fanbox.cc/creators/artist", "https://www.fanbox.cc/@artist", &fanbox.CreatorRef{}},
		{" https://www.fanbox.cc/@artist/?tracking=fixture#part ", "https://www.fanbox.cc/@artist", &fanbox.CreatorRef{}},
		{"https://www.fanbox.cc/@artist/posts", "https://www.fanbox.cc/@artist/posts", &fanbox.CreatorPostsRef{}},
		{"https://www.fanbox.cc/@artist/posts/42", "https://www.fanbox.cc/@artist/posts/42", &fanbox.PostRef{}},
		{"https://www.fanbox.cc/@artist/posts/tag/%E7%B5%B5", "https://www.fanbox.cc/@artist/posts/tag/%E7%B5%B5", &fanbox.CreatorPostsRef{}},
		{"https://artist.fanbox.cc", "https://www.fanbox.cc/@artist", &fanbox.CreatorRef{}},
		{"https://artist.fanbox.cc/posts/42", "https://www.fanbox.cc/@artist", &fanbox.CreatorRef{}},
		{"https://ARTIST.FANBOX.CC:443/any/path", "https://www.fanbox.cc/@artist", &fanbox.CreatorRef{}},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			ref, err := client.ParseURL(tc.raw)
			require.NoError(t, err)
			require.IsType(t, tc.ref, ref)
			canonical, err := ref.CanonicalURL()
			require.NoError(t, err)
			require.Equal(t, tc.canonical, canonical)
		})
	}
}

func TestFanboxURLFailures(t *testing.T) {
	client, err := fanbox.New("session-fixture", fanbox.Options{})
	require.NoError(t, err)
	for _, raw := range []string{"http://www.fanbox.cc/@artist", "https://user:secret-fixture@www.fanbox.cc/@artist", "https://fanbox.cc.evil.invalid/@artist", "https://nested.artist.fanbox.cc/", "https://www.fanbox.cc/@", "https://www.fanbox.cc/@artist/posts/tag", "https://www.fanbox.cc/@artist/unknown", "https://www.fanbox.cc/%zz"} {
		t.Run(raw, func(t *testing.T) {
			_, err := client.ParseURL(raw)
			require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
			require.NotContains(t, err.Error(), "secret-fixture")
		})
	}
}
