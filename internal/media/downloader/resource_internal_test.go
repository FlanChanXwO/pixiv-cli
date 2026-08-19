package downloader

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// newTestImageResource 构造一个带 "pixiv" 产品 JSON-payload ref 的 ImageResource，
// 与真实 SDK 映射一致，使 ArtworkVariantResource 能解码并替换 variant。
func newTestImageResource(t *testing.T, id int64, page int) pixiv.ImageResource {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"k": "artwork", "id": id, "p": page, "v": "original"})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	ref, err := sdk.NewResourceRef("pixiv", payload)
	if err != nil {
		t.Fatalf("NewResourceRef: %v", err)
	}
	return pixiv.ImageResource{Resource: sdk.Resource{Ref: ref, URL: "https://i.pximg.net/img-original/img/42_p0.png"}, Variant: "original"}
}

// TestResourceForQualityOriginalReusesPageRef 验证 finding #8：original 质量直接
// 复用页面既有的 Resource.Ref，不构造变体。
func TestResourceForQualityOriginalReusesPageRef(t *testing.T) {
	image := newTestImageResource(t, 42, 0)
	for _, q := range []DownloadQuality{DownloadQualityOriginal, ""} {
		got, err := resourceForQuality(image, q)
		if err != nil {
			t.Fatalf("resourceForQuality(%q): %v", q, err)
		}
		if got != image.Resource.Ref {
			t.Fatalf("resourceForQuality(%q) = %v, want passthrough %v", q, got, image.Resource.Ref)
		}
	}
}

// TestResourceForQualityRequestsVariant 验证 finding #8：非 original 质量请求对应
// variant 的 ResourceRef，payload 编码 variant 字段。
func TestResourceForQualityRequestsVariant(t *testing.T) {
	image := newTestImageResource(t, 42, 0)
	cases := []struct {
		quality DownloadQuality
		variant string
	}{
		{DownloadQualityRegular, "regular"},
		{DownloadQualitySmall, "small"},
		{DownloadQualityThumb, "thumb"},
		{DownloadQualityMini, "mini"},
	}
	for _, tc := range cases {
		t.Run(string(tc.quality), func(t *testing.T) {
			got, err := resourceForQuality(image, tc.quality)
			if err != nil {
				t.Fatalf("resourceForQuality(%q): %v", tc.quality, err)
			}
			payload, err := sdk.ResourceRefPayload(got)
			if err != nil {
				t.Fatalf("ResourceRefPayload: %v", err)
			}
			if !strings.Contains(string(payload), `"v":"`+tc.variant+`"`) {
				t.Fatalf("quality %q payload = %q, want variant %q", tc.quality, payload, tc.variant)
			}
		})
	}
}

// TestResourceForQualityVariantPreservesIdentity 验证变体 ref 保留稳定身份
// (kind、id、page) 并只替换 variant。
func TestResourceForQualityVariantPreservesIdentity(t *testing.T) {
	image := newTestImageResource(t, 42, 2)
	got, err := resourceForQuality(image, DownloadQualityRegular)
	if err != nil {
		t.Fatalf("resourceForQuality: %v", err)
	}
	payload, err := sdk.ResourceRefPayload(got)
	if err != nil {
		t.Fatalf("ResourceRefPayload: %v", err)
	}
	var rp struct {
		Kind string `json:"k"`
		ID   int64  `json:"id"`
		Page int    `json:"p"`
		V    string `json:"v"`
	}
	if err := json.Unmarshal(payload, &rp); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if rp.Kind != "artwork" || rp.ID != 42 || rp.Page != 2 || rp.V != "regular" {
		t.Fatalf("variant ref payload = %+v, want artwork/42/2/regular", rp)
	}
}
