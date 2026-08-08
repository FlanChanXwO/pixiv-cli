package pixiv

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func oauthResponse() *http.Response {
	body := `{"access_token":"new-access-token","refresh_token":"new-refresh-token","expires_in":3600,"user":{"id":42,"name":"tester"}}`
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func oauthResponseWithoutIdentity() *http.Response {
	body := `{"access_token":"new-access-token","refresh_token":"new-refresh-token","expires_in":3600,"user":{"name":"tester"}}`
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestNewRejectsEmptyToken(t *testing.T) {
	if _, err := New(""); sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
	if _, err := NewWith("  ", Options{}); sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}

func TestNewClientStatic(t *testing.T) {
	client, err := New("access-token")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if client.UserID() != 0 {
		t.Fatalf("UserID = %d, want 0 for New", client.UserID())
	}
	if client.app == nil || client.resClient == nil {
		t.Fatal("client adapters not wired")
	}
}

func TestOpenRotatesCredentials(t *testing.T) {
	var captured url.Values
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "oauth.secure.pixiv.net" {
			_ = req.ParseForm()
			captured = req.PostForm
			return oauthResponse(), nil
		}
		return nil, errors.New("unexpected host " + req.URL.Host)
	})
	httpClient := &http.Client{Transport: rt}
	client, creds, err := OpenWith(context.Background(), "old-refresh-token", Options{HTTPClient: httpClient})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if client.UserID() != 42 || client.Username() != "tester" {
		t.Fatalf("client identity = %d/%q", client.UserID(), client.Username())
	}
	if creds.AccessToken() != "new-access-token" || creds.RefreshToken() != "new-refresh-token" {
		t.Fatalf("credentials not rotated: access=%q refresh=%q", creds.AccessToken(), creds.RefreshToken())
	}
	if creds.ExpiresAt.IsZero() {
		t.Fatal("expiry not captured")
	}
	if captured.Get("grant_type") != "refresh_token" {
		t.Fatalf("grant_type = %q", captured.Get("grant_type"))
	}
}

func TestOpenRejectsMissingAccountIdentity(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "oauth.secure.pixiv.net" {
			return oauthResponseWithoutIdentity(), nil
		}
		return nil, errors.New("unexpected host " + req.URL.Host)
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

func TestCredentialsRedactTokens(t *testing.T) {
	creds := Credentials{
		UserID:       42,
		Username:     "tester",
		ExpiresAt:    time.Unix(0, 0),
		accessToken:  "secret-access",
		refreshToken: "secret-refresh",
	}
	for _, s := range []string{creds.String(), creds.GoString()} {
		if strings.Contains(s, "secret-access") || strings.Contains(s, "secret-refresh") {
			t.Fatalf("token leaked in %q", s)
		}
	}
	data, err := json.Marshal(creds)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if strings.Contains(string(data), "secret-access") || strings.Contains(string(data), "secret-refresh") {
		t.Fatalf("token leaked in JSON %q", data)
	}
}

func TestQueryDigestStable(t *testing.T) {
	q1 := url.Values{"word": {"test"}, "sort": {"date_desc"}}
	q2 := url.Values{"sort": {"date_desc"}, "word": {"test"}}
	if queryDigest(q1) != queryDigest(q2) {
		t.Fatal("query digest should be order-independent")
	}
	q3 := url.Values{"word": {"test"}, "sort": {"date_asc"}}
	if queryDigest(q1) == queryDigest(q3) {
		t.Fatal("query digest should differ for different values")
	}
}

func TestCursorContinuationRoundTrip(t *testing.T) {
	client, err := NewWith("token", Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	base := url.Values{"word": {"test"}}
	cur, err := client.buildCursor("SearchArtworks", base, "offset", 30, true)
	if err != nil {
		t.Fatalf("buildCursor: %v", err)
	}
	if cur.IsZero() {
		t.Fatal("cursor unexpectedly zero")
	}
	offset, err := client.continuationOffset("SearchArtworks", base, cur)
	if err != nil {
		t.Fatalf("continuationOffset: %v", err)
	}
	if offset != 30 {
		t.Fatalf("offset = %d, want 30", offset)
	}
}

func TestCursorRejectsQueryMismatch(t *testing.T) {
	client, _ := New("token")
	base := url.Values{"word": {"test"}}
	cur, _ := client.buildCursor("SearchArtworks", base, "offset", 30, true)
	if _, err := client.continuationOffset("SearchArtworks", url.Values{"word": {"other"}}, cur); sdk.ReasonOf(err) != sdk.InvalidCursor {
		t.Fatalf("expected InvalidCursor for query mismatch, got %v", err)
	}
	if _, err := client.continuationOffset("Artwork", base, cur); sdk.ReasonOf(err) != sdk.InvalidCursor {
		t.Fatalf("expected InvalidCursor for operation mismatch, got %v", err)
	}
}

func TestCursorZeroIsNoContinuation(t *testing.T) {
	client, _ := New("token")
	base := url.Values{"word": {"test"}}
	offset, err := client.continuationOffset("SearchArtworks", base, sdk.Cursor{})
	if err != nil || offset != 0 {
		t.Fatalf("zero cursor: offset=%d err=%v", offset, err)
	}
}
