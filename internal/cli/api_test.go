package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pixivdeps "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivdeps"
	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/config"
	filesecret "github.com/FlanChanXwO/pixiv-cli/internal/storage/file/secret"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchRoutesArgumentsAndPrintsSDKJSON(t *testing.T) {
	useTempPaths(t)
	var got pixiv.SearchArtworksRequest
	setTestSDKCommandClient(t, &sdkCommandFake{search: func(_ context.Context, request pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		got = request
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{commandArtwork(123)}}, nil
	}})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "初音ミク", "--json"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, pixiv.SearchArtworksRequest{
		Word: "初音ミク", Target: pixiv.SearchTargetPartialMatchForTags, Sort: pixiv.SortModeDateDesc,
		ContentType: pixiv.SearchContentTypeAll,
		AIMode:      pixiv.SearchAIModeAll, AspectRatio: pixiv.SearchAspectRatioAll,
		Resolution: pixiv.SearchResolutionAll,
	}, got)
	assert.Contains(t, stdout.String(), `"illusts"`)
	assert.Contains(t, stdout.String(), `"id": 123`)
	assert.Contains(t, stdout.String(), `"title": "work"`)
}

func TestSearchReadsOneImplicitValueFromStdin(t *testing.T) {
	useTempPaths(t)
	var got pixiv.SearchArtworksRequest
	setTestSDKCommandClient(t, &sdkCommandFake{search: func(_ context.Context, request pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		got = request
		return sdk.Page[pixiv.Artwork]{}, nil
	}})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "--json"}, strings.NewReader("  miku hatsune  \r\n"), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "  miku hatsune  ", got.Word)
	assert.Empty(t, stderr.String())
}

func TestSearchTreatsDashPrefixedKeywordAfterEndOfOptionsAsLiteral(t *testing.T) {
	useTempPaths(t)
	var got pixiv.SearchArtworksRequest
	setTestSDKCommandClient(t, &sdkCommandFake{search: func(_ context.Context, request pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		got = request
		return sdk.Page[pixiv.Artwork]{}, nil
	}})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "--", "--starts-with-dash"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "--starts-with-dash", got.Word)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestNovelSearchRoutesFiltersAndPrintsJSON(t *testing.T) {
	useTempPaths(t)
	var got pixiv.SearchNovelsRequest
	setTestSDKCommandClient(t, &sdkCommandFake{searchNovel: func(_ context.Context, request pixiv.SearchNovelsRequest) (sdk.Page[pixiv.Novel], error) {
		got = request
		return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{
			ID: 9, Title: "novel", Caption: "description", XRestrict: 1, TextLength: 500, IsOriginal: true,
			User: pixiv.User{ID: 3, Name: "author"}, Tags: []pixiv.Tag{},
		}}}, nil
	}})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "novel", "search", "miku", "--search-by", "title-caption", "--period", "week", "--json"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, pixiv.SearchNovelsRequest{
		Word: "miku", Target: pixiv.SearchTargetTitleAndCaption, Sort: pixiv.SortModeDateDesc, Duration: pixiv.DurationFilter("within_last_week"),
	}, got)
	assert.Contains(t, stdout.String(), `"novels"`)
	assert.Contains(t, stdout.String(), `"id": 9`)
	assert.Contains(t, stdout.String(), `"title": "novel"`)
}

func TestUserSearchPrintsPreviews(t *testing.T) {
	useTempPaths(t)
	var got pixiv.SearchUsersRequest
	setTestSDKCommandClient(t, &sdkCommandFake{searchUser: func(_ context.Context, request pixiv.SearchUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
		got = request
		return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 8, Name: "author", Account: "account", Comment: "profile"}}}}, nil
	}})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "user", "search", "author", "--json"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, pixiv.SearchUsersRequest{Word: "author"}, got)
	assert.Contains(t, stdout.String(), `"user_previews"`)
	assert.Contains(t, stdout.String(), `"id": 8`)
}

func TestDetailRendersCaptionAsSafePlainText(t *testing.T) {
	useTempPaths(t)
	setTestSDKCommandClient(t, &sdkCommandFake{detail: func(context.Context, int64) (pixiv.Artwork, error) {
		return pixiv.Artwork{ID: 42, Title: "work", User: pixiv.User{ID: 7, Name: "artist"}, Caption: "<p>Line one<br>Line two &amp; \u001bunsafe</p>"}, nil
	}})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "detail", "42"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Contains(t, stdout.String(), "caption:\nLine one\nLine two & \\x1bunsafe\n")
	assert.NotContains(t, stdout.String(), "<p>")
}

func TestDetailAcceptsArtworkURL(t *testing.T) {
	useTempPaths(t)
	var gotID int64
	setTestSDKCommandClient(t, &sdkCommandFake{detail: func(_ context.Context, id int64) (pixiv.Artwork, error) {
		gotID = id
		return commandArtwork(id), nil
	}})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "detail", "https://www.pixiv.net/en/artworks/42?utm_source=share#page=1", "--json"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, int64(42), gotID)
	var detail pixiv.Artwork
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &detail))
	assert.Equal(t, int64(42), detail.ID)
	assert.Equal(t, "work", detail.Title)
}

