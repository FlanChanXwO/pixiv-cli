package resource_test

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/protocol"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/resource"
)

type mediaTransport struct {
	url     string
	request protocol.MediaRequest
}

func (t *mediaTransport) OpenMediaWithRequest(_ context.Context, rawURL string, request protocol.MediaRequest) (*http.Response, error) {
	t.url = rawURL
	t.request = request
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(nilReader{})}, nil
}

type nilReader struct{}

func (nilReader) Read([]byte) (int, error) { return 0, io.EOF }

func TestOpenValidatesMediaURLAndForwardsControlledHeaders(t *testing.T) {
	transport := &mediaTransport{}
	client := resource.New(transport)
	response, err := client.Open(context.Background(), "https://downloads.fanbox.cc/file.zip", resource.Request{
		Method:          http.MethodHead,
		Range:           "bytes=0-1",
		IfNoneMatch:     `"etag"`,
		IfModifiedSince: "Wed, 21 Oct 2015 07:28:00 GMT",
		IfRange:         `"etag"`,
	})
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if transport.url != "https://downloads.fanbox.cc/file.zip" || transport.request.Method != http.MethodHead || transport.request.Range != "bytes=0-1" {
		t.Fatalf("transport request = %+v url=%q", transport.request, transport.url)
	}
}

func TestValidateURLRejectsUnsafeOrPathlessLocators(t *testing.T) {
	for _, rawURL := range []string{
		"http://downloads.fanbox.cc/file.zip",
		"https://user:pass@downloads.fanbox.cc/file.zip",
		"https://example.invalid/file.zip",
		"https://downloads.fanbox.cc/",
	} {
		if err := resource.ValidateURL(rawURL); err == nil {
			t.Fatalf("ValidateURL(%q) unexpectedly succeeded", rawURL)
		}
	}
}
