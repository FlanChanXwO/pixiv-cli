package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActionFirstSearchRoutesNovelWithEntityTypeAndShortFlags(t *testing.T) {
	useTempPaths(t)
	var got pixiv.SearchNovelsRequest
	setTestSDKCommandClient(t, &sdkCommandFake{searchNovel: func(_ context.Context, request pixiv.SearchNovelsRequest) (sdk.Page[pixiv.Novel], error) {
		got = request
		return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 12, Title: "novel", User: pixiv.User{ID: 42}}}}, nil
	}})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "miku", "-t", "novel", "-p", "1", "-l", "1", "-j"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, pixiv.SearchNovelsRequest{Word: "miku", Target: pixiv.SearchTargetPartialMatchForTags, Sort: pixiv.SortModeDateDesc}, got)
	assert.Contains(t, stdout.String(), `"id": 12`)
}

func TestActionFirstSearchShortFlagsMatchLongForms(t *testing.T) {
	var requests [2]pixiv.SearchNovelsRequest
	var outputs [2]string
	for index, flags := range [2][]string{
		{"--type", "novel", "--page", "1", "--limit", "1", "--json"},
		{"-t", "novel", "-p", "1", "-l", "1", "-j"},
	} {
		t.Run(fmt.Sprintf("form-%d", index), func(t *testing.T) {
			useTempPaths(t)
			setTestSDKCommandClient(t, &sdkCommandFake{searchNovel: func(_ context.Context, request pixiv.SearchNovelsRequest) (sdk.Page[pixiv.Novel], error) {
				requests[index] = request
				return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 12, Title: "novel", User: pixiv.User{ID: 42}}}}, nil
			}})

			var stdout, stderr bytes.Buffer
			code := Run(append([]string{"pixiv", "search", "miku"}, flags...), strings.NewReader(""), &stdout, &stderr)

			require.Equal(t, 0, code, stderr.String())
			outputs[index] = stdout.String()
		})
	}

	assert.Equal(t, requests[0], requests[1])
	assert.Equal(t, outputs[0], outputs[1])
}

