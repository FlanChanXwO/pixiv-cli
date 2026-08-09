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
	fanboxE2EPostOnlyEnv    = "FANBOX_E2E_POST_ONLY"
	fanboxE2ESolverURLEnv   = "FANBOX_E2E_SOLVER_URL"
	fanboxE2ESolverProxyEnv = "FANBOX_E2E_SOLVER_PROXY"
)

type fanboxE2ETargets struct {
	CreatorID string
	Tag       string
	PostID    string
	PostURL   string
}

type fanboxE2EPostTarget struct {
	PostID  string
	PostURL string
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

func TestFanboxPostTargetRequiresOnlyPostValues(t *testing.T) {
	values := map[string]string{
		fanboxE2EPostIDEnv:  "post-1",
		fanboxE2EPostURLEnv: "https://www.fanbox.cc/@creator-1/posts/post-1",
	}
	values[fanboxE2EPostIDEnv] = ""
	if _, err := fanboxPostTargetFrom(func(name string) string { return values[name] }); err == nil {
		t.Fatal("fanbox post-only target accepted a missing post ID")
	}

	values[fanboxE2EPostIDEnv] = "post-1"
	target, err := fanboxPostTargetFrom(func(name string) string { return values[name] })
	if err != nil {
		t.Fatalf("fanbox post-only target rejected explicit values: %v", err)
	}
	if target.PostID != "post-1" || target.PostURL == "" {
		t.Fatalf("fanbox post-only target = %+v", target)
	}

	values[fanboxE2EPostURLEnv] += "?signed=1"
	if _, err := fanboxPostTargetFrom(func(name string) string { return values[name] }); err == nil {
		t.Fatal("fanbox post-only target accepted a signed-query post URL")
	}
}

func fanboxE2ETargetsFrom(getenv func(string) string) (fanboxE2ETargets, error) {
	targets := fanboxE2ETargets{}
	var err error
	if targets.CreatorID, err = fanboxE2EValueFrom(getenv, fanboxE2ECreatorIDEnv); err != nil {
		return fanboxE2ETargets{}, err
	}
	if targets.Tag, err = fanboxE2EValueFrom(getenv, fanboxE2ETagEnv); err != nil {
		return fanboxE2ETargets{}, err
	}
	if targets.PostID, err = fanboxE2EValueFrom(getenv, fanboxE2EPostIDEnv); err != nil {
		return fanboxE2ETargets{}, err
	}
	if targets.PostURL, err = fanboxE2EPageURLFrom(getenv, fanboxE2EPostURLEnv); err != nil {
		return fanboxE2ETargets{}, err
	}
	return targets, nil
}

func fanboxPostTargetFrom(getenv func(string) string) (fanboxE2EPostTarget, error) {
	target := fanboxE2EPostTarget{}
	var err error
	if target.PostID, err = fanboxE2EValueFrom(getenv, fanboxE2EPostIDEnv); err != nil {
		return fanboxE2EPostTarget{}, err
	}
	if target.PostURL, err = fanboxE2EPageURLFrom(getenv, fanboxE2EPostURLEnv); err != nil {
		return fanboxE2EPostTarget{}, err
	}
	return target, nil
}

func fanboxE2EValueFrom(getenv func(string) string, name string) (string, error) {
	value := strings.TrimSpace(getenv(name))
	if value == "" {
		return "", fmt.Errorf("%s is required for the real FANBOX SDK e2e", name)
	}
	if strings.IndexFunc(value, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return "", fmt.Errorf("%s contains a control character", name)
	}
	return value, nil
}

func fanboxE2EPageURLFrom(getenv func(string) string, name string) (string, error) {
	value, err := fanboxE2EValueFrom(getenv, name)
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Hostname() != "www.fanbox.cc" && parsed.Hostname() != "fanbox.cc") || parsed.Path == "" {
		return "", fmt.Errorf("%s must be an unsigned HTTPS FANBOX page URL", name)
	}
	return value, nil
}

