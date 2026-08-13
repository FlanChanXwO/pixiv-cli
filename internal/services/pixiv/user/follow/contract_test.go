package follow_test

import (
	"context"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/user/follow"
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

func TestAddAndRemoveKeepFollowCommitBoundary(t *testing.T) {
	transport := &fakeTransport{}
	client := follow.New(transport)
	if err := client.Add(context.Background(), follow.Request{UserID: 7, Restrict: "public"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if transport.path != "/v1/user/follow/add" || transport.form.Get("user_id") != "7" || transport.form.Get("restrict") != "public" {
		t.Fatalf("add request = %q %v", transport.path, transport.form)
	}
	if err := client.Remove(context.Background(), 7); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if transport.path != "/v1/user/follow/delete" || transport.form.Get("user_id") != "7" {
		t.Fatalf("remove request = %q %v", transport.path, transport.form)
	}
}
