package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/auth"
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

	setTestCLIClientFactory(t, func(cfg clientConfig) (cliPixivClient, error) {
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
	require.NoError(t, auth.WritePrivateFile(configPath, []byte("[output]\njson = true\n")))

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode(pixiv.IllustList{
			Illusts: []pixiv.Illust{
				{ID: 321, Title: "Snow", User: pixiv.User{Name: "artist"}},
			},
		}))
	}))
	defer api.Close()

	setTestCLIClientFactory(t, func(cfg clientConfig) (cliPixivClient, error) {
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

func TestSearchProxyFlagOverridesEnvAndConfig(t *testing.T) {
	_, configPath := useTempPaths(t)
	require.NoError(t, auth.WritePrivateFile(configPath, []byte("[network]\nhttps_proxy = \"http://file-proxy\"\n")))
	t.Setenv("https_proxy", "http://env-proxy")

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode(pixiv.IllustList{}))
	}))
	defer api.Close()

	var seenProxy string
	setTestCLIClientFactory(t, func(cfg clientConfig) (cliPixivClient, error) {
		seenProxy = cfg.HTTPSProxy
		return pixiv.New(cfg.RefreshToken, pixiv.WithHTTPClient(api.Client()), pixiv.WithBaseURLs(api.URL, api.URL)), nil
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "miku", "--proxy", "http://flag-proxy"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "http://flag-proxy", seenProxy)
}

func TestSearchEmptyProxyFlagClearsEnvAndConfig(t *testing.T) {
	_, configPath := useTempPaths(t)
	require.NoError(t, auth.WritePrivateFile(configPath, []byte("[network]\nhttps_proxy = \"http://file-proxy\"\n")))
	t.Setenv("https_proxy", "http://env-proxy")

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode(pixiv.IllustList{}))
	}))
	defer api.Close()

	seenProxy := "unset"
	setTestCLIClientFactory(t, func(cfg clientConfig) (cliPixivClient, error) {
		seenProxy = cfg.HTTPSProxy
		return pixiv.New(cfg.RefreshToken, pixiv.WithHTTPClient(api.Client()), pixiv.WithBaseURLs(api.URL, api.URL)), nil
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "miku", "--proxy", ""}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Empty(t, seenProxy)
}

func TestSearchNoProxyFlagIsUnknown(t *testing.T) {
	useTempPaths(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "miku", "--proxy", "http://flag-proxy", "--no-proxy"}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	assert.Contains(t, stderr.String(), `unknown flag: --no-proxy`)
}

func TestDataCommandsProxyFlagPassesRuntimeOverride(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "detail", args: []string{"pixiv", "detail", "42", "--proxy", "http://flag-proxy"}},
		{name: "ranking", args: []string{"pixiv", "ranking", "--proxy", "http://flag-proxy"}},
		{name: "recommended", args: []string{"pixiv", "recommended", "--proxy", "http://flag-proxy"}},
		{name: "download", args: []string{"pixiv", "download", "42", "--proxy", "http://flag-proxy"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, configPath := useTempPaths(t)
			require.NoError(t, auth.WritePrivateFile(configPath, []byte("[network]\nhttps_proxy = \"http://file-proxy\"\n[download]\npath = \""+strings.ReplaceAll(t.TempDir(), "\\", "\\\\")+"\"\n")))
			t.Setenv("https_proxy", "http://env-proxy")
			t.Setenv("PIXIV_REFRESH_TOKEN", "token")

			var seenProxy string
			setTestCLIClientFactory(t, func(cfg clientConfig) (cliPixivClient, error) {
				seenProxy = cfg.HTTPSProxy
				return proxyFlagPixivClient{}, nil
			})

			var stdout, stderr bytes.Buffer
			code := Run(tt.args, strings.NewReader(""), &stdout, &stderr)

			require.Equal(t, 0, code, stderr.String())
			assert.Equal(t, "http://flag-proxy", seenProxy)
		})
	}
}

