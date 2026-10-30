package main_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli"
	_ "github.com/FlanChanXwO/pixiv-cli/internal/cli/fanbox/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	"github.com/FlanChanXwO/pixiv-cli/internal/fanbox/account"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
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

func TestEscapesUpstreamText(t *testing.T) {
	proxy := fixtureProxy(t)
	seedFanbox(t)
	seedPixivAccount(t, os.Getenv("HOME"), "artist", false)
	for _, args := range [][]string{
		{"fanbox", "creators", "--kind", "following"}, {"fanbox", "posts", "artist"},
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

func seedFanbox(t *testing.T) {
	t.Helper()
	home := isolatedHome(t)
	db, err := database.Open(filepath.Join(home, paths.AppDataDirName))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	acct := account.New(7, "artist", "artist", []byte("synthetic-session"))
	acct.ValidatedAt = 1
	require.NoError(t, db.SaveFanboxCredential(t.Context(), acct))
}

func TestFanboxPropagatesWriteError(t *testing.T) {
	proxy := fixtureProxy(t)
	seedFanbox(t)
	for _, args := range [][]string{{"creators", "--kind", "following"}, {"posts", "artist"}} {
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
