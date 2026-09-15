package mypixiv_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/mypixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

type fakeTransport struct {
	path  string
	query url.Values
	body  string
	calls int
}

func (f *fakeTransport) GetJSON(_ context.Context, path string, query url.Values, out any) error {
	f.calls++
	f.path, f.query = path, query
	return json.Unmarshal([]byte(f.body), out)
}
func TestMyPixivMapsUserIDFilterAndOffset(t *testing.T) {
	f := &fakeTransport{body: `{"user_previews":[{"user":{"id":101}}]}`}
	result, err := mypixiv.New(f).List(context.Background(), mypixiv.Request{UserID: 10, Offset: 6})
	if err != nil || f.path != "/v1/user/mypixiv" || f.query.Get("user_id") != "10" || f.query.Get("filter") != "for_android" || f.query.Get("offset") != "6" || len(result.Items) != 1 || result.Items[0].User.ID != 101 {
		t.Fatalf("result=%#v request=%q %v err=%v", result, f.path, f.query, err)
	}
}

func TestListRejectsNonPositiveUserIDBeforeTransport(t *testing.T) {
	for _, userID := range []int64{0, -1} {
		t.Run(strconv.FormatInt(userID, 10), func(t *testing.T) {
			transport := &fakeTransport{body: `{"user_previews":[]}`}
			_, err := mypixiv.New(transport).List(context.Background(), mypixiv.Request{UserID: userID})
			if err == nil {
				t.Fatal("non-positive user ID unexpectedly succeeded")
			}
			if transport.calls != 0 {
				t.Fatalf("invalid request reached transport %d time(s)", transport.calls)
			}
		})
	}
}

func TestListRejectsNullUsersAndPreservesEmptyUsers(t *testing.T) {
	for _, test := range []struct {
		name      string
		body      string
		wantError bool
	}{
		{name: "null", body: `{"user_previews":null}`, wantError: true},
		{name: "empty", body: `{"user_previews":[]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			transport := &fakeTransport{body: test.body}
			result, err := mypixiv.New(transport).List(context.Background(), mypixiv.Request{UserID: 10})
			if test.wantError {
				if !errors.Is(err, protocol.ErrMalformedResponse) {
					t.Fatalf("null users error = %v, want malformed response", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if result.Items == nil {
				t.Fatal("empty users list mapped to nil items")
			}
		})
	}
}
