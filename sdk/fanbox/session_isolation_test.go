package fanbox_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
)

func TestParallelClientsKeepSessionCookiesAndTransportStateIsolated(t *testing.T) {
	type observation struct {
		mu     sync.Mutex
		cookie string
	}
	makeClient := func(session, postID string, observed *observation) *fanbox.Client {
		rt := roundTripFunc(func(request *http.Request) (*http.Response, error) {
			observed.mu.Lock()
			observed.cookie = request.Header.Get("Cookie")
			observed.mu.Unlock()
			body := `{"body":{"post":{"id":"` + postID + `","title":"title","publishedDatetime":"2024-01-01T00:00:00Z","isRestricted":false,"isPinned":false}}}`
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		client, err := fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: session}, fanbox.Options{HTTPClient: &http.Client{Transport: rt}})
		if err != nil {
			t.Fatal(err)
		}
		return client
	}
	firstObservation := &observation{}
	secondObservation := &observation{}
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

	if firstObservation.cookie != "FANBOXSESSID=first-session" {
		t.Fatalf("first cookie = %q", firstObservation.cookie)
	}
	if secondObservation.cookie != "FANBOXSESSID=second-session" {
		t.Fatalf("second cookie = %q", secondObservation.cookie)
	}
}
