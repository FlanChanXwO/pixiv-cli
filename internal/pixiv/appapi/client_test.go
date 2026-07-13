package appapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/model"
)

func TestRefreshAndRetryOnAuthError(t *testing.T) {
	session := &fakeSession{token: "old-access"}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/illust/detail" {
			t.Fatalf("unexpected api path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer new-access" {
			http.Error(w, `{"error":{"message":"unauthorized"}}`, http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(model.IllustDetail{Illust: model.Illust{ID: 42, Title: "ok"}})
	}))
	defer api.Close()

	client := New(WithBaseURL(api.URL), WithSession(session))
	result, err := client.IllustDetail(context.Background(), 42)
	if err != nil {
		t.Fatalf("IllustDetail returned error: %v", err)
	}
	if result.Illust.ID != 42 || session.refreshCalls != 1 {
		t.Fatalf("unexpected result/session state: result=%+v refresh calls=%d", result, session.refreshCalls)
	}
}

type fakeSession struct {
	token        string
	refreshCalls int
}

func (s *fakeSession) AccessToken() string { return s.token }
func (s *fakeSession) Refresh(context.Context) error {
	s.refreshCalls++
	s.token = "new-access"
	return nil
}

func TestUserDetailFetchesUserName(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/user/detail" {
			t.Fatalf("unexpected api path %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("user_id"); got != "123" {
			t.Fatalf("user_id = %q, want 123", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user": map[string]any{"id": 123, "name": "alice"},
		})
	}))
	defer api.Close()

	client := New(WithBaseURL(api.URL), WithAccessToken("access"))
	user, err := client.UserDetail(context.Background(), 123)
	if err != nil {
		t.Fatalf("UserDetail returned error: %v", err)
	}
	if user.ID != 123 || user.Name != "alice" {
		t.Fatalf("unexpected user: %+v", user)
	}
}

func TestWithAccessTokenSendsBearerAuthorization(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer direct-access" {
			t.Fatalf("Authorization = %q, want bearer access token", got)
		}
		_ = json.NewEncoder(w).Encode(model.IllustDetail{Illust: model.Illust{ID: 42}})
	}))
	defer api.Close()

	client := New(WithBaseURL(api.URL), WithAccessToken("direct-access"))
	if _, err := client.IllustDetail(context.Background(), 42); err != nil {
		t.Fatalf("IllustDetail returned error: %v", err)
	}
}

func TestBookmarkMutationRefreshesAuthAndAcceptsEmptySuccess(t *testing.T) {
	session := &fakeSession{token: "old-access"}
	api := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v2/illust/bookmark/add" {
			t.Fatalf("method=%s path=%s", request.Method, request.URL.Path)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if request.Form.Get("illust_id") != "42" || request.Form.Get("restrict") != "public" || len(request.Form["tags[]"]) != 2 {
			t.Fatalf("form=%v", request.Form)
		}
		if request.Header.Get("Authorization") != "Bearer new-access" {
			http.Error(writer, `{"error":{"message":"unauthorized"}}`, http.StatusUnauthorized)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer api.Close()

	client := New(WithBaseURL(api.URL), WithSession(session))
	if err := client.AddBookmark(context.Background(), 42, "public", []string{"a", "b"}); err != nil {
		t.Fatalf("AddBookmark returned error: %v", err)
	}
	if session.refreshCalls != 1 {
		t.Fatalf("refresh calls=%d", session.refreshCalls)
	}
}

func TestMutationServerErrorDoesNotRefreshOrReplay(t *testing.T) {
	session := &fakeSession{token: "old-access"}
	requests := 0
	api := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.Method != http.MethodPost || request.URL.Path != "/v1/user/follow/delete" {
			t.Fatalf("method=%s path=%s", request.Method, request.URL.Path)
		}
		writer.WriteHeader(http.StatusBadGateway)
		_, _ = writer.Write([]byte("unauthorized"))
	}))
	defer api.Close()

	client := New(WithBaseURL(api.URL), WithSession(session))
	if err := client.UnfollowUser(context.Background(), 42); err == nil {
		t.Fatal("UnfollowUser unexpectedly succeeded")
	}
	if requests != 1 || session.refreshCalls != 0 {
		t.Fatalf("requests=%d refresh_calls=%d", requests, session.refreshCalls)
	}
}
