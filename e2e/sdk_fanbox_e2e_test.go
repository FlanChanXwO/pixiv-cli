package e2e

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	fanboxsdk "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
)

// fanboxE2EKeychainService/Account 是 release-prep 约定的 Keychain 条目。
const (
	fanboxE2EKeychainService = "pixiv-cli-e2e-fanbox"
	fanboxE2EKeychainAccount = "fanbox-e2e"
)

// TestRealFanboxSDKRead is the v1 release-prep real FANBOX SDK e2e. It runs only
// when FANBOX_SDK_E2E=1 and the authorized Keychain item exists. The session is
// read through the Keychain API and never enters argv, env, logs, or test names.
func TestRealFanboxSDKRead(t *testing.T) {
	if os.Getenv("FANBOX_SDK_E2E") != "1" {
		t.Skip("set FANBOX_SDK_E2E=1 to run the real FANBOX SDK e2e")
	}
	session := readFanboxKeychainSession(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	options := fanboxsdk.Options{}
	if proxy := os.Getenv("PIXIV_WEB_API_PROXY"); proxy != "" {
		options.ProxyURL = proxy
	}
	client, err := fanboxsdk.OpenWith(fanboxsdk.SessionCredentials{FANBOXSESSID: session}, options)
	if err != nil {
		t.Fatalf("fanbox.Open: %v", err)
	}
	defer client.CloseIdleConnections()

	if err := client.ValidateSession(ctx); err != nil {
		t.Fatalf("ValidateSession: %v", err)
	}
	user, err := client.CurrentUser(ctx, fanboxsdk.CurrentUserRequest{})
	if err != nil {
		t.Fatalf("CurrentUser: %v", err)
	}
	if user.UserID <= 0 || user.DisplayName == "" {
		t.Fatalf("current user incomplete: %+v", user)
	}

	// 读取一个稳定 detail/list 与至少一个第一方 Resource。
	var page sdk.Page[fanboxsdk.Post]
	if user.CreatorID != "" {
		page, err = client.CreatorPosts(ctx, fanboxsdk.CreatorPostsRequest{CreatorID: user.CreatorID})
	} else {
		page, err = client.Home(ctx, fanboxsdk.HomeRequest{})
	}
	if err != nil {
		t.Fatalf("post list: %v", err)
	}
	if len(page.Items) == 0 {
		t.Fatal("no posts available for real e2e")
	}
	post, err := client.Post(ctx, fanboxsdk.PostRequest{PostID: page.Items[0].ID})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if post.Body == nil || len(post.Body.Assets) == 0 {
		t.Skip("first post has no assets; skipping resource read")
	}
	asset := post.Body.Assets[0]
	if asset.Resource.Ref.IsZero() || asset.Resource.URL == "" {
		t.Fatal("asset resource has no ref or url")
	}
	response, err := client.OpenResource(ctx, sdk.OpenResourceRequest{Ref: asset.Resource.Ref, Method: sdk.ResourceMethodHead})
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

// readFanboxKeychainSession 通过 macOS Keychain 读取授权的 FANBOXSESSID。
func readFanboxKeychainSession(t *testing.T) string {
	t.Helper()
	output, err := exec.Command("security", "find-generic-password",
		"-s", fanboxE2EKeychainService, "-a", fanboxE2EKeychainAccount, "-w").Output()
	if err != nil {
		t.Skipf("fanbox e2e keychain item unavailable: %v", err)
	}
	value := strings.TrimSpace(string(output))
	if value == "" {
		t.Fatal("fanbox e2e keychain item is empty")
	}
	return value
}