func TestDetailRejectsUnsupportedURLBeforeOpeningSDK(t *testing.T) {
	useTempPaths(t)
	setTestSDKCommandFactory(t, &sdkCommandFake{})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "detail", "https://www.pixiv.net/users/7?secret=must-not-echo"}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	assert.Empty(t, stdout.String())
	assert.NotContains(t, stderr.String(), "must-not-echo")
	assert.Contains(t, stderr.String(), "supported Pixiv")
}

func TestSearchHelpUsesEnglishExamples(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "--help"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Contains(t, stdout.String(), `pixiv search "miku" --json`)
	assert.NotContains(t, stdout.String(), "初音ミク")
}

func TestNovelAndUserSearchHelpUsesEnglishExamples(t *testing.T) {
	for _, test := range []struct {
		name    string
		args    []string
		example string
	}{
		{name: "novel", args: []string{"pixiv", "novel", "search", "--help"}, example: `pixiv novel search "miku" --json`},
		{name: "user", args: []string{"pixiv", "user", "search", "--help"}, example: `pixiv user search "miku" --json`},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(test.args, strings.NewReader(""), &stdout, &stderr)

			require.Equal(t, 0, code, stderr.String())
			assert.Contains(t, stdout.String(), test.example)
			assert.NotContains(t, stdout.String(), "初音ミク")
		})
	}
}

func TestSearchRejectsDownloadOnlyFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--download-path", "/tmp/downloads"},
		{"--filename-template", "{id}"},
	} {
		t.Run(args[0], func(t *testing.T) {
			useTempPaths(t)
			setTestSDKCommandClient(t, &sdkCommandFake{search: func(context.Context, pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
				return sdk.Page[pixiv.Artwork]{}, nil
			}})

			var stdout, stderr bytes.Buffer
			code := Run(append([]string{"pixiv", "search", "miku"}, args...), strings.NewReader(""), &stdout, &stderr)

			require.NotZero(t, code)
			assert.Contains(t, stderr.String(), "unknown option '"+args[0]+"'")
		})
	}
}

func TestSearchUsesSearchByAndRejectsRemovedTargetFlags(t *testing.T) {
	useTempPaths(t)
	var got pixiv.SearchArtworksRequest
	setTestSDKCommandClient(t, &sdkCommandFake{search: func(_ context.Context, request pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		got = request
		return sdk.Page[pixiv.Artwork]{}, nil
	}})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "miku", "--search-by", "title-caption", "--json"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, pixiv.SearchTargetTitleAndCaption, got.Target)

	stdout.Reset()
	stderr.Reset()
	for _, removed := range []string{"--target", "--search-target"} {
		stdout.Reset()
		stderr.Reset()
		code = Run([]string{"pixiv", "search", "miku", removed, "title-caption"}, strings.NewReader(""), &stdout, &stderr)
		require.NotZero(t, code)
		assert.Contains(t, stderr.String(), "unknown option '"+removed+"'")
	}
}

func TestSearchUsesPeriodInsteadOfDuration(t *testing.T) {
	useTempPaths(t)
	var got pixiv.SearchArtworksRequest
	setTestSDKCommandClient(t, &sdkCommandFake{search: func(_ context.Context, request pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		got = request
		return sdk.Page[pixiv.Artwork]{}, nil
	}})

	var stdout, stderr bytes.Buffer
	for _, test := range []struct {
		period          string
		duration        pixiv.DurationFilter
		explicitDateSet bool
	}{
		{period: "week", duration: pixiv.DurationFilter("within_last_week")},
		{period: "half-year", explicitDateSet: true},
		{period: "year", explicitDateSet: true},
	} {
		stdout.Reset()
		stderr.Reset()
		code := Run([]string{"pixiv", "search", "miku", "--period", test.period, "--json"}, strings.NewReader(""), &stdout, &stderr)
		require.Equal(t, 0, code, stderr.String())
		assert.Equal(t, test.duration, got.Duration)
		if test.explicitDateSet {
			assert.NotEmpty(t, got.StartDate)
			assert.NotEmpty(t, got.EndDate)
			assert.LessOrEqual(t, got.StartDate, got.EndDate)
		} else {
			assert.Empty(t, got.StartDate)
			assert.Empty(t, got.EndDate)
		}
	}

	stdout.Reset()
	stderr.Reset()
	code := Run([]string{"pixiv", "search", "miku", "--duration", "within_last_week"}, strings.NewReader(""), &stdout, &stderr)
	require.NotZero(t, code)
	assert.Contains(t, stderr.String(), "unknown option '--duration'")
}

func TestSearchMapsKeywordDateAndBookmarkFilters(t *testing.T) {
	useTempPaths(t)
	var got pixiv.SearchArtworksRequest
	setTestSDKCommandClient(t, &sdkCommandFake{search: func(_ context.Context, request pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		got = request
		return sdk.Page[pixiv.Artwork]{}, nil
	}})

	var stdout, stderr bytes.Buffer
	code := Run([]string{
		"pixiv", "search", "miku", "--search-by", "tag-title-caption", "--start-date", "2026-01-01", "--end-date", "2026-01-31",
		"--bookmark-min", "1000", "--bookmark-max", "10000", "--bookmark-strategy", "best_effort", "--json",
	}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, pixiv.SearchTargetKeyword, got.Target)
	assert.Equal(t, "2026-01-01", got.StartDate)
	assert.Equal(t, "2026-01-31", got.EndDate)
	if got.BookmarkMin == nil || *got.BookmarkMin != 1000 || got.BookmarkMax == nil || *got.BookmarkMax != 10000 {
		t.Fatalf("bookmark filters = %+v", got)
	}
}

