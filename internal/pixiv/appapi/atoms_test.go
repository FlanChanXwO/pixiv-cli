package appapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTrendingAndUgoiraRejectMissingRequiredWire(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, path, body string
		call             func(*Client) error
	}{
		{"trending missing", "/v1/trending-tags/illust", `{}`, func(c *Client) error { _, e := c.TrendingTagsIllust(context.Background()); return e }},
		{"trending null", "/v1/trending-tags/illust", `{"trend_tags":null}`, func(c *Client) error { _, e := c.TrendingTagsIllust(context.Background()); return e }},
		{"trending bad item", "/v1/trending-tags/illust", `{"trend_tags":[{"tag":"","illust":{"id":1}}]}`, func(c *Client) error { _, e := c.TrendingTagsIllust(context.Background()); return e }},
		{"ugoira missing", "/v1/ugoira/metadata", `{}`, func(c *Client) error { _, e := c.UgoiraMetadata(context.Background(), 1); return e }},
		{"ugoira no medium", "/v1/ugoira/metadata", `{"ugoira_metadata":{"zip_urls":{},"frames":[{"file":"0.jpg"}]}}`, func(c *Client) error { _, e := c.UgoiraMetadata(context.Background(), 1); return e }},
		{"ugoira empty frames", "/v1/ugoira/metadata", `{"ugoira_metadata":{"zip_urls":{"medium":"m"},"frames":[]}}`, func(c *Client) error { _, e := c.UgoiraMetadata(context.Background(), 1); return e }},
		{"ugoira bad frame", "/v1/ugoira/metadata", `{"ugoira_metadata":{"zip_urls":{"medium":"m"},"frames":[{"file":""}]}}`, func(c *Client) error { _, e := c.UgoiraMetadata(context.Background(), 1); return e }},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tt.path {
					t.Fatalf("path=%s", r.URL.Path)
				}
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			err := tt.call(New(WithBaseURL(server.URL), WithAccessToken("token")))
			if !errors.Is(err, ErrMalformedResponse) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestTrendingExplicitEmptyAndDuplicateLastAreValid(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"trend_tags":[{"tag":"old","illust":{"id":1}}],"trend_tags":[]}`))
	}))
	defer server.Close()
	got, err := New(WithBaseURL(server.URL), WithAccessToken("token")).TrendingTagsIllust(context.Background())
	if err != nil || got.TrendTags == nil || len(got.TrendTags) != 0 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}
