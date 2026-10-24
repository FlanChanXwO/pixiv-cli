package fanboxapi_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/fanboxapi"
)

func TestClientCachesLocatorsAndVerifiedIdentity(t *testing.T) {
	identityCalls := 0
	session, err := fanboxapi.NewSessionWithOptions("FANBOXSESSID=client-canary", fanboxapi.SessionOptions{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			identityCalls++
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(homeMetadata))}, nil
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	client := fanboxapi.NewClient(session)
	client.CacheLocator("ref-1", fanboxapi.Locator{URL: "https://downloads.fanbox.cc/a.png", Filename: "a.png"})
	if locator, ok := client.CachedLocator("ref-1"); !ok || locator.Filename != "a.png" {
		t.Fatalf("cached locator = %+v ok=%v", locator, ok)
	}
	for range 2 {
		userID, err := client.VerifiedUserID(context.Background())
		if err != nil || userID != 800 {
			t.Fatalf("VerifiedUserID() = %d, %v", userID, err)
		}
	}
	if identityCalls != 1 {
		t.Fatalf("identity calls = %d, want 1", identityCalls)
	}
}

func TestClientRejectsContinuationOutsideFanboxHosts(t *testing.T) {
	session, err := fanboxapi.NewSessionWithOptions("FANBOXSESSID=client-canary", fanboxapi.SessionOptions{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("transport was called for a rejected continuation")
			return nil, errors.New("unexpected")
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := fanboxapi.NewClient(session).GetJSONURL(context.Background(), "https://example.invalid/post.listHome", nil); err == nil {
		t.Fatal("GetJSONURL accepted a foreign host")
	}
}

const homeMetadata = `<meta name="metadata" content='{"context":{"user":{"userId":"800","name":"verified"}}}'>`
