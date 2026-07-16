package appapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/model"
)

func TestRefreshAndRetryOnceOnTypedAuthStatus(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			session := &fakeSession{token: "old-access"}
			requests := 0
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.URL.Path != "/v1/illust/detail" {
					t.Fatalf("unexpected api path %s", r.URL.Path)
				}
				if r.Header.Get("Authorization") != "Bearer new-access" {
					http.Error(w, `{"error":{"message":"auth required"}}`, status)
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
			if result.Illust.ID != 42 || session.refreshCalls != 1 || requests != 2 {
				t.Fatalf("result=%+v refresh calls=%d requests=%d", result, session.refreshCalls, requests)
			}
		})
	}
}

func TestGETNonAuthStatusDoesNotRefreshOrReplayForAuthWordsInBody(t *testing.T) {
	session := &fakeSession{token: "old-access"}
	requests := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/v1/illust/detail" {
			t.Fatalf("unexpected api path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("oauth unauthorized invalid_grant"))
	}))
	defer api.Close()

	client := New(WithBaseURL(api.URL), WithSession(session))
	if _, err := client.IllustDetail(context.Background(), 42); err == nil {
		t.Fatal("IllustDetail unexpectedly succeeded")
	}
	if requests != 1 || session.refreshCalls != 0 {
		t.Fatalf("requests=%d refresh calls=%d", requests, session.refreshCalls)
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
			"user":              map[string]any{"id": 123, "name": "alice"},
			"profile":           map[string]any{},
			"profile_publicity": map[string]any{"gender": false, "region": false, "birth_day": false, "birth_year": false, "job": false, "pawoo": false},
			"workspace":         map[string]any{},
		})
	}))
	defer api.Close()

	client := New(WithBaseURL(api.URL), WithAccessToken("access"))
	detail, err := client.UserDetail(context.Background(), 123)
	if err != nil {
		t.Fatalf("UserDetail returned error: %v", err)
	}
	if detail.User.ID != 123 || detail.User.Name != "alice" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
}

func TestUserDetailNormalizesProfilePublicityWireValues(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/user/detail" {
			t.Fatalf("unexpected api path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"user":{"id":123},
			"profile":{},
			"profile_publicity":{"gender":"pub\u006cic","region":"private","birth_day":true,"birth_year":false,"job":"public","pawoo":true},
			"workspace":{}
		}`))
	}))
	defer api.Close()

	client := New(WithBaseURL(api.URL), WithAccessToken("access"))
	detail, err := client.UserDetail(context.Background(), 123)
	if err != nil {
		t.Fatalf("UserDetail returned error: %v", err)
	}
	got := detail.ProfilePublicity
	if !got.Gender || got.Region || !got.BirthDay || got.BirthYear || !got.Job || !got.Pawoo {
		t.Fatalf("profile publicity = %#v", got)
	}
}

func TestUserDetailNormalizesMissingProfilePublicityFieldsToFalse(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"user":{"id":123},
			"profile":{},
			"profile_publicity":{"gender":"public"},
			"workspace":{}
		}`))
	}))
	defer api.Close()

	client := New(WithBaseURL(api.URL), WithAccessToken("access"))
	detail, err := client.UserDetail(context.Background(), 123)
	if err != nil {
		t.Fatalf("UserDetail returned error: %v", err)
	}
	got := detail.ProfilePublicity
	if !got.Gender || got.Region || got.BirthDay || got.BirthYear || got.Job || got.Pawoo {
		t.Fatalf("profile publicity = %#v", got)
	}
}

func TestUserDetailRejectsMalformedProfilePublicityWireValues(t *testing.T) {
	tests := []struct {
		name      string
		publicity string
	}{
		{name: "unknown string", publicity: `{"gender":"friends","region":true,"birth_day":true,"birth_year":true,"job":true,"pawoo":true}`},
		{name: "null", publicity: `{"gender":null,"region":true,"birth_day":true,"birth_year":true,"job":true,"pawoo":true}`},
		{name: "number", publicity: `{"gender":1,"region":true,"birth_day":true,"birth_year":true,"job":true,"pawoo":true}`},
		{name: "array", publicity: `{"gender":[],"region":true,"birth_day":true,"birth_year":true,"job":true,"pawoo":true}`},
		{name: "object", publicity: `{"gender":{},"region":true,"birth_day":true,"birth_year":true,"job":true,"pawoo":true}`},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"user":{"id":123},"profile":{},"profile_publicity":` + test.publicity + `,"workspace":{}}`))
			}))
			defer api.Close()

			client := New(WithBaseURL(api.URL), WithAccessToken("access"))
			detail, err := client.UserDetail(context.Background(), 123)
			if detail != nil || !errors.Is(err, ErrMalformedResponse) {
				t.Fatalf("detail=%#v err=%v", detail, err)
			}
		})
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
