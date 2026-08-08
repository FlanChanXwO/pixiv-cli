package e2e

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	fanboxsdk "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
)

// fanboxE2EKeychainService/Account 是 release-prep 约定的 Keychain 条目。
const (
	fanboxE2EKeychainService = "pixiv-cli-e2e-fanbox"
	fanboxE2EKeychainAccount = "fanbox-e2e"
)

const (
	fanboxE2ECreatorIDEnv   = "FANBOX_E2E_CREATOR_ID"
	fanboxE2ETagEnv         = "FANBOX_E2E_TAG"
	fanboxE2EPostIDEnv      = "FANBOX_E2E_POST_ID"
	fanboxE2EPostURLEnv     = "FANBOX_E2E_POST_URL"
	fanboxE2ESolverURLEnv   = "FANBOX_E2E_SOLVER_URL"
	fanboxE2ESolverProxyEnv = "FANBOX_E2E_SOLVER_PROXY"
)

type fanboxE2ETargets struct {
	CreatorID string
	Tag       string
	PostID    string
	PostURL   string
}

func TestFanboxE2ETargetsRequireExplicitValues(t *testing.T) {
	values := map[string]string{
		"FANBOX_E2E_CREATOR_ID": "creator-1",
		"FANBOX_E2E_TAG":        "tag-1",
		"FANBOX_E2E_POST_ID":    "post-1",
		"FANBOX_E2E_POST_URL":   "https://www.fanbox.cc/@creator-1/posts/post-1",
	}
	values["FANBOX_E2E_TAG"] = ""
	if _, err := fanboxE2ETargetsFrom(func(name string) string { return values[name] }); err == nil {
		t.Fatal("fanbox E2E targets accepted a missing explicit target")
	}

	values["FANBOX_E2E_TAG"] = "tag-1"
	targets, err := fanboxE2ETargetsFrom(func(name string) string { return values[name] })
	if err != nil {
		t.Fatalf("fanbox E2E targets rejected explicit non-secret values: %v", err)
	}
	if targets.CreatorID != "creator-1" || targets.Tag != "tag-1" || targets.PostID != "post-1" || targets.PostURL == "" {
		t.Fatalf("fanbox E2E targets = %+v", targets)
	}

	values["FANBOX_E2E_POST_URL"] = "https://www.fanbox.cc/@creator-1/posts/post-1?signed=1"
	if _, err := fanboxE2ETargetsFrom(func(name string) string { return values[name] }); err == nil {
		t.Fatal("fanbox E2E targets accepted a signed-query post URL")
	}
}

func fanboxE2ETargetsFrom(getenv func(string) string) (fanboxE2ETargets, error) {
	targets := fanboxE2ETargets{}
	values := []struct {
		name  string
		value *string
	}{
		{name: fanboxE2ECreatorIDEnv, value: &targets.CreatorID},
		{name: fanboxE2ETagEnv, value: &targets.Tag},
		{name: fanboxE2EPostIDEnv, value: &targets.PostID},
		{name: fanboxE2EPostURLEnv, value: &targets.PostURL},
	}
	for _, item := range values {
		value := strings.TrimSpace(getenv(item.name))
		if value == "" {
			return fanboxE2ETargets{}, fmt.Errorf("%s is required for the real FANBOX SDK e2e", item.name)
		}
		if strings.IndexFunc(value, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
			return fanboxE2ETargets{}, fmt.Errorf("%s contains a control character", item.name)
		}
		if item.name == fanboxE2EPostURLEnv {
			parsed, err := url.Parse(value)
			if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
				(parsed.Hostname() != "www.fanbox.cc" && parsed.Hostname() != "fanbox.cc") || parsed.Path == "" {
				return fanboxE2ETargets{}, fmt.Errorf("%s must be an unsigned HTTPS FANBOX page URL", item.name)
			}
		}
		*item.value = value
	}
	return targets, nil
}

func TestFollowFanboxPostPageAllowsTerminalPage(t *testing.T) {
	called := false
	followFanboxPostPage(t, "terminal", sdk.Page[fanboxsdk.Post]{
		Items: []fanboxsdk.Post{{ID: "post-1"}},
	}, func(sdk.Cursor) (sdk.Page[fanboxsdk.Post], error) {
		called = true
		return sdk.Page[fanboxsdk.Post]{}, nil
	})
	if called {
		t.Fatal("terminal FANBOX post page unexpectedly followed a missing cursor")
	}
}

