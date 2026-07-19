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
		Filters: sdk.SearchIllustFilters{
			Rating: sdk.SearchRatingAll, ContentType: sdk.SearchContentTypeAll,
			AIMode: sdk.SearchAIModeAll, AspectRatio: sdk.SearchAspectRatioAll,
			Resolution: sdk.SearchResolutionAll,
		},
	}, got)
	assert.JSONEq(t, `{"illusts":[{"id":123,"title":"work","type":"","page_count":0,"total_bookmarks":0,"total_view":0,"x_restrict":0,"user":{"id":0,"name":"artist","account":"","comment":"","is_followed":false,"profile_image_urls":{}},"tags":null,"image_urls":{"square_medium":"","medium":"","large":"","original":""},"meta_single_page":{"original_image_url":""},"meta_pages":null,"ai_type":0,"create_date":"","width":0,"height":0,"tools":null}]}`, stdout.String())
}

func TestSearchPassesStableFiltersToSDKAndFollowsCursorUntilLimit(t *testing.T) {
	useTempPaths(t)
	var cursors []sdk.Cursor
	var filters []sdk.SearchIllustFilters
	setTestSDKCommandClient(t, sdkCommandFake{search: func(_ context.Context, request sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
		cursors = append(cursors, request.Cursor)
		filters = append(filters, request.Filters)
		switch request.Cursor {
		case "":
			return &sdk.IllustListResult{Illusts: []sdk.Illust{{ID: 4}}, NextCursor: "second"}, nil
		case "second":
			return &sdk.IllustListResult{Illusts: []sdk.Illust{{ID: 5}}}, nil
		default:
			return nil, fmt.Errorf("unexpected cursor %q", request.Cursor)
		}
	}})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "miku", "--rating", "r18", "--type", "comics", "--ai-mode", "only", "--resolution", "high", "--aspect-ratio", "portrait", "--tool", "CLIP STUDIO PAINT", "--limit", "2", "--json"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, []sdk.Cursor{"", "second"}, cursors)
	wantFilters := sdk.SearchIllustFilters{
		Rating: sdk.SearchRatingR18, ContentType: sdk.SearchContentTypeManga,
		AIMode: sdk.SearchAIModeOnly, AspectRatio: sdk.SearchAspectRatioPortrait,
		Resolution: sdk.SearchResolutionHigh, Tool: "CLIP STUDIO PAINT",
	}
	assert.Equal(t, []sdk.SearchIllustFilters{wantFilters, wantFilters}, filters)
	var out struct {
		Illusts []sdk.Illust `json:"illusts"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out))
	assert.Equal(t, []int64{4, 5}, []int64{out.Illusts[0].ID, out.Illusts[1].ID})
}

func TestSearchMapsRemainingCanonicalFiltersToSDK(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want sdk.SearchIllustFilters
	}{
		{name: "medium resolution", args: []string{"--resolution", "medium"}, want: sdk.SearchIllustFilters{Resolution: sdk.SearchResolutionMedium}},
		{name: "low resolution", args: []string{"--resolution", "low"}, want: sdk.SearchIllustFilters{Resolution: sdk.SearchResolutionLow}},
		{name: "landscape", args: []string{"--aspect-ratio", "landscape"}, want: sdk.SearchIllustFilters{AspectRatio: sdk.SearchAspectRatioLandscape}},
		{name: "square", args: []string{"--aspect-ratio", "square"}, want: sdk.SearchIllustFilters{AspectRatio: sdk.SearchAspectRatioSquare}},
		{name: "illust and ugoira", args: []string{"--type", "illust-and-ugoira"}, want: sdk.SearchIllustFilters{ContentType: sdk.SearchContentTypeIllustAndUgoira}},
		{name: "illust", args: []string{"--type", "illust"}, want: sdk.SearchIllustFilters{ContentType: sdk.SearchContentTypeIllust}},
		{name: "manga", args: []string{"--type", "manga"}, want: sdk.SearchIllustFilters{ContentType: sdk.SearchContentTypeManga}},
		{name: "ugoira", args: []string{"--type", "ugoira"}, want: sdk.SearchIllustFilters{ContentType: sdk.SearchContentTypeUgoira}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			useTempPaths(t)
			var got sdk.SearchIllustFilters
			setTestSDKCommandClient(t, sdkCommandFake{search: func(_ context.Context, request sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
				got = request.Filters
				return &sdk.IllustListResult{}, nil
			}})
			var stdout, stderr bytes.Buffer
			args := append([]string{"pixiv", "search", "miku", "--json"}, test.args...)
			require.Equal(t, 0, Run(args, strings.NewReader(""), &stdout, &stderr), stderr.String())
			if test.want.Resolution != "" {
				assert.Equal(t, test.want.Resolution, got.Resolution)
			}
			if test.want.AspectRatio != "" {
				assert.Equal(t, test.want.AspectRatio, got.AspectRatio)
			}
			if test.want.ContentType != "" {
				assert.Equal(t, test.want.ContentType, got.ContentType)
			}
		})
	}
}

func TestSearchDeprecatedAITypeUsesDocumentedSemanticMapping(t *testing.T) {
	for _, test := range []struct {
		value string
		want  sdk.SearchAIMode
	}{{"0", sdk.SearchAIModeExclude}, {"1", sdk.SearchAIModeOnly}, {"2", sdk.SearchAIModeAll}} {
		t.Run(test.value, func(t *testing.T) {
			useTempPaths(t)
			var got sdk.SearchIllustRequest
			setTestSDKCommandClient(t, sdkCommandFake{search: func(_ context.Context, request sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
				got = request
				return &sdk.IllustListResult{}, nil
			}})
			var stdout, stderr bytes.Buffer
			code := Run([]string{"pixiv", "search", "miku", "--ai-type", test.value, "--json"}, strings.NewReader(""), &stdout, &stderr)
			require.Equal(t, 0, code, stderr.String())
			assert.Equal(t, test.want, got.Filters.AIMode)
		})
	}
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
		{name: "ai mode", args: []string{"--ai-mode", "sometimes"}, want: "ai-mode must be one of"},
		{name: "resolution", args: []string{"--resolution", "huge"}, want: "resolution must be one of"},
		{name: "aspect ratio", args: []string{"--aspect-ratio", "wide"}, want: "aspect-ratio must be one of"},
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

func TestSearchRejectsConflictingCompatibilityFlags(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "ai", args: []string{"--ai-mode", "only", "--ai-type", "1"}, want: "ai-mode and ai-type cannot be used together"},
		{name: "r18", args: []string{"--r18", "--rating", "sfw"}, want: "r18 conflicts with rating"},
	} {
		t.Run(test.name, func(t *testing.T) {
			useTempPaths(t)
			setTestSDKCommandClient(t, sdkCommandFake{})
			var stdout, stderr bytes.Buffer
			code := Run(append([]string{"pixiv", "search", "miku"}, test.args...), strings.NewReader(""), &stdout, &stderr)
			require.NotZero(t, code)
			assert.Contains(t, stderr.String(), test.want)
		})
	}
}

func TestSearchR18AliasSetsRatingWithoutChangingWord(t *testing.T) {
	useTempPaths(t)
	var got sdk.SearchIllustRequest
	setTestSDKCommandClient(t, sdkCommandFake{search: func(_ context.Context, request sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
		got = request
		return &sdk.IllustListResult{}, nil
	}})
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "miku", "--r18", "--json"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "miku", got.Word)
	assert.Equal(t, sdk.SearchRatingR18, got.Filters.Rating)
}

func TestSearchOptionsRoutesWordAndPrintsJSON(t *testing.T) {
	useTempPaths(t)
	var got sdk.SearchIllustOptionsRequest
	setTestSDKCommandClient(t, sdkCommandFake{searchOptions: func(_ context.Context, request sdk.SearchIllustOptionsRequest) (*sdk.SearchIllustOptionsResult, error) {
		got = request
		return &sdk.SearchIllustOptionsResult{Tools: []string{"CLIP STUDIO PAINT", "Photoshop"}}, nil
	}})
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search-options", "初音", "ミク", "--json"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, sdk.SearchIllustOptionsRequest{Word: "初音 ミク"}, got)
	assert.JSONEq(t, `{"tools":["CLIP STUDIO PAINT","Photoshop"]}`, stdout.String())
}

func TestSearchOptionsTextClearlyPrintsEmptyTools(t *testing.T) {
	useTempPaths(t)
	setTestSDKCommandClient(t, sdkCommandFake{searchOptions: func(context.Context, sdk.SearchIllustOptionsRequest) (*sdk.SearchIllustOptionsResult, error) {
		return &sdk.SearchIllustOptionsResult{Tools: []string{}}, nil
	}})
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search-options", "miku"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "search options for \"miku\"\ntools: none\n", stdout.String())
}

func TestSearchOptionsTextEscapesControlCharactersInToolNames(t *testing.T) {
	useTempPaths(t)
	tool := "safe\nline\r\x1b[31mred"
	setTestSDKCommandClient(t, sdkCommandFake{searchOptions: func(context.Context, sdk.SearchIllustOptionsRequest) (*sdk.SearchIllustOptionsResult, error) {
		return &sdk.SearchIllustOptionsResult{Tools: []string{tool}}, nil
	}})
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search-options", "miku"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "search options for \"miku\"\ntools:\n- safe\\nline\\r\\x1b[31mred\n", stdout.String())
	assert.NotContains(t, stdout.String(), "\x1b")

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "search-options", "miku", "--json"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	var result sdk.SearchIllustOptionsResult
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	assert.Equal(t, []string{tool}, result.Tools)
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
		{name: "search options", args: []string{"pixiv", "search-options", "miku", "--proxy", "http://flag-proxy"}},
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
		{name: "search options", args: []string{"pixiv", "search-options", "miku", "--proxy", ""}},
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
		{"pixiv", "search-options", "miku", "--no-proxy"},
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
		searchOptions: func(context.Context, sdk.SearchIllustOptionsRequest) (*sdk.SearchIllustOptionsResult, error) {
			return &sdk.SearchIllustOptionsResult{Tools: []string{}}, nil
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