func TestActionFirstSearchRoutesUserWithEntityTypeAndShortJSON(t *testing.T) {
	useTempPaths(t)
	var got pixiv.SearchUsersRequest
	setTestSDKCommandClient(t, &sdkCommandFake{searchUser: func(_ context.Context, request pixiv.SearchUsersRequest) (sdk.Page[pixiv.UserPreview], error) {
		got = request
		return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 13, Name: "artist"}}}}, nil
	}})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "artist", "--type", "user", "-j"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, pixiv.SearchUsersRequest{Word: "artist"}, got)
	var result struct {
		Users []pixiv.UserPreview `json:"user_previews"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	assert.Len(t, result.Users, 1)
}

func TestActionFirstArtworkSearchUsesContentType(t *testing.T) {
	useTempPaths(t)
	var got pixiv.SearchArtworksRequest
	setTestSDKCommandClient(t, &sdkCommandFake{search: func(_ context.Context, request pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		got = request
		return sdk.Page[pixiv.Artwork]{}, nil
	}})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "miku", "-t", "artwork", "--content-type", "manga", "-j"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, pixiv.SearchContentTypeManga, got.ContentType)
}

func TestActionFirstRootHelpListsCanonicalActions(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "--help"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	for _, action := range []string{"search", "detail", "ranking", "series", "comment", "bookmark", "download", "user", "timeline", "mypixiv", "recommended"} {
		assert.Contains(t, stdout.String(), action)
	}
}

func TestDownloadHelpExposesOutputShortName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "download", "--help"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Contains(t, stdout.String(), "-o, --output")
	assert.Contains(t, stdout.String(), "--download-path")
}

func TestActionFirstDetailRoutesNovelThroughTypedRequest(t *testing.T) {
	useTempPaths(t)
	var got pixiv.NovelRequest
	setTestSDKCommandClient(t, &sdkCommandFake{novelDetail: func(_ context.Context, request pixiv.NovelRequest) (pixiv.Novel, error) {
		got = request
		return pixiv.Novel{ID: 21, Title: "novel"}, nil
	}})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "detail", "21", "-t", "novel", "-j"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, pixiv.NovelRequest{NovelID: 21}, got)
	assert.Contains(t, stdout.String(), `"id": 21`)
}

func TestActionFirstSeriesAndCommentRouteTypedReads(t *testing.T) {
	useTempPaths(t)
	var seriesRequest pixiv.ArtworkSeriesRequest
	var commentRequest pixiv.ArtworkCommentsRequest
	setTestSDKCommandClient(t, &sdkCommandFake{
		artworkSeries: func(_ context.Context, request pixiv.ArtworkSeriesRequest) (sdk.Page[pixiv.Artwork], error) {
			seriesRequest = request
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{commandArtwork(31)}}, nil
		},
		artworkComments: func(_ context.Context, request pixiv.ArtworkCommentsRequest) (pixiv.CommentPage, error) {
			commentRequest = request
			return pixiv.CommentPage{Page: sdk.Page[pixiv.Comment]{Items: []pixiv.Comment{{ID: 32, Comment: "hello"}}}}, nil
		},
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "series", "31", "-t", "artwork", "-j"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, pixiv.ArtworkSeriesRequest{SeriesID: 31}, seriesRequest)
	assert.Contains(t, stdout.String(), `"id": 31`)

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "comment", "31", "-t", "artwork", "-j"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, pixiv.ArtworkCommentsRequest{ArtworkID: 31}, commentRequest)
	assert.Contains(t, stdout.String(), `"id": 32`)
}

func TestActionFirstBookmarkReadsAndUserRelations(t *testing.T) {
	useTempPaths(t)
	var tagRequest pixiv.UserArtworkBookmarkTagsRequest
	var detailRequest pixiv.ArtworkBookmarkRequest
	var followerRequest pixiv.UserFollowersRequest
	setTestSDKCommandClient(t, &sdkCommandFake{
		bookmarkTags: func(_ context.Context, request pixiv.UserArtworkBookmarkTagsRequest) (sdk.Page[pixiv.BookmarkTag], error) {
			tagRequest = request
			return sdk.Page[pixiv.BookmarkTag]{Items: []pixiv.BookmarkTag{{Name: "favorite", Count: 2}}}, nil
		},
		bookmarkDetail: func(_ context.Context, request pixiv.ArtworkBookmarkRequest) (pixiv.ArtworkBookmarkDetail, error) {
			detailRequest = request
			return pixiv.ArtworkBookmarkDetail{Restrict: pixiv.RestrictPrivate, Tags: []string{"favorite"}}, nil
		},
		userFollowers: func(_ context.Context, request pixiv.UserFollowersRequest) (sdk.Page[pixiv.UserPreview], error) {
			followerRequest = request
			return sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 33}}}}, nil
		},
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "bookmark", "tags", "44", "-j"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, pixiv.UserArtworkBookmarkTagsRequest{UserID: 44, Restrict: pixiv.RestrictPublic}, tagRequest)
	assert.Contains(t, stdout.String(), "favorite")

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "bookmark", "detail", "45", "-j"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, pixiv.ArtworkBookmarkRequest{ArtworkID: 45}, detailRequest)
	assert.Contains(t, stdout.String(), "private")

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "user", "followers", "46", "-j"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, pixiv.UserFollowersRequest{UserID: 46, Restrict: pixiv.RestrictPublic}, followerRequest)
	assert.Contains(t, stdout.String(), `"id": 33`)
}

func TestActionFirstSearchBookmarkRangeUsesApplicationFilterOutcome(t *testing.T) {
	useTempPaths(t)
	var requests []pixiv.SearchArtworksRequest
	setTestSDKCommandClient(t, &sdkCommandFake{search: func(_ context.Context, request pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		requests = append(requests, request)
		if request.Cursor.IsZero() {
			low := commandArtwork(51)
			low.TotalBookmarks = 2
			high := commandArtwork(52)
			high.TotalBookmarks = 20
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{low, high}}, nil
		}
		return sdk.Page[pixiv.Artwork]{}, nil
	}})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "miku", "--bookmark-min", "10", "-l", "1", "-j"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Len(t, requests, 1)
	assert.Nil(t, requests[0].BookmarkMin, "auto/local must enumerate the normal candidate source")
	assert.Contains(t, stdout.String(), `"id": 52`)
	assert.NotContains(t, stdout.String(), `"id": 51`)
	assert.Contains(t, stdout.String(), `"strategy": "local"`)
}

func TestActionFirstSearchTrendingTagsHasNoImplicitPagination(t *testing.T) {
	useTempPaths(t)
	called := false
	setTestSDKCommandClient(t, &sdkCommandFake{trendingTags: func(_ context.Context, request pixiv.TrendingArtworkTagsRequest) ([]pixiv.TrendingTag, error) {
		called = true
		assert.Equal(t, pixiv.TrendingArtworkTagsRequest{}, request)
		return []pixiv.TrendingTag{{Tag: "miku", TranslatedName: "Hatsune Miku"}}, nil
	}})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "--trending-tags", "-j"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.True(t, called)
	assert.Contains(t, stdout.String(), "miku")
	assert.NotContains(t, stdout.String(), "limit")
}

func TestActionFirstRecommendedUsesShortEntityType(t *testing.T) {
	useTempPaths(t)
	called := false
	setTestSDKCommandClient(t, &sdkCommandFake{novelRecommended: func(_ context.Context, request pixiv.RecommendedNovelsRequest) (sdk.Page[pixiv.Novel], error) {
		called = true
		assert.Equal(t, pixiv.RecommendedNovelsRequest{}, request)
		return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 61, Title: "recommended", User: pixiv.User{ID: 42}}}}, nil
	}})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "recommended", "-t", "novel", "-j"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.True(t, called)
	assert.Contains(t, stdout.String(), `"id": 61`)
}
