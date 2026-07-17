package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/auth"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
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
	assert.JSONEq(t, `{"illusts":[{"id":123,"title":"work","type":"","page_count":0,"total_bookmarks":0,"total_view":0,"x_restrict":0,"user":{"id":0,"name":"artist","account":"","comment":"","is_followed":false,"profile_image_urls":{}},"tags":null,"image_urls":{"square_medium":"","medium":"","large":"","original":""},"meta_single_page":{"original_image_url":""},"meta_pages":null,"ai_type":0,"create_date":"","width":0,"height":0}]}`, stdout.String())
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

func TestRunSearchRejectsMalformedExplicitProxyWithoutLeakingSensitiveComponents(t *testing.T) {
	useTempPaths(t)
	proxy := "http://proxy-user-secret:proxy-pass-secret@proxy-host-secret.invalid/proxy-path-secret-%zz?proxy-query-secret=value"

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "miku", "--proxy", proxy}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "invalid proxy configuration")
	for _, secret := range []string{"proxy-user-secret", "proxy-pass-secret", "proxy-host-secret", "proxy-path-secret", "proxy-query-secret"} {
		assert.NotContains(t, stderr.String(), secret)
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
		{name: "recommended", args: []string{"pixiv", "recommended", "illust", "--proxy", "http://flag-proxy"}},
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
		{name: "recommended", args: []string{"pixiv", "recommended", "illust", "--proxy", ""}},
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
		{"pixiv", "recommended", "illust", "--no-proxy"},
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

func TestDownloadDelegatesOperationSnapshotAndFlagOverrides(t *testing.T) {
	useTempPaths(t)
	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("download"), "same-context")
	client := &sdkCommandFake{}
	var gotClientRequest application.SDKClientRequest
	setTestDownloadCommandServices(t, func(request application.SDKClientRequest) (application.SDKClient, error) {
		gotClientRequest = request
		return client, nil
	}, config.RuntimeConfig{DownloadPath: "/runtime/path", FilenameTemplate: "runtime-template"}, func(gotClient application.DownloadClient, gotPath, gotTemplate string) (application.DownloadManager, error) {
		require.Same(t, client, gotClient)
		require.Equal(t, "/flag/path", gotPath)
		require.Equal(t, "flag-template", gotTemplate)
		return downloadManagerFake{download: func(gotContext context.Context, gotIDs []int64) ([]application.DownloadedArtwork, error) {
			require.Same(t, ctx, gotContext)
			require.Equal(t, []int64{42, 84}, gotIDs)
			return []application.DownloadedArtwork{{
				IllustID: 42,
				Title:    "work",
				Author:   "artist",
				Files:    []application.DownloadedFile{{Path: "/flag/path/42.jpg"}},
			}}, nil
		}}, nil
	})

	var stdout, stderr bytes.Buffer
	code := RunContext(ctx, []string{
		"pixiv", "download", "42", "84",
		"--uid", "9",
		"--refresh-token", "refresh",
		"--download-path", "/flag/path",
		"--filename-template", "flag-template",
		"--proxy", "http://flag-proxy",
	}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	require.NotNil(t, gotClientRequest.HTTPSProxyOverride)
	require.Equal(t, "http://flag-proxy", *gotClientRequest.HTTPSProxyOverride)
	require.Equal(t, int64(9), gotClientRequest.UserID)
	require.Equal(t, "refresh", gotClientRequest.RefreshToken)
	require.Equal(t, "downloaded 42 \"work\" by artist\n  /flag/path/42.jpg\n", stdout.String())
}

func TestDownloadDelegatesRuntimePathAndTemplateWithoutFlags(t *testing.T) {
	useTempPaths(t)
	setTestDownloadCommandServices(t, func(application.SDKClientRequest) (application.SDKClient, error) {
		return &sdkCommandFake{}, nil
	}, config.RuntimeConfig{DownloadPath: "/runtime/path", FilenameTemplate: "runtime-template"}, func(_ application.DownloadClient, path, template string) (application.DownloadManager, error) {
		require.Equal(t, "/runtime/path", path)
		require.Equal(t, "runtime-template", template)
		return downloadManagerFake{download: func(context.Context, []int64) ([]application.DownloadedArtwork, error) {
			return nil, nil
		}}, nil
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "download", "42"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	require.Empty(t, stdout.String())
}

func TestDownloadReportsFactoryFailure(t *testing.T) {
	useTempPaths(t)
	want := errors.New("download factory failed")
	setTestDownloadCommandServices(t, func(application.SDKClientRequest) (application.SDKClient, error) {
		return &sdkCommandFake{}, nil
	}, config.RuntimeConfig{}, func(application.DownloadClient, string, string) (application.DownloadManager, error) {
		return nil, want
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "download", "42"}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), want.Error())
}

func TestDownloadReportsManagerFailure(t *testing.T) {
	useTempPaths(t)
	want := errors.New("download manager failed")
	setTestDownloadCommandServices(t, func(application.SDKClientRequest) (application.SDKClient, error) {
		return &sdkCommandFake{}, nil
	}, config.RuntimeConfig{}, func(application.DownloadClient, string, string) (application.DownloadManager, error) {
		return downloadManagerFake{download: func(context.Context, []int64) ([]application.DownloadedArtwork, error) {
			return nil, want
		}}, nil
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "download", "42"}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), want.Error())
}

func TestDownloadPreservesJSONOutputShape(t *testing.T) {
	useTempPaths(t)
	setTestDownloadCommandServices(t, func(application.SDKClientRequest) (application.SDKClient, error) {
		return &sdkCommandFake{}, nil
	}, config.RuntimeConfig{}, func(application.DownloadClient, string, string) (application.DownloadManager, error) {
		return downloadManagerFake{download: func(context.Context, []int64) ([]application.DownloadedArtwork, error) {
			return []application.DownloadedArtwork{{
				IllustID: 42,
				Title:    "work",
				Author:   "artist",
				Type:     "illust",
				Files:    []application.DownloadedFile{{Path: "/downloads/42.jpg", Page: 2}},
			}}, nil
		}}, nil
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "download", "42", "--json"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	require.JSONEq(t, `[{"IllustID":42,"Title":"work","Author":"artist","Type":"illust","Files":[{"Path":"/downloads/42.jpg","Page":2}]}]`, stdout.String())
}

type downloadManagerFake struct {
	download func(context.Context, []int64) ([]application.DownloadedArtwork, error)
}

func (m downloadManagerFake) Download(ctx context.Context, ids []int64) ([]application.DownloadedArtwork, error) {
	return m.download(ctx, ids)
}

func setTestDownloadCommandServices(t *testing.T, newClient application.SDKClientFactory, runtime config.RuntimeConfig, newManager application.DownloadManagerFactory) {
	t.Helper()
	old := newCLIServices
	newCLIServices = func(*slog.Logger) application.Services {
		return application.Services{
			SDK: application.SDKService{
				NewClient:   newClient,
				LoadRuntime: func() (config.RuntimeConfig, error) { return runtime, nil },
			},
			Download: application.DownloadService{NewManager: newManager},
		}
	}
	t.Cleanup(func() { newCLIServices = old })
}

func TestDownloadProxyFlagPassesRuntimeOverride(t *testing.T) {
	for _, useProxy := range []bool{true, false} {
		t.Run(fmt.Sprintf("use_proxy=%t", useProxy), func(t *testing.T) {
			_, configPath := useTempPaths(t)
			proxy := newTestForwardProxy(t)
			downloadPath := t.TempDir()
			require.NoError(t, auth.WritePrivateFile(configPath, []byte("[network]\nhttps_proxy = \""+proxy.URL+"\"\n[download]\npath = \""+strings.ReplaceAll(downloadPath, "\\", "\\\\")+"\"\n")))
			t.Setenv("https_proxy", proxy.URL)
			t.Setenv("PIXIV_REFRESH_TOKEN", "token")

			var upstream *httptest.Server
			upstream = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/auth/token":
					require.NoError(t, r.ParseForm())
					assert.Equal(t, "token", r.Form.Get("refresh_token"))
					_, _ = io.WriteString(w, `{"access_token":"access","refresh_token":"token","user":{"id":123,"name":"proxy-user"}}`)
				case "/v1/illust/detail":
					assert.Equal(t, "42", r.URL.Query().Get("illust_id"))
					_, _ = fmt.Fprintf(w, `{"illust":{"id":42,"title":"proxy","type":"illust","page_count":1,"user":{"id":123,"name":"artist"},"tags":[],"image_urls":{},"meta_single_page":{"original_image_url":%q},"meta_pages":[]}}`, upstream.URL+"/resource/proxy.jpg")
				case "/ajax/illust/42/pages":
					_, _ = io.WriteString(w, `{"error":false,"body":[]}`)
				case "/resource/proxy.jpg":
					_, _ = io.WriteString(w, "image")
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(upstream.Close)
			upstreamURL, err := url.Parse(upstream.URL)
			require.NoError(t, err)
			resourcePolicy := sdk.ResourcePolicy{Mirrors: []sdk.ResourceMirrorPolicy{{Host: upstreamURL.Host, PathPrefixes: []string{"/resource/"}}}}
			probe, err := sdk.NewClient(sdk.Options{ResourcePolicy: resourcePolicy})
			require.NoError(t, err)
			_, err = probe.ParseResourceRef(upstream.URL + "/resource/proxy.jpg")
			require.NoError(t, err)
			setTestPublicSDKFactoryWithHTTPClient(t, upstream.URL, upstream.URL, upstream.URL, resourcePolicy, func(proxyValue string) (*http.Client, error) {
				transport := upstream.Client().Transport.(*http.Transport).Clone()
				transport.Proxy = nil
				if proxyValue != "" {
					proxyURL, err := url.Parse(proxyValue)
					if err != nil {
						return nil, err
					}
					transport.Proxy = http.ProxyURL(proxyURL)
				}
				return &http.Client{Transport: transport}, nil
			}, func(request application.SDKClientRequest) {
				require.NotNil(t, request.HTTPSProxyOverride)
				if useProxy {
					assert.Equal(t, proxy.URL, *request.HTTPSProxyOverride)
					return
				}
				assert.Empty(t, *request.HTTPSProxyOverride)
			})
			var stdout, stderr bytes.Buffer
			proxyValue := ""
			if useProxy {
				proxyValue = proxy.URL
			}
			require.Equal(t, 0, Run([]string{"pixiv", "download", "42", "--proxy", proxyValue}, strings.NewReader(""), &stdout, &stderr), stderr.String())
			assert.Contains(t, stdout.String(), `downloaded 42 "proxy" by artist`)
			files, err := os.ReadDir(downloadPath)
			require.NoError(t, err)
			require.Len(t, files, 1)
			if useProxy {
				assert.NotZero(t, proxy.Requests())
			} else {
				assert.Zero(t, proxy.Requests())
			}
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
