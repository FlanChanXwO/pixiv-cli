package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimelineCommandsRouteTypedAppSDKRequestsAndOutputLists(t *testing.T) {
	useTempPaths(t)
	var followingIllust pixiv.FollowingArtworksRequest
	var followingNovel pixiv.FollowingNovelsRequest
	var latestIllust pixiv.LatestArtworksRequest
	var latestNovel pixiv.LatestNovelsRequest
	setTestSDKCommandClient(t, sdkCommandFake{
		followingArtworks: func(_ context.Context, request pixiv.FollowingArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
			followingIllust = request
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{commandArtwork(11)}}, nil
		},
		followingNovels: func(_ context.Context, request pixiv.FollowingNovelsRequest) (sdk.Page[pixiv.Novel], error) {
			followingNovel = request
			return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 12, Title: "follow novel", User: pixiv.User{Name: "writer"}}}}, nil
		},
		latestArtworks: func(_ context.Context, request pixiv.LatestArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
			latestIllust = request
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{commandArtwork(13)}}, nil
		},
		latestNovels: func(_ context.Context, request pixiv.LatestNovelsRequest) (sdk.Page[pixiv.Novel], error) {
			latestNovel = request
			return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 14, Title: "latest novel", User: pixiv.User{Name: "writer"}}}}, nil
		},
	})

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, Run([]string{"pixiv", "timeline", "following", "--type", "illust", "--restrict", "private", "--limit", "1", "--json"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, pixiv.RestrictPrivate, followingIllust.Restrict)
	var illustOut struct {
		Illusts []pixiv.Artwork `json:"illusts"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &illustOut))
	assert.Len(t, illustOut.Illusts, 1)

	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "timeline", "following", "--type", "novel"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, pixiv.RestrictPublic, followingNovel.Restrict)
	assert.Contains(t, stdout.String(), "new novels from followed users")
	assert.Contains(t, stdout.String(), "follow novel")

	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "timeline", "latest", "--type", "manga", "--json"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, pixiv.SearchContentTypeManga, latestIllust.ContentType)
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &illustOut))
	assert.Len(t, illustOut.Illusts, 1)

	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "timeline", "latest", "--type", "novel"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.True(t, latestNovel.Cursor.IsZero())
	assert.Contains(t, stdout.String(), "latest novels")
}

func TestTimelineDoesNotRetainFeedCompatibilityAlias(t *testing.T) {
	useTempPaths(t)
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "feed", "latest", "--type", "illust"}, strings.NewReader(""), &stdout, &stderr)
	if code == 0 || !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("removed command result=%d stderr=%q", code, stderr.String())
	}
}

func TestMyPixivCommandsRouteAggregateAndSpecificUserRequests(t *testing.T) {
	useTempPaths(t)
	var myPixivUsers pixiv.MyPixivUsersRequest
	var myPixivIllusts pixiv.MyPixivArtworksRequest
	var myPixivNovels pixiv.MyPixivNovelsRequest
	var userArtworks pixiv.UserArtworksRequest
	var userNovels pixiv.UserNovelsRequest
	setTestSDKCommandClient(t, sdkCommandFake{
		currentUserID: func(context.Context) (int64, error) { return 77, nil },
		myPixivUsers: func(_ context.Context, request pixiv.MyPixivUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
			myPixivUsers = request
			return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 8, Name: "friend"}}}}, nil
		},
		myPixivArtworks: func(_ context.Context, request pixiv.MyPixivArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
			myPixivIllusts = request
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{commandArtwork(21)}}, nil
		},
		myPixivNovels: func(_ context.Context, request pixiv.MyPixivNovelsRequest) (sdk.Page[pixiv.Novel], error) {
			myPixivNovels = request
			return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 22, Title: "MyPixiv novel", User: pixiv.User{Name: "writer"}}}}, nil
		},
		artworks: func(_ context.Context, request pixiv.UserArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
			userArtworks = request
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{commandArtwork(23)}}, nil
		},
		userNovels: func(_ context.Context, request pixiv.UserNovelsRequest) (sdk.Page[pixiv.Novel], error) {
			userNovels = request
			return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 24, Title: "friend novel", User: pixiv.User{Name: "writer"}}}}, nil
		},
	})

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, Run([]string{"pixiv", "mypixiv", "users", "--json"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	var usersOut struct {
		Users []pixiv.UserPreview `json:"user_previews"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &usersOut))
	assert.Len(t, usersOut.Users, 1)
	assert.True(t, myPixivUsers.Cursor.IsZero())

	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "mypixiv", "works", "--type", "illust", "--json"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.True(t, myPixivIllusts.Cursor.IsZero())

	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "mypixiv", "works", "--type", "novel"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.True(t, myPixivNovels.Cursor.IsZero())
	assert.Contains(t, stdout.String(), "MyPixiv novels")

	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "mypixiv", "works", "55", "--type", "manga", "--json"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, int64(55), userArtworks.UserID)
	assert.Equal(t, pixiv.ArtworkKindManga, userArtworks.Kind)

	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "mypixiv", "works", "55", "--type", "novel"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, int64(55), userNovels.UserID)
	assert.Contains(t, stdout.String(), "friend novel")
}

func TestTimelineAndMyPixivRejectUnsupportedTypeCombinationsBeforeSDKCalls(t *testing.T) {
	useTempPaths(t)
	opened := 0
	setTestSDKCommandClient(t, sdkCommandFake{latestArtworks: func(context.Context, pixiv.LatestArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		opened++
		return sdk.Page[pixiv.Artwork]{}, nil
	}})

	for _, args := range [][]string{
		{"pixiv", "timeline", "following", "--type", "manga"},
		{"pixiv", "timeline", "latest", "--type", "ugoira"},
		{"pixiv", "mypixiv", "works", "--type", "manga"},
		{"pixiv", "mypixiv", "works", "12", "--type", "ugoira"},
	} {
		var stdout, stderr bytes.Buffer
		assert.NotZero(t, Run(args, strings.NewReader(""), &stdout, &stderr), args)
		assert.Empty(t, stdout.String(), args)
		assert.Contains(t, stderr.String(), "type", args)
	}
	assert.Zero(t, opened)
}
