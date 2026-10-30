package main_test

import (
	"errors"
	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	"github.com/FlanChanXwO/pixiv-cli/internal/fanbox/account"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies"
	_ "github.com/FlanChanXwO/pixiv-cli/internal/cli/fanbox/auth"
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

func TestFanboxEscapesUpstreamText(t *testing.T) {
	proxy := fixtureProxy(t)
	seedFanbox(t)
	for _, args := range [][]string{{"creators", "--kind", "following"}, {"posts", "artist"}} {
		stdout, stderr, code := runCLI(t, append([]string{"fanbox", "--proxy", proxy}, args...)...)
		require.Zero(t, code, stderr)
		require.NotContains(t, stdout, "\x1b")
		require.Contains(t, stdout, `line\n\x1b[31m`)
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
