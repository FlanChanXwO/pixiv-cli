package pixiv_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pixiv"
)

func TestClientIllustDetailUsesAppPagesWithoutWebEnrichment(t *testing.T) {
	t.Parallel()

	const illustID int64 = 123
	var appRequests atomic.Int32
	appServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appRequests.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/v1/illust/detail" {
			t.Errorf("App request = %s %s, want GET /v1/illust/detail", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("illust_id"); got != "123" {
			t.Errorf("App illust_id = %q, want 123", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-access-token" {
			t.Errorf("App Authorization = %q, want bearer access token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"illust": {
				"id": 123,
				"title": "App detail",
				"type": "illust",
				"page_count": 2,
				"total_bookmarks": 21,
				"total_view": 34,
				"x_restrict": 0,
				"user": {"id": 456, "name": "artist", "account": "artist", "comment": "", "is_followed": false},
				"tags": [{"name": "創作", "translated_name": "original"}],
				"image_urls": {
					"square_medium": "https://i.pximg.net/app-square.jpg",
					"medium": "https://i.pximg.net/app-medium.jpg",
					"large": "https://i.pximg.net/app-large.jpg",
					"original": ""
				},
				"meta_single_page": {"original_image_url": ""},
				"meta_pages": [
					{
						"page_index": 0,
						"width": 1200,
						"height": 1600,
						"extension": "png",
						"image_urls": {
							"square_medium": "https://i.pximg.net/page-0-thumb.jpg",
							"medium": "https://i.pximg.net/page-0-small.jpg",
							"large": "https://i.pximg.net/page-0-regular.jpg",
							"original": "https://i.pximg.net/page-0-original.png"
						}
					},
					{
						"page_index": 1,
						"width": 2400,
						"height": 1800,
						"extension": "jpg",
						"image_urls": {
							"square_medium": "https://i.pximg.net/page-1-thumb.jpg",
							"medium": "https://i.pximg.net/page-1-small.jpg",
							"large": "https://i.pximg.net/page-1-regular.jpg",
							"original": "https://i.pximg.net/page-1-original.jpg"
						}
					}
				],
				"ai_type": 2,
				"create_date": "2025-01-02T03:04:05+09:00",
				"width": 1200,
				"height": 1600
			}
		}`)
	}))
	defer appServer.Close()

	var webRequests atomic.Int32
	webServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webRequests.Add(1)
		http.NotFound(w, r)
	}))
	defer webServer.Close()

	client, err := pixiv.NewClient(pixiv.Options{
		HTTPClient:    appServer.Client(),
		AppAPIBaseURL: appServer.URL,
		WebAPIBaseURL: webServer.URL,
		AccessToken:   "test-access-token",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	detail, err := client.IllustDetail(context.Background(), illustID)
	if err != nil {
		t.Fatalf("IllustDetail() error = %v", err)
	}
	if appRequests.Load() != 1 || webRequests.Load() != 0 {
		t.Fatalf("backend requests = App %d, Web %d; want App 1, Web 0", appRequests.Load(), webRequests.Load())
	}

	illust := detail.Illust
	if illust.ID != illustID || illust.Title != "App detail" {
		t.Errorf("App detail fields = (%d, %q), want (123, %q)", illust.ID, illust.Title, "App detail")
	}
	if illust.AIType != 2 || illust.CreateDate != "2025-01-02T03:04:05+09:00" {
		t.Errorf("App metadata = (ai_type %d, create_date %q)", illust.AIType, illust.CreateDate)
	}
	if illust.Width != 1200 || illust.Height != 1600 {
		t.Errorf("App dimensions = %dx%d, want 1200x1600", illust.Width, illust.Height)
	}
	if len(illust.MetaPages) != 2 {
		t.Fatalf("len(meta_pages) = %d, want 2", len(illust.MetaPages))
	}

	wantPages := []pixiv.MetaPage{
		{
			PageIndex: 0,
			Width:     1200,
			Height:    1600,
			Extension: "png",
			ImageURLs: pixiv.ImageURLs{
				SquareMedium: "https://i.pximg.net/page-0-thumb.jpg",
				Medium:       "https://i.pximg.net/page-0-small.jpg",
				Large:        "https://i.pximg.net/page-0-regular.jpg",
				Original:     "https://i.pximg.net/page-0-original.png",
			},
		},
		{
			PageIndex: 1,
			Width:     2400,
			Height:    1800,
			Extension: "jpg",
			ImageURLs: pixiv.ImageURLs{
				SquareMedium: "https://i.pximg.net/page-1-thumb.jpg",
				Medium:       "https://i.pximg.net/page-1-small.jpg",
				Large:        "https://i.pximg.net/page-1-regular.jpg",
				Original:     "https://i.pximg.net/page-1-original.jpg",
			},
		},
	}
	for i, want := range wantPages {
		if got := illust.MetaPages[i]; got != want {
			t.Errorf("meta_pages[%d] = %#v, want %#v", i, got, want)
		}
	}

	encoded, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("json.Marshal(detail) error = %v", err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatalf("json.Unmarshal(detail envelope) error = %v", err)
	}
	if _, ok := envelope["illust"]; !ok {
		t.Fatalf("detail JSON keys = %v, want illust envelope", encoded)
	}
	var illustJSON map[string]json.RawMessage
	if err := json.Unmarshal(envelope["illust"], &illustJSON); err != nil {
		t.Fatalf("json.Unmarshal(illust) error = %v", err)
	}
	for _, key := range []string{
		"url", "id", "title", "type", "page_count", "total_bookmarks", "total_view", "x_restrict",
		"user", "tags", "image_urls", "meta_single_page", "meta_pages",
		"ai_type", "create_date", "width", "height",
	} {
		if _, ok := illustJSON[key]; !ok {
			t.Errorf("illust JSON missing key %q: %s", key, encoded)
		}
	}

	encodedText := string(encoded)
	if !strings.Contains(encodedText, `"url":"https://www.pixiv.net/artworks/123"`) {
		t.Errorf("illust JSON missing artwork url: %s", encodedText)
	}
	illustStart := strings.Index(encodedText, `"illust":{`)
	if illustStart < 0 {
		t.Fatalf("detail JSON missing illust object: %s", encodedText)
	}
	illustBody := encodedText[illustStart+len(`"illust":`):]
	if !strings.HasPrefix(illustBody, `{"url":`) {
		t.Errorf("illust JSON field order = %s, want url first", illustBody)
	}

	var pagesJSON []map[string]json.RawMessage
	if err := json.Unmarshal(illustJSON["meta_pages"], &pagesJSON); err != nil {
		t.Fatalf("json.Unmarshal(meta_pages) error = %v", err)
	}
	if len(pagesJSON) != 2 {
		t.Fatalf("JSON len(meta_pages) = %d, want 2", len(pagesJSON))
	}
	for i, pageJSON := range pagesJSON {
		for _, key := range []string{"page_index", "width", "height", "extension", "image_urls"} {
			if _, ok := pageJSON[key]; !ok {
				t.Errorf("meta_pages[%d] JSON missing key %q: %s", i, key, encoded)
			}
		}

		var imageURLsJSON map[string]json.RawMessage
		if err := json.Unmarshal(pageJSON["image_urls"], &imageURLsJSON); err != nil {
			t.Fatalf("json.Unmarshal(meta_pages[%d].image_urls) error = %v", i, err)
		}
		for _, key := range []string{"square_medium", "medium", "large", "original"} {
			if _, ok := imageURLsJSON[key]; !ok {
				t.Errorf("meta_pages[%d].image_urls JSON missing key %q: %s", i, key, encoded)
			}
		}
	}
}

func TestClientIllustDetailDerivesSingleAppPage(t *testing.T) {
	t.Parallel()

	appServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/illust/detail" || r.URL.Query().Get("illust_id") != "124" {
			t.Fatalf("App request = %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"illust":{
			"id":124,"page_count":1,"width":1200,"height":1600,
			"image_urls":{"square_medium":"https://i.pximg.net/thumb.jpg","medium":"https://i.pximg.net/medium.jpg","large":"https://i.pximg.net/large.jpg"},
			"meta_single_page":{"original_image_url":"https://i.pximg.net/124_p0.png"},
			"meta_pages":[]
		}}`))
	}))
	t.Cleanup(appServer.Close)

	var webRequests atomic.Int32
	webServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webRequests.Add(1)
		http.NotFound(w, r)
	}))
	t.Cleanup(webServer.Close)

	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: appServer.Client(), AppAPIBaseURL: appServer.URL, WebAPIBaseURL: webServer.URL, AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := client.IllustDetail(context.Background(), 124)
	if err != nil {
		t.Fatalf("IllustDetail() error = %v", err)
	}
	if len(detail.Illust.MetaPages) != 1 {
		t.Fatalf("len(meta_pages) = %d, want 1", len(detail.Illust.MetaPages))
	}
	page := detail.Illust.MetaPages[0]
	if page.PageIndex != 0 || page.Width != 1200 || page.Height != 1600 || page.Extension != "png" || page.ImageURLs.Original != "https://i.pximg.net/124_p0.png" || page.ImageURLs.Large != "https://i.pximg.net/large.jpg" {
		t.Fatalf("derived page = %#v", page)
	}
	if webRequests.Load() != 0 {
		t.Fatalf("Web requests = %d, want 0", webRequests.Load())
	}
}

func TestClientIllustDetailRejectsIncompleteAppPages(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/illust/detail" {
			t.Fatalf("request path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"illust":{"id":125,"page_count":2,"meta_pages":[{"page_index":0,"image_urls":{"original":"https://img/125_p0.png"}}]}}`))
	}))
	t.Cleanup(server.Close)

	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, WebAPIBaseURL: server.URL, AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := client.IllustDetail(context.Background(), 125)
	if detail != nil {
		t.Fatalf("IllustDetail() detail = %#v, want nil", detail)
	}
	var typed *pixiv.Error
	if !errors.As(err, &typed) || typed.Code != pixiv.CodeMalformedUpstreamResponse || typed.Backend != pixiv.BackendAppAPI || typed.Operation != pixiv.OperationIllustDetail || typed.IllustID != 125 {
		t.Fatalf("IllustDetail() error = %#v", err)
	}
}
