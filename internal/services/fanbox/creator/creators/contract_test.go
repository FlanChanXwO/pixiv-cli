package creators_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/creator/creators"
)

type jsonTransport struct {
	responses []string
	requests  []string
}

func (t *jsonTransport) GetJSON(_ context.Context, endpoint string, target any) error {
	t.requests = append(t.requests, endpoint)
	if len(t.responses) == 0 {
		return nil
	}
	body := t.responses[0]
	t.responses = t.responses[1:]
	return json.Unmarshal([]byte(body), target)
}

func TestProfileMapsFieldsAndRoute(t *testing.T) {
	transport := &jsonTransport{responses: []string{`{"body":{"creatorId":"creator value","user":{"name":"Creator Name","iconUrl":"https://i.pximg.net/icon.png"},"hasAdultContent":true,"isFollowing":true,"coverImageUrl":"https://i.pximg.net/cover.png","plan":{"fee":500,"hasSupportingPlan":true}}}`}}
	profile, err := creators.New(transport).Profile(context.Background(), creators.ProfileRequest{CreatorID: "creator value"})
	if err != nil {
		t.Fatal(err)
	}
	if profile.ID != "creator value" || profile.DisplayName != "Creator Name" || profile.PlanFee != 500 || !profile.HasSupportingPlan {
		t.Fatalf("profile = %+v", profile)
	}
	parsed, err := url.Parse(transport.requests[0])
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/creator.get" || parsed.Query().Get("creatorId") != "creator value" {
		t.Fatalf("profile endpoint = %q", transport.requests[0])
	}
}

func TestListUsesServerContinuationWithoutRebuildingIt(t *testing.T) {
	nextURL := "https://api.fanbox.cc/creator.paginate?cursor=server-value"
	transport := &jsonTransport{responses: []string{
		`{"body":{"plans":[{"creatorId":"supported"}],"pageUrls":["` + nextURL + `"]}}`,
		`{"body":{"plans":[{"creatorId":"second"}]}}`,
	}}
	client := creators.New(transport)
	page, err := client.List(context.Background(), creators.ListRequest{Kind: creators.Supporting})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "supported" || page.NextURL != nextURL {
		t.Fatalf("page = %+v", page)
	}
	page, err = client.List(context.Background(), creators.ListRequest{Kind: creators.Supporting, NextURL: page.NextURL})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "second" {
		t.Fatalf("continuation page = %+v", page)
	}
	if len(transport.requests) != 2 || transport.requests[1] != nextURL {
		t.Fatalf("requests = %v, want exact continuation", transport.requests)
	}
}
