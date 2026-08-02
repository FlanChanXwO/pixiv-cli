package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/authdb"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixivsdk "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

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
	appDataDir := filepath.Join(home, localstate.AppDataDirName)
	db, err := authdb.Open(appDataDir)
	if err != nil {
		t.Fatalf("open authdb: %v", err)
	}
	defer db.Close()
	accounts, err := db.ListPixiv(context.Background())
	if err != nil || len(accounts) == 0 {
		t.Skip("no local pixiv account; cannot run real e2e")
	}
	account := accounts[0]

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, credentials, err := pixivsdk.Open(ctx, string(account.RefreshToken))
	if err != nil {
		t.Fatalf("pixiv.Open: %v", err)
	}
	defer client.CloseIdleConnections()
	if credentials.UserID <= 0 || credentials.AccessToken() == "" || credentials.RefreshToken() == "" {
		t.Fatal("open did not return verified credentials")
	}
	// rotation 持久化失败时不得发起内容请求。
	if err := db.RotatePixivCredentials(ctx, credentials.UserID, []byte(credentials.RefreshToken())); err != nil {
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

	page, err := client.UserArtworks(ctx, pixivsdk.UserArtworksRequest{UserID: user.User.ID, Kind: pixivsdk.ArtworkKindIllustration})
	if err != nil {
		t.Fatalf("UserArtworks: %v", err)
	}
	if len(page.Items) == 0 {
		t.Fatal("no artworks for current user")
	}
	artwork, err := client.Artwork(ctx, pixivsdk.ArtworkRequest{ArtworkID: page.Items[0].ID})
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
