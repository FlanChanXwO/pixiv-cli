package pixiv_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	. "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
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

func oauthResponse(userID int64, name string) *http.Response {
	return jsonResponse(fmt.Sprintf(`{"access_token":"new-access-token","refresh_token":"new-refresh-token","expires_in":3600,"user":{"id":%d,"name":%q}}`, userID, name))
}

func TestNewRejectsEmptyToken(t *testing.T) {
	if _, err := New(""); sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
	if _, err := NewWith("  ", Options{}); sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}

func TestOpenRotatesCredentials(t *testing.T) {
	var captured string
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "oauth.secure.pixiv.net" {
			return nil, errors.New("unexpected host " + req.URL.Host)
		}
		_ = req.ParseForm()
		captured = req.PostForm.Get("grant_type")
		return oauthResponse(42, "tester"), nil
	})
	client, creds, err := OpenWith(context.Background(), "old-refresh-token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if client.UserID() != 42 || client.Username() != "tester" {
		t.Fatalf("client identity = %d/%q", client.UserID(), client.Username())
	}
	if creds.AccessToken() != "new-access-token" || creds.RefreshToken() != "new-refresh-token" || creds.ExpiresAt.IsZero() {
		t.Fatalf("credentials not rotated: %#v", creds)
	}
	if captured != "refresh_token" {
		t.Fatalf("grant_type = %q", captured)
	}
}

func TestCurrentUserUsesVerifiedOAuthIdentity(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "oauth.secure.pixiv.net" {
			return oauthResponse(42, "tester"), nil
		}
		if req.URL.Host != "app-api.pixiv.net" || req.URL.Path != "/v1/user/detail" {
			return nil, errors.New("unexpected request " + req.URL.String())
		}
		if req.URL.Query().Get("user_id") != "42" || req.Header.Get("X-User-Id") != "42" {
			return nil, errors.New("current user request does not use verified identity")
		}
		return jsonResponse(`{"user":{"id":42,"name":"tester"},"profile":{},"profile_publicity":{},"workspace":{}}`), nil
	})
	client, _, err := OpenWith(context.Background(), "old-refresh-token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("OpenWith: %v", err)
	}
	result, err := client.CurrentUser(context.Background(), CurrentUserRequest{})
	if err != nil {
		t.Fatalf("CurrentUser: %v", err)
	}
	if result.User.ID != 42 || result.User.Name != "tester" {
		t.Fatalf("current user = %#v", result)
	}
}

func TestOpenRejectsMissingAccountIdentity(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(`{"access_token":"new-access-token","refresh_token":"new-refresh-token","expires_in":3600,"user":{"name":"tester"}}`), nil
	})
	client, credentials, err := OpenWith(context.Background(), "old-refresh-token", Options{HTTPClient: &http.Client{Transport: rt}})
	if sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
		t.Fatalf("expected MalformedUpstreamResponse, got %v", err)
	}
	if client != nil {
		t.Fatal("client must not be returned without verified account identity")
	}
	if credentials.UserID != 0 || credentials.AccessToken() != "" || credentials.RefreshToken() != "" {
		t.Fatalf("credentials must be empty on malformed response: %#v", credentials)
	}
}

func TestOpenRejectsEmptyRefreshToken(t *testing.T) {
	if _, _, err := Open(context.Background(), ""); sdk.ReasonOf(err) != sdk.CredentialsExpired {
		t.Fatalf("expected CredentialsExpired, got %v", err)
	}
}

func TestPublicScopedCursorsCrossClients(t *testing.T) {
	newClient := func(userID int64) *Client {
		rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == "oauth.secure.pixiv.net" {
				return oauthResponse(userID, ""), nil
			}
			if req.URL.Query().Get("offset") != "" {
				return jsonResponse(`{"novels":[],"user_previews":[],"next_url":null}`), nil
			}
			switch req.URL.Path {
			case "/v1/search/novel":
				return jsonResponse(`{"novels":[{"id":2001,"title":"novel","create_date":"2026-01-01T00:00:00Z","user":{"id":7},"x_restrict":0,"text_length":12,"is_original":true}],"next_url":"https://app-api.pixiv.net/v1/search/novel?word=novel&offset=30"}`), nil
			case "/v1/search/user":
				return jsonResponse(`{"user_previews":[{"user":{"id":3001}}],"next_url":"https://app-api.pixiv.net/v1/search/user?word=artist&offset=20"}`), nil
			default:
				return nil, errors.New("unexpected path " + req.URL.Path)
			}
		})
		client, _, err := OpenWith(context.Background(), "refresh", Options{HTTPClient: &http.Client{Transport: rt}})
		if err != nil {
			t.Fatalf("OpenWith: %v", err)
		}
		return client
	}

	first, second := newClient(42), newClient(43)
	novels := SearchNovelsRequest{Word: "novel"}
	page, err := first.SearchNovels(context.Background(), novels)
	if err != nil {
		t.Fatalf("SearchNovels: %v", err)
	}
	if _, err := second.SearchNovels(context.Background(), SearchNovelsRequest{Word: "novel", Cursor: page.Next}); err != nil {
		t.Fatalf("public novel cursor: %v", err)
	}
	users := SearchUsersRequest{Word: "artist"}
	userPage, err := first.SearchUsers(context.Background(), users)
	if err != nil {
		t.Fatalf("SearchUsers: %v", err)
	}
	if _, err := second.SearchUsers(context.Background(), SearchUsersRequest{Word: "artist", Cursor: userPage.Next}); err != nil {
		t.Fatalf("public user cursor: %v", err)
	}
}

