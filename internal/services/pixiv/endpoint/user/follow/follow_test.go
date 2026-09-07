package follow_test

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/follow"
)

type fakeTransport struct {
	path  string
	form  url.Values
	calls int
	err   error
}

func (f *fakeTransport) PostForm(_ context.Context, path string, form url.Values) error {
	f.calls++
	f.path = path
	f.form = form
	return f.err
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

func TestFollowRejectsInvalidRequestsBeforeTransport(t *testing.T) {
	tests := []struct {
		name string
		call func(*follow.Client) error
	}{
		{name: "add zero user", call: func(client *follow.Client) error {
			return client.Add(context.Background(), follow.Request{UserID: 0, Restrict: "public"})
		}},
		{name: "add negative user", call: func(client *follow.Client) error {
			return client.Add(context.Background(), follow.Request{UserID: -1, Restrict: "public"})
		}},
		{name: "add empty restrict", call: func(client *follow.Client) error {
			return client.Add(context.Background(), follow.Request{UserID: 7})
		}},
		{name: "add unknown restrict", call: func(client *follow.Client) error {
			return client.Add(context.Background(), follow.Request{UserID: 7, Restrict: "friends"})
		}},
		{name: "remove zero user", call: func(client *follow.Client) error {
			return client.Remove(context.Background(), 0)
		}},
		{name: "remove negative user", call: func(client *follow.Client) error {
			return client.Remove(context.Background(), -1)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := &fakeTransport{}
			if err := test.call(follow.New(transport)); err == nil {
				t.Fatal("invalid follow request unexpectedly succeeded")
			}
			if transport.calls != 0 {
				t.Fatalf("invalid request reached transport %d time(s)", transport.calls)
			}
		})
	}
}

func TestFollowPropagatesTransportError(t *testing.T) {
	wantErr := errors.New("transport failed")
	transport := &fakeTransport{err: wantErr}
	if err := follow.New(transport).Add(context.Background(), follow.Request{UserID: 7, Restrict: "public"}); !errors.Is(err, wantErr) {
		t.Fatalf("Add error = %v, want %v", err, wantErr)
	}
	if transport.calls != 1 {
		t.Fatalf("transport calls = %d, want 1", transport.calls)
	}
}