func TestDataCommandsEmptyProxyFlagClearsRuntimeProxy(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "detail", args: []string{"pixiv", "detail", "42", "--proxy", ""}},
		{name: "ranking", args: []string{"pixiv", "ranking", "--proxy", ""}},
		{name: "recommended", args: []string{"pixiv", "recommended", "--proxy", ""}},
		{name: "download", args: []string{"pixiv", "download", "42", "--proxy", ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, configPath := useTempPaths(t)
			require.NoError(t, auth.WritePrivateFile(configPath, []byte("[network]\nhttps_proxy = \"http://file-proxy\"\n[download]\npath = \""+strings.ReplaceAll(t.TempDir(), "\\", "\\\\")+"\"\n")))
			t.Setenv("https_proxy", "http://env-proxy")
			t.Setenv("PIXIV_REFRESH_TOKEN", "token")

			seenProxy := "unset"
			setTestCLIClientFactory(t, func(cfg clientConfig) (cliPixivClient, error) {
				seenProxy = cfg.HTTPSProxy
				return proxyFlagPixivClient{}, nil
			})

			var stdout, stderr bytes.Buffer
			code := Run(tt.args, strings.NewReader(""), &stdout, &stderr)

			require.Equal(t, 0, code, stderr.String())
			assert.Empty(t, seenProxy)
		})
	}
}

func TestNetworkDataCommandsNoProxyFlagIsUnknown(t *testing.T) {
	tests := [][]string{
		{"pixiv", "detail", "42", "--no-proxy"},
		{"pixiv", "ranking", "--no-proxy"},
		{"pixiv", "recommended", "--no-proxy"},
		{"pixiv", "download", "42", "--no-proxy"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args[1:], " "), func(t *testing.T) {
			useTempPaths(t)

			var stdout, stderr bytes.Buffer
			code := Run(args, strings.NewReader(""), &stdout, &stderr)

			require.NotZero(t, code)
			assert.Contains(t, stderr.String(), `unknown flag: --no-proxy`)
		})
	}
}

func TestSearchCanSelectStoredTokenByUIDAndDeprecatedProfile(t *testing.T) {
	authPath, _ := useTempPaths(t)
	require.NoError(t, auth.SaveAuthStore(authPath, auth.AuthStore{
		DefaultUserID: 111,
		Accounts: []auth.Account{
			{UserID: 111, RefreshToken: "main-token"},
			{UserID: 222, RefreshToken: "other-token"},
		},
	}))

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode(pixiv.IllustList{}))
	}))
	defer api.Close()

	var seenTokens []string
	setTestCLIClientFactory(t, func(cfg clientConfig) (cliPixivClient, error) {
		seenTokens = append(seenTokens, cfg.RefreshToken)
		return pixiv.New(cfg.RefreshToken, pixiv.WithHTTPClient(api.Client()), pixiv.WithBaseURLs(api.URL, api.URL)), nil
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "miku", "--uid", "222"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "search", "miku", "--profile", "111"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())

	assert.Equal(t, []string{"other-token", "main-token"}, seenTokens)
}

type proxyFlagPixivClient struct{}

func (proxyFlagPixivClient) Refresh(context.Context) error { return nil }

func (proxyFlagPixivClient) RefreshTokenValue() string { return "token" }

func (proxyFlagPixivClient) UserID() int64 { return 123 }

func (proxyFlagPixivClient) UserName() string { return "proxy-user" }

func (proxyFlagPixivClient) UserDetail(context.Context, int64) (*pixiv.User, error) {
	return &pixiv.User{ID: 123, Name: "proxy-user"}, nil
}

func (proxyFlagPixivClient) SearchIllust(context.Context, string, string, string, string, int) (*pixiv.IllustList, error) {
	return &pixiv.IllustList{}, nil
}

func (proxyFlagPixivClient) IllustDetail(context.Context, int64) (*pixiv.IllustDetail, error) {
	return &pixiv.IllustDetail{Illust: pixiv.Illust{
		ID:        42,
		Title:     "proxy",
		Type:      string(pixiv.IllustTypeIllust),
		PageCount: 1,
		User:      pixiv.User{Name: "artist"},
		MetaSinglePage: pixiv.SinglePage{
			OriginalImageURL: "https://img.example/proxy.jpg",
		},
	}}, nil
}

func (proxyFlagPixivClient) IllustRanking(context.Context, string, string, int) (*pixiv.IllustList, error) {
	return &pixiv.IllustList{}, nil
}

func (proxyFlagPixivClient) IllustRecommended(context.Context, int) (*pixiv.IllustList, error) {
	return &pixiv.IllustList{}, nil
}

func (proxyFlagPixivClient) UgoiraMetadata(context.Context, int64) (*pixiv.UgoiraMetadataResult, error) {
	return &pixiv.UgoiraMetadataResult{}, nil
}

func (proxyFlagPixivClient) Download(_ context.Context, _ string, dst io.Writer) error {
	_, err := dst.Write([]byte("image"))
	return err
}
