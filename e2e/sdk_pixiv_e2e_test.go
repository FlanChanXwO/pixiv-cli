package e2e

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/application/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/filesystem"
	"github.com/FlanChanXwO/pixiv-cli/internal/persistence/authdb"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixivsdk "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestPixivE2ERotationUsesStoredRevision(t *testing.T) {
	account := authdb.PixivAccount{UserID: 42, CredentialRevision: 7}
	var gotUserID, gotRevision int64
	var gotToken []byte
	rotate := func(_ context.Context, userID, revision int64, token []byte) error {
		gotUserID = userID
		gotRevision = revision
		gotToken = append([]byte(nil), token...)
		return nil
	}

	if err := persistPixivE2ERotation(context.Background(), rotate, account, "rotated-refresh"); err != nil {
		t.Fatalf("persistPixivE2ERotation: %v", err)
	}
	if gotUserID != account.UserID || gotRevision != account.CredentialRevision || string(gotToken) != "rotated-refresh" {
		t.Fatalf("rotation arguments = user=%d revision=%d token=%q", gotUserID, gotRevision, gotToken)
	}
}

// persistPixivE2ERotation keeps the release-prep path on the same CAS contract
// as the application service: the expected revision comes from the account
// read that supplied the refresh token, never from a fixed initial value.
func persistPixivE2ERotation(
	ctx context.Context,
	rotate func(context.Context, int64, int64, []byte) error,
	account authdb.PixivAccount,
	refreshToken string,
) error {
	return rotate(ctx, account.UserID, account.CredentialRevision, []byte(refreshToken))
}

// TestRealPixivSDKRead is the v1 release-prep real Pixiv SDK e2e. It runs only
// when PIXIV_SDK_E2E=1 and a local pixiv-cli.db account exists. The refresh
// token is read from the auth database inside the test process and never enters
// argv, env dumps, logs, test names, or diffs. Rotated credentials are persisted
// back through the repository before any content request.
func TestRealPixivSDKRead(t *testing.T) {
	if os.Getenv("PIXIV_SDK_E2E") != "1" {
		t.Skip("set PIXIV_SDK_E2E=1 to run the real Pixiv SDK e2e")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	appDataDir := filepath.Join(home, filesystem.AppDataDirName)
	db, err := authdb.Open(appDataDir)
	if err != nil {
		t.Fatalf("open authdb: %v", err)
	}
	defer db.Close()
	accounts, err := db.ListPixiv(context.Background())
	if err != nil {
		t.Fatalf("list local pixiv accounts: %v", err)
	}
	if len(accounts) == 0 {
		t.Fatal("no local pixiv account; explicit real e2e has no credential source")
	}
	account := accounts[0]
	defaultID, hasDefault, err := config.ReadPixivDefaultUserID()
	if err != nil {
		t.Fatalf("read pixiv default account: %v", err)
	}
	if hasDefault {
		found := false
		for _, candidate := range accounts {
			if candidate.UserID == defaultID {
				account = candidate
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("configured pixiv account %d is not present in authdb", defaultID)
		}
	}

	ctx := t.Context()

	options := pixivsdk.Options{}
	if proxy := os.Getenv("PIXIV_E2E_PROXY"); proxy != "" {
		proxyURL, err := url.Parse(proxy)
		if err != nil {
			t.Fatalf("parse proxy: %v", err)
		}
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = http.ProxyURL(proxyURL)
		options.HTTPClient = &http.Client{Transport: transport}
	}
	client, credentials, err := pixivsdk.OpenWith(ctx, string(account.RefreshToken), options)
	if err != nil {
		t.Fatalf("pixiv.Open: %v", err)
	}
	defer client.CloseIdleConnections()
	if credentials.UserID <= 0 || credentials.AccessToken() == "" || credentials.RefreshToken() == "" {
		t.Fatal("open did not return verified credentials")
	}
	if credentials.UserID != account.UserID {
		t.Fatalf("open returned a different account identity: stored user %d, verified user %d", account.UserID, credentials.UserID)
	}
	// rotation 持久化失败时不得发起内容请求。
	if err := persistPixivE2ERotation(ctx, db.RotatePixivCredentials, account, credentials.RefreshToken()); err != nil {
		t.Fatalf("persist rotation: %v", err)
	}

	// 验证身份 + 一个稳定 detail + 一个 Resource HEAD。
	user, err := client.CurrentUser(ctx, pixivsdk.CurrentUserRequest{})
	if err != nil {
		t.Fatalf("CurrentUser: %v", err)
	}
	if user.User.ID <= 0 {
		t.Fatal("current user has no id")
	}
	if user.User.ID != account.UserID {
		t.Fatalf("current user identity differs from selected account: stored user %d, current user %d", account.UserID, user.User.ID)
	}

	searchPage, err := client.SearchArtworks(ctx, pixivsdk.SearchArtworksRequest{Word: "初音ミク"})
	if err != nil {
		t.Fatalf("SearchArtworks: %v", err)
	}
	if len(searchPage.Items) == 0 {
		t.Fatal("search returned no artworks")
	}
	artwork, err := client.Artwork(ctx, pixivsdk.ArtworkRequest{ArtworkID: searchPage.Items[0].ID})
	if err != nil {
		t.Fatalf("Artwork: %v", err)
	}
	if artwork.ID <= 0 || len(artwork.Pages) == 0 {
		t.Fatalf("artwork detail incomplete: %+v", artwork)
	}
	resource := artwork.Pages[0].Image.Resource
	if resource.Ref.IsZero() || resource.URL == "" {
		t.Fatal("page resource has no ref or url")
	}
	response, err := client.OpenResource(ctx, sdk.OpenResourceRequest{Ref: resource.Ref, Method: sdk.ResourceMethodHead})
	if err != nil {
		t.Fatalf("OpenResource HEAD: %v", err)
	}
	if response.Body != nil {
		_ = response.Body.Close()
	}
	if response.StatusCode != 200 {
		t.Fatalf("resource HEAD status = %d", response.StatusCode)
	}
}