func TestReadPermissionErrorIsClassified(t *testing.T) {
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	if _, err := client.Artwork(context.Background(), ArtworkRequest{ArtworkID: 1}); sdk.ReasonOf(err) != sdk.Forbidden {
		t.Fatalf("ReasonOf = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.Forbidden, err)
	}
}

func TestAppOnlyFailureDoesNotFallBackOrReplay(t *testing.T) {
	for _, test := range []struct {
		name string
		call func(*Client) error
	}{
		{name: "read", call: func(c *Client) error {
			_, err := c.Artwork(context.Background(), ArtworkRequest{ArtworkID: 1})
			return err
		}},
		{name: "mutation", call: func(c *Client) error {
			return c.AddArtworkBookmark(context.Background(), AddArtworkBookmarkRequest{ArtworkID: 1})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.URL.Host != "app-api.pixiv.net" {
					return nil, errors.New("unexpected non-App API host")
				}
				return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
			})
			client, err := NewWith("access-token", Options{HTTPClient: &http.Client{Transport: rt}})
			if err != nil {
				t.Fatalf("NewWith: %v", err)
			}
			if err := test.call(client); sdk.ReasonOf(err) != sdk.CredentialsExpired {
				t.Fatalf("error reason = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.CredentialsExpired, err)
			}
			if calls != 1 {
				t.Fatalf("App API calls = %d, want 1", calls)
			}
		})
	}
}

func TestNovelContentIsUnsupportedWithoutNetwork(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, io.ErrUnexpectedEOF
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	_, err = client.NovelContent(context.Background(), NovelContentRequest{NovelID: 1})
	if sdk.ReasonOf(err) != sdk.ContentUnavailable || calls != 0 {
		t.Fatalf("reason = %q calls = %d, want ContentUnavailable without network (err=%v)", sdk.ReasonOf(err), calls, err)
	}
}

func TestPacingWithNilLoggerDoesNotPanic(t *testing.T) {
	requests := 0
	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests++
		return jsonResponse(`{"illust":{"id":42,"title":"paced","type":"illust","page_count":1,"create_date":"2026-01-01T00:00:00Z","user":{"id":7,"name":"artist"},"meta_single_page":{"original_image_url":"https://i.pximg.net/img-original/img/42_p0.png"}}}`), nil
	})}
	client, err := NewWith("fixture-token", Options{HTTPClient: httpClient, Pacing: Pacing{MinInterval: time.Millisecond}})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := client.Artwork(context.Background(), ArtworkRequest{ArtworkID: 42}); err != nil {
			t.Fatalf("Artwork with nil logger: %v", err)
		}
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
}

func cursorWithPayload(t *testing.T, cursor sdk.Cursor, payload string) sdk.Cursor {
	t.Helper()
	text, err := cursor.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText: %v", err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(string(text))
	if err != nil {
		t.Fatalf("Decode cursor: %v", err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("Unmarshal cursor: %v", err)
	}
	envelope["pl"], err = json.Marshal([]byte(payload))
	if err != nil {
		t.Fatalf("Marshal payload: %v", err)
	}
	raw, err = json.Marshal(envelope)
	if err != nil {
		t.Fatalf("Marshal cursor: %v", err)
	}
	var tampered sdk.Cursor
	if err := tampered.UnmarshalText([]byte(base64.RawURLEncoding.EncodeToString(raw))); err != nil {
		t.Fatalf("UnmarshalText: %v", err)
	}
	return tampered
}
