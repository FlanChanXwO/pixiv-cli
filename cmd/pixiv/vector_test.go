package main_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/vectorindex"
	"github.com/FlanChanXwO/pixiv-cli/internal/vectormodel"
	"github.com/stretchr/testify/require"
)

func TestVectorAutomaticSearchInputs(t *testing.T) {
	home := isolatedHome(t)
	t.Chdir(t.TempDir())
	picture, err := vectormodel.ProbeImage()
	require.NoError(t, err)
	var imageRequest atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/reference" {
			w.Write(picture)
			return
		}
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		data, err := json.Marshal(body)
		require.NoError(t, err)
		imageRequest.Store(bytes.Contains(data, []byte("inline_data")))
		fmt.Fprint(w, `{"embedding":{"values":[1,0]}}`)
	}))
	defer server.Close()
	seedVectorIndex(t, home, server.URL)
	require.NoError(t, os.WriteFile("cat", []byte("ordinary document"), 0600))
	require.NoError(t, os.WriteFile("picture", picture, 0600))
	for _, tc := range []struct {
		input string
		image bool
	}{{"a girl", false}, {"cat", false}, {"picture", true}, {server.URL + "/reference", true}} {
		stdout, stderr, code := runCLI(t, "vector", "search", tc.input, "--no-sync", "--json")
		require.Zero(t, code, stderr)
		require.Empty(t, stderr)
		require.Equal(t, tc.image, imageRequest.Load())
		require.Contains(t, stdout, `"page_index": 0`)
	}
	stdout, stderr, code := runCLI(t, "vector", "search", "./cat", "--no-sync")
	require.NotZero(t, code)
	require.Empty(t, stdout)
	require.Contains(t, stderr, "image content")
}

func seedVectorIndex(t *testing.T, home, service string) {
	t.Helper()
	root := filepath.Join(home, config.AppDataDirName)
	require.NoError(t, os.MkdirAll(root, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.toml"), []byte(fmt.Sprintf("[update]\ncheck_enabled=false\n[vector]\nmodel='fixture'\napi_key='fixture-key'\nbase_url=%q\n", service)), 0600))
	store, err := vectorindex.Open(t.Context(), root)
	require.NoError(t, err)
	require.NoError(t, store.Close())
	db, err := sql.Open("sqlite3", filepath.Join(vectorindex.Directory(root), "index.db"))
	require.NoError(t, err)
	defer db.Close()
	model, err := vectormodel.Identifier("fixture", service)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO artwork VALUES('23','{"id":"23","type":"artwork","url":"https://www.pixiv.net/artworks/23"}','2026-11-20T00:00:00Z'); INSERT INTO page VALUES('23',0,'https://i.pximg.net/23.png','');`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO embedding VALUES('23',0,?,X'0000803f00000000','')`, model)
	require.NoError(t, err)
}

func TestVectorSyncFiltersValidateBeforeModelLoading(t *testing.T) {
	isolatedHome(t)
	for _, flag := range []string{"--restrict", "--rating", "--content-type"} {
		stdout, stderr, code := runCLI(t, "vector", "sync", flag, "invalid")
		require.NotZero(t, code)
		require.Empty(t, stdout)
		require.Contains(t, stderr, strings.TrimPrefix(flag, "--"))
		require.NotContains(t, stderr, "not set up")
	}
}

func TestVectorAddRejectsInvalidNDJSONWithoutPlaceholder(t *testing.T) {
	home := isolatedHome(t)
	seedPixivAccount(t, home, "fixture", false)
	seedVectorIndex(t, home, "http://127.0.0.1:1")
	var out, diagnostic bytes.Buffer
	code := cli.Run(t.Context(), []string{"pixiv", "vector", "add", "--json"}, strings.NewReader("{\"type\":\"artwork\",broken}\n"), &out, &diagnostic)
	require.NotZero(t, code)
	require.Contains(t, diagnostic.String(), "input line 1")
	require.Contains(t, out.String(), `"recorded": 0`)
}

func pixivVectorHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/v1beta/models/fixture:embedContent" {
		fmt.Fprint(w, `{"embedding":{"values":[1,0]}}`)
		return
	}
	pixivHandler(w, r)
}

func assertVectorAddPipeline(t *testing.T, proxy string) {
	t.Helper()
	for _, entry := range []struct{ key, value string }{{"vector.model", "fixture"}, {"vector.api_key", "fixture-key"}, {"vector.base_url", "https://app-api.pixiv.net"}} {
		_, stderr, code := runCLI(t, "config", "set", entry.key, entry.value)
		require.Zero(t, code, stderr)
	}
	var out, diagnostic bytes.Buffer
	input := `{"type":"artwork","id":"202","url":"https://www.pixiv.net/artworks/202"}` + "\n"
	code := cli.Run(t.Context(), []string{"pixiv", "vector", "add", "--json", "--proxy", proxy}, strings.NewReader(input+input), &out, &diagnostic)
	require.Zero(t, code, diagnostic.String())
	require.Contains(t, out.String(), `"recorded": 1`)
	require.Contains(t, out.String(), `"embedded": 1`)
	stdout, stderr, code := runCLI(t, "vector", "add", "202", "https://www.pixiv.net/artworks/202", "--json", "--proxy", proxy)
	require.Zero(t, code, stderr)
	require.Contains(t, stdout, `"recorded": 1`)
	require.Contains(t, stdout, `"embedded": 0`)
}
