package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/auth"
	sdk "github.com/FlanChanXwO/pixiv-cli/pkg/pixiv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchRoutesArgumentsAndPrintsSDKJSON(t *testing.T) {
	useTempPaths(t)
	var got sdk.SearchIllustRequest
	setTestSDKCommandClient(t, sdkCommandFake{search: func(_ context.Context, request sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
		got = request
		return &sdk.IllustListResult{Illusts: []sdk.Illust{commandIllust(123)}}, nil
	}})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "初音ミク", "--json"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, sdk.SearchIllustRequest{
		Word: "初音ミク", Target: sdk.SearchTargetPartialMatchForTags, Sort: sdk.SortModeDateDesc,
	}, got)
	assert.JSONEq(t, `{"illusts":[{"id":123,"title":"work","type":"","page_count":0,"total_bookmarks":0,"total_view":0,"x_restrict":0,"user":{"id":0,"name":"artist","account":"","comment":"","is_followed":false},"tags":null,"image_urls":{"square_medium":"","medium":"","large":"","original":""},"meta_single_page":{"original_image_url":""},"meta_pages":null,"ai_type":0,"create_date":"","width":0,"height":0}]}`, stdout.String())
}

func TestSearchFiltersResultsAndFollowsCursorUntilLimit(t *testing.T) {
	useTempPaths(t)
	var cursors []sdk.Cursor
	setTestSDKCommandClient(t, sdkCommandFake{search: func(_ context.Context, request sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
		cursors = append(cursors, request.Cursor)
		switch request.Cursor {
		case "":
			return &sdk.IllustListResult{Illusts: []sdk.Illust{
				{ID: 1, Type: "illust", XRestrict: 1, AIType: 1},
				{ID: 2, Type: "manga", XRestrict: 0, AIType: 1},
				{ID: 3, Type: "manga", XRestrict: 1, AIType: 0},
				{ID: 4, Type: "manga", XRestrict: 1, AIType: 1},
			}, NextCursor: "second"}, nil
		case "second":
			return &sdk.IllustListResult{Illusts: []sdk.Illust{
				{ID: 5, Type: "manga", XRestrict: 1, AIType: 1},
			}}, nil
		default:
			return nil, fmt.Errorf("unexpected cursor %q", request.Cursor)
		}
	}})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "miku", "--rating", "r18", "--type", "comics", "--ai-type", "1", "--limit", "2", "--json"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, []sdk.Cursor{"", "second"}, cursors)
	var out struct {
		Illusts []sdk.Illust `json:"illusts"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out))
	assert.Equal(t, []int64{4, 5}, []int64{out.Illusts[0].ID, out.Illusts[1].ID})
}

func TestSearchAITypeFilterPreservesAnonymousWebResult(t *testing.T) {
	useTempPaths(t)
	web := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/ajax/search/artworks/miku", request.URL.Path)
		_, _ = io.WriteString(writer, `{"error":false,"body":{"illustManga":{"data":[{"id":"1","title":"AI work","illustType":"0","xRestrict":"0","aiType":"1","userId":"10","userName":"artist","pageCount":"1"}]}}}`)
	}))
	defer web.Close()
	setTestSDKCommandFactory(t, func(application.SDKClientRequest) (application.SDKClient, error) {
		return sdk.NewClient(sdk.Options{HTTPClient: web.Client(), WebAPIBaseURL: web.URL, WebFallbackEnabled: true})
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "miku", "--ai-type", "1", "--json"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	var out struct {
		Illusts []sdk.Illust `json:"illusts"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out))
	require.Len(t, out.Illusts, 1)
	assert.Equal(t, 1, out.Illusts[0].AIType)
}

