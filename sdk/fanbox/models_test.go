package fanbox_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
)

func TestPostJSONIsExplicitAndOpaque(t *testing.T) {
	ref, err := sdk.NewResourceRef("fanbox", []byte("post:123:image:0"))
	if err != nil {
		t.Fatalf("NewResourceRef: %v", err)
	}
	expires := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	secret := sdk.Resource{
		Ref:                 ref,
		URL:                 "https://signed.example/secret?token=do-not-leak",
		RequestHeaders:      map[string]string{"Cookie": "secret-cookie"},
		ExpiresAt:           &expires,
		RequiresCredentials: true,
	}
	post := fanbox.Post{
		ID:          "123",
		Title:       "post",
		PublishedAt: expires,
		Cover:       fanbox.ImageResource{Resource: secret},
		Body: &fanbox.PostBody{
			Text: "body",
			Blocks: []fanbox.PostBlock{{
				Kind:  fanbox.PostBlockImage,
				Image: &fanbox.PostImageBlock{Resource: secret, Caption: "caption"},
			}},
			Assets: []fanbox.Asset{{ID: "asset", Kind: fanbox.AssetKindImage, Resource: secret, Thumbnail: fanbox.ImageResource{Resource: secret}}},
		},
	}
	raw, err := json.Marshal(post)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	encoded := string(raw)
	for _, forbidden := range []string{"signed.example", "do-not-leak", "secret-cookie", "Cookie", "expires_at", "request_headers", `"url"`} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("post JSON leaked %q: %s", forbidden, encoded)
		}
	}
	for _, required := range []string{`"published_at"`, `"is_restricted"`, `"requires_credentials":true`} {
		if !strings.Contains(encoded, required) {
			t.Fatalf("post JSON omitted %q: %s", required, encoded)
		}
	}
}

func TestCreatorJSONFlattensSummary(t *testing.T) {
	creator := fanbox.Creator{
		CreatorSummary:  fanbox.CreatorSummary{ID: "creator", Name: "name"},
		HasAdultContent: true,
		PlanFee:         500,
	}
	raw, err := json.Marshal(creator)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	for _, want := range []string{`"id":"creator"`, `"name":"name"`, `"has_adult_content":true`, `"plan_fee":500`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("creator JSON = %s, missing %s", raw, want)
		}
	}
}
