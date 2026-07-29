package webapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUgoiraRejectsMissingRequiredWebWire(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		`{"error":false}`, `{"error":false,"body":null}`,
		`{"error":false,"body":{"src":"m","frames":[{"file":"0.jpg"}]}}`,
		`{"error":false,"body":{"originalSrc":"o","frames":[]}}`,
		`{"error":false,"body":{"originalSrc":"o","frames":[{"file":""}]}}`,
	} {
		body := body
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer server.Close()
			_, err := New(WithHTTPClient(server.Client()), WithWebBase(server.URL)).UgoiraMetadata(context.Background(), 1)
			if !errors.Is(err, ErrMalformedResponse) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}
