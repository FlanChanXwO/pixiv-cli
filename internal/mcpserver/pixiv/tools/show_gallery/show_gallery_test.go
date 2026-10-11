package show_gallery

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestGalleryQueriesConcurrentlyAndPreservesOrder(t *testing.T) {
	entered := make(chan struct{}, 10)
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	client, err := pixiv.NewWith("fixture", pixiv.Options{HTTPClient: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		id := r.URL.Query().Get("illust_id")
		if id == "99" {
			return &http.Response{StatusCode: 404, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"illust":{"id":%s,"title":"art","type":"illust","create_date":"2026-01-01T00:00:00Z","user":{"id":7}}}`, id)))}, nil
	})}})
	require.NoError(t, err)
	app := runtime.NewApp(runtime.SDKPorts{Execute: func(ctx context.Context, _ runtime.Account, fn func(context.Context, *pixiv.Client) (bool, error)) error {
		_, e := fn(ctx, client)
		return e
	}}, runtime.Account{})
	result := make(chan output, 1)
	go func() {
		_, out, _ := handle(t.Context(), app, input{IllustIDs: []int64{43, 99, 42, 43}})
		result <- out
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			close(release)
			<-result
			t.Fatal("metadata queries were serialized")
		}
	}
	close(release)
	out := <-result
	require.Len(t, out.Records, 2)
	require.Equal(t, "43", out.Records[0].ID())
	require.Equal(t, "42", out.Records[1].ID())
	require.Len(t, out.Failures, 3)
	require.EqualValues(t, 99, out.Failures[1].IllustID)
	require.Len(t, entered, 1, "duplicate artwork queried only once")
}
