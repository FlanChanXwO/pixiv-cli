package detail_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/novel/detail"
)

type fakeTransport struct {
	path  string
	query url.Values
	body  string
	raw   []byte
}

func (f *fakeTransport) GetJSON(_ context.Context, path string, query url.Values, out any) error {
	f.path = path
	f.query = query
	return json.Unmarshal([]byte(f.body), out)
}

func (f *fakeTransport) GetRaw(_ context.Context, path string, query url.Values) ([]byte, error) {
	f.path = path
	f.query = query
	return f.raw, nil
}

func TestDetailMapsNovelAndSeriesReferences(t *testing.T) {
	transport := &fakeTransport{body: `{"novel":{"id":12,"title":"story","caption":"body","x_restrict":0,"text_length":8,"is_original":true,"create_date":"2024-01-02T03:04:05+00:00","user":{"id":7,"name":"writer"}},"series_next":{"id":14,"title":"series"},"series_prev":{"id":10}}`}
	result, err := detail.New(transport).Detail(context.Background(), 12)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if transport.path != "/v1/novel/detail" || transport.query.Get("novel_id") != "12" || result.Novel.ID != 12 || result.Novel.User.ID != 7 || result.SeriesNextID != 14 || result.SeriesPrevID != 10 || result.SeriesTitle != "series" {
		t.Fatalf("result = %#v request=%q %v", result, transport.path, transport.query)
	}
}

func TestDetailAndContentRejectMalformedOrEmptyResponses(t *testing.T) {
	for _, body := range []string{`{}`, `{"novel":null}`} {
		_, err := detail.New(&fakeTransport{body: body}).Detail(context.Background(), 12)
		if err == nil {
			t.Fatalf("body %s unexpectedly succeeded", body)
		}
	}
	_, err := detail.New(&fakeTransport{raw: []byte(" \n\t")}).Content(context.Background(), 12)
	if err == nil {
		t.Fatal("empty content unexpectedly succeeded")
	}
	content, err := detail.New(&fakeTransport{raw: []byte("<body>text</body>")}).Content(context.Background(), 12)
	if err != nil || string(content) != "<body>text</body>" {
		t.Fatalf("content=%q err=%v", content, err)
	}
}
