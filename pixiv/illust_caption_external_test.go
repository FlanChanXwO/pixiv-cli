package pixiv_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pixiv"
)

func TestIllustDetailMapsAppCaption(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/illust/detail" || r.URL.Query().Get("illust_id") != "73" {
			t.Fatalf("request = %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		fmt.Fprint(w, `{"illust":{"id":73,"caption":"<p>Line one<br />Line two</p>","page_count":1,"width":10,"height":20,"user":{"id":8},"tags":[],"image_urls":{},"meta_single_page":{"original_image_url":"https://img.example/73.jpg"},"meta_pages":[]}}`)
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "access"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.IllustDetail(context.Background(), 73)
	if err != nil {
		t.Fatal(err)
	}
	if result.Illust.Caption != "<p>Line one<br />Line two</p>" {
		t.Fatalf("caption = %q", result.Illust.Caption)
	}
}

func TestAnonymousIllustDetailMapsWebDescriptionAsCaption(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ajax/illust/74":
			fmt.Fprint(w, `{"error":false,"body":{"id":"74","description":"<p>Web caption</p>","pageCount":1,"userId":"8","userName":"artist","urls":{"original":"https://img.example/74.jpg"}}}`)
		case "/ajax/illust/74/pages":
			fmt.Fprint(w, `{"error":false,"body":[{"urls":{"original":"https://img.example/74.jpg"},"width":10,"height":20}]}`)
		default:
			t.Fatalf("request path = %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), WebAPIBaseURL: server.URL, WebFallbackEnabled: true})
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.IllustDetail(context.Background(), 74)
	if err != nil {
		t.Fatal(err)
	}
	if result.Illust.Caption != "<p>Web caption</p>" {
		t.Fatalf("caption = %q", result.Illust.Caption)
	}
}
