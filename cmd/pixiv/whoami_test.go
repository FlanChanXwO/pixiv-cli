package main_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli"
	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	pixivaccount "github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
	"github.com/FlanChanXwO/pixiv-cli/internal/version"
	"github.com/stretchr/testify/require"
)

func TestWhoamiText(t *testing.T) {
	home := isolatedHome(t)
	seedPixivAccount(t, home, "alice", true)

	stdout, stderr, code := runWhoami(t)
	require.Equal(t, 0, code)
	require.Empty(t, stderr)
	require.Equal(t, "alice (12345678) · Premium\n", stdout)
}

func TestWhoamiJSONMatchesAuthList(t *testing.T) {
	home := isolatedHome(t)
	seedPixivAccount(t, home, "alice", true)

	stdout, stderr, code := runWhoami(t, "--json")
	require.Equal(t, 0, code)
	require.Empty(t, stderr)

	list, listErr, listCode := runCLI(t, "auth", "list", "--json")
	require.Equal(t, 0, listCode)
	require.Empty(t, listErr)
	var listed struct {
		Accounts []json.RawMessage `json:"accounts"`
	}
	require.NoError(t, json.Unmarshal([]byte(list), &listed))
	require.Len(t, listed.Accounts, 1)
	require.JSONEq(t, string(listed.Accounts[0]), stdout)
}

func TestWhoamiJSONMatchesAuthListAfterExpiredFreeze(t *testing.T) {
	home := isolatedHome(t)
	seedPixivAccount(t, home, "alice", true)
	db, err := database.Open(filepath.Join(home, paths.AppDataDirName))
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	expired := int64(1)
	require.NoError(t, db.Freeze(t.Context(), 12345678, expired))

	stdout, stderr, code := runWhoami(t, "--json")
	require.Equal(t, 0, code)
	require.Empty(t, stderr)
	require.NotContains(t, stdout, "pool_frozen_until")

	list, listErr, listCode := runCLI(t, "auth", "list", "--json")
	require.Equal(t, 0, listCode)
	require.Empty(t, listErr)
	var listed struct {
		Accounts []json.RawMessage `json:"accounts"`
	}
	require.NoError(t, json.Unmarshal([]byte(list), &listed))
	require.Len(t, listed.Accounts, 1)
	require.JSONEq(t, string(listed.Accounts[0]), stdout)
}

func TestWhoamiUnknownUsername(t *testing.T) {
	home := isolatedHome(t)
	seedPixivAccount(t, home, "", true)

	stdout, stderr, code := runWhoami(t)
	require.Equal(t, 0, code)
	require.Empty(t, stderr)
	require.Equal(t, "12345678\n", stdout)
}

func TestWhoamiWithoutAccount(t *testing.T) {
	isolatedHome(t)

	stdout, stderr, code := runWhoami(t)
	require.Equal(t, 1, code)
	require.Empty(t, stdout)
	require.Equal(t, "error: not logged in; run \"pixiv auth login\"\n", stderr)
}

func TestWhoamiNoNetwork(t *testing.T) {
	home := isolatedHome(t)
	seedPixivAccount(t, home, "alice", false)
	var requests atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected network request", http.StatusBadGateway)
	}))
	defer proxy.Close()
	t.Setenv("https_proxy", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	originalVersion := version.Version
	version.Version = "9.9.9"
	t.Cleanup(func() { version.Version = originalVersion })

	stdout, stderr, code := runWhoami(t)
	require.Equal(t, 0, code)
	require.Equal(t, "alice (12345678)\n", stdout)
	require.Empty(t, stderr)
	require.Zero(t, requests.Load())
}

func isolatedHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func seedPixivAccount(t *testing.T, home, username string, premium bool) {
	t.Helper()
	db, err := database.Open(filepath.Join(home, paths.AppDataDirName))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	account := pixivaccount.New(12345678, username, []byte("synthetic-refresh-token"))
	account.PremiumStatus = &premium
	require.NoError(t, db.SavePixivCredential(t.Context(), account))
	require.NoError(t, db.SetPixivSchedulable(t.Context(), []int64{12345678}, false))
}

func runWhoami(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	return runCLI(t, append([]string{"whoami"}, args...)...)
}

func runCLI(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := cli.Run(t.Context(), append([]string{"pixiv"}, args...), strings.NewReader(""), &stdout, &stderr)
	return stdout.String(), stderr.String(), code
}
