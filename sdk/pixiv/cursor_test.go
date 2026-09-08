package pixiv

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

type cursorT19RoundTripFunc func(*http.Request) (*http.Response, error)

func (f cursorT19RoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func newCursorT19Client(t *testing.T) (*Client, *int) {
	t.Helper()
	calls := 0
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: cursorT19RoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("invalid cursor reached transport")
	})}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	return client, &calls
}

func TestRemainingPixivOffsetCursorsRejectNonPositiveContinuation(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		op    string
		query url.Values
		call  func(*Client, sdk.Cursor) error
	}{
		{
			name:  "related artworks",
			op:    "RelatedArtworks",
			query: url.Values{"illust_id": {"1"}},
			call: func(client *Client, cursor sdk.Cursor) error {
				_, err := client.RelatedArtworks(ctx, RelatedArtworksRequest{ArtworkID: 1, Cursor: cursor})
				return err
			},
		},
		{
			name:  "artwork ranking",
			op:    "ArtworkRanking",
			query: url.Values{"mode": {string(RankingModeDay)}},
			call: func(client *Client, cursor sdk.Cursor) error {
				_, err := client.ArtworkRanking(ctx, ArtworkRankingRequest{Cursor: cursor})
				return err
			},
		},
		{
			name:  "following artworks",
			op:    "FollowingArtworks",
			query: url.Values{"restrict": {string(RestrictPublic)}},
			call: func(client *Client, cursor sdk.Cursor) error {
				_, err := client.FollowingArtworks(ctx, FollowingArtworksRequest{Cursor: cursor})
				return err
			},
		},
		{
			name:  "user artworks",
			op:    "UserArtworks",
			query: url.Values{"user_id": {"1"}, "type": {"illust"}},
			call: func(client *Client, cursor sdk.Cursor) error {
				_, err := client.UserArtworks(ctx, UserArtworksRequest{UserID: 1, Cursor: cursor})
				return err
			},
		},
		{
			name:  "artwork bookmark tags",
			op:    "UserArtworkBookmarkTags",
			query: url.Values{"user_id": {"1"}, "restrict": {""}},
			call: func(client *Client, cursor sdk.Cursor) error {
				_, err := client.UserArtworkBookmarkTags(ctx, UserArtworkBookmarkTagsRequest{UserID: 1, Cursor: cursor})
				return err
			},
		},
		{
			name:  "MyPixiv artworks",
			op:    "MyPixivArtworks",
			query: url.Values{},
			call: func(client *Client, cursor sdk.Cursor) error {
				_, err := client.MyPixivArtworks(ctx, MyPixivArtworksRequest{Cursor: cursor})
				return err
			},
		},
		{
			name:  "artwork comments",
			op:    "ArtworkComments",
			query: url.Values{"illust_id": {"1"}},
			call: func(client *Client, cursor sdk.Cursor) error {
				_, err := client.ArtworkComments(ctx, ArtworkCommentsRequest{ArtworkID: 1, Cursor: cursor})
				return err
			},
		},
		{
			name:  "novel comments",
			op:    "NovelComments",
			query: url.Values{"novel_id": {"1"}},
			call: func(client *Client, cursor sdk.Cursor) error {
				_, err := client.NovelComments(ctx, NovelCommentsRequest{NovelID: 1, Cursor: cursor})
				return err
			},
		},
		{
			name:  "search users",
			op:    "SearchUsers",
			query: url.Values{"word": {"artist"}},
			call: func(client *Client, cursor sdk.Cursor) error {
				_, err := client.SearchUsers(ctx, SearchUsersRequest{Word: "artist", Cursor: cursor})
				return err
			},
		},
		{
			name:  "related users",
			op:    "RelatedUsers",
			query: url.Values{"seed_user_id": {"1"}},
			call: func(client *Client, cursor sdk.Cursor) error {
				_, err := client.RelatedUsers(ctx, RelatedUsersRequest{UserID: 1, Cursor: cursor})
				return err
			},
		},
		{
			name:  "user following",
			op:    "UserFollowing",
			query: url.Values{"user_id": {"1"}, "restrict": {string(RestrictPublic)}},
			call: func(client *Client, cursor sdk.Cursor) error {
				_, err := client.UserFollowing(ctx, UserFollowingRequest{UserID: 1, Cursor: cursor})
				return err
			},
		},
		{
			name:  "user followers",
			op:    "UserFollowers",
			query: url.Values{"user_id": {"1"}, "restrict": {string(RestrictPublic)}},
			call: func(client *Client, cursor sdk.Cursor) error {
				_, err := client.UserFollowers(ctx, UserFollowersRequest{UserID: 1, Cursor: cursor})
				return err
			},
		},
		{
			name:  "blocked users",
			op:    "UserBlockedUsers",
			query: url.Values{"user_id": {"1"}},
			call: func(client *Client, cursor sdk.Cursor) error {
				_, err := client.UserBlockedUsers(ctx, UserBlockedUsersRequest{UserID: 1, Cursor: cursor})
				return err
			},
		},
		{
			name:  "MyPixiv users",
			op:    "MyPixivUsers",
			query: url.Values{},
			call: func(client *Client, cursor sdk.Cursor) error {
				client.userID = 1
				_, err := client.MyPixivUsers(ctx, MyPixivUsersRequest{Cursor: cursor})
				return err
			},
		},
		{
			name:  "following novels",
			op:    "FollowingNovels",
			query: url.Values{"restrict": {string(RestrictPublic)}},
			call: func(client *Client, cursor sdk.Cursor) error {
				_, err := client.FollowingNovels(ctx, FollowingNovelsRequest{Cursor: cursor})
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, calls := newCursorT19Client(t)
			cursor, err := client.buildContinuationCursor(test.op, test.query, continuationEnvelope{Key: "offset", Value: 0})
			if err != nil {
				t.Fatalf("buildContinuationCursor: %v", err)
			}
			if reason := sdk.ReasonOf(test.call(client, cursor)); reason != sdk.InvalidCursor {
				t.Fatalf("ReasonOf = %q, want %q", reason, sdk.InvalidCursor)
			}
			if *calls != 0 {
				t.Fatalf("invalid continuation reached transport %d time(s)", *calls)
			}
		})
	}
}

