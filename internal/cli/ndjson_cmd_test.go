package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVisualListAutoNDJSONWritesToNonTerminal(t *testing.T) {
	useTempPaths(t)
	setTestSDKCommandClient(t, &sdkCommandFake{search: func(_ context.Context, _ pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		item := commandArtwork(71)
		item.Kind = pixiv.ArtworkKindIllustration
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{item}}, nil
	}})
	output, err := os.CreateTemp(t.TempDir(), "records-*.ndjson")
	require.NoError(t, err)
	defer output.Close()
	var stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "miku"}, strings.NewReader(""), output, &stderr)
	require.Equal(t, 0, code, stderr.String())
	require.NoError(t, output.Close())
	body, err := os.ReadFile(output.Name())
	require.NoError(t, err)
	var record map[string]any
	require.NoError(t, json.Unmarshal(body, &record))
	assert.Equal(t, "71", record["id"])
	assert.Equal(t, "illustration", record["type"])
}

func TestSearchNDJSONWritesRecordsBeforeFetchingNextPage(t *testing.T) {
	useTempPaths(t)
	nextCursor := testCursor(t, "next")
	var stdout, stderr bytes.Buffer
	setTestSDKCommandClient(t, &sdkCommandFake{search: func(_ context.Context, request pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		switch {
		case request.Cursor.IsZero():
			item := commandArtwork(11)
			item.Kind = pixiv.ArtworkKindIllustration
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{item}, Next: nextCursor}, nil
		case request.Cursor.String() == nextCursor.String():
			require.Contains(t, stdout.String(), `"id":"11"`)
			item := commandArtwork(12)
			item.Kind = pixiv.ArtworkKindIllustration
			return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{item}}, nil
		default:
			return sdk.Page[pixiv.Artwork]{}, assert.AnError
		}
	}})

	code := Run([]string{"pixiv", "search", "miku", "--ndjson", "--limit", "0"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	require.Len(t, lines, 2)
	for index, line := range lines {
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		assert.Equal(t, []string{"11", "12"}[index], record["id"])
		assert.Equal(t, "illustration", record["type"])
		assert.Equal(t, "https://www.pixiv.net/artworks/"+[]string{"11", "12"}[index], record["url"])
	}
	assert.Empty(t, stderr.String())
}

func TestNovelSearchNDJSONWritesNovelRecord(t *testing.T) {
	useTempPaths(t)
	novel := pixiv.Novel{ID: 31, Title: "novel", User: pixiv.User{ID: 8, Name: "writer"}}
	setTestSDKCommandClient(t, &sdkCommandFake{searchNovel: func(_ context.Context, _ pixiv.SearchNovelsRequest) (sdk.Page[pixiv.Novel], error) {
		return sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{novel}}, nil
	}})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "novel", "search", "miku", "--ndjson"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	var record map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &record))
	assert.Equal(t, "31", record["id"])
	assert.Equal(t, "novel", record["type"])
	assert.Equal(t, "https://www.pixiv.net/novel/show.php?id=31", record["url"])
	assert.Equal(t, novel.Title, record["title"])
}
