package download

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	commands "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandRejectsMissingSourceBeforeResolvingServices(t *testing.T) {
	serviceCalls := 0
	cmd := newTestCommand(testData{
		Reader:    bytes.NewReader(nil),
		Writer:    &bytes.Buffer{},
		WrapUsage: func(err error) error { return err },
		Service: func() (*session.Fanbox, error) {
			serviceCalls++
			return nil, errors.New("service must not be resolved")
		},
	})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(nil)

	err := cmd.Execute()

	require.EqualError(t, err, "usage: pixiv fanbox download SOURCE...")
	assert.Zero(t, serviceCalls)
}

type testData struct {
	Reader    io.Reader
	Writer    io.Writer
	WrapUsage func(error) error
	Service   func() (*session.Fanbox, error)
}

func newTestCommand(d testData) *cobra.Command {
	env := commands.Env{In: d.Reader, Out: d.Writer, Err: io.Discard, WrapUsage: d.WrapUsage}
	return New(env, d.Service, func() (string, error) { return "", nil })
}

// TestSafePathSegmentSanitizesTraversal 验证路径段清理：
// ../ 与路径分隔符在单段内被中和，空段与纯 dot/dotdot 被拒绝。
func TestSafePathSegmentSanitizesTraversal(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"normal", "normal"},
		{"../escape", ".._escape"},
		{"..", ""},
		{".", ""},
		{"", ""},
		{"  ", ""},
		{"a/b", "a_b"},
		{`a\b`, "a_b"},
		{"a:b", "a_b"},
		{"../../etc", ".._.._etc"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			require.Equal(t, tc.want, safePathSegment(tc.in))
		})
	}
}

// TestPathInsideBaseRejectsTraversal 验证最终路径包含校验：即使段被清理后，
// 一个构造的绝对路径或 ../ 逃逸也被 pathInsideBase 拒绝。
// TestSavePostAssetsKeepsAssetPathInsideDownloadDir 验证端到端
// 行为：即使 creator id、post id 或 asset name 携带 ../ 或绝对路径段，最终落盘
// 文件也位于 baseDir 之内，并且 SaveResource 成功写出该文件。
func TestSavePostAssetsKeepsAssetPathInsideDownloadDir(t *testing.T) {
	cases := []struct {
		name      string
		creatorID string
		postID    string
		assetName string
	}{
		{name: "traversal creator", creatorID: "../../etc", postID: "p1", assetName: "img.png"},
		{name: "traversal post", creatorID: "creator", postID: "../escape", assetName: "img.png"},
		{name: "traversal asset", creatorID: "creator", postID: "p1", assetName: "../../../etc/passwd"},
		{name: "absolute asset name", creatorID: "creator", postID: "p1", assetName: "/etc/passwd"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baseDir := t.TempDir()
			client := stubSavingClient(t)
			resource := stubImageResource(t, client)
			cmd := commandRunner{env: commands.Env{Out: io.Discard}}
			post := fanbox.Post{
				CreatorID: tc.creatorID,
				ID:        tc.postID,
				Body: &fanbox.PostBody{
					Assets: []fanbox.Asset{{
						ID: "asset-1", Kind: fanbox.AssetKindImage, Name: tc.assetName, Resource: resource,
					}},
				},
			}
			require.NoError(t, cmd.savePostAssets(context.Background(), client, baseDir, post, map[string]struct{}{}))
			// The saved file must exist and live inside baseDir.
			matches := findSavedFiles(t, baseDir)
			require.Len(t, matches, 1, "exactly one asset should be saved")
			require.True(t, pathInsideBase(matches[0], baseDir),
				"saved file %q escaped baseDir %q", matches[0], baseDir)
		})
	}
}

// TestSavePostAssetsRejectsEmptyPathIdentity 验证空 creator/post id 被拒绝而非
// 落盘到 baseDir 根。
func TestSavePostAssetsRejectsEmptyPathIdentity(t *testing.T) {
	baseDir := t.TempDir()
	client := stubSavingClient(t)
	resource := stubImageResource(t, client)
	cmd := commandRunner{env: commands.Env{Out: io.Discard}}
	post := fanbox.Post{CreatorID: "", ID: "p1", Body: &fanbox.PostBody{
		Assets: []fanbox.Asset{{ID: "a1", Kind: fanbox.AssetKindImage, Resource: resource}},
	}}
	require.Error(t, cmd.savePostAssets(context.Background(), client, baseDir, post, map[string]struct{}{}))
	require.Empty(t, findSavedFiles(t, baseDir))
}

func findSavedFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	require.NoError(t, filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	}))
	return files
}

// stubSavingClient builds a real FANBOX client against an in-process test server
// that serves post.info and media bytes, so SaveResource writes a real file.
func stubSavingClient(t *testing.T) *fanbox.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/post.info", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"body":{"post":{"id":"p-stub","title":"stub","publishedDatetime":"2024-01-01T00:00:00Z","isRestricted":false,"isPinned":false,"body":{"images":[{"id":"img-stub","originalUrl":"https://downloads.fanbox.cc/img-stub.png"}]}}}}`)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = io.WriteString(w, "bytes")
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	client, err := fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: "s"}, fanbox.Options{
		HTTPClient: &http.Client{Transport: rewriteToServerRT{base: server.URL}},
	})
	require.NoError(t, err)
	return client
}

// stubImageResource obtains a real image resource ref via the SDK so SaveResource
// can re-resolve and open it against the in-process test server.
func stubImageResource(t *testing.T, client *fanbox.Client) sdk.Resource {
	t.Helper()
	post, err := client.Post(context.Background(), fanbox.PostRequest{PostID: "p-stub"})
	require.NoError(t, err)
	require.NotEmpty(t, post.Body.Assets)
	return post.Body.Assets[0].Resource
}

// rewriteToServerRT rewrites every request's URL to point at the in-process
// test server, preserving the path so the media/post.info routes resolve.
type rewriteToServerRT struct{ base string }

func (rt rewriteToServerRT) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := req.Clone(req.Context())
	req2.URL.Scheme = "http"
	req2.URL.Host = strings.TrimPrefix(strings.TrimPrefix(rt.base, "http://"), "https://")
	req2.Host = req2.URL.Host
	return http.DefaultTransport.RoundTrip(req2)
}