func fanboxE2EOptionsFrom(getenv func(string) string) fanboxsdk.Options {
	options := fanboxsdk.Options{}
	if proxy := getenv("PIXIV_E2E_PROXY"); proxy != "" {
		options.ProxyURL = proxy
	}
	if solverURL := strings.TrimSpace(getenv(fanboxE2ESolverURLEnv)); solverURL != "" {
		options.FlareSolverr = &fanboxsdk.FlareSolverrOptions{
			URL:      solverURL,
			ProxyURL: strings.TrimSpace(getenv(fanboxE2ESolverProxyEnv)),
		}
	}
	return options
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

	client, err := fanboxsdk.OpenWith(
		fanboxsdk.SessionCredentials{FANBOXSESSID: session},
		fanboxE2EOptionsFrom(os.Getenv),
	)
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
	var fileAssets []fanboxsdk.Asset
	for _, asset := range post.Body.Assets {
		if asset.Kind == fanboxsdk.AssetKindFile {
			fileAssets = append(fileAssets, asset)
		}
	}
	if len(fileAssets) == 0 {
		t.Fatal("explicit post target has no file attachment")
	}

	reference, err := client.ResolveURL(ctx, fanboxsdk.ResolveURLRequest{RawURL: targets.PostURL})
	if err != nil {
		t.Fatalf("ResolveURL: %v", err)
	}
	if reference.Kind != fanboxsdk.ReferenceKindPost || reference.PostID != targets.PostID {
		t.Fatalf("ResolveURL returned kind=%q post=%q", reference.Kind, reference.PostID)
	}

	resourceDir := t.TempDir()
	for index, fileAsset := range fileAssets {
		if fileAsset.Resource.Ref.IsZero() || fileAsset.Resource.URL == "" {
			t.Fatalf("file attachment %d resource has no ref or url", index)
		}

		response, err := client.OpenResource(ctx, sdk.OpenResourceRequest{Ref: fileAsset.Resource.Ref, Method: sdk.ResourceMethodHead})
		if err != nil {
			t.Fatalf("OpenResource HEAD for file attachment %d: %v", index, err)
		}
		expected := response.ContentLength()
		if response.Body != nil {
			_ = response.Body.Close()
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			t.Fatalf("resource HEAD status for file attachment %d = %d", index, response.StatusCode)
		}

		outputPath := filepath.Join(resourceDir, fmt.Sprintf("fanbox-e2e-file-resource-%d.bin", index))
		saved, err := client.SaveResource(ctx, fileAsset.Resource.Ref, sdk.SaveOptions{Path: outputPath})
		if err != nil {
			t.Fatalf("SaveResource for file attachment %d: %v", index, err)
		}
		if saved.Size <= 0 {
			t.Fatalf("SaveResource wrote no bytes for file attachment %d", index)
		}
		info, err := os.Stat(outputPath)
		if err != nil {
			t.Fatalf("stat saved resource %d: %v", index, err)
		}
		if info.Size() != saved.Size {
			t.Fatalf("saved resource %d size = %d, reported size = %d", index, info.Size(), saved.Size)
		}
		if expected > 0 && expected != saved.Size {
			t.Fatalf("saved resource %d size = %d, HEAD content length = %d", index, saved.Size, expected)
		}
	}
}

// TestRealFanboxSDKPostInfo verifies one explicit post.info target without
// requiring a file attachment. A successful post may legitimately expose zero
// first-party file assets, as with public article posts.
func TestRealFanboxSDKPostInfo(t *testing.T) {
	if os.Getenv("FANBOX_SDK_E2E") != "1" {
		t.Skip("set FANBOX_SDK_E2E=1 to run the real FANBOX SDK e2e")
	}
	if os.Getenv(fanboxE2EPostOnlyEnv) != "1" {
		t.Skip("set FANBOX_E2E_POST_ONLY=1 to run the post-only FANBOX SDK e2e")
	}
	target, err := fanboxPostTargetFrom(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	session := readFanboxKeychainSession(t)
	client, err := fanboxsdk.OpenWith(
		fanboxsdk.SessionCredentials{FANBOXSESSID: session},
		fanboxE2EOptionsFrom(os.Getenv),
	)
	if err != nil {
		t.Fatalf("fanbox.Open: %v", err)
	}
	defer client.CloseIdleConnections()

	ctx := t.Context()
	post, err := client.Post(ctx, fanboxsdk.PostRequest{PostID: target.PostID})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if post.ID != target.PostID {
		t.Fatalf("Post returned id=%q, want %q", post.ID, target.PostID)
	}
	if post.Body == nil {
		t.Fatal("Post returned no body for the explicit post.info target")
	}
	fileAssets := 0
	for _, asset := range post.Body.Assets {
		if asset.Kind == fanboxsdk.AssetKindFile {
			fileAssets++
		}
	}
	t.Logf("post.info body verified; assets=%d file_assets=%d", len(post.Body.Assets), fileAssets)

	reference, err := client.ResolveURL(ctx, fanboxsdk.ResolveURLRequest{RawURL: target.PostURL})
	if err != nil {
		t.Fatalf("ResolveURL: %v", err)
	}
	if reference.Kind != fanboxsdk.ReferenceKindPost || reference.PostID != target.PostID {
		t.Fatalf("ResolveURL returned kind=%q post=%q", reference.Kind, reference.PostID)
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
