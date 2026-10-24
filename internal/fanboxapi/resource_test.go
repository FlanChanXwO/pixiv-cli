package fanboxapi_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/fanboxapi"
)

func TestValidateURLRejectsUnsafeOrPathlessLocators(t *testing.T) {
	for _, rawURL := range []string{
		"http://downloads.fanbox.cc/file.zip",
		"https://user:pass@downloads.fanbox.cc/file.zip",
		"https://example.invalid/file.zip",
		"https://downloads.fanbox.cc/",
	} {
		if err := fanboxapi.ValidateURL(rawURL); err == nil {
			t.Fatalf("ValidateURL(%q) unexpectedly succeeded", rawURL)
		}
	}
}

// TestClientOpenMediaValidatesURLBeforeTransport pins the path requirement the
// product client adds on top of the session's media host policy.
func TestClientOpenMediaValidatesURLBeforeTransport(t *testing.T) {
	called := false
	session, err := fanboxapi.NewSessionWithOptions("FANBOXSESSID=media-canary", fanboxapi.SessionOptions{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			called = true
			return nil, errors.New("unexpected transport call")
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	client := fanboxapi.NewClient(session)
	for _, target := range []string{
		"https://downloads.fanbox.cc/",
		"http://downloads.fanbox.cc/file.zip",
		"https://example.invalid/file.zip",
	} {
		if _, err := client.OpenMedia(context.Background(), target, fanboxapi.MediaRequest{Method: http.MethodGet}); err == nil {
			t.Fatalf("OpenMedia(%q) unexpectedly succeeded", target)
		}
	}
	if called {
		t.Fatal("transport was called for a rejected URL")
	}
}
