package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-mcp-server/internal/pixiv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchSupportsFlagAfterWord(t *testing.T) {
	useTempPaths(t)

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/search/illust", r.URL.Path)
		assert.Equal(t, "初音ミク", r.URL.Query().Get("word"))
		assert.Equal(t, "12", r.URL.Query().Get("offset"))
		assert.Equal(t, "partial_match_for_tags", r.URL.Query().Get("search_target"))
		assert.Equal(t, "date_desc", r.URL.Query().Get("sort"))
		require.NoError(t, json.NewEncoder(w).Encode(pixiv.IllustList{
			Illusts: []pixiv.Illust{
				{ID: 123, Title: "Miku", User: pixiv.User{Name: "artist"}},
			},
		}))
	}))
	defer api.Close()

	setTestCLIClientFactory(t, func(cfg clientConfig) (*pixiv.Client, error) {
		return pixiv.New(cfg.RefreshToken, pixiv.WithHTTPClient(api.Client()), pixiv.WithBaseURLs(api.URL, api.URL)), nil
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "初音ミク", "--json", "--offset", "12"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	var out pixiv.IllustList
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out))
	require.Len(t, out.Illusts, 1)
	assert.Equal(t, int64(123), out.Illusts[0].ID)
}

func TestSearchUsesOutputJSONFromConfig(t *testing.T) {
	_, configPath := useTempPaths(t)
	require.NoError(t, writePrivateFile(configPath, []byte("[output]\njson = true\n")))

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode(pixiv.IllustList{
			Illusts: []pixiv.Illust{
				{ID: 321, Title: "Snow", User: pixiv.User{Name: "artist"}},
			},
		}))
	}))
	defer api.Close()

	setTestCLIClientFactory(t, func(cfg clientConfig) (*pixiv.Client, error) {
		return pixiv.New(cfg.RefreshToken, pixiv.WithHTTPClient(api.Client()), pixiv.WithBaseURLs(api.URL, api.URL)), nil
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "雪ミク"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	var out pixiv.IllustList
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out))
	require.Len(t, out.Illusts, 1)
	assert.Equal(t, int64(321), out.Illusts[0].ID)
}
