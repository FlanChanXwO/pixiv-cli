package pixiv_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	storageauth "github.com/FlanChanXwO/pixiv-cli/internal/storage/auth"
	"github.com/FlanChanXwO/pixiv-cli/pixiv"
)

func TestBookmarkSearchBlocksNonPremiumAccountAndPersistsMembershipCache(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	configPath := filepath.Join(dir, "config.toml")
	requireNoError(t, storageauth.SaveAuthStore(authPath, storageauth.AuthStore{
		DefaultUserID: 7,
		Accounts:      []storageauth.Account{{UserID: 7, RefreshToken: "stored-refresh-token"}},
	}))
	requireNoError(t, os.WriteFile(configPath, []byte("[premium]\nstatus_cache_ttl = \"24h\"\n"), 0o600))

	var oauthCalls, profileCalls, searchCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/token":
			oauthCalls.Add(1)
			_, _ = w.Write([]byte(`{"access_token":"access","refresh_token":"rotated-refresh-token","user":{"id":7,"name":"cached-user"}}`))
		case "/v1/user/detail":
			profileCalls.Add(1)
			if r.URL.Query().Get("user_id") != "7" {
				t.Fatalf("profile user_id=%q", r.URL.Query().Get("user_id"))
			}
			_, _ = w.Write([]byte(`{"user":{"id":7},"profile":{"is_premium":false},"profile_publicity":{},"workspace":{}}`))
		case "/v1/search/illust":
			searchCalls.Add(1)
			http.Error(w, "bookmark search must be blocked locally", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := pixiv.OpenDefaultWith(pixiv.OpenDefaultOptions{
		AuthFilePath: authPath, ConfigFilePath: configPath, HTTPClient: server.Client(),
		OAuthBaseURL: server.URL, AppAPIBaseURL: server.URL,
	})
	requireNoError(t, err)
	minimum := 1
	request := pixiv.SearchIllustRequest{Word: "miku", Filters: pixiv.SearchIllustFilters{BookmarkMin: &minimum}}

	for attempt := 0; attempt < 2; attempt++ {
		result, err := client.SearchIllust(context.Background(), request)
		if result != nil || !errors.Is(err, pixiv.ErrForbidden) {
			t.Fatalf("attempt %d result=%#v err=%v", attempt, result, err)
		}
	}
	if oauthCalls.Load() != 2 || profileCalls.Load() != 1 || searchCalls.Load() != 0 {
		t.Fatalf("oauth=%d profile=%d search=%d", oauthCalls.Load(), profileCalls.Load(), searchCalls.Load())
	}
	store, err := storageauth.LoadAuthStore(authPath)
	requireNoError(t, err)
	_, account, ok := store.Get(7)
	if !ok || account.PremiumStatus == nil || *account.PremiumStatus || account.PremiumStatusCheckedAt == nil {
		t.Fatalf("stored premium cache=%#v", account)
	}
}

func TestAccountCredentialRefreshPreservesPremiumStatusCache(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	premium := true
	checkedAt := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
	requireNoError(t, storageauth.SaveAuthStore(authPath, storageauth.AuthStore{
		DefaultUserID: 7,
		Accounts: []storageauth.Account{{
			UserID: 7, RefreshToken: "stored-refresh-token", PremiumStatus: &premium, PremiumStatusCheckedAt: &checkedAt,
		}},
	}))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/token" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"access","refresh_token":"rotated-refresh-token","user":{"id":7,"name":"refreshed-user"}}`))
	}))
	defer server.Close()
	client, err := pixiv.OpenDefaultWith(pixiv.OpenDefaultOptions{AuthFilePath: authPath, HTTPClient: server.Client(), OAuthBaseURL: server.URL})
	requireNoError(t, err)

	_, err = client.CheckAccount(context.Background(), 7)
	requireNoError(t, err)
	store, err := storageauth.LoadAuthStore(authPath)
	requireNoError(t, err)
	_, account, ok := store.Get(7)
	if !ok || account.PremiumStatus == nil || !*account.PremiumStatus || account.PremiumStatusCheckedAt == nil || !account.PremiumStatusCheckedAt.Equal(checkedAt) {
		t.Fatalf("premium cache was lost: %#v", account)
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
