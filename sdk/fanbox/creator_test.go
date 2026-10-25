package fanbox_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
)

func TestCreatorsPagination(t *testing.T) {
	const nextURL = "https://api.fanbox.cc/plan.listSupporting?page=2"
	apiCalls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "www.fanbox.cc" {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(homeMetadataBody(42, "tester")))}, nil
		}
		if req.URL.Host != "api.fanbox.cc" || req.URL.Path != "/plan.listSupporting" {
			t.Errorf("request = %s", req.URL.String())
		}
		apiCalls++
		if apiCalls == 1 {
			return jsonResponse(`{"body":{"plans":[{"creatorId":"supported-1"}],"pageUrls":["` + nextURL + `"]}}`), nil
		}
		if req.URL.String() != nextURL {
			t.Errorf("continuation request = %s, want %s", req.URL.String(), nextURL)
		}
		return jsonResponse(`{"body":{"plans":[{"creatorId":"supported-2"}]}}`), nil
	})
	client := testClient(t, rt)
	page, err := client.Creators(context.Background(), fanbox.CreatorsRequest{Kind: fanbox.CreatorListSupporting})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != "supported-1" || page.Next.IsZero() {
		t.Fatalf("page 1 = %+v err=%v", page, err)
	}
	page2, err := client.Creators(context.Background(), fanbox.CreatorsRequest{Kind: fanbox.CreatorListSupporting, Cursor: page.Next})
	if err != nil || len(page2.Items) != 1 || page2.Items[0].ID != "supported-2" || apiCalls != 2 {
		t.Fatalf("page 2 = %+v apiCalls=%d err=%v", page2, apiCalls, err)
	}
}