func TestSearchRejectsInvalidFilterValuesBeforeOpeningSDK(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "rating", args: []string{"--rating", "adult"}, want: "rating must be one of"},
		{name: "type", args: []string{"--type", "novel"}, want: "type must be one of"},
		{name: "ai type", args: []string{"--ai-type", "3"}, want: "ai-type must be 0, 1, or 2"},
	} {
		t.Run(test.name, func(t *testing.T) {
			useTempPaths(t)
			calls := 0
			setTestSDKCommandFactory(t, func(application.SDKClientRequest) (application.SDKClient, error) {
				calls++
				return sdkCommandFake{}, nil
			})
			var stdout, stderr bytes.Buffer
			code := Run(append([]string{"pixiv", "search", "miku"}, test.args...), strings.NewReader(""), &stdout, &stderr)

			require.NotZero(t, code)
			assert.Contains(t, stderr.String(), test.want)
			assert.Zero(t, calls)
		})
	}
}

func TestSearchUsesOutputJSONFromConfig(t *testing.T) {
	_, configPath := useTempPaths(t)
	require.NoError(t, auth.WritePrivateFile(configPath, []byte("[output]\njson = true\n")))
	setTestSDKCommandClient(t, sdkCommandFake{search: func(context.Context, sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
		return &sdk.IllustListResult{Illusts: []sdk.Illust{commandIllust(321)}}, nil
	}})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "雪ミク"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Contains(t, stdout.String(), `"id": 321`)
}

func TestSDKDataCommandsPassProxyOverride(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "search", args: []string{"pixiv", "search", "miku", "--proxy", "http://flag-proxy"}},
		{name: "detail", args: []string{"pixiv", "detail", "42", "--proxy", "http://flag-proxy"}},
		{name: "ranking", args: []string{"pixiv", "ranking", "--proxy", "http://flag-proxy"}},
		{name: "recommended", args: []string{"pixiv", "recommended", "--proxy", "http://flag-proxy"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useTempPaths(t)
			var got application.SDKClientRequest
			setTestSDKCommandFactory(t, func(request application.SDKClientRequest) (application.SDKClient, error) {
				got = request
				return proxySDKClient(), nil
			})

			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, Run(tt.args, strings.NewReader(""), &stdout, &stderr), stderr.String())
			require.NotNil(t, got.HTTPSProxyOverride)
			assert.Equal(t, "http://flag-proxy", *got.HTTPSProxyOverride)
		})
	}
}

func TestSDKDataCommandsEmptyProxyOverrideClearsRuntimeProxy(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "search", args: []string{"pixiv", "search", "miku", "--proxy", ""}},
		{name: "detail", args: []string{"pixiv", "detail", "42", "--proxy", ""}},
		{name: "ranking", args: []string{"pixiv", "ranking", "--proxy", ""}},
		{name: "recommended", args: []string{"pixiv", "recommended", "--proxy", ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useTempPaths(t)
			var got application.SDKClientRequest
			setTestSDKCommandFactory(t, func(request application.SDKClientRequest) (application.SDKClient, error) {
				got = request
				return proxySDKClient(), nil
			})

			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, Run(tt.args, strings.NewReader(""), &stdout, &stderr), stderr.String())
			require.NotNil(t, got.HTTPSProxyOverride)
			assert.Empty(t, *got.HTTPSProxyOverride)
		})
	}
}

func TestSearchPassesSelectedUIDWithoutResolvingCredentialInCLI(t *testing.T) {
	authPath, _ := useTempPaths(t)
	require.NoError(t, auth.SaveAuthStore(authPath, auth.AuthStore{
		DefaultUserID: 111,
		Accounts:      []auth.Account{{UserID: 111, RefreshToken: "main-token"}, {UserID: 222, RefreshToken: "other-token"}},
	}))
	var requests []application.SDKClientRequest
	setTestSDKCommandFactory(t, func(request application.SDKClientRequest) (application.SDKClient, error) {
		requests = append(requests, request)
		return proxySDKClient(), nil
	})

	for _, args := range [][]string{
		{"pixiv", "search", "miku", "--uid", "222"},
		{"pixiv", "search", "miku", "--profile", "111"},
	} {
		var stdout, stderr bytes.Buffer
		require.Equal(t, 0, Run(args, strings.NewReader(""), &stdout, &stderr), stderr.String())
	}
	require.Len(t, requests, 2)
	assert.Equal(t, int64(222), requests[0].UserID)
	assert.Equal(t, int64(111), requests[1].UserID)
	assert.Empty(t, requests[0].RefreshToken)
	assert.Empty(t, requests[1].RefreshToken)
}

