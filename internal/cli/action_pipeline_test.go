package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/application/config"
	downloadapp "github.com/FlanChanXwO/pixiv-cli/internal/application/download"
	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pixiv"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBookmarkAddConsumesNDJSONAndSkipsFailedRecord(t *testing.T) {
	useTempPaths(t)
	var requests []pixiv.AddBookmarkRequest
	setTestSDKCommandClient(t, sdkCommandFake{addBookmark: func(_ context.Context, request pixiv.AddBookmarkRequest) error {
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

func TestDownloadConsumesNDJSONWithoutWritingAReport(t *testing.T) {
	useTempPaths(t)
	var requests []downloadapp.DownloadRequest
	setTestDownloadCommandServices(t, func(pixivapp.SDKClientRequest) (pixivapp.ClientSet, error) {
		return testClientSet(t, &sdkCommandFake{}), nil
	}, config.RuntimeConfig{}, func(downloadapp.DownloadClient, string, string) (downloadapp.DownloadManager, error) {
		return downloadManagerFake{download: func(_ context.Context, request downloadapp.DownloadRequest) ([]downloadapp.DownloadedArtwork, error) {
			requests = append(requests, request)
			return []downloadapp.DownloadedArtwork{{IllustID: request.IllustIDs[0], Files: []downloadapp.DownloadedFile{{Path: "/downloads/work.jpg"}}}}, nil
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
	assert.Equal(t, downloadapp.DownloadQualityMini, requests[0].Quality)
	assert.Empty(t, stdout.String())
	var diagnostic map[string]any
	require.NoError(t, json.Unmarshal(stderr.Bytes(), &diagnostic))
	assert.Equal(t, "download", diagnostic["operation"])
	assert.Equal(t, "unsupported_type", diagnostic["code"])
}

func TestBookmarkAddFailFastDoesNotReadTheNextActionRecord(t *testing.T) {
	useTempPaths(t)
	called := false
	setTestSDKCommandClient(t, sdkCommandFake{addBookmark: func(context.Context, pixiv.AddBookmarkRequest) error {
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
