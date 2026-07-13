package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/bootstrap"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/auth"
	sdk "github.com/FlanChanXwO/pixiv-cli/pkg/pixiv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sdkCommandFake struct {
	currentUserID  func(context.Context) (int64, error)
	search         func(context.Context, sdk.SearchIllustRequest) (*sdk.IllustListResult, error)
	detail         func(context.Context, int64) (*sdk.IllustDetail, error)
	ranking        func(context.Context, sdk.IllustRankingRequest) (*sdk.IllustListResult, error)
	recommended    func(context.Context, sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error)
	artworks       func(context.Context, sdk.UserArtworksRequest) (*sdk.IllustListResult, error)
	bookmarks      func(context.Context, sdk.UserBookmarksRequest) (*sdk.IllustListResult, error)
	following      func(context.Context, sdk.UserFollowingRequest) (*sdk.UserListResult, error)
	addBookmark    func(context.Context, sdk.AddBookmarkRequest) error
	removeBookmark func(context.Context, sdk.RemoveBookmarkRequest) error
	follow         func(context.Context, sdk.FollowUserRequest) error
	unfollow       func(context.Context, sdk.UnfollowUserRequest) error
}

func unimplementedSDKCommand() error { return errors.New("unexpected sdk command") }
func (f sdkCommandFake) CurrentUserID(ctx context.Context) (int64, error) {
	if f.currentUserID != nil {
		return f.currentUserID(ctx)
	}
	return 0, unimplementedSDKCommand()
}
func (f sdkCommandFake) SearchIllust(ctx context.Context, r sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
	if f.search != nil {
		return f.search(ctx, r)
	}
	return nil, unimplementedSDKCommand()
}
func (f sdkCommandFake) IllustDetail(ctx context.Context, id int64) (*sdk.IllustDetail, error) {
	if f.detail != nil {
		return f.detail(ctx, id)
	}
	return nil, unimplementedSDKCommand()
}
func (f sdkCommandFake) IllustRanking(ctx context.Context, r sdk.IllustRankingRequest) (*sdk.IllustListResult, error) {
	if f.ranking != nil {
		return f.ranking(ctx, r)
	}
	return nil, unimplementedSDKCommand()
}
func (f sdkCommandFake) IllustRecommended(ctx context.Context, r sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error) {
	if f.recommended != nil {
		return f.recommended(ctx, r)
	}
	return nil, unimplementedSDKCommand()
}
func (f sdkCommandFake) UserArtworks(ctx context.Context, r sdk.UserArtworksRequest) (*sdk.IllustListResult, error) {
	if f.artworks != nil {
		return f.artworks(ctx, r)
	}
	return nil, unimplementedSDKCommand()
}
func (f sdkCommandFake) UserBookmarks(ctx context.Context, r sdk.UserBookmarksRequest) (*sdk.IllustListResult, error) {
	if f.bookmarks != nil {
		return f.bookmarks(ctx, r)
	}
	return nil, unimplementedSDKCommand()
}
func (f sdkCommandFake) UserFollowing(ctx context.Context, r sdk.UserFollowingRequest) (*sdk.UserListResult, error) {
	if f.following != nil {
		return f.following(ctx, r)
	}
	return nil, unimplementedSDKCommand()
}
func (f sdkCommandFake) AddBookmark(ctx context.Context, r sdk.AddBookmarkRequest) error {
	if f.addBookmark != nil {
		return f.addBookmark(ctx, r)
	}
	return unimplementedSDKCommand()
}
func (f sdkCommandFake) RemoveBookmark(ctx context.Context, r sdk.RemoveBookmarkRequest) error {
	if f.removeBookmark != nil {
		return f.removeBookmark(ctx, r)
	}
	return unimplementedSDKCommand()
}
func (f sdkCommandFake) FollowUser(ctx context.Context, r sdk.FollowUserRequest) error {
	if f.follow != nil {
		return f.follow(ctx, r)
	}
	return unimplementedSDKCommand()
}
func (f sdkCommandFake) UnfollowUser(ctx context.Context, r sdk.UnfollowUserRequest) error {
	if f.unfollow != nil {
		return f.unfollow(ctx, r)
	}
	return unimplementedSDKCommand()
}

func setTestSDKCommandClient(t *testing.T, client application.SDKClient) {
	t.Helper()
	old := newCLIServices
	newCLIServices = func(logger *slog.Logger) application.Services {
		services := bootstrap.NewServices(logger)
		services.SDK.NewClient = func(application.SDKClientRequest) (application.SDKClient, error) { return client, nil }
		return services
	}
	t.Cleanup(func() { newCLIServices = old })
}

