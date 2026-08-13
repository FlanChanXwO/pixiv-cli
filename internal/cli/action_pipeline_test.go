package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/config"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBookmarkAddConsumesNDJSONAndSkipsFailedRecord(t *testing.T) {
	useTempPaths(t)
	var requests []pixiv.AddBookmarkRequest
	setTestSDKCommandClient(t, &sdkCommandFake{addBookmark: func(_ context.Context, request pixiv.AddBookmarkRequest) error {
		requests = append(requests, request)
		return nil
	}})
	input := strings.Join([]string{
		`{"id":"81","type":"illust","url":"https://www.pixiv.net/artworks/81"}`,
		`{"id":"82","type":"novel","url":"https://www.pixiv.net/novel/show.php?id=82"}`,
	}, "\n")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "bookmark", "add", "--on-error", "skip"}, strings.NewReader(input), &stdout, &stderr)

	assert.Equal(t, 1, code)
	assert.Equal(t, []pixiv.AddBookmarkRequest{{ArtworkID: 81, Restrict: pixiv.RestrictPublic}}, requests)
	assert.Empty(t, stdout.String())
	var diagnostic map[string]any
	require.NoError(t, json.Unmarshal(stderr.Bytes(), &diagnostic))
	assert.Equal(t, "bookmark_add", diagnostic["operation"])
	assert.Equal(t, "unsupported_type", diagnostic["code"])
	assert.Equal(t, "82", diagnostic["id"])
}

func TestBookmarkAddConsumesOneRawIDFromStdin(t *testing.T) {
	useTempPaths(t)
	var requests []pixiv.AddBookmarkRequest
	setTestSDKCommandClient(t, &sdkCommandFake{addBookmark: func(_ context.Context, request pixiv.AddBookmarkRequest) error {
		requests = append(requests, request)
		return nil
	}})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "bookmark", "add"}, strings.NewReader("81\r\n"), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, []pixiv.AddBookmarkRequest{{ArtworkID: 81, Restrict: pixiv.RestrictPublic}}, requests)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestFollowAddConsumesOneRawIDFromStdin(t *testing.T) {
	useTempPaths(t)
	var request pixiv.FollowUserRequest
	setTestSDKCommandClient(t, &sdkCommandFake{follow: func(_ context.Context, got pixiv.FollowUserRequest) error {
		request = got
		return nil
	}})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "follow", "add"}, strings.NewReader("88\n"), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, pixiv.FollowUserRequest{UserID: 88, Restrict: pixiv.RestrictPublic}, request)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestDownloadConsumesOneRawIDFromStdin(t *testing.T) {
	useTempPaths(t)
	var requests []downloader.DownloadRequest
	setTestDownloadCommandServices(t, &sdkCommandFake{}, nil, config.RuntimeConfig{}, func(downloader.DownloadClient, string, string) (downloader.DownloadManager, error) {
		return downloadManagerFake{download: func(_ context.Context, request downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error) {
			requests = append(requests, request)
			return []downloader.DownloadedArtwork{{IllustID: request.IllustIDs[0], Files: []downloader.DownloadedFile{{Path: "/downloads/work.jpg"}}}}, nil
		}}, nil
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "download"}, strings.NewReader("91\r\n"), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	require.Len(t, requests, 1)
	assert.Equal(t, []int64{91}, requests[0].IllustIDs)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestDownloadConsumesNDJSONWithoutWritingAReport(t *testing.T) {
	useTempPaths(t)
	var requests []downloader.DownloadRequest
	setTestDownloadCommandServices(t, &sdkCommandFake{}, nil, config.RuntimeConfig{}, func(downloader.DownloadClient, string, string) (downloader.DownloadManager, error) {
		return downloadManagerFake{download: func(_ context.Context, request downloader.DownloadRequest) ([]downloader.DownloadedArtwork, error) {
			requests = append(requests, request)
			return []downloader.DownloadedArtwork{{IllustID: request.IllustIDs[0], Files: []downloader.DownloadedFile{{Path: "/downloads/work.jpg"}}}}, nil
		}}, nil
	})
	input := strings.Join([]string{
		`{"id":"91","type":"illust","url":"https://www.pixiv.net/artworks/91"}`,
		`{"id":"92","type":"novel","url":"https://www.pixiv.net/novel/show.php?id=92"}`,
	}, "\n")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "download", "--quality", "mini", "--on-error", "skip"}, strings.NewReader(input), &stdout, &stderr)

	assert.Equal(t, 1, code)
	require.Len(t, requests, 1)
	assert.Equal(t, []int64{91}, requests[0].IllustIDs)
	assert.Equal(t, downloader.DownloadQualityMini, requests[0].Quality)
	assert.Empty(t, stdout.String())
	var diagnostic map[string]any
	require.NoError(t, json.Unmarshal(stderr.Bytes(), &diagnostic))
	assert.Equal(t, "download", diagnostic["operation"])
	assert.Equal(t, "unsupported_type", diagnostic["code"])
}

func TestBookmarkAddFailFastDoesNotReadTheNextActionRecord(t *testing.T) {
	useTempPaths(t)
	called := false
	setTestSDKCommandClient(t, &sdkCommandFake{addBookmark: func(context.Context, pixiv.AddBookmarkRequest) error {
		called = true
		return nil
	}})
	input := strings.Join([]string{
		`{"id":"101","type":"novel","url":"https://www.pixiv.net/novel/show.php?id=101"}`,
		`{"id":"102","type":"illust","url":"https://www.pixiv.net/artworks/102"}`,
	}, "\n")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "bookmark", "add", "--on-error", "fail-fast"}, strings.NewReader(input), &stdout, &stderr)

	assert.Equal(t, 1, code)
	assert.False(t, called)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), `"line":1`)
	assert.NotContains(t, stderr.String(), `"line":2`)
}
