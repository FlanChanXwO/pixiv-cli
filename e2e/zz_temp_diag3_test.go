package e2e

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	config "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
	pixivsdk "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

type diag3Transport struct {
	base http.RoundTripper
	logf func(string, ...any)
}

func (t *diag3Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	if strings.Contains(req.URL.Path, "bookmark/detail") {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		head := string(body)
		if len(head) > 500 {
			head = head[:500]
		}
		t.logf("[diag3] %s status=%d body=%s", req.URL.Path, resp.StatusCode, head)
		resp.Body = io.NopCloser(strings.NewReader(string(body)))
	}
	return resp, nil
}

func TestZZTempBookmarkDetailDiag(t *testing.T) {
	if os.Getenv("PIXIV_SDK_E2E") != "1" {
		t.Skip("diag")
	}
	ctx := context.Background()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	db, err := database.Open(filepath.Join(home, paths.AppDataDirName))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	accounts, err := db.ListPixiv(ctx)
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	account := accounts[0]
	defaultID, hasDefault, err := config.ReadPixivDefaultUserID()
	if err == nil && hasDefault {
		for _, candidate := range accounts {
			if candidate.UserID == defaultID {
				account = candidate
				break
			}
		}
	}
	options := pixivsdk.Options{}
	if proxy := os.Getenv("PIXIV_E2E_PROXY"); proxy != "" {
		proxyURL, err := url.Parse(proxy)
		if err != nil {
			t.Fatalf("parse proxy: %v", err)
		}
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = http.ProxyURL(proxyURL)
		options.HTTPClient = &http.Client{Transport: &diag3Transport{base: transport, logf: t.Logf}}
	}
	client, credentials, err := pixivsdk.OpenWith(ctx, string(account.RefreshTokenCopy()), options)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer client.CloseIdleConnections()
	if err := db.RotatePixivCredentials(ctx, account.UserID, account.CredentialRevision, []byte(credentials.RefreshToken())); err != nil {
		t.Fatalf("persist rotation: %v", err)
	}
	search, err := client.SearchArtworks(ctx, pixivsdk.SearchArtworksRequest{Word: "風景", ContentType: pixivsdk.SearchContentTypeIllust})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	absentID := search.Items[len(search.Items)-1].ID
	_, err = client.ArtworkBookmark(ctx, pixivsdk.ArtworkBookmarkRequest{ArtworkID: absentID})
	t.Logf("absent artwork bookmark detail err = %v", err)
}