// TestRealFanboxSDKRead is the v1 release-prep real FANBOX SDK e2e. It runs only
// when FANBOX_SDK_E2E=1 and the authorized Keychain item exists. The session is
// read through the Keychain API and never enters argv, env, logs, or test names.
func TestRealFanboxSDKRead(t *testing.T) {
	if os.Getenv("FANBOX_SDK_E2E") != "1" {
		t.Skip("set FANBOX_SDK_E2E=1 to run the real FANBOX SDK e2e")
	}
	targets, err := fanboxE2ETargetsFrom(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	session := readFanboxKeychainSession(t)

	ctx := t.Context()

	options := fanboxsdk.Options{}
	if proxy := os.Getenv("PIXIV_E2E_PROXY"); proxy != "" {
		options.ProxyURL = proxy
	}
	if solverURL := strings.TrimSpace(os.Getenv(fanboxE2ESolverURLEnv)); solverURL != "" {
		options.FlareSolverr = &fanboxsdk.FlareSolverrOptions{
			URL:      solverURL,
			ProxyURL: strings.TrimSpace(os.Getenv(fanboxE2ESolverProxyEnv)),
		}
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

	if _, err := client.Creator(ctx, fanboxsdk.CreatorRequest{CreatorID: targets.CreatorID}); err != nil {
		t.Fatalf("Creator: %v", err)
	}
	if _, err := client.CreatorTags(ctx, fanboxsdk.CreatorTagsRequest{CreatorID: targets.CreatorID}); err != nil {
		t.Fatalf("CreatorTags: %v", err)
	}
	for _, kind := range []fanboxsdk.CreatorListKind{fanboxsdk.CreatorListSupporting, fanboxsdk.CreatorListFollowing} {
		creatorPage, err := client.Creators(ctx, fanboxsdk.CreatorsRequest{Kind: kind})
		if err != nil {
			t.Fatalf("Creators(%s): %v", kind, err)
		}
		followFanboxPageIfPresent(t, fmt.Sprintf("Creators(%s)", kind), creatorPage, func(cursor sdk.Cursor) (sdk.Page[fanboxsdk.CreatorSummary], error) {
			return client.Creators(ctx, fanboxsdk.CreatorsRequest{Kind: kind, Cursor: cursor})
		})
	}
	homePage, err := client.Home(ctx, fanboxsdk.HomeRequest{})
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	followFanboxPageIfPresent(t, "Home", homePage, func(cursor sdk.Cursor) (sdk.Page[fanboxsdk.Post], error) {
		return client.Home(ctx, fanboxsdk.HomeRequest{Cursor: cursor})
	})
	supportingPage, err := client.Supporting(ctx, fanboxsdk.SupportingRequest{})
	if err != nil {
		t.Fatalf("Supporting: %v", err)
	}
	followFanboxPageIfPresent(t, "Supporting", supportingPage, func(cursor sdk.Cursor) (sdk.Page[fanboxsdk.Post], error) {
		return client.Supporting(ctx, fanboxsdk.SupportingRequest{Cursor: cursor})
	})

	creatorPage, err := client.CreatorPosts(ctx, fanboxsdk.CreatorPostsRequest{CreatorID: targets.CreatorID})
	if err != nil {
		t.Fatalf("CreatorPosts: %v", err)
	}
	followFanboxPostPage(t, "CreatorPosts", creatorPage, func(cursor sdk.Cursor) (sdk.Page[fanboxsdk.Post], error) {
		return client.CreatorPosts(ctx, fanboxsdk.CreatorPostsRequest{CreatorID: targets.CreatorID, Cursor: cursor})
	})
	taggedPage, err := client.TaggedPosts(ctx, fanboxsdk.TaggedPostsRequest{CreatorID: targets.CreatorID, Tag: targets.Tag})
	if err != nil {
		t.Fatalf("TaggedPosts: %v", err)
	}
	// A creator may legitimately have no tagged posts. The successful API
	// response is still evidence for the explicit tag query; follow a cursor
	// only when the server returned one.
	followFanboxPageIfPresent(t, "TaggedPosts", taggedPage, func(cursor sdk.Cursor) (sdk.Page[fanboxsdk.Post], error) {
		return client.TaggedPosts(ctx, fanboxsdk.TaggedPostsRequest{CreatorID: targets.CreatorID, Tag: targets.Tag, Cursor: cursor})
	})

	post, err := client.Post(ctx, fanboxsdk.PostRequest{PostID: targets.PostID})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if post.Body == nil {
		t.Fatal("Post returned no body for the explicit file-resource target")
	}
	var fileAsset *fanboxsdk.Asset
	for index := range post.Body.Assets {
		if post.Body.Assets[index].Kind == fanboxsdk.AssetKindFile {
			fileAsset = &post.Body.Assets[index]
			break
		}
	}
	if fileAsset == nil {
		t.Fatal("explicit post target has no file attachment")
	}
	if fileAsset.Resource.Ref.IsZero() || fileAsset.Resource.URL == "" {
		t.Fatal("file attachment resource has no ref or url")
	}

	reference, err := client.ResolveURL(ctx, fanboxsdk.ResolveURLRequest{RawURL: targets.PostURL})
	if err != nil {
		t.Fatalf("ResolveURL: %v", err)
	}
	if reference.Kind != fanboxsdk.ReferenceKindPost || reference.PostID != targets.PostID {
		t.Fatalf("ResolveURL returned kind=%q post=%q", reference.Kind, reference.PostID)
	}

	response, err := client.OpenResource(ctx, sdk.OpenResourceRequest{Ref: fileAsset.Resource.Ref, Method: sdk.ResourceMethodHead})
	if err != nil {
		t.Fatalf("OpenResource HEAD: %v", err)
	}
	if response.Body != nil {
		_ = response.Body.Close()
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		t.Fatalf("resource HEAD status = %d", response.StatusCode)
	}

	outputPath := filepath.Join(t.TempDir(), "fanbox-e2e-file-resource.bin")
	saved, err := client.SaveResource(ctx, fileAsset.Resource.Ref, sdk.SaveOptions{Path: outputPath})
	if err != nil {
		t.Fatalf("SaveResource: %v", err)
	}
	if saved.Size <= 0 {
		t.Fatal("SaveResource wrote no file bytes")
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("stat saved resource: %v", err)
	}
	if info.Size() != saved.Size {
		t.Fatalf("saved resource size = %d, reported size = %d", info.Size(), saved.Size)
	}
	if expected := response.ContentLength(); expected > 0 && expected != saved.Size {
		t.Fatalf("saved resource size = %d, HEAD content length = %d", saved.Size, expected)
	}
}

// followFanboxPostPage follows exactly one server cursor to prove the public
// cursor binding and continuation request. This is an acceptance assertion,
// not a production result cap; the SDK itself keeps returning server pages.
func followFanboxPostPage(
	t *testing.T,
	operation string,
	page sdk.Page[fanboxsdk.Post],
	next func(sdk.Cursor) (sdk.Page[fanboxsdk.Post], error),
) {
	t.Helper()
	if len(page.Items) == 0 {
		t.Fatalf("%s returned no items for the explicit non-empty target", operation)
	}
	followFanboxPageIfPresent(t, operation, page, next)
}

// followFanboxPageIfPresent verifies one continuation whenever the upstream
// exposes it. The extra request is an acceptance assertion, not a production
// page cap; each public SDK method remains free to follow all server cursors.
func followFanboxPageIfPresent[T any](
	t *testing.T,
	operation string,
	page sdk.Page[T],
	next func(sdk.Cursor) (sdk.Page[T], error),
) {
	t.Helper()
	if page.Next.IsZero() {
		return
	}
	continued, err := next(page.Next)
	if err != nil {
		t.Fatalf("%s continuation: %v", operation, err)
	}
	if len(continued.Items) == 0 && continued.Next.IsZero() {
		// A valid final page may be empty only when the server has advanced past
		// its last item; the continuation request itself is the evidence here.
		return
	}
}

// readFanboxKeychainSession 通过 macOS Keychain 读取授权的 FANBOXSESSID。
func readFanboxKeychainSession(t *testing.T) string {
	t.Helper()
	output, err := exec.Command("security", "find-generic-password",
		"-s", fanboxE2EKeychainService, "-a", fanboxE2EKeychainAccount, "-w").Output()
	if err != nil {
		t.Fatalf("fanbox e2e keychain item unavailable: %v", err)
	}
	value := strings.TrimSpace(string(output))
	if value == "" {
		t.Fatal("fanbox e2e keychain item is empty")
	}
	return value
}
