package fanbox_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/stretchr/testify/require"
)

func TestIdentityScopedCursor(t *testing.T) {
	const nextURL = "https://api.fanbox.cc/post.listHome?maxPublishedDatetime=2024-01-01T00:00:00Z"
	mint := testClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "www.fanbox.cc" {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(homeMetadataBody(42, "account-a")))}, nil
		}
		return jsonResponse(`{"body":{"posts":[{"id":"home-1","title":"a","publishedDatetime":"2024-01-01T00:00:00Z","creatorId":"c","isRestricted":false,"isPinned":false}],"pageUrls":["` + nextURL + `"]}}`), nil
	}))
	page, err := mint.Home(context.Background(), fanbox.HomeRequest{})
	require.NoError(t, err)
	require.False(t, page.Next.IsZero())

	consumeFeedCalled := false
	consumer := testClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "www.fanbox.cc" {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(homeMetadataBody(99, "account-b")))}, nil
		}
		consumeFeedCalled = true
		return nil, errors.New("account B feed must not be fetched with account A's cursor")
	}))
	if _, err := consumer.Home(context.Background(), fanbox.HomeRequest{Cursor: page.Next}); sdk.ReasonOf(err) != sdk.InvalidCursor || consumeFeedCalled {
		t.Fatalf("cross-account continuation = %v (fetched=%t)", err, consumeFeedCalled)
	}
}

func TestIdentityScopedCursorContinuesSameAccount(t *testing.T) {
	const nextURL = "https://api.fanbox.cc/post.listHome?maxPublishedDatetime=2024-01-01T00:00:00Z"
	calls := 0
	client := testClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "www.fanbox.cc" {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(homeMetadataBody(42, "tester")))}, nil
		}
		calls++
		if calls == 1 {
			return jsonResponse(`{"body":{"posts":[{"id":"home-1","title":"a","publishedDatetime":"2024-01-01T00:00:00Z","creatorId":"c","isRestricted":false,"isPinned":false}],"pageUrls":["` + nextURL + `"]}}`), nil
		}
		if req.URL.String() != nextURL {
			t.Errorf("continuation request = %s, want %s", req.URL.String(), nextURL)
		}
		return jsonResponse(`{"body":{"posts":[{"id":"home-2","title":"b","publishedDatetime":"2024-01-01T00:00:00Z","creatorId":"c","isRestricted":false,"isPinned":false}]}}`), nil
	}))
	page, err := client.Home(context.Background(), fanbox.HomeRequest{})
	require.NoError(t, err)
	page2, err := client.Home(context.Background(), fanbox.HomeRequest{Cursor: page.Next})
	require.NoError(t, err)
	if len(page2.Items) != 1 || page2.Items[0].ID != "home-2" || calls != 2 {
		t.Fatalf("page 2 = %+v calls=%d", page2.Items, calls)
	}
}
