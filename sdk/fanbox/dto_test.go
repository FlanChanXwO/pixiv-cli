package fanbox_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
)

func TestPostDTOIsExplicitAndOpaque(t *testing.T) {
	ref, err := sdk.NewResourceRef("fanbox", []byte("post:123:image:0"))
	if err != nil {
		t.Fatalf("NewResourceRef: %v", err)
	}
	expires := time.Now().Add(time.Hour)
	post := fanbox.Post{
		ID:          "123",
		Title:       "post",
		PublishedAt: time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC),
		Cover: fanbox.ImageResource{Resource: sdk.Resource{
			Ref:                 ref,
			URL:                 "https://signed.example/secret?token=do-not-leak",
			RequestHeaders:      map[string]string{"Cookie": "secret-cookie"},
			ExpiresAt:           &expires,
			RequiresCredentials: true,
		}},
		Body: &fanbox.PostBody{
			Text: "body",
			Blocks: []fanbox.PostBlock{{
				Kind:  fanbox.PostBlockImage,
				Image: &fanbox.PostImageBlock{Resource: sdk.Resource{Ref: ref}, Caption: "caption"},
			}},
			Assets: []fanbox.Asset{{ID: "asset", Kind: fanbox.AssetKindImage, Resource: sdk.Resource{Ref: ref}}},
		},
	}

	dto := fanbox.ToPostDTO(post)
	if dto.ID != post.ID || dto.Body == nil || len(dto.Body.Blocks) != 1 {
		t.Fatalf("post DTO lost fields: %+v", dto)
	}
	data, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	encoded := string(data)
	for _, forbidden := range []string{"signed.example", "do-not-leak", "Cookie", "secret-cookie", "expires_at", "request_headers"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("post DTO leaked %q: %s", forbidden, encoded)
		}
	}
	for _, required := range []string{`"published_at"`, `"is_restricted"`, `"requires_credentials"`} {
		if !strings.Contains(encoded, required) {
			t.Fatalf("post DTO omitted %q: %s", required, encoded)
		}
	}

	dto.Body.Blocks[0].Image.Caption = "changed"
	if post.Body.Blocks[0].Image.Caption != "caption" {
		t.Fatal("DTO shares the source block")
	}
}

func TestCreatorDTOFlattensSummary(t *testing.T) {
	creator := fanbox.Creator{
		CreatorSummary:  fanbox.CreatorSummary{ID: "creator", Name: "name"},
		HasAdultContent: true,
		PlanFee:         500,
	}
	dto := fanbox.ToCreatorDTO(creator)
	if dto.ID != creator.ID || dto.Name != creator.Name || dto.PlanFee != creator.PlanFee || !dto.HasAdultContent {
		t.Fatalf("creator DTO lost embedded summary fields: %+v", dto)
	}
}
