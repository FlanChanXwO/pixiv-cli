package main_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/account"
	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli"
	_ "github.com/FlanChanXwO/pixiv-cli/internal/cli/fanbox/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/credential"
	"github.com/stretchr/testify/require"
)

// TestBrowserProvidersStayRegistered 覆盖 fanbox auth 的 blank import：删掉任一 provider
// 的子包导入，这里会先于真实浏览器访问失败。
func TestBrowserProvidersStayRegistered(t *testing.T) {
	for _, name := range []string{"chrome", "edge", "firefox", "safari"} {
		provider, err := browsercookies.New(name)
		if errors.Is(err, browsercookies.ErrUnknownBrowser) {
			t.Fatalf("browser provider %q is not registered", name)
		}
		if err == nil {
			_ = provider.Close()
		}
	}
}

func assertEscapesUpstreamText(t *testing.T, proxy string) {
	t.Helper()
	db, err := account.Open(t.Context(), filepath.Join(os.Getenv("HOME"), config.AppDataDirName))
	require.NoError(t, err)
	acct := account.Fanbox{UserID: 7, DisplayName: "artist", CreatorID: "artist", Credential: credential.New([]byte("synthetic-session"))}
	acct.ValidatedAt = time.Unix(1, 0).UTC()
	require.NoError(t, db.SaveFanboxCredential(t.Context(), acct))

	for _, args := range [][]string{
		{"fanbox", "creators", "--relation", "following"}, {"fanbox", "posts", "artist"},
		{"detail", "101"}, {"detail", "101", "--type", "novel"},
		{"search", "unsafe"}, {"search", "unsafe", "--type", "novel"},
		{"comment", "101", "--type", "artwork"}, {"bookmark", "tags"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, stderr, code := runCLI(t, append(args, "--proxy", proxy)...)
			require.Zero(t, code, stderr)
			require.NotContains(t, stdout, "\x1b")
			require.Contains(t, stdout, `line\n\x1b[31m`)
		})
	}
}

func assertFanboxPropagatesWriteError(t *testing.T, proxy string) {
	t.Helper()
	for _, args := range [][]string{{"creators", "--relation", "following"}, {"posts", "artist"}} {
		t.Run(args[0], func(t *testing.T) {
			var stderr bytes.Buffer
			code := cli.Run(t.Context(), append([]string{"pixiv", "fanbox", "--proxy", proxy}, args...), strings.NewReader(""), failedOutput{}, &stderr)
			require.Equal(t, 1, code)
			require.Equal(t, "error: fixture write failed\n", stderr.String())
		})
	}
}

type failedOutput struct{}

func (failedOutput) Write([]byte) (int, error) { return 0, errors.New("fixture write failed") }

func assertAdditionalEscaping(t *testing.T, proxy string) {
	t.Helper()
	for _, args := range [][]string{{"series", "101", "--type", "novel"}, {"bookmark", "detail", "101"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, stderr, code := runCLI(t, append(args, "--proxy", proxy)...)
			require.Zero(t, code, stderr)
			require.NotContains(t, stdout, "\x1b")
			require.Contains(t, stdout, `line\n\x1b[31m`)
		})
	}
}

func assertFanboxDownload(t *testing.T, proxy string) {
	t.Helper()
	directory := t.TempDir()
	t.Setenv("DOWNLOAD_PATH", directory)
	_, stderr, code := runCLI(t, "fanbox", "download", "artist", "--proxy", proxy)
	require.Zero(t, code, stderr)
	base := filepath.Join(directory, "fanbox", "artist", "101")
	data, err := os.ReadFile(filepath.Join(base, "bundle.zip"))
	require.NoError(t, err)
	require.Equal(t, "fixture-archive", string(data))
	image, err := os.ReadFile(filepath.Join(base, "photo.jpg"))
	require.NoError(t, err)
	require.Equal(t, "fixture-image", string(image))
	metadata, err := os.ReadFile(filepath.Join(base, "post.json"))
	require.NoError(t, err)
	require.Contains(t, string(metadata), "fixture-secret")
	require.Contains(t, string(metadata), "paragraph password")
	attachment, err := os.ReadFile(filepath.Join(base, "metadata_post.json"))
	require.NoError(t, err)
	require.Equal(t, "fixture-archive", string(attachment))
	info, err := os.Stat(filepath.Join(base, "post.json"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	assertFanboxDownloadConflict(t, proxy)
	for _, format := range []string{"--json", "--ndjson"} {
		stdout, stderr, code := runCLI(t, "fanbox", "post", "101", format, "--proxy", proxy)
		require.Zero(t, code, stderr)
		var post map[string]any
		require.NoError(t, json.Unmarshal([]byte(stdout), &post))
		require.Contains(t, stdout, "fixture-secret")
		require.Contains(t, stdout, "paragraph password")
		require.NotContains(t, stdout, "downloads.fanbox.cc")
	}
}

func assertFanboxDownloadConflict(t *testing.T, proxy string) {
	t.Helper()
	_, stderr, code := runCLI(t, "fanbox", "download", "102", "--proxy", proxy)
	require.Equal(t, 1, code)
	require.Contains(t, stderr, "resolve to the same download path")
}