func TestSearchRejectsInvalidDateAndBookmarkRangesBeforeSearch(t *testing.T) {
	useTempPaths(t)
	calls := 0
	setTestSDKCommandClient(t, &sdkCommandFake{search: func(_ context.Context, _ pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		calls++
		return sdk.Page[pixiv.Artwork]{}, nil
	}})

	for _, args := range [][]string{
		{"pixiv", "search", "miku", "--period", "week", "--start-date", "2026-01-01"},
		{"pixiv", "search", "miku", "--start-date", "2026-02-30"},
		{"pixiv", "search", "miku", "--start-date", "2026-02-01", "--end-date", "2026-01-31"},
		{"pixiv", "search", "miku", "--bookmark-min", "10000", "--bookmark-max", "1000"},
	} {
		var stdout, stderr bytes.Buffer
		code := Run(args, strings.NewReader(""), &stdout, &stderr)
		require.NotZero(t, code, "args=%v", args)
	}
	assert.Zero(t, calls)
}

func TestListHelpDoesNotExposeInternalLimitSentinel(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "--help"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.NotContains(t, stdout.String(), "default -1")
	assert.Contains(t, stdout.String(), "omitted returns one upstream batch; 0 returns all results")
}

func TestRankingHelpListsAllModesAndAuthenticationRequirement(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "ranking", "--help"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	for _, mode := range []string{
		"day", "day_male", "day_female", "week", "week_original", "week_rookie", "month",
		"day_manga", "week_manga", "month_manga", "week_rookie_manga",
		"day_r18", "day_male_r18", "day_female_r18", "week_r18", "week_r18g",
	} {
		assert.Contains(t, stdout.String(), mode)
	}
	assert.Contains(t, stdout.String(), "require authentication")
}

func TestCommandHelpListsRemainingParameterDomains(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "search sort", args: []string{"pixiv", "search", "--help"}, want: "date_desc, date_asc"},
		{name: "user artworks type", args: []string{"pixiv", "user", "artworks", "--help"}, want: "illust, manga, ugoira"},
		{name: "download filename template", args: []string{"pixiv", "download", "--help"}, want: "{id}, {title}, {author}"},
		{name: "login callback address", args: []string{"pixiv", "auth", "login", "--help"}, want: "127.0.0.1:0"},
		{name: "login timeout", args: []string{"pixiv", "auth", "login", "--help"}, want: "0 adds no deadline"},
		{name: "auth export force", args: []string{"pixiv", "auth", "export", "--help"}, want: "requires --output"},
		{name: "config key", args: []string{"pixiv", "config", "set", "--help"}, want: "filename_template"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(test.args, strings.NewReader(""), &stdout, &stderr)

			require.Equal(t, 0, code, stderr.String())
			assert.Contains(t, stdout.String(), test.want)
		})
	}
}

func TestEveryOtherDataCommandRejectsDownloadOnlyFlags(t *testing.T) {
	commands := [][]string{
		{"pixiv", "detail", "1"},
		{"pixiv", "ranking"},
		{"pixiv", "recommended", "illust"},
		{"pixiv", "user", "detail", "1"},
		{"pixiv", "user", "artworks"},
		{"pixiv", "user", "bookmarks"},
		{"pixiv", "user", "following"},
		{"pixiv", "bookmark", "add", "1"},
		{"pixiv", "bookmark", "remove", "1"},
		{"pixiv", "follow", "add", "1"},
		{"pixiv", "follow", "remove", "1"},
	}
	for _, command := range commands {
		for _, flag := range []string{"--download-path", "--filename-template"} {
			t.Run(strings.Join(append(command[1:], flag), " "), func(t *testing.T) {
				useTempPaths(t)

				var stdout, stderr bytes.Buffer
				args := append(append([]string(nil), command...), flag, "ignored")
				code := Run(args, strings.NewReader(""), &stdout, &stderr)

				require.NotZero(t, code)
				assert.Contains(t, stderr.String(), "unknown option '"+flag+"'")
			})
		}
	}
}

func TestRankingPassesExtendedModeToSDK(t *testing.T) {
	useTempPaths(t)
	var got pixiv.ArtworkRankingRequest
	setTestSDKCommandClient(t, &sdkCommandFake{ranking: func(_ context.Context, request pixiv.ArtworkRankingRequest) (sdk.Page[pixiv.Artwork], error) {
		got = request
		return sdk.Page[pixiv.Artwork]{}, nil
	}})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "ranking", "--mode", "week_r18g", "--json"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run() code=%d stderr=%s", code, stderr.String())
	}
	if got.Mode != pixiv.RankingModeWeekR18G || got.Date != "" || !got.Cursor.IsZero() {
		t.Fatalf("ranking request = %#v", got)
	}
}

