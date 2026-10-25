package fanbox_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/stretchr/testify/require"
)

func TestSessionIdentity(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "www.fanbox.cc" || !strings.Contains(req.Header.Get("Cookie"), "FANBOXSESSID") {
			t.Errorf("request = %s cookie=%q", req.URL, req.Header.Get("Cookie"))
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(homeMetadataBody(42, "tester")))}, nil
	})
	client := testClient(t, rt)
	if err := client.ValidateSession(context.Background()); err != nil {
		t.Fatalf("ValidateSession: %v", err)
	}
	user, err := client.CurrentUser(context.Background(), fanbox.CurrentUserRequest{})
	if err != nil || user.UserID != 42 || user.DisplayName != "tester" {
		t.Fatalf("user = %+v err=%v", user, err)
	}

	expired := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{}, Body: http.NoBody}, nil
	}))
	if err := expired.ValidateSession(context.Background()); sdk.ReasonOf(err) != sdk.CredentialsExpired {
		t.Fatalf("expected CredentialsExpired, got %v", err)
	}
}

func TestOpenWithOptions(t *testing.T) {
	const wantUA = "fanbox-test-agent/1"
	client, err := fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: "session-value"}, fanbox.Options{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if got := req.Header.Get("User-Agent"); got != wantUA {
				t.Errorf("User-Agent = %q, want %q", got, wantUA)
			}
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(homeMetadataBody(42, "tester")))}, nil
		})},
		UserAgent: wantUA,
	})
	require.NoError(t, err)
	require.NoError(t, client.ValidateSession(context.Background()))

	called := false
	client, err = fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: "session-value"}, fanbox.Options{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			called = true
			return nil, errors.New("unexpected transport call")
		})},
		UserAgent: "invalid\nagent",
	})
	require.Error(t, err)
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	require.Nil(t, client)
	require.False(t, called)

	for _, test := range []struct {
		name    string
		options fanbox.Options
	}{
		{name: "service userinfo", options: fanbox.Options{FlareSolverr: &fanbox.FlareSolverrOptions{URL: "http://user:pass@solver.example"}}},
		{name: "service path prefix", options: fanbox.Options{FlareSolverr: &fanbox.FlareSolverrOptions{URL: "http://solver.example/prefix"}}},
		{name: "upstream userinfo", options: fanbox.Options{FlareSolverr: &fanbox.FlareSolverrOptions{URL: "http://solver.example", ProxyURL: "socks5://user:pass@proxy.example"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: "session-value"}, test.options)
			require.Error(t, err)
			require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
			require.NotContains(t, err.Error(), "user")
			require.NotContains(t, err.Error(), "pass")
		})
	}
}

func TestSessionCredentialsRedaction(t *testing.T) {
	credentials := fanbox.SessionCredentials{FANBOXSESSID: "session-canary"}
	for _, rendered := range []string{credentials.String(), credentials.GoString(), fmt.Sprintf("%v", credentials)} {
		if strings.Contains(rendered, "session-canary") {
			t.Fatalf("credentials leaked session value: %q", rendered)
		}
	}
}

func TestParallelClientsKeepSessionCookiesIsolated(t *testing.T) {
	type observation struct{ cookie string }
	makeClient := func(session, postID string, seen *observation) *fanbox.Client {
		client, err := fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: session}, fanbox.Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			seen.cookie = req.Header.Get("Cookie")
			return jsonResponse(`{"body":{"post":{"id":"` + postID + `","title":"p","publishedDatetime":"2024-01-01T00:00:00Z","isRestricted":false,"isPinned":false}}}`), nil
		})}})
		if err != nil {
			t.Fatal(err)
		}
		return client
	}
	firstObservation, secondObservation := &observation{}, &observation{}
	first := makeClient("first-session", "first-post", firstObservation)
	second := makeClient("second-session", "second-post", secondObservation)
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		if _, err := first.Post(context.Background(), fanbox.PostRequest{PostID: "first-post"}); err != nil {
			t.Errorf("first client error = %v", err)
		}
	}()
	go func() {
		defer group.Done()
		if _, err := second.Post(context.Background(), fanbox.PostRequest{PostID: "second-post"}); err != nil {
			t.Errorf("second client error = %v", err)
		}
	}()
	group.Wait()
	if firstObservation.cookie != "FANBOXSESSID=first-session" || secondObservation.cookie != "FANBOXSESSID=second-session" {
		t.Fatalf("cookies = %q / %q", firstObservation.cookie, secondObservation.cookie)
	}
}
