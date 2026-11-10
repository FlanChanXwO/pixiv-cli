package fanbox_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/stretchr/testify/require"
)

func TestCreatorProfile(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/creator.get" || r.URL.Query().Get("creatorId") != "artist" || r.Header.Get("Cookie") != "FANBOXSESSID=session-fixture" {
			t.Error("incorrect profile request")
		}
		fmt.Fprint(w, `{"body":{"user":{"name":"fixture","iconUrl":"https://pixiv.pximg.net/icon.png"},"hasAdultContent":true,"isFollowing":true,"coverImageUrl":"https://artist.fanbox.cc/cover.jpg","plan":{"fee":500,"hasSupportingPlan":true}}}`)
	}))
	defer server.Close()
	httpClient := server.Client()
	routeToServer(httpClient, server)
	client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
	require.NoError(t, err)
	ref := client.Creator(" artist ")
	creator, err := ref.Get(t.Context())
	require.NoError(t, err)
	require.NotNil(t, creator.CreatorRef)
	require.Equal(t, "artist", creator.ID)
	require.Equal(t, "fixture", creator.Name)
	require.True(t, creator.HasAdultContent && creator.IsFollowing && creator.HasSupportingPlan)
	require.Equal(t, 500, creator.PlanFee)
	require.Equal(t, "https://pixiv.pximg.net/icon.png", creator.Icon.URL)
	require.Equal(t, "https://artist.fanbox.cc/cover.jpg", creator.Cover.URL)
	data, err := json.Marshal(creator)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, creator))
	require.Equal(t, "fixture", creator.Name)
	_, err = creator.Get(t.Context())
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	require.NoError(t, json.Unmarshal([]byte(`{"id":"artist"}`), ref))
	_, err = ref.Tags(t.Context())
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
}

func TestCreatorProfileFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		reason     sdk.Reason
	}{
		{"missing body", `{}`, 200, sdk.MalformedUpstreamResponse},
		{"null body", `{"body":null}`, 200, sdk.MalformedUpstreamResponse},
		{"malformed", `{"body":`, 200, sdk.MalformedUpstreamResponse},
		{"missing user", `{"body":{"creatorId":"artist"}}`, 200, sdk.MalformedUpstreamResponse},
		{"missing name", `{"body":{"user":{"name":" "}}}`, 200, sdk.MalformedUpstreamResponse},
		{"forbidden image", `{"body":{"user":{"name":"fixture","iconUrl":"https://foreign.invalid/icon"}}}`, 200, sdk.ResourceForbidden},
		{"expired", `{}`, 401, sdk.CredentialsExpired},
		{"forbidden", `{}`, 403, sdk.Forbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			httpClient := server.Client()
			routeToServer(httpClient, server)
			client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
			require.NoError(t, err)
			_, err = client.Creator("artist").Get(t.Context())
			require.Equal(t, tc.reason, sdk.ReasonOf(err))
		})
	}
}

func TestCreatorTags(t *testing.T) {
	for _, body := range []string{`[{"tag":"art","url":"https://artist.fanbox.cc/tags/art"}]`, `{"tags":[{"tag":"art","url":"https://artist.fanbox.cc/tags/art"}]}`} {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/tag.getFeatured" || r.URL.Query().Get("creatorId") != "artist" {
				t.Error("incorrect tag request")
			}
			fmt.Fprintf(w, `{"body":%s}`, body)
		}))
		defer server.Close()
		httpClient := server.Client()
		routeToServer(httpClient, server)
		client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
		require.NoError(t, err)
		tags, err := client.Creator("artist").Tags(t.Context())
		require.NoError(t, err)
		require.Equal(t, []fanbox.CreatorTag{{Name: "art", URL: "https://artist.fanbox.cc/tags/art"}}, tags)
	}
}

func TestCreatorTagFailures(t *testing.T) {
	for _, body := range []string{`{}`, `{"body":null}`, `{"body":{}}`, `{"body":{"tags":null}}`, `{"body":{"tags":{}}}`, `{"body":[{}]}`, `{"body":[{"tag":" "}]}`, `{"body":`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			httpClient := server.Client()
			routeToServer(httpClient, server)
			client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
			require.NoError(t, err)
			_, err = client.Creator("artist").Tags(t.Context())
			require.Equal(t, sdk.MalformedUpstreamResponse, sdk.ReasonOf(err))
		})
	}
}

func TestDelayedChallengeReusesFreshClearance(t *testing.T) {
	var solves atomic.Int32
	solver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		solves.Add(1)
		fmt.Fprint(w, `{"status":"ok","solution":{"userAgent":"fixture","cookies":[{"name":"cf_clearance","value":"clearance-fixture"}]}}`)
	}))
	defer solver.Close()
	started, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Cookie"), "cf_clearance=") {
			fmt.Fprint(w, `{"body":{"user":{"name":"fixture"}}}`)
			return
		}
		if r.URL.Query().Get("creatorId") == "slow" {
			close(started)
			<-release
		}
		w.Header().Set("Cf-Mitigated", "challenge")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	httpClient := server.Client()
	routeToServer(httpClient, server)
	client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient, FlareSolverrURL: solver.URL})
	require.NoError(t, err)
	result := make(chan error, 1)
	go func() { _, err := client.Creator("slow").Get(t.Context()); result <- err }()
	<-started
	_, err = client.Creator("fast").Get(t.Context())
	close(release)
	require.NoError(t, err)
	require.NoError(t, <-result)
	require.EqualValues(t, 1, solves.Load())
	require.NoError(t, client.Close(t.Context()))
}
