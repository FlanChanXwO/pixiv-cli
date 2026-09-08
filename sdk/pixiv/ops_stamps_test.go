package pixiv_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	. "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestStampsMapsImageResourceAndUsesNoQuery(t *testing.T) {
	const stampURL = "https://s.pximg.net/common/images/stamp/generated-stamps/1.png"
	var calls int
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Host != "app-api.pixiv.net" || req.URL.Path != "/v1/stamps" || req.URL.RawQuery != "" {
			return nil, errors.New("unexpected stamps request: " + req.URL.String())
		}
		return jsonResponse(`{"stamps":[{"stamp_id":1,"stamp_url":"` + stampURL + `"}]}`), nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	result, err := client.Stamps(context.Background(), StampsRequest{})
	if err != nil {
		t.Fatalf("Stamps: %v", err)
	}
	if calls != 1 || len(result) != 1 {
		t.Fatalf("calls/result = %d/%#v", calls, result)
	}
	if result[0].ID != 1 || result[0].Image.Resource.URL != stampURL {
		t.Fatalf("stamp = %#v", result[0])
	}
	if result[0].Image.Resource.Ref.IsZero() {
		t.Fatal("stamp image is missing an opaque resource reference")
	}
	payload, err := sdk.ResourceRefPayload(result[0].Image.Resource.Ref)
	if err != nil {
		t.Fatalf("ResourceRefPayload: %v", err)
	}
	if !strings.Contains(string(payload), `"k":"stamp"`) || !strings.Contains(string(payload), `"id":1`) {
		t.Fatalf("stamp resource payload = %s", payload)
	}
}

func TestStampCommentMutationsKeepStampSeparateFromTextAndReply(t *testing.T) {
	var requests []string
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "app-api.pixiv.net" {
			return nil, errors.New("unexpected host: " + req.URL.Host)
		}
		if req.URL.Path != "/v1/illust/comment/add" && req.URL.Path != "/v1/novel/comment/add" {
			return nil, errors.New("unexpected stamp path: " + req.URL.Path)
		}
		if err := req.ParseForm(); err != nil {
			return nil, err
		}
		requests = append(requests, req.URL.Path+" "+req.PostForm.Encode())
		if len(req.PostForm) != 3 || req.PostForm.Get("comment") != "stamp" || req.PostForm.Get("stamp_id") != "9" || req.PostForm.Get("parent_comment_id") != "" {
			return nil, errors.New("stamp form is not independent: " + req.PostForm.Encode())
		}
		return jsonResponse(`{"comment_id":901}`), nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	artworkResult, err := client.StampArtworkComment(context.Background(), StampArtworkCommentRequest{
		ArtworkID: 123,
		Comment:   "stamp",
		StampID:   9,
	})
	if err != nil {
		t.Fatalf("StampArtworkComment: %v", err)
	}
	novelResult, err := client.StampNovelComment(context.Background(), StampNovelCommentRequest{
		NovelID: 456,
		Comment: "stamp",
		StampID: 9,
	})
	if err != nil {
		t.Fatalf("StampNovelComment: %v", err)
	}
	if artworkResult.CommentID != 901 || novelResult.CommentID != 901 {
		t.Fatalf("mutation results = %#v/%#v", artworkResult, novelResult)
	}
	if len(requests) != 2 || !strings.HasPrefix(requests[0], "/v1/illust/comment/add ") || !strings.HasPrefix(requests[1], "/v1/novel/comment/add ") {
		t.Fatalf("requests = %#v", requests)
	}
}

func TestStampCommentMutationsRejectInvalidInputsBeforeNetwork(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("invalid stamp input reached transport")
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	artworkCases := []StampArtworkCommentRequest{
		{ArtworkID: 0, Comment: "body", StampID: 9},
		{ArtworkID: 1, Comment: "", StampID: 9},
		{ArtworkID: 1, Comment: "body", StampID: 0},
	}
	for _, request := range artworkCases {
		_, err := client.StampArtworkComment(context.Background(), request)
		if sdk.ReasonOf(err) != sdk.InvalidArgument {
			t.Fatalf("StampArtworkComment(%#v) reason = %q, err = %v", request, sdk.ReasonOf(err), err)
		}
	}

	novelCases := []StampNovelCommentRequest{
		{NovelID: 0, Comment: "body", StampID: 9},
		{NovelID: 1, Comment: "", StampID: 9},
		{NovelID: 1, Comment: "body", StampID: 0},
	}
	for _, request := range novelCases {
		_, err := client.StampNovelComment(context.Background(), request)
		if sdk.ReasonOf(err) != sdk.InvalidArgument {
			t.Fatalf("StampNovelComment(%#v) reason = %q, err = %v", request, sdk.ReasonOf(err), err)
		}
	}
	if calls != 0 {
		t.Fatalf("invalid stamp inputs reached transport %d time(s)", calls)
	}
}

func TestStampsRejectMalformedResponse(t *testing.T) {
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(`{"stamps":[{"stamp_id":1,"stamp_url":"https://example.com/stamp.png"}]}`), nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	_, err = client.Stamps(context.Background(), StampsRequest{})
	if sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
		t.Fatalf("Stamps reason = %q, err = %v", sdk.ReasonOf(err), err)
	}
}

func TestStampResourceReferenceCanBeReopenedWithoutEmbeddedURL(t *testing.T) {
	const stampURL = "https://s.pximg.net/common/images/stamp/generated-stamps/1.png?signature=sentinel"
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Host {
		case "app-api.pixiv.net":
			if req.URL.Path != "/v1/stamps" || req.URL.RawQuery != "" {
				return nil, errors.New("unexpected stamps resolver request: " + req.URL.String())
			}
			return jsonResponse(`{"stamps":[{"stamp_id":1,"stamp_url":"` + stampURL + `"}]}`), nil
		case "s.pximg.net":
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"image/png"}},
				Body:       io.NopCloser(strings.NewReader("DATA")),
			}, nil
		default:
			return nil, errors.New("unexpected stamp resource host: " + req.URL.Host)
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
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "DATA" {
		t.Fatalf("body = %q err=%v", body, err)
	}
}

func TestStampDTOIsOutputSafe(t *testing.T) {
	ref, err := sdk.NewResourceRef("pixiv", []byte(`{"k":"stamp","id":1}`))
	if err != nil {
		t.Fatalf("NewResourceRef: %v", err)
	}
	dto := ToStampDTO(Stamp{
		ID: 1,
		Image: ImageResource{Resource: sdk.Resource{
			Ref:            ref,
			URL:            "https://s.pximg.net/stamp.png?token=do-not-leak",
			RequestHeaders: map[string]string{"Cookie": "secret-cookie"},
		}},
	})
	if dto.ID != 1 || dto.Image.Resource == nil || dto.Image.Resource.Ref != ref.String() {
		t.Fatalf("stamp DTO lost fields: %+v", dto)
	}
	data, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	encoded := string(data)
	for _, forbidden := range []string{"s.pximg.net", "do-not-leak", "Cookie", "secret-cookie", "request_headers"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("stamp DTO leaked %q: %s", forbidden, encoded)
		}
	}
}
