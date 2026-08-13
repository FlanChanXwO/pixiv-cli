package detail_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/user/detail"
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

const detailBody = `{"user":{"id":51,"name":"artist","account":"artist","profile_image_urls":{"medium":"https://i.example/profile.jpg"}},"profile":{"webpage":"https://example.invalid","total_novels":3},"profile_publicity":{"gender":"public","region":"private"},"workspace":{"pc":"mac"}}`

func TestDetailMapsProfileAndWireVisibility(t *testing.T) {
	transport := &fakeTransport{body: detailBody}
	result, err := detail.New(transport).Detail(context.Background(), 51)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if transport.path != "/v1/user/detail" || transport.query.Get("user_id") != "51" || result.User.ID != 51 || result.Profile.TotalNovels != 3 || result.ProfilePublicity.Gender != true || result.ProfilePublicity.Region != false || result.Workspace.PC != "mac" || result.User.ProfileImageURLs.Medium == nil {
		t.Fatalf("result=%#v request=%q %v", result, transport.path, transport.query)
	}
}

func TestCurrentUsesUserMeRouteAndRejectsMissingEnvelope(t *testing.T) {
	transport := &fakeTransport{body: detailBody}
	result, err := detail.New(transport).Current(context.Background())
	if err != nil || transport.path != "/v1/user/me" || len(transport.query) != 0 || result.User.ID != 51 {
		t.Fatalf("result=%#v request=%q %v err=%v", result, transport.path, transport.query, err)
	}
	for _, body := range []string{`{}`, `{"user":null,"profile":{},"profile_publicity":{},"workspace":{}}`} {
		_, err := detail.New(&fakeTransport{body: body}).Detail(context.Background(), 51)
		if err == nil {
			t.Fatalf("body %s unexpectedly succeeded", body)
		}
	}
}
