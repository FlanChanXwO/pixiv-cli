package stamps_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/stamps"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

type fakeTransport struct {
	path  string
	query url.Values
	body  string
}

func (f *fakeTransport) GetJSON(_ context.Context, path string, query url.Values, out any) error {
	f.path = path
	f.query = query
	return json.Unmarshal([]byte(f.body), out)
}

func TestListMapsStampIDAndResourceURL(t *testing.T) {
	transport := &fakeTransport{
		body: `{"stamps":[{"stamp_id":1,"stamp_url":"https://s.pximg.net/common/images/stamp/generated-stamps/1.png"}]}`,
	}

	result, err := stamps.New(transport).List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if transport.path != "/v1/stamps" || len(transport.query) != 0 {
		t.Fatalf("request = %q %v", transport.path, transport.query)
	}
	if len(result.Items) != 1 || result.Items[0].ID != 1 || result.Items[0].URL != "https://s.pximg.net/common/images/stamp/generated-stamps/1.png" {
		t.Fatalf("result = %#v", result)
	}
}

func TestListRejectsUntrustedResourceHost(t *testing.T) {
	_, err := stamps.New(&fakeTransport{
		body: `{"stamps":[{"stamp_id":1,"stamp_url":"https://example.com/stamp.png"}]}`,
	}).List(context.Background())
	if !errors.Is(err, protocol.ErrMalformedResponse) {
		t.Fatalf("error = %v, want malformed response", err)
	}
}

func TestListRequiresStampsAndValidFields(t *testing.T) {
	for _, body := range []string{
		`{}`,
		`{"stamps":null}`,
		`{"stamps":[{"stamp_id":0,"stamp_url":"https://s.pximg.net/stamp.png"}]}`,
		`{"stamps":[{"stamp_id":-1,"stamp_url":"https://s.pximg.net/stamp.png"}]}`,
		`{"stamps":[{"stamp_id":1}]}`,
		`{"stamps":[{"stamp_id":1,"stamp_url":"http://s.pximg.net/stamp.png"}]}`,
		`{"stamps":[{"stamp_id":1,"stamp_url":"https://s.pximg.net"}]}`,
		`{"stamps":[{"stamp_id":1,"stamp_url":"https://user:pass@s.pximg.net/stamp.png"}]}`,
	} {
		_, err := stamps.New(&fakeTransport{body: body}).List(context.Background())
		if !errors.Is(err, protocol.ErrMalformedResponse) {
			t.Fatalf("body %s error = %v, want malformed response", body, err)
		}
	}

	result, err := stamps.New(&fakeTransport{body: `{"stamps":[]}`}).List(context.Background())
	if err != nil {
		t.Fatalf("empty list: %v", err)
	}
	if result.Items == nil || len(result.Items) != 0 {
		t.Fatalf("empty result = %#v, want non-nil empty list", result)
	}
}

func TestListRejectsNonNullContinuation(t *testing.T) {
	_, err := stamps.New(&fakeTransport{
		body: `{"stamps":[],"next_url":"https://app-api.pixiv.net/v1/stamps?offset=40"}`,
	}).List(context.Background())
	if !errors.Is(err, protocol.ErrMalformedResponse) {
		t.Fatalf("error = %v, want malformed response", err)
	}

	result, err := stamps.New(&fakeTransport{body: `{"stamps":[],"next_url":null}`}).List(context.Background())
	if err != nil || result.Items == nil || len(result.Items) != 0 {
		t.Fatalf("null continuation result = %#v, err = %v", result, err)
	}
}

type errorTransport struct{ err error }

func (f *errorTransport) GetJSON(context.Context, string, url.Values, any) error {
	return f.err
}

func TestListPropagatesTransportFailure(t *testing.T) {
	want := errors.New("transport failed")
	_, err := stamps.New(&errorTransport{err: want}).List(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want transport error", err)
	}
}

func TestListRejectsMissingTransport(t *testing.T) {
	for _, client := range []*stamps.Client{stamps.New(nil), nil} {
		_, err := client.List(context.Background())
		if err == nil || err.Error() != "stamps transport is not configured" {
			t.Fatalf("client = %#v, error = %v", client, err)
		}
	}
}