func TestNetworkDataCommandsNoProxyFlagClearsRuntimeProxy(t *testing.T) {
	for _, args := range [][]string{
		{"pixiv", "search", "miku", "--no-proxy"},
		{"pixiv", "detail", "42", "--no-proxy"},
		{"pixiv", "ranking", "--no-proxy"},
		{"pixiv", "recommended", "--no-proxy"},
	} {
		t.Run(strings.Join(args[1:], " "), func(t *testing.T) {
			useTempPaths(t)
			var got application.SDKClientRequest
			setTestSDKCommandFactory(t, func(request application.SDKClientRequest) (application.SDKClient, error) {
				got = request
				return proxySDKClient(), nil
			})
			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, Run(args, strings.NewReader(""), &stdout, &stderr), stderr.String())
			require.NotNil(t, got.HTTPSProxyOverride)
			assert.Empty(t, *got.HTTPSProxyOverride)
		})
	}
}

func TestNetworkCommandsRejectConflictingProxyFlags(t *testing.T) {
	for _, args := range [][]string{
		{"pixiv", "search", "miku", "--proxy", "http://flag-proxy", "--no-proxy"},
		{"pixiv", "mcp", "--proxy", "http://flag-proxy", "--no-proxy"},
	} {
		t.Run(strings.Join(args[1:], " "), func(t *testing.T) {
			useTempPaths(t)
			var stdout, stderr bytes.Buffer
			code := Run(args, strings.NewReader(""), &stdout, &stderr)
			require.NotZero(t, code)
			assert.Contains(t, stderr.String(), "use either --proxy or --no-proxy, not both")
		})
	}
}

func TestDownloadProxyFlagPassesRuntimeOverride(t *testing.T) {
	for _, proxy := range []string{"http://flag-proxy", ""} {
		t.Run(proxy, func(t *testing.T) {
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
			require.Equal(t, 0, Run([]string{"pixiv", "download", "42", "--proxy", proxy}, strings.NewReader(""), &stdout, &stderr), stderr.String())
			assert.Equal(t, proxy, seenProxy)
		})
	}
}

func proxySDKClient() sdkCommandFake {
	return sdkCommandFake{
		search: func(context.Context, sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
			return &sdk.IllustListResult{}, nil
		},
		detail: func(context.Context, int64) (*sdk.IllustDetail, error) {
			return &sdk.IllustDetail{Illust: commandIllust(42)}, nil
		},
		ranking: func(context.Context, sdk.IllustRankingRequest) (*sdk.IllustListResult, error) {
			return &sdk.IllustListResult{}, nil
		},
		recommended: func(context.Context, sdk.IllustRecommendedRequest) (*sdk.IllustListResult, error) {
			return &sdk.IllustListResult{}, nil
		},
	}
}

type proxyFlagPixivClient struct{}

func (proxyFlagPixivClient) Refresh(context.Context) error { return nil }
func (proxyFlagPixivClient) RefreshTokenValue() string     { return "token" }
func (proxyFlagPixivClient) UserID() int64                 { return 123 }
func (proxyFlagPixivClient) UserName() string              { return "proxy-user" }
func (proxyFlagPixivClient) UserDetail(context.Context, int64) (*pixiv.User, error) {
	return &pixiv.User{ID: 123, Name: "proxy-user"}, nil
}
func (proxyFlagPixivClient) SearchIllust(context.Context, string, string, string, string, int) (*pixiv.IllustList, error) {
	return &pixiv.IllustList{}, nil
}
func (proxyFlagPixivClient) IllustDetail(context.Context, int64) (*pixiv.IllustDetail, error) {
	return &pixiv.IllustDetail{Illust: pixiv.Illust{ID: 42, Title: "proxy", Type: string(pixiv.IllustTypeIllust), PageCount: 1, User: pixiv.User{Name: "artist"}, MetaSinglePage: pixiv.SinglePage{OriginalImageURL: "https://img.example/proxy.jpg"}}}, nil
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