func TestRemainingPixivValueCursorsRejectNonPositiveContinuation(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		op    string
		query url.Values
		key   string
		call  func(*Client, sdk.Cursor) error
	}{
		{
			name:  "artwork series",
			op:    "ArtworkSeries",
			query: url.Values{"illust_series_id": {"1"}},
			key:   "last_order",
			call: func(client *Client, cursor sdk.Cursor) error {
				_, err := client.ArtworkSeries(ctx, ArtworkSeriesRequest{SeriesID: 1, Cursor: cursor})
				return err
			},
		},
		{
			name:  "artwork bookmarks",
			op:    "UserArtworkBookmarks",
			query: url.Values{"user_id": {"1"}, "restrict": {string(RestrictPrivate)}},
			key:   "max_bookmark_id",
			call: func(client *Client, cursor sdk.Cursor) error {
				_, err := client.UserArtworkBookmarks(ctx, UserArtworkBookmarksRequest{UserID: 1, Restrict: RestrictPrivate, Cursor: cursor})
				return err
			},
		},
		{
			name:  "novel bookmarks",
			op:    "UserNovelBookmarks",
			query: url.Values{"user_id": {"1"}, "restrict": {string(RestrictPrivate)}},
			key:   "max_bookmark_id",
			call: func(client *Client, cursor sdk.Cursor) error {
				_, err := client.UserNovelBookmarks(ctx, UserNovelBookmarksRequest{UserID: 1, Restrict: RestrictPrivate, Cursor: cursor})
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, calls := newCursorT19Client(t)
			cursor, err := client.buildContinuationCursor(test.op, test.query, continuationEnvelope{Key: test.key, Value: 0})
			if err != nil {
				t.Fatalf("buildContinuationCursor: %v", err)
			}
			if reason := sdk.ReasonOf(test.call(client, cursor)); reason != sdk.InvalidCursor {
				t.Fatalf("ReasonOf = %q, want %q", reason, sdk.InvalidCursor)
			}
			if *calls != 0 {
				t.Fatalf("invalid continuation reached transport %d time(s)", *calls)
			}
		})
	}
}

func TestLatestArtworksRejectsNonPositiveContinuationValues(t *testing.T) {
	ctx := context.Background()
	for _, key := range []string{"offset", "max_illust_id"} {
		t.Run(key, func(t *testing.T) {
			client, calls := newCursorT19Client(t)
			cursor, err := client.buildContinuationCursor("LatestArtworks", url.Values{"content_type": {"illust"}}, continuationEnvelope{Key: key, Value: 0})
			if err != nil {
				t.Fatalf("buildContinuationCursor: %v", err)
			}
			_, err = client.LatestArtworks(ctx, LatestArtworksRequest{Cursor: cursor})
			if reason := sdk.ReasonOf(err); reason != sdk.InvalidCursor {
				t.Fatalf("ReasonOf = %q, want %q", reason, sdk.InvalidCursor)
			}
			if *calls != 0 {
				t.Fatalf("invalid continuation reached transport %d time(s)", *calls)
			}
		})
	}
}

func TestRemainingIdentityScopedPixivCursorsBindClientInstance(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		op    string
		query url.Values
		call  func(*Client, sdk.Cursor) error
	}{
		{
			name:  "related users",
			op:    "RelatedUsers",
			query: url.Values{"seed_user_id": {"1"}},
			call: func(client *Client, cursor sdk.Cursor) error {
				_, err := client.RelatedUsers(ctx, RelatedUsersRequest{UserID: 1, Cursor: cursor})
				return err
			},
		},
		{
			name:  "user following",
			op:    "UserFollowing",
			query: url.Values{"user_id": {"1"}, "restrict": {string(RestrictPublic)}},
			call: func(client *Client, cursor sdk.Cursor) error {
				_, err := client.UserFollowing(ctx, UserFollowingRequest{UserID: 1, Cursor: cursor})
				return err
			},
		},
		{
			name:  "user followers",
			op:    "UserFollowers",
			query: url.Values{"user_id": {"1"}, "restrict": {string(RestrictPublic)}},
			call: func(client *Client, cursor sdk.Cursor) error {
				_, err := client.UserFollowers(ctx, UserFollowersRequest{UserID: 1, Cursor: cursor})
				return err
			},
		},
		{
			name:  "blocked users",
			op:    "UserBlockedUsers",
			query: url.Values{"user_id": {"1"}},
			call: func(client *Client, cursor sdk.Cursor) error {
				_, err := client.UserBlockedUsers(ctx, UserBlockedUsersRequest{UserID: 1, Cursor: cursor})
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			first, _ := newCursorT19Client(t)
			second, calls := newCursorT19Client(t)
			cursor, err := first.buildContinuationCursor(test.op, test.query, continuationEnvelope{Key: "offset", Value: 10})
			if err != nil {
				t.Fatalf("buildContinuationCursor: %v", err)
			}
			if reason := sdk.ReasonOf(test.call(second, cursor)); reason != sdk.InvalidCursor {
				t.Fatalf("ReasonOf = %q, want %q", reason, sdk.InvalidCursor)
			}
			if *calls != 0 {
				t.Fatalf("cross-client cursor reached transport %d time(s)", *calls)
			}
		})
	}
}
