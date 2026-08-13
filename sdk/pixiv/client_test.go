package pixiv_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	. "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
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
	if client == nil {
		t.Fatal("New returned a nil client")
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
