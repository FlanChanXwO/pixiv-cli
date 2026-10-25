// sdk/fanbox 测试共享的 HTTP fixture；各方法测试按生产文件同名拆分。

package fanbox_test

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func homeMetadataBody(userID int64, name string) string {
	return `<html><head><meta name="metadata" content='{"context":{"user":{"userId":` + strconv.FormatInt(userID, 10) + `,"name":"` + name + `"}}}'></head></html>`
}

func testClient(t *testing.T, rt roundTripFunc) *fanbox.Client {
	t.Helper()
	client, err := fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: "session-value"}, fanbox.Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return client
}
