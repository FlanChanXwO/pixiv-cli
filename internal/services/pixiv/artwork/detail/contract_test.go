package detail_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/artwork/detail"
)

type fakeTransport struct {
	path  string
	query url.Values
	body  string
}

func (f *fakeTransport) GetJSON(_ context.Context, path string, query url.Values, out any) error {
	f.path = path
	f.query = query
	return decodeJSON(f.body, out)
}

func decodeJSON(body string, out any) error {
	return json.Unmarshal([]byte(body), out)
}

func TestArtworkMapsRouteAndPages(t *testing.T) {
	transport := &fakeTransport{body: `{"illust":{"id":123,"title":"two pages","type":"manga","page_count":2,"user":{"id":7,"name":"artist"},"tags":[{"name":"cat","translated_name":"猫"}],"image_urls":{"large":"https://i.example/cover.jpg"},"meta_pages":[{"page_index":0,"width":1000,"height":1200,"extension":"jpg","image_urls":{"original":"https://i.example/0.jpg"}},{"page_index":1,"width":1000,"height":1200,"extension":"jpg","image_urls":{"original":"https://i.example/1.jpg"}}],"illust_ai_type":2,"create_date":"2024-01-02T03:04:05+00:00"}}`}
	client := detail.New(transport)

	result, err := client.Artwork(context.Background(), 123)
	if err != nil {
		t.Fatalf("Artwork: %v", err)
	}
	if transport.path != "/v1/illust/detail" || transport.query.Get("illust_id") != "123" {
		t.Fatalf("request = %q %v", transport.path, transport.query)
	}
	if result.Artwork.ID != 123 || result.Artwork.User.ID != 7 || len(result.Artwork.MetaPages) != 2 || result.Artwork.AIType != 2 {
		t.Fatalf("artwork = %#v", result.Artwork)
	}
}

func TestUgoiraMapsRequiredMetadata(t *testing.T) {
	transport := &fakeTransport{body: `{"ugoira_metadata":{"zip_urls":{"medium":"https://i.example/m.zip","original":"https://i.example/o.zip"},"frames":[{"file":"000000.jpg","delay":80}]}}`}
	result, err := detail.New(transport).UgoiraMetadata(context.Background(), 123)
	if err != nil {
		t.Fatalf("UgoiraMetadata: %v", err)
	}
	if result.Metadata.ZipURLs.Medium == "" || len(result.Metadata.Frames) != 1 || result.Metadata.Frames[0].File != "000000.jpg" {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}

func TestArtworkRejectsNullOrMissingEnvelope(t *testing.T) {
	for _, body := range []string{`{}`, `{"illust":null}`} {
		_, err := detail.New(&fakeTransport{body: body}).Artwork(context.Background(), 123)
		if err == nil {
			t.Fatalf("body %s unexpectedly succeeded", body)
		}
	}
}