func TestSearchPassesStableFiltersToSDKAndFollowsCursorUntilLimit(t *testing.T) {
	useTempPaths(t)
	secondCursor := testCursor(t, "second")
	var cursors []sdk.Cursor
	setTestSDKCommandClient(t, &sdkCommandFake{search: func(_ context.Context, request pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		cursors = append(cursors, request.Cursor)
		switch {
		case request.Cursor.IsZero():
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{{ID: 4, AIType: 2}}, Next: secondCursor}, nil
		case request.Cursor.String() == secondCursor.String():
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{{ID: 5, AIType: 2}}}, nil
		default:
			return sdk.Page[pixiv.Artwork]{}, fmt.Errorf("unexpected cursor %q", request.Cursor.String())
		}
	}})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "miku", "--type", "artwork", "--content-type", "manga", "--ai-mode", "only", "--resolution", "high", "--aspect-ratio", "portrait", "--draw-tool", "CLIP STUDIO PAINT", "--limit", "2", "--json"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, []sdk.Cursor{{}, secondCursor}, cursors)
	var out struct {
		Illusts []pixiv.Artwork `json:"illusts"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out))
	assert.Equal(t, []int64{4, 5}, []int64{out.Illusts[0].ID, out.Illusts[1].ID})

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "search", "miku", "--tool", "CLIP STUDIO PAINT"}, strings.NewReader(""), &stdout, &stderr)
	require.NotZero(t, code)
	assert.Contains(t, stderr.String(), "unknown option '--tool'")
}

func TestSearchMapsRemainingCanonicalFiltersToSDK(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		check func(pixiv.SearchArtworksRequest) bool
	}{
		{name: "medium resolution", args: []string{"--resolution", "medium"}, check: func(r pixiv.SearchArtworksRequest) bool { return r.Resolution == pixiv.SearchResolutionMedium }},
		{name: "low resolution", args: []string{"--resolution", "low"}, check: func(r pixiv.SearchArtworksRequest) bool { return r.Resolution == pixiv.SearchResolutionLow }},
		{name: "landscape", args: []string{"--aspect-ratio", "landscape"}, check: func(r pixiv.SearchArtworksRequest) bool { return r.AspectRatio == pixiv.SearchAspectRatioLandscape }},
		{name: "square", args: []string{"--aspect-ratio", "square"}, check: func(r pixiv.SearchArtworksRequest) bool { return r.AspectRatio == pixiv.SearchAspectRatioSquare }},
		{name: "illust and ugoira", args: []string{"--content-type", "illust-and-ugoira"}, check: func(r pixiv.SearchArtworksRequest) bool {
			return r.ContentType == pixiv.SearchContentTypeIllustAndUgoira
		}},
		{name: "illust", args: []string{"--content-type", "illust"}, check: func(r pixiv.SearchArtworksRequest) bool { return r.ContentType == pixiv.SearchContentTypeIllust }},
		{name: "manga", args: []string{"--content-type", "manga"}, check: func(r pixiv.SearchArtworksRequest) bool { return r.ContentType == pixiv.SearchContentTypeManga }},
		{name: "ugoira", args: []string{"--content-type", "ugoira"}, check: func(r pixiv.SearchArtworksRequest) bool { return r.ContentType == pixiv.SearchContentTypeUgoira }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			useTempPaths(t)
			var got pixiv.SearchArtworksRequest
			setTestSDKCommandClient(t, &sdkCommandFake{search: func(_ context.Context, request pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
				got = request
				return sdk.Page[pixiv.Artwork]{}, nil
			}})
			var stdout, stderr bytes.Buffer
			args := append([]string{"pixiv", "search", "miku", "--json"}, test.args...)
			require.Equal(t, 0, Run(args, strings.NewReader(""), &stdout, &stderr), stderr.String())
			if !test.check(got) {
				t.Fatalf("request = %+v", got)
			}
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
		{name: "type", args: []string{"--type", "unknown"}, want: "type must be one of"},
		{name: "ai mode", args: []string{"--ai-mode", "sometimes"}, want: "ai-mode must be one of"},
		{name: "resolution", args: []string{"--resolution", "huge"}, want: "resolution must be one of"},
		{name: "aspect ratio", args: []string{"--aspect-ratio", "wide"}, want: "aspect-ratio must be one of"},
	} {
		t.Run(test.name, func(t *testing.T) {
			useTempPaths(t)
			setTestSDKCommandFactory(t, &sdkCommandFake{})

			var stdout, stderr bytes.Buffer
			code := Run(append([]string{"pixiv", "search", "miku"}, test.args...), strings.NewReader(""), &stdout, &stderr)

			require.NotZero(t, code)
			assert.Contains(t, stderr.String(), test.want)
		})
	}
}

func TestSearchRejectsRemovedCompatibilityFlags(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "ai-type", args: []string{"--ai-type", "1"}, want: "unknown option '--ai-type'"},
		{name: "r18", args: []string{"--r18"}, want: "unknown option '--r18'"},
		{name: "profile", args: []string{"--profile", "111"}, want: "unknown option '--profile'"},
		{name: "offset", args: []string{"--offset", "1"}, want: "unknown option '--offset'"},
		{name: "comics type", args: []string{"--type", "comics"}, want: "type must be one of"},
	} {
		t.Run(test.name, func(t *testing.T) {
			useTempPaths(t)
			setTestSDKCommandFactory(t, &sdkCommandFake{})

			var stdout, stderr bytes.Buffer
			code := Run(append([]string{"pixiv", "search", "miku"}, test.args...), strings.NewReader(""), &stdout, &stderr)
			require.NotZero(t, code)
			assert.Contains(t, stderr.String(), test.want)
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
	require.NoError(t, filesecret.WritePrivateFile(configPath, []byte("[output]\njson = true\n"), localstate.PrivateFileMode))
	setTestSDKCommandClient(t, &sdkCommandFake{search: func(context.Context, pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{commandArtwork(321)}}, nil
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
			var got pixivdeps.Request
			setTestSDKCommandFactoryObserve(t, proxySDKClientPtr(), func(request pixivdeps.Request) {
				got = request
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
			var got pixivdeps.Request
			setTestSDKCommandFactoryObserve(t, proxySDKClientPtr(), func(request pixivdeps.Request) {
				got = request
			})

			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, Run(tt.args, strings.NewReader(""), &stdout, &stderr), stderr.String())
			require.NotNil(t, got.HTTPSProxyOverride)
			assert.Empty(t, *got.HTTPSProxyOverride)
		})
	}
}

func TestDataCommandsRejectCredentialSelectionFlags(t *testing.T) {
	useTempPaths(t)
	setTestSDKCommandFactory(t, proxySDKClientPtr())

	for _, args := range [][]string{
		{"pixiv", "search", "miku", "--uid", "222"},
		{"pixiv", "search", "miku", "--refresh-token", "secret"},
	} {
		var stdout, stderr bytes.Buffer
		require.NotZero(t, Run(args, strings.NewReader(""), &stdout, &stderr), stderr.String())
		assert.Contains(t, stderr.String(), "unknown option")
	}
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
			var got pixivdeps.Request
			setTestSDKCommandFactoryObserve(t, proxySDKClientPtr(), func(request pixivdeps.Request) {
				got = request
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
	var gotClientRequest pixivdeps.Request
	var downloadRequests []downloader.DownloadRequest
	setTestDownloadCommandServices(t, client, func(request pixivdeps.Request) {
		gotClientRequest = request
	}, config.RuntimeConfig{DownloadPath: "/runtime/path", FilenameTemplate: "runtime-template"}, func(gotClient downloader.DownloadClient, gotPath, gotTemplate string) (downloader.DownloadManager, error) {
		require.NotNil(t, gotClient)
		require.Equal(t, "/flag/path", gotPath)
		require.Equal(t, "flag-template", gotTemplate)
		return downloadManagerFake{download: func(gotContext context.Context, request downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error) {
			require.Same(t, ctx, gotContext)
			downloadRequests = append(downloadRequests, request)
			items := make([]downloader.DownloadedArtwork, 0, len(request.IllustIDs))
			for _, id := range request.IllustIDs {
				items = append(items, downloader.DownloadedArtwork{
					IllustID: id,
					Title:    "work",
					Author:   "artist",
					Files:    []downloader.DownloadedFile{{Path: fmt.Sprintf("/flag/path/%d.jpg", id)}},
				})
			}
			return items, nil
		}}, nil
	})

	var stdout, stderr bytes.Buffer
	code := RunContext(ctx, []string{
		"pixiv", "download", "42", "84",
		"--download-path", "/flag/path",
		"--filename-template", "flag-template",
		"--ugoira-mode", "apng",
		"--proxy", "http://flag-proxy",
	}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	require.NotNil(t, gotClientRequest.HTTPSProxyOverride)
	require.Equal(t, "http://flag-proxy", *gotClientRequest.HTTPSProxyOverride)
	assert.Zero(t, gotClientRequest.UserID)
	require.Len(t, downloadRequests, 1)
	require.Equal(t, []int64{42, 84}, downloadRequests[0].IllustIDs)
	require.Equal(t, downloader.UgoiraFormatAPNG, downloadRequests[0].UgoiraFormat)
	assert.Empty(t, stdout.String())
}

func TestDownloadOutputShortNameMatchesDownloadPath(t *testing.T) {
	for _, flag := range []string{"--download-path", "-o"} {
		t.Run(flag, func(t *testing.T) {
			useTempPaths(t)
			var gotPath string
			setTestDownloadCommandServices(t, &sdkCommandFake{}, nil, config.RuntimeConfig{}, func(_ downloader.DownloadClient, path, _ string) (downloader.DownloadManager, error) {
				gotPath = path
				return downloadManagerFake{download: func(context.Context, downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error) {
					return nil, nil
				}}, nil
			})

			var stdout, stderr bytes.Buffer
			code := Run([]string{"pixiv", "download", "42", flag, "/flag/path"}, strings.NewReader(""), &stdout, &stderr)

			require.Equal(t, 0, code, stderr.String())
			assert.Equal(t, "/flag/path", gotPath)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestDownloadAcceptsArtworkAndUserURLsWithoutWritingAReport(t *testing.T) {
	useTempPaths(t)
	client := &sdkCommandFake{artworks: func(_ context.Context, request pixiv.UserArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		switch request.Kind {
		case pixiv.ArtworkKindIllustration:
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{{ID: 2, Kind: pixiv.ArtworkKindIllustration}}}, nil
		case pixiv.ArtworkKindManga:
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{{ID: 3, Kind: pixiv.ArtworkKindManga}}}, nil
		case pixiv.ArtworkKindUgoira:
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{{ID: 4, Kind: pixiv.ArtworkKindUgoira}}}, nil
		default:
			return sdk.Page[pixiv.Artwork]{}, errors.New("unexpected kind")
		}
	}}
	var requests []downloader.DownloadRequest
	setTestDownloadCommandServices(t, client, nil, config.RuntimeConfig{}, func(downloader.DownloadClient, string, string) (downloader.DownloadManager, error) {
		return downloadManagerFake{download: func(_ context.Context, request downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error) {
			requests = append(requests, request)
			items := make([]downloader.DownloadedArtwork, 0, len(request.IllustIDs))
			for _, id := range request.IllustIDs {
				items = append(items, downloader.DownloadedArtwork{
					IllustID: id, Title: "work", Author: "artist", Type: "illust",
					Files: []downloader.DownloadedFile{{Path: fmt.Sprintf("/downloads/%d.jpg", id), Page: 1}},
				})
			}
			return items, nil
		}}, nil
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "download", "https://www.pixiv.net/artworks/1", "https://www.pixiv.net/en/users/7/artworks"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	require.Len(t, requests, 1)
	require.Equal(t, []int64{1, 2, 3, 4}, requests[0].IllustIDs)
	assert.Empty(t, stdout.String())
}

func TestDownloadDelegatesRuntimePathAndTemplateWithoutFlags(t *testing.T) {
	useTempPaths(t)
	setTestDownloadCommandServices(t, &sdkCommandFake{}, nil, config.RuntimeConfig{DownloadPath: "/runtime/path", FilenameTemplate: "runtime-template"}, func(_ downloader.DownloadClient, path, template string) (downloader.DownloadManager, error) {
		require.Equal(t, "/runtime/path", path)
		require.Equal(t, "runtime-template", template)
		return downloadManagerFake{download: func(context.Context, downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error) {
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
	setTestDownloadCommandServices(t, &sdkCommandFake{}, nil, config.RuntimeConfig{}, func(downloader.DownloadClient, string, string) (downloader.DownloadManager, error) {
		return nil, want
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "download", "42"}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), want.Error())
}

func TestDownloadReportsManagerFailure(t *testing.T) {
	_, configPath := useTempPaths(t)
	want := errors.New("download manager failed")
	setTestDownloadCommandServices(t, &sdkCommandFake{}, nil, config.RuntimeConfig{}, func(downloader.DownloadClient, string, string) (downloader.DownloadManager, error) {
		return downloadManagerFake{download: func(context.Context, downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error) {
			return nil, want
		}}, nil
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "download", "42"}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), "download completed with 1 failures")
	_, err := os.Stat(filepath.Join(filepath.Dir(configPath), "logs"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestDownloadDoesNotAcceptJSONReportOutput(t *testing.T) {
	useTempPaths(t)
	setTestDownloadCommandServices(t, &sdkCommandFake{}, nil, config.RuntimeConfig{}, func(downloader.DownloadClient, string, string) (downloader.DownloadManager, error) {
		return downloadManagerFake{download: func(context.Context, downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error) {
			return []downloader.DownloadedArtwork{{
				IllustID: 42,
				Title:    "work",
				Author:   "artist",
				Type:     "illust",
				Files:    []downloader.DownloadedFile{{Path: "/downloads/42.jpg", Page: 2}},
			}}, nil
		}}, nil
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "download", "42"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	require.Empty(t, stdout.String())
}

func TestDownloadRejectsInvalidPagesAndQuality(t *testing.T) {
	useTempPaths(t)
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "pages", args: []string{"--pages", "0"}, want: "invalid page"},
		{name: "quality", args: []string{"--quality", "huge"}, want: "quality must be one of"},
		{name: "ugoira", args: []string{"--ugoira-mode", "zip"}, want: "ugoira format must be one of"},
	} {
		t.Run(test.name, func(t *testing.T) {
			setTestDownloadCommandServices(t, &sdkCommandFake{}, nil, config.RuntimeConfig{}, func(downloader.DownloadClient, string, string) (downloader.DownloadManager, error) {
				return downloadManagerFake{}, nil
			})
			var stdout, stderr bytes.Buffer
			code := Run(append([]string{"pixiv", "download", "42"}, test.args...), strings.NewReader(""), &stdout, &stderr)
			require.NotZero(t, code)
			assert.Contains(t, stderr.String(), test.want)
		})
	}
}

type downloadManagerFake struct {
	download func(context.Context, downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error)
}

func (m downloadManagerFake) Download(ctx context.Context, request downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error) {
	return m.download(ctx, request)
}

func setTestDownloadCommandServices(t *testing.T, fake *sdkCommandFake, observe func(pixivdeps.Request), runtime config.RuntimeConfig, newManager downloader.DownloadManagerFactory) {
	t.Helper()
	old := newCLIRunResources
	client := openCLIWireClient(t, fake)
	open := func(request pixivdeps.Request) (*pixiv.Client, error) {
		if observe != nil {
			observe(request)
		}
		return client, nil
	}
	newCLIRunResources = func() (*runResources, error) {
		return &runResources{
			runtimeLoaded: true,
			runtimeConfig: runtime,
			sdkLoaded:     true,
			sdk: pixivSDKPorts{
				open: open,
				pooled: func(ctx context.Context, request pixivdeps.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
					if _, err := open(request); err != nil {
						return err
					}
					_, err := attempt(ctx, client)
					return err
				},
				jsonOut: func(override *bool) (bool, error) {
					if override != nil {
						return *override, nil
					}
					return false, nil
				},
			},
			downloadLoaded: true,
			download:       downloader.DownloadService{NewManager: newManager},
		}, nil
	}
	t.Cleanup(func() { newCLIRunResources = old })
}

func TestDownloadProxyFlagPassesRuntimeOverride(t *testing.T) {
	for _, useProxy := range []bool{true, false} {
		t.Run(fmt.Sprintf("use_proxy=%t", useProxy), func(t *testing.T) {
			authPath, configPath := useTempPaths(t)
			proxy := newTestForwardProxy(t)
			downloadPath := t.TempDir()
			require.NoError(t, filesecret.WritePrivateFile(configPath, []byte("[network]\nhttps_proxy = \""+proxy.URL+"\"\n[download]\npath = \""+strings.ReplaceAll(downloadPath, "\\", "\\\\")+"\"\n"), localstate.PrivateFileMode))
			t.Setenv("https_proxy", proxy.URL)
			require.NoError(t, saveTestAuthStore(t, authPath, testAuthStore{DefaultUserID: 123, Accounts: []testAuthAccount{{UserID: 123, RefreshToken: "token"}}}))

			var upstream *httptest.Server
			upstream = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/auth/token":
					require.NoError(t, r.ParseForm())
					assert.Equal(t, "token", r.Form.Get("refresh_token"))
					_, _ = io.WriteString(w, `{"access_token":"access","refresh_token":"token","user":{"id":123,"name":"proxy-user"}}`)
				case "/v1/illust/detail":
					assert.Equal(t, "42", r.URL.Query().Get("illust_id"))
					_, _ = fmt.Fprintf(w, `{"illust":{"id":42,"title":"proxy","type":"illust","page_count":1,"create_date":"2024-01-01T00:00:00Z","user":{"id":123,"name":"artist"},"tags":[],"image_urls":{},"meta_single_page":{"original_image_url":%q},"meta_pages":[]}}`, upstream.URL+"/resource/proxy.jpg")
				case "/resource/proxy.jpg":
					_, _ = io.WriteString(w, "image")
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(upstream.Close)
			upstreamURL, err := url.Parse(upstream.URL)
			require.NoError(t, err)
			resourcePolicy := pixiv.ResourcePolicy{AllowedHosts: []string{upstreamURL.Hostname()}}
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
			}, func(request pixivdeps.Request) {
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
			assert.Empty(t, stdout.String())
			files, err := os.ReadDir(downloadPath)
			require.NoError(t, err)
			var artifacts []os.DirEntry
			for _, file := range files {
				if file.Name() != ".pixiv-cache" {
					artifacts = append(artifacts, file)
				}
			}
			require.Len(t, artifacts, 1)
			if useProxy {
				assert.NotZero(t, proxy.Requests())
			} else {
				assert.Zero(t, proxy.Requests())
			}
		})
	}
}

func proxySDKClientPtr() *sdkCommandFake {
	client := proxySDKClient()
	return &client
}

func proxySDKClient() sdkCommandFake {
	return sdkCommandFake{
		search: func(context.Context, pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
			return sdk.Page[pixiv.Artwork]{}, nil
		},
		detail: func(context.Context, int64) (pixiv.Artwork, error) {
			return commandArtwork(42), nil
		},
		ranking: func(context.Context, pixiv.ArtworkRankingRequest) (sdk.Page[pixiv.Artwork], error) {
			return sdk.Page[pixiv.Artwork]{}, nil
		},
		recommended: func(context.Context, pixiv.RecommendedArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
			return sdk.Page[pixiv.Artwork]{}, nil
		},
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

// rateLimitedOnceWriter 模拟一个下游 writer 失败时碰巧返回可分类的 Pixiv 429。
// 它绝不是上游请求失败，账号池必须将这次 stdout 尝试视为不可重放。
type rateLimitedOnceWriter struct {
	err    error
	failed bool
	bytes.Buffer
}

func (w *rateLimitedOnceWriter) Write(data []byte) (int, error) {
	if !w.failed {
		w.failed = true
		return 0, w.err
	}
	return w.Buffer.Write(data)
}

// TestNovelSearchReturnsFailureWhenTextStdoutFails 固定小说文本列表也把 stdout
// 写失败原样返回；作品列表的同一契约由 TestSearchReturnsFailureWhenTextStdoutFails
// 覆盖。
func TestNovelSearchReturnsFailureWhenTextStdoutFails(t *testing.T) {
	useTempPaths(t)
	want := errors.New("stdout unavailable")
	setTestSDKCommandClient(t, &sdkCommandFake{searchNovel: func(context.Context, pixiv.SearchNovelsRequest) (sdk.Page[pixiv.Novel], error) {
		return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 42, Title: "work", User: pixiv.User{Name: "artist"}}}}, nil
	}})

	var stderr bytes.Buffer
	code := Run([]string{"pixiv", "novel", "search", "miku"}, strings.NewReader(""), failingWriter{err: want}, &stderr)
	require.NotZero(t, code)
	assert.Contains(t, stderr.String(), want.Error())
}

func TestSearchReturnsFailureWhenTextStdoutFails(t *testing.T) {
	useTempPaths(t)
	want := errors.New("stdout unavailable")
	setTestSDKCommandClient(t, &sdkCommandFake{search: func(context.Context, pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{commandArtwork(42)}}, nil
	}})

	var stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "miku"}, strings.NewReader(""), failingWriter{err: want}, &stderr)
	require.NotZero(t, code)
	assert.Contains(t, stderr.String(), want.Error())
}

func TestDetailReturnsFailureWhenTextStdoutFails(t *testing.T) {
	useTempPaths(t)
	want := errors.New("stdout unavailable")
	setTestSDKCommandClient(t, &sdkCommandFake{detail: func(context.Context, int64) (pixiv.Artwork, error) {
		return commandArtwork(42), nil
	}})

	var stderr bytes.Buffer
	code := Run([]string{"pixiv", "detail", "42"}, strings.NewReader(""), failingWriter{err: want}, &stderr)
	require.NotZero(t, code)
	assert.Contains(t, stderr.String(), want.Error())
}

func TestSearchDoesNotReplayWhenWriterReturnsRateLimitedError(t *testing.T) {
	useTempPaths(t)
	rateLimited := sdk.NewError("pixiv", "search", sdk.RateLimited, sdk.WithRetry(sdk.RetryAdvice{Safe: true, HasAfter: true}))
	client := sdkCommandFake{search: func(context.Context, pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{commandArtwork(42)}}, nil
	}}
	wireClient := openCLIWireClient(t, &client)
	old := newCLIRunResources
	poolCalls := 0
	newCLIRunResources = func() (*runResources, error) {
		resources := newTestResources(t)
		resources.sdk.open = func(pixivdeps.Request) (*pixiv.Client, error) { return wireClient, nil }
		resources.sdk.pooled = func(ctx context.Context, _ pixivdeps.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			poolCalls++
			committed, err := attempt(ctx, wireClient)
			var typed *sdk.Error
			if err != nil && !committed && errors.As(err, &typed) && typed.Reason == sdk.RateLimited && typed.Retry.HasAfter {
				poolCalls++
				_, err = attempt(ctx, wireClient)
			}
			return err
		}
		return resources, nil
	}
	t.Cleanup(func() { newCLIRunResources = old })

	writer := &rateLimitedOnceWriter{err: rateLimited}
	var stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "miku"}, strings.NewReader(""), writer, &stderr)

	require.NotZero(t, code)
	require.Equal(t, 1, poolCalls, "a writer failure must not trigger account-pool replay")
	assert.Empty(t, writer.String(), "the simulated second successful write must never run")
	assert.Contains(t, stderr.String(), rateLimited.Error())
}

// TestRecommendedAllNDJSONDoesNotReplayAfterWriterFailure 固定 NDJSON 推荐流的
// 提交边界：一条记录写出后 stdout 失败必须被视为已提交，账号池不得重放。
func TestRecommendedAllNDJSONDoesNotReplayAfterWriterFailure(t *testing.T) {
	useTempPaths(t)
	rateLimited := sdk.NewError("pixiv", "recommend", sdk.RateLimited, sdk.WithRetry(sdk.RetryAdvice{Safe: true, HasAfter: true}))
	illust := commandArtwork(42)
	illust.Kind = pixiv.ArtworkKindIllustration
	client := sdkCommandFake{recommended: func(context.Context, pixiv.RecommendedArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{illust}}, nil
	}}
	wireClient := openCLIWireClient(t, &client)
	old := newCLIRunResources
	poolCalls := 0
	newCLIRunResources = func() (*runResources, error) {
		resources := newTestResources(t)
		resources.sdk.open = func(pixivdeps.Request) (*pixiv.Client, error) { return wireClient, nil }
		resources.sdk.pooled = func(ctx context.Context, _ pixivdeps.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			poolCalls++
			committed, err := attempt(ctx, wireClient)
			var typed *sdk.Error
			if err != nil && !committed && errors.As(err, &typed) && typed.Reason == sdk.RateLimited && typed.Retry.HasAfter {
				poolCalls++
				_, err = attempt(ctx, wireClient)
			}
			return err
		}
		return resources, nil
	}
	t.Cleanup(func() { newCLIRunResources = old })

	writer := &rateLimitedOnceWriter{err: rateLimited}
	var stderr bytes.Buffer
	code := Run([]string{"pixiv", "recommended", "all", "--ndjson"}, strings.NewReader(""), writer, &stderr)

	require.NotZero(t, code)
	require.Equal(t, 1, poolCalls, "a writer failure must not trigger account-pool replay")
	assert.Empty(t, writer.String(), "the simulated second successful write must never run")
	assert.Contains(t, stderr.String(), rateLimited.Error())
}
