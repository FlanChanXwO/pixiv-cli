package pixiv_test

import (
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/stretchr/testify/require"
)

func TestParseReferenceNormalizesArtworkAndUserURLs(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want pixiv.Reference
	}{
		{name: "numeric artwork ID", raw: "42", want: pixiv.Reference{Kind: pixiv.ReferenceKindArtwork, ID: 42}},
		{name: "artwork URL", raw: "https://www.pixiv.net/artworks/42", want: pixiv.Reference{Kind: pixiv.ReferenceKindArtwork, ID: 42}},
		{name: "localized artwork URL with query and fragment", raw: "https://pixiv.net/zh-CN/artworks/42?utm_source=share#page=1", want: pixiv.Reference{Kind: pixiv.ReferenceKindArtwork, ID: 42}},
		{name: "user profile URL", raw: "https://www.pixiv.net/users/7", want: pixiv.Reference{Kind: pixiv.ReferenceKindUser, ID: 7}},
		{name: "user artworks URL", raw: "https://www.pixiv.net/users/7/artworks", want: pixiv.Reference{Kind: pixiv.ReferenceKindUser, ID: 7}},
		{name: "localized user artworks URL", raw: "https://www.pixiv.net/en/users/7/artworks", want: pixiv.Reference{Kind: pixiv.ReferenceKindUser, ID: 7}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pixiv.ParseReference(tt.raw)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestParseArtworkReferenceRejectsUnsupportedTargetsWithoutEchoingInput(t *testing.T) {
	for _, raw := range []string{
		"https://www.pixiv.net/users/7",
		"https://www.pixiv.net/novel/show.php?id=42",
		"https://www.pixiv.net/member_illust.php?illust_id=42",
		"https://fanbox.cc/@artist",
		"https://pixiv.me/artist",
		"https://example.invalid/artworks/42?secret=must-not-echo",
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := pixiv.ParseArtworkReference(raw)
			require.Error(t, err)
			require.NotContains(t, err.Error(), raw)
			require.NotContains(t, err.Error(), "must-not-echo")
		})
	}

	_, err := pixiv.ParseReference("https://www.pixiv.net/artworks/0")
	require.Error(t, err)
	require.False(t, strings.Contains(err.Error(), "artworks/0"))
}

func TestParseUserReferenceIsSymmetricWithArtworkReference(t *testing.T) {
	for _, raw := range []string{"7", "https://www.pixiv.net/users/7", "https://www.pixiv.net/en/users/7/artworks"} {
		got, err := pixiv.ParseUserReference(raw)
		require.NoError(t, err)
		require.EqualValues(t, 7, got)
	}
	_, err := pixiv.ParseUserReference("https://www.pixiv.net/artworks/7?secret=must-not-echo")
	require.Error(t, err)
	require.NotContains(t, err.Error(), "must-not-echo")
}
