package pixiv_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestFollowUserRejectsUnknownRestrictBeforeNetwork(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return nil, io.ErrUnexpectedEOF
	})
	client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	err = client.FollowUser(context.Background(), pixiv.FollowUserRequest{
		UserID:   7,
		Restrict: pixiv.Restrict("unknown"),
	})
	if sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("ReasonOf = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.InvalidArgument, err)
	}
	if calls != 0 {
		t.Fatalf("invalid restrict reached upstream %d time(s)", calls)
	}
}

func TestFollowUserDefaultsEmptyRestrictToPublic(t *testing.T) {
	var captured *http.Request
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		captured = req
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader("{}")),
		}, nil
	})
	client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	if err := client.FollowUser(context.Background(), pixiv.FollowUserRequest{UserID: 7}); err != nil {
		t.Fatalf("FollowUser: %v", err)
	}
	if captured == nil {
		t.Fatal("follow request was not captured")
	}
	if err := captured.ParseForm(); err != nil {
		t.Fatalf("ParseForm: %v", err)
	}
	if captured.URL.Path != "/v1/user/follow/add" || captured.PostForm.Get("restrict") != "public" {
		t.Fatalf("request = %s form=%v", captured.URL.Path, captured.PostForm)
	}
}
