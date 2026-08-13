package visibility_test

import (
	"context"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/user/visibility"
)

type fakeTransport struct {
	path string
	form url.Values
}

func (f *fakeTransport) PostForm(_ context.Context, path string, form url.Values) error {
	f.path = path
	f.form = form
	return nil
}

func TestSetUsesExplicitAIShowValue(t *testing.T) {
	for _, test := range []struct {
		visible bool
		want    string
	}{
		{visible: true, want: "1"},
		{visible: false, want: "0"},
	} {
		transport := &fakeTransport{}
		if err := visibility.New(transport).Set(context.Background(), test.visible); err != nil {
			t.Fatalf("Set(%v): %v", test.visible, err)
		}
		if transport.path != "/v1/user/edit-ai-show-settings" || transport.form.Get("ai_show") != test.want {
			t.Fatalf("request = %q %v", transport.path, transport.form)
		}
	}
}
