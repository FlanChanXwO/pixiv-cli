package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimelineCommandsRouteTypedAppSDKRequestsAndOutputLists(t *testing.T) {
	useTempPaths(t)
	var followingIllust sdk.FollowingIllustsRequest
	var followingNovel sdk.FollowingNovelsRequest
	var latestIllust sdk.LatestIllustsRequest
	var latestNovel sdk.LatestNovelsRequest
	setTestSDKCommandClient(t, sdkCommandFake{
		followingIllusts: func(_ context.Context, request sdk.FollowingIllustsRequest) (*sdk.IllustListResult, error) {
			followingIllust = request
			return &sdk.IllustListResult{Illusts: []sdk.Illust{commandIllust(11)}}, nil
		},
		followingNovels: func(_ context.Context, request sdk.FollowingNovelsRequest) (*sdk.NovelListResult, error) {
			followingNovel = request
			return &sdk.NovelListResult{Novels: []sdk.Novel{{ID: 12, Title: "follow novel", User: sdk.User{Name: "writer"}}}}, nil
		},
		latestIllusts: func(_ context.Context, request sdk.LatestIllustsRequest) (*sdk.IllustListResult, error) {
			latestIllust = request
			return &sdk.IllustListResult{Illusts: []sdk.Illust{commandIllust(13)}}, nil
		},
		latestNovels: func(_ context.Context, request sdk.LatestNovelsRequest) (*sdk.NovelListResult, error) {
			latestNovel = request
			return &sdk.NovelListResult{Novels: []sdk.Novel{{ID: 14, Title: "latest novel", User: sdk.User{Name: "writer"}}}}, nil
		},
	})

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, Run([]string{"pixiv", "timeline", "following", "--type", "illust", "--restrict", "private", "--limit", "1", "--json"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, sdk.RestrictPrivate, followingIllust.Restrict)
	var illustOut struct {
		Illusts []sdk.Illust `json:"illusts"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &illustOut))
	assert.Len(t, illustOut.Illusts, 1)

	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "timeline", "following", "--type", "novel"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, sdk.RestrictPublic, followingNovel.Restrict)
	assert.Contains(t, stdout.String(), "new novels from followed users")
	assert.Contains(t, stdout.String(), "follow novel")

	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "timeline", "latest", "--type", "manga", "--json"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, sdk.IllustTypeManga, latestIllust.Type)
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &illustOut))
	assert.Len(t, illustOut.Illusts, 1)

	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "timeline", "latest", "--type", "novel"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, sdk.Cursor(""), latestNovel.Cursor)
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
	var myPixivUsers sdk.MyPixivUsersRequest
	var myPixivIllusts sdk.MyPixivIllustsRequest
	var myPixivNovels sdk.MyPixivNovelsRequest
	var userArtworks sdk.UserArtworksRequest
	var userNovels sdk.UserNovelsRequest
	setTestSDKCommandClient(t, sdkCommandFake{
		currentUserID: func(context.Context) (int64, error) { return 77, nil },
		myPixivUsers: func(_ context.Context, request sdk.MyPixivUsersRequest) (*sdk.UserListResult, error) {
			myPixivUsers = request
			return &sdk.UserListResult{UserPreviews: []sdk.UserPreview{{User: sdk.User{ID: 8, Name: "friend"}}}}, nil
		},
		myPixivIllusts: func(_ context.Context, request sdk.MyPixivIllustsRequest) (*sdk.IllustListResult, error) {
			myPixivIllusts = request
			return &sdk.IllustListResult{Illusts: []sdk.Illust{commandIllust(21)}}, nil
		},
		myPixivNovels: func(_ context.Context, request sdk.MyPixivNovelsRequest) (*sdk.NovelListResult, error) {
			myPixivNovels = request
			return &sdk.NovelListResult{Novels: []sdk.Novel{{ID: 22, Title: "MyPixiv novel", User: sdk.User{Name: "writer"}}}}, nil
		},
		artworks: func(_ context.Context, request sdk.UserArtworksRequest) (*sdk.IllustListResult, error) {
			userArtworks = request
			return &sdk.IllustListResult{Illusts: []sdk.Illust{commandIllust(23)}}, nil
		},
		userNovels: func(_ context.Context, request sdk.UserNovelsRequest) (*sdk.NovelListResult, error) {
			userNovels = request
			return &sdk.NovelListResult{Novels: []sdk.Novel{{ID: 24, Title: "friend novel", User: sdk.User{Name: "writer"}}}}, nil
		},
	})

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, Run([]string{"pixiv", "mypixiv", "users", "--json"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, int64(77), myPixivUsers.UserID)
	var usersOut struct {
		Users []sdk.UserPreview `json:"user_previews"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &usersOut))
	assert.Len(t, usersOut.Users, 1)

	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "mypixiv", "works", "--type", "illust", "--json"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, sdk.Cursor(""), myPixivIllusts.Cursor)

	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "mypixiv", "works", "--type", "novel"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, sdk.Cursor(""), myPixivNovels.Cursor)
	assert.Contains(t, stdout.String(), "MyPixiv novels")

	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "mypixiv", "works", "55", "--type", "manga", "--json"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, int64(55), userArtworks.UserID)
	assert.Equal(t, sdk.IllustTypeManga, userArtworks.Type)

	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "mypixiv", "works", "55", "--type", "novel"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, int64(55), userNovels.UserID)
	assert.Contains(t, stdout.String(), "friend novel")
}

func TestTimelineAndMyPixivRejectUnsupportedTypeCombinationsBeforeSDKCalls(t *testing.T) {
	useTempPaths(t)
	opened := 0
	setTestSDKCommandClient(t, sdkCommandFake{latestIllusts: func(context.Context, sdk.LatestIllustsRequest) (*sdk.IllustListResult, error) {
		opened++
		return nil, nil
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
