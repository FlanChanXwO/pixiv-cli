package pixiv

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// cursorRoundTripFunc 让同包测试在无效 continuation 到达传输前失败。
type cursorRoundTripFunc func(*http.Request) (*http.Response, error)

func (f cursorRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func newCursorClient(t *testing.T) (*Client, *int) {
	t.Helper()
	calls := 0
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: cursorRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("invalid cursor reached transport")
	})}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	return client, &calls
}

func TestPixivCursorsRejectInvalidContinuationBeforeNetwork(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name   string
		op     string
		query  url.Values
		env    continuationEnvelope
		invoke func(*Client, sdk.Cursor) error
	}{
		{name: "related artworks", op: "RelatedArtworks", query: url.Values{"illust_id": {"1"}}, env: continuationEnvelope{Key: "offset", Value: 0}, invoke: func(c *Client, cursor sdk.Cursor) error {
			_, err := c.RelatedArtworks(ctx, RelatedArtworksRequest{ArtworkID: 1, Cursor: cursor})
			return err
		}},
		{name: "artwork ranking", op: "ArtworkRanking", query: url.Values{"mode": {"day"}}, env: continuationEnvelope{Key: "offset", Value: 0}, invoke: func(c *Client, cursor sdk.Cursor) error {
			_, err := c.ArtworkRanking(ctx, ArtworkRankingRequest{Cursor: cursor})
			return err
		}},
		{name: "artwork comments", op: "ArtworkComments", query: url.Values{"illust_id": {"1"}}, env: continuationEnvelope{Key: "offset", Value: 0}, invoke: func(c *Client, cursor sdk.Cursor) error {
			_, err := c.ArtworkComments(ctx, ArtworkCommentsRequest{ArtworkID: 1, Cursor: cursor})
			return err
		}},
		{name: "related users", op: "RelatedUsers", query: url.Values{"seed_user_id": {"1"}}, env: continuationEnvelope{Key: "offset", Value: 0}, invoke: func(c *Client, cursor sdk.Cursor) error {
			_, err := c.RelatedUsers(ctx, RelatedUsersRequest{UserID: 1, Cursor: cursor})
			return err
		}},
		{name: "latest max illust id", op: "LatestArtworks", query: url.Values{"content_type": {"illust"}}, env: continuationEnvelope{Key: "max_illust_id", Value: 0}, invoke: func(c *Client, cursor sdk.Cursor) error {
			_, err := c.LatestArtworks(ctx, LatestArtworksRequest{Cursor: cursor})
			return err
		}},
		{name: "recommended unknown param", op: "RecommendedArtworks", env: continuationEnvelope{Params: url.Values{"unexpected": {"1"}}}, invoke: func(c *Client, cursor sdk.Cursor) error {
			_, err := c.RecommendedArtworks(ctx, RecommendedArtworksRequest{Cursor: cursor})
			return err
		}},
		{name: "related gapped arrays", op: "RelatedArtworks", query: url.Values{"illust_id": {"123"}}, env: continuationEnvelope{Params: url.Values{
			"illust_id": {"123"}, "seed_illust_ids[0]": {"456"}, "viewed[1]": {"123"},
		}}, invoke: func(c *Client, cursor sdk.Cursor) error {
			_, err := c.RelatedArtworks(ctx, RelatedArtworksRequest{ArtworkID: 123, Cursor: cursor})
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, calls := newCursorClient(t)
			cursor, err := client.buildContinuationCursor(test.op, test.query, test.env)
			if err != nil {
				t.Fatalf("buildContinuationCursor: %v", err)
			}
			if reason := sdk.ReasonOf(test.invoke(client, cursor)); reason != sdk.InvalidCursor {
				t.Fatalf("ReasonOf = %q, want %q", reason, sdk.InvalidCursor)
			}
			if *calls != 0 {
				t.Fatalf("invalid continuation reached transport %d time(s)", *calls)
			}
		})
	}
}

func TestIdentityScopedPixivCursorsBindClient(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		op    string
		query url.Values
		call  func(*Client, sdk.Cursor) error
	}{
		{name: "related users", op: "RelatedUsers", query: url.Values{"seed_user_id": {"1"}}, call: func(c *Client, cursor sdk.Cursor) error {
			_, err := c.RelatedUsers(ctx, RelatedUsersRequest{UserID: 1, Cursor: cursor})
			return err
		}},
		{name: "user following", op: "UserFollowing", query: url.Values{"user_id": {"1"}, "restrict": {"public"}}, call: func(c *Client, cursor sdk.Cursor) error {
			_, err := c.UserFollowing(ctx, UserFollowingRequest{UserID: 1, Cursor: cursor})
			return err
		}},
		{name: "user followers", op: "UserFollowers", query: url.Values{"user_id": {"1"}, "restrict": {"public"}}, call: func(c *Client, cursor sdk.Cursor) error {
			_, err := c.UserFollowers(ctx, UserFollowersRequest{UserID: 1, Cursor: cursor})
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			first, _ := newCursorClient(t)
			second, calls := newCursorClient(t)
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