func commandIllust(id int64) sdk.Illust {
	return sdk.Illust{ID: id, Title: "work", User: sdk.User{Name: "artist"}}
}

func TestUserCommandsRouteOptionalIDAndMutationsThroughSDK(t *testing.T) {
	useTempPaths(t)
	var gotArtwork sdk.UserArtworksRequest
	var gotBookmarks sdk.UserBookmarksRequest
	var gotFollowing sdk.UserFollowingRequest
	var gotBookmark sdk.AddBookmarkRequest
	var gotFollow sdk.FollowUserRequest
	var removedBookmark sdk.RemoveBookmarkRequest
	var unfollowed sdk.UnfollowUserRequest
	currentCalls := 0
	setTestSDKCommandClient(t, sdkCommandFake{
		currentUserID: func(context.Context) (int64, error) { currentCalls++; return 77, nil },
		artworks: func(_ context.Context, request sdk.UserArtworksRequest) (*sdk.IllustListResult, error) {
			gotArtwork = request
			return &sdk.IllustListResult{Illusts: []sdk.Illust{commandIllust(1)}}, nil
		},
		bookmarks: func(_ context.Context, request sdk.UserBookmarksRequest) (*sdk.IllustListResult, error) {
			gotBookmarks = request
			return &sdk.IllustListResult{}, nil
		},
		following: func(_ context.Context, request sdk.UserFollowingRequest) (*sdk.UserListResult, error) {
			gotFollowing = request
			return &sdk.UserListResult{}, nil
		},
		addBookmark: func(_ context.Context, request sdk.AddBookmarkRequest) error { gotBookmark = request; return nil },
		follow:      func(_ context.Context, request sdk.FollowUserRequest) error { gotFollow = request; return nil },
		removeBookmark: func(_ context.Context, request sdk.RemoveBookmarkRequest) error {
			removedBookmark = request
			return nil
		},
		unfollow: func(_ context.Context, request sdk.UnfollowUserRequest) error { unfollowed = request; return nil },
	})

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, Run([]string{"pixiv", "user", "artworks", "--json"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, int64(77), gotArtwork.UserID)
	assert.Equal(t, 1, currentCalls)

	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "user", "bookmarks", "88", "--restrict", "private", "--tag", "favourite", "--json"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, sdk.UserBookmarksRequest{UserID: 88, Restrict: sdk.RestrictPrivate, Tag: "favourite"}, gotBookmarks)

	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "user", "following", "99", "--json"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, sdk.UserFollowingRequest{UserID: 99, Restrict: sdk.RestrictPublic}, gotFollowing)

	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "bookmark", "add", "42", "--restrict", "private", "--tag", "first", "--tag", "second"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, sdk.AddBookmarkRequest{IllustID: 42, Restrict: sdk.RestrictPrivate, Tags: []string{"first", "second"}}, gotBookmark)

	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "follow", "add", "88", "--restrict", "private"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, sdk.FollowUserRequest{UserID: 88, Restrict: sdk.RestrictPrivate}, gotFollow)

	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "bookmark", "remove", "42"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, sdk.RemoveBookmarkRequest{IllustID: 42}, removedBookmark)

	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "follow", "remove", "88"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, sdk.UnfollowUserRequest{UserID: 88}, unfollowed)
}

