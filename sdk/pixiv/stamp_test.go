package pixiv_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	. "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestStampsMapImageResources(t *testing.T) {
	const stampURL = "https://s.pximg.net/common/images/stamp/generated-stamps/1.png"
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "app-api.pixiv.net" || req.URL.Path != "/v1/stamps" || req.URL.RawQuery != "" {
			t.Fatalf("unexpected stamps request: %s", req.URL)
		}
		return jsonResponse(`{"stamps":[{"stamp_id":1,"stamp_url":"` + stampURL + `"}]}`), nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	stamps, err := client.Stamps(context.Background(), StampsRequest{})
	if err != nil {
		t.Fatalf("Stamps: %v", err)
	}
	if len(stamps) != 1 || stamps[0].ID != 1 || stamps[0].Image.Resource.URL != stampURL || stamps[0].Image.Resource.Ref.IsZero() {
		t.Fatalf("stamps = %#v", stamps)
	}
	payload, err := sdk.ResourceRefPayload(stamps[0].Image.Resource.Ref)
	if err != nil {
		t.Fatalf("ResourceRefPayload: %v", err)
	}
	if !strings.Contains(string(payload), `"k":"stamp"`) || !strings.Contains(string(payload), `"id":1`) {
		t.Fatalf("stamp resource payload = %s", payload)
	}
}

func TestStampsRejectMalformedResponse(t *testing.T) {
	for name, body := range map[string]string{
		"missing list":      `{}`,
		"null list":         `{"stamps":null}`,
		"zero id":           `{"stamps":[{"stamp_id":0,"stamp_url":"https://s.pximg.net/stamp.png"}]}`,
		"negative id":       `{"stamps":[{"stamp_id":-1,"stamp_url":"https://s.pximg.net/stamp.png"}]}`,
		"missing url":       `{"stamps":[{"stamp_id":1}]}`,
		"plain http":        `{"stamps":[{"stamp_id":1,"stamp_url":"http://s.pximg.net/stamp.png"}]}`,
		"root path":         `{"stamps":[{"stamp_id":1,"stamp_url":"https://s.pximg.net"}]}`,
		"userinfo":          `{"stamps":[{"stamp_id":1,"stamp_url":"https://user:pass@s.pximg.net/stamp.png"}]}`,
		"untrusted host":    `{"stamps":[{"stamp_id":1,"stamp_url":"https://example.com/stamp.png"}]}`,
		"non-null next_url": `{"stamps":[],"next_url":"https://app-api.pixiv.net/v1/stamps?offset=40"}`,
	} {
		t.Run(name, func(t *testing.T) {
			rt := roundTripFunc(func(*http.Request) (*http.Response, error) { return jsonResponse(body), nil })
			client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
			if err != nil {
				t.Fatalf("NewWith: %v", err)
			}
			if _, err := client.Stamps(context.Background(), StampsRequest{}); sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
				t.Fatalf("Stamps reason = %q, err = %v", sdk.ReasonOf(err), err)
			}
		})
	}

	client, _ := newWireClient(t, `{"stamps":[]}`)
	stamps, err := client.Stamps(context.Background(), StampsRequest{})
	if err != nil || stamps == nil || len(stamps) != 0 {
		t.Fatalf("empty stamps = %#v err=%v, want non-nil empty slice", stamps, err)
	}
}

func TestStampResourceReferenceCanBeReopenedWithoutEmbeddedURL(t *testing.T) {
	const stampURL = "https://s.pximg.net/common/images/stamp/generated-stamps/1.png?signature=sentinel"
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Host {
		case "app-api.pixiv.net":
			if req.URL.Path != "/v1/stamps" || req.URL.RawQuery != "" {
				return nil, http.ErrNotSupported
			}
			return jsonResponse(`{"stamps":[{"stamp_id":1,"stamp_url":"` + stampURL + `"}]}`), nil
		case "s.pximg.net":
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"image/png"}}, Body: http.NoBody}, nil
		default:
			return nil, http.ErrNotSupported
		}
	})
	httpClient := &http.Client{Transport: rt}
	first, err := NewWith("token", Options{HTTPClient: httpClient})
	if err != nil {
		t.Fatalf("NewWith first: %v", err)
	}
	stamps, err := first.Stamps(context.Background(), StampsRequest{})
	if err != nil {
		t.Fatalf("Stamps: %v", err)
	}
	second, err := NewWith("token", Options{HTTPClient: httpClient})
	if err != nil {
		t.Fatalf("NewWith second: %v", err)
	}
	response, err := second.OpenResource(context.Background(), sdk.OpenResourceRequest{Ref: stamps[0].Image.Resource.Ref})
	if err != nil {
		t.Fatalf("OpenResource: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
}

func TestStampJSONIsOutputSafe(t *testing.T) {
	ref, err := sdk.NewResourceRef("pixiv", []byte(`{"k":"stamp","id":1}`))
	if err != nil {
		t.Fatalf("NewResourceRef: %v", err)
	}
	stamp := Stamp{
		ID: 1,
		Image: ImageResource{Resource: sdk.Resource{
			Ref:            ref,
			URL:            "https://s.pximg.net/stamp.png?token=do-not-leak",
			RequestHeaders: map[string]string{"Cookie": "secret-cookie"},
		}},
	}
	data, err := json.Marshal(stamp)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	for _, forbidden := range []string{"s.pximg.net", "do-not-leak", "Cookie", "secret-cookie", "request_headers"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("stamp JSON leaked %q: %s", forbidden, data)
		}
	}
}
