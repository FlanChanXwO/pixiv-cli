package pixiv_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	. "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestArtworkPublicMappingPreservesPages(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1/illust/detail" {
			t.Fatalf("path = %q", req.URL.Path)
		}
		body := `{"illust":{"id":9001,"title":"test art","type":"manga","create_date":"2024-05-01T10:00:00+09:00","page_count":2,"image_urls":{"original":"https://i.pximg.net/img/original/1.png"},"meta_pages":[{"page_index":0,"width":1000,"height":800,"image_urls":{"original":"https://i.pximg.net/img/p0.png"}},{"page_index":1,"width":1000,"height":800,"image_urls":{"original":"https://i.pximg.net/img/p1.png"}}],"user":{"id":7,"name":"artist"},"tags":[]}}`
		return jsonResponse(body), nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	artwork, err := client.Artwork(context.Background(), ArtworkRequest{ArtworkID: 9001})
	if err != nil {
		t.Fatalf("Artwork: %v", err)
	}
	if artwork.Kind != ArtworkKindManga || artwork.ID != 9001 || len(artwork.Pages) != 2 {
		t.Fatalf("artwork = %+v", artwork)
	}
	if artwork.Cover.Resource.Ref.IsZero() || artwork.Pages[1].Image.Resource.Ref.IsZero() {
		t.Fatal("artwork resources lost opaque references")
	}
	if !strings.Contains(artwork.Pages[1].Image.Resource.URL, "p1.png") {
		t.Fatalf("page URL = %q", artwork.Pages[1].Image.Resource.URL)
	}
}

func TestArtworkPublicMappingRejectsMissingPublishTime(t *testing.T) {
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(`{"illust":{"id":1,"type":"illust","image_urls":{"original":"https://i.pximg.net/img/1.png"},"user":{"id":7,"name":"artist"}}}`), nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	_, err = client.Artwork(context.Background(), ArtworkRequest{ArtworkID: 1})
	if sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
		t.Fatalf("reason = %q, want %q", sdk.ReasonOf(err), sdk.MalformedUpstreamResponse)
	}
}

func TestArtworkPagesPublicMappingDerivesSinglePage(t *testing.T) {
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(`{"illust":{"id":5,"type":"illust","create_date":"2024-01-01T00:00:00Z","image_urls":{"original":"https://i.pximg.net/img/5.png"},"meta_single_page":{"original_image_url":"https://i.pximg.net/img/5.png"},"user":{"id":7,"name":"artist"}}}`), nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	pages, err := client.ArtworkPages(context.Background(), ArtworkPagesRequest{ArtworkID: 5})
	if err != nil {
		t.Fatalf("ArtworkPages: %v", err)
	}
	if len(pages) != 1 || pages[0].PageIndex != 0 || !strings.Contains(pages[0].Image.Resource.URL, "5.png") {
		t.Fatalf("pages = %+v", pages)
	}
}

func TestUgoiraPublicMappingAndFilenameValidation(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
			return jsonResponse(`{"ugoira_metadata":{"zip_urls":{"medium":"https://i.pximg.net/zip/m.zip","original":"https://i.pximg.net/zip/o.zip"},"frames":[{"file":"0.jpg","delay":100},{"file":"1.jpg","delay":200}]}}`), nil
		})
		client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
		if err != nil {
			t.Fatalf("NewWith: %v", err)
		}
		meta, err := client.UgoiraMetadata(context.Background(), UgoiraMetadataRequest{ArtworkID: 777})
		if err != nil || len(meta.Archives) != 2 || len(meta.Frames) != 2 || meta.Frames[1].DelayMilliseconds != 200 {
			t.Fatalf("metadata = %+v err=%v", meta, err)
		}
	})

	t.Run("unsafe filename", func(t *testing.T) {
		rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
			return jsonResponse(`{"ugoira_metadata":{"zip_urls":{"original":"https://i.pximg.net/zip/o.zip"},"frames":[{"file":"../evil.jpg","delay":100}]}}`), nil
		})
		client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
		if err != nil {
			t.Fatalf("NewWith: %v", err)
		}
		_, err = client.UgoiraMetadata(context.Background(), UgoiraMetadataRequest{ArtworkID: 1})
		if sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
			t.Fatalf("reason = %q, want %q", sdk.ReasonOf(err), sdk.MalformedUpstreamResponse)
		}
	})
}

func TestNovelContentPublicParserPreservesUnknownBlock(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1/novel/content" {
			t.Fatalf("path = %q", req.URL.Path)
		}
		body := `<html><body><div class="novel-view"><div class="novel-body"><p class="noveltext">known text</p><div class="novel_something">unknown block payload</div></div></div></body></html>`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	content, err := client.NovelContent(context.Background(), NovelContentRequest{NovelID: 1})
	if err != nil {
		t.Fatalf("NovelContent: %v", err)
	}
	if len(content.Blocks) != 2 || content.Blocks[1].Kind != NovelBlockUnknown || content.Blocks[1].Unknown == nil {
		t.Fatalf("content = %+v", content)
	}
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