func TestListPaginationAndValidationUseOpaqueCursorWithoutCursorFlag(t *testing.T) {
	useTempPaths(t)
	var cursors []sdk.Cursor
	setTestSDKCommandClient(t, sdkCommandFake{search: func(_ context.Context, request sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
		cursors = append(cursors, request.Cursor)
		switch request.Cursor {
		case "":
			return &sdk.IllustListResult{Illusts: []sdk.Illust{commandIllust(1), commandIllust(2)}, NextCursor: "next"}, nil
		case "next":
			return &sdk.IllustListResult{Illusts: []sdk.Illust{commandIllust(3), commandIllust(4)}}, nil
		default:
			return nil, errors.New("unexpected cursor")
		}
	}})

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, Run([]string{"pixiv", "search", "miku", "--json"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, []sdk.Cursor{""}, cursors, "default consumes exactly one upstream batch")

	cursors = nil
	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "search", "miku", "--json", "--page", "2", "--limit", "2"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, []sdk.Cursor{"", "next"}, cursors)
	var out struct {
		Illusts []sdk.Illust `json:"illusts"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out))
	require.Equal(t, []int64{3, 4}, []int64{out.Illusts[0].ID, out.Illusts[1].ID})

	cursors = nil
	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "search", "miku", "--json", "--offset", "2"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, []sdk.Cursor{"", "next"}, cursors, "deprecated offset traverses opaque cursors internally")
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out))
	require.Equal(t, []int64{3, 4}, []int64{out.Illusts[0].ID, out.Illusts[1].ID})

	for _, args := range [][]string{
		{"pixiv", "search", "miku", "--page", "0", "--limit", "1"},
		{"pixiv", "search", "miku", "--page", "1"},
		{"pixiv", "search", "miku", "--page", "1", "--limit", "1", "--offset", "1"},
		{"pixiv", "search", "miku", "--limit", "-1"},
		{"pixiv", "search", "miku", "--cursor", "secret"},
	} {
		stdout.Reset()
		stderr.Reset()
		assert.NotZero(t, Run(args, strings.NewReader(""), &stdout, &stderr), args)
	}
}

func TestListJSONSpoolsUntilFullSuccessAndDetailKeepsPages(t *testing.T) {
	useTempPaths(t)
	searchCalls := 0
	setTestSDKCommandClient(t, sdkCommandFake{
		search: func(_ context.Context, request sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
			searchCalls++
			if request.Cursor == "" {
				return &sdk.IllustListResult{Illusts: []sdk.Illust{commandIllust(1)}, NextCursor: "next"}, nil
			}
			return nil, errors.New("upstream failed")
		},
		detail: func(_ context.Context, id int64) (*sdk.IllustDetail, error) {
			return &sdk.IllustDetail{Illust: sdk.Illust{ID: id, MetaPages: []sdk.MetaPage{{PageIndex: 0, Width: 100, Height: 200}, {PageIndex: 1, Width: 300, Height: 400}}}}, nil
		},
	})

	var stdout, stderr bytes.Buffer
	assert.NotZero(t, Run([]string{"pixiv", "search", "miku", "--json", "--limit", "0"}, strings.NewReader(""), &stdout, &stderr))
	assert.Empty(t, stdout.String())
	assert.Equal(t, 2, searchCalls)

	stdout.Reset()
	stderr.Reset()
	require.Equal(t, 0, Run([]string{"pixiv", "detail", "9", "--json"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	var detail sdk.IllustDetail
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &detail))
	assert.Len(t, detail.Illust.MetaPages, 2)
}

func TestRunContextCancelsSDKNetworkCommand(t *testing.T) {
	useTempPaths(t)
	started := make(chan struct{})
	setTestSDKCommandClient(t, sdkCommandFake{search: func(ctx context.Context, _ sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- RunContext(ctx, []string{"pixiv", "search", "miku"}, strings.NewReader(""), &stdout, &stderr)
	}()
	<-started
	cancel()
	assert.NotZero(t, <-done)
	assert.Contains(t, stderr.String(), "context canceled")
}

func TestUserArtworksWithoutIDUsesConcreteOAuthIdentity(t *testing.T) {
	authPath, configPath := useTempPaths(t)
	require.NoError(t, auth.SaveAuthStore(authPath, auth.AuthStore{DefaultUserID: 7, Accounts: []auth.Account{{UserID: 7, RefreshToken: "stored-token"}}}))
	t.Setenv("PIXIV_REFRESH_TOKEN", "environment-token")
	var requestedUserIDs []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/auth/token":
			require.NoError(t, request.ParseForm())
			assert.Equal(t, "environment-token", request.Form.Get("refresh_token"))
			_, _ = io.WriteString(w, `{"access_token":"access","refresh_token":"rotated","user":{"id":202}}`)
		case "/v1/user/illusts":
			requestedUserIDs = append(requestedUserIDs, request.URL.Query().Get("user_id"))
			_, _ = io.WriteString(w, `{"illusts":[]}`)
		default:
			t.Fatalf("unexpected path=%s", request.URL.Path)
		}
	}))
	defer server.Close()

	old := newCLIServices
	newCLIServices = func(logger *slog.Logger) application.Services {
		services := bootstrap.NewServices(logger)
		services.SDK.NewClient = func(request application.SDKClientRequest) (application.SDKClient, error) {
			return sdk.OpenDefault(sdk.Options{AuthFilePath: authPath, ConfigFilePath: configPath, HTTPClient: server.Client(), OAuthBaseURL: server.URL, AppAPIBaseURL: server.URL, UserID: request.UserID, RefreshToken: request.RefreshToken})
		}
		return services
	}
	t.Cleanup(func() { newCLIServices = old })

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, Run([]string{"pixiv", "user", "artworks", "--json"}, strings.NewReader(""), &stdout, &stderr), stderr.String())
	assert.Equal(t, []string{"202"}, requestedUserIDs)
}

var _ io.Writer = (*bytes.Buffer)(nil)
