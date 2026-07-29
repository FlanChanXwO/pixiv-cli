package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchNDJSONWritesRecordsBeforeFetchingNextPage(t *testing.T) {
	useTempPaths(t)
	var stdout, stderr bytes.Buffer
	setTestSDKCommandClient(t, sdkCommandFake{search: func(_ context.Context, request sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
		switch request.Cursor {
		case "":
			item := commandIllust(11)
			item.Type = string(sdk.IllustTypeIllust)
			return &sdk.IllustListResult{Illusts: []sdk.Illust{item}, NextCursor: "next"}, nil
		case "next":
			require.Contains(t, stdout.String(), `"id":"11"`)
			item := commandIllust(12)
			item.Type = string(sdk.IllustTypeIllust)
			return &sdk.IllustListResult{Illusts: []sdk.Illust{item}}, nil
		default:
			return nil, assert.AnError
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
		assert.Equal(t, "illust", record["type"])
		assert.Equal(t, "https://www.pixiv.net/artworks/"+[]string{"11", "12"}[index], record["url"])
	}
	assert.Empty(t, stderr.String())
}

func TestNovelSearchNDJSONWritesNovelRecord(t *testing.T) {
	useTempPaths(t)
	novel := sdk.Novel{ID: 31, URL: "https://www.pixiv.net/novel/show.php?id=31", Title: "novel", User: sdk.User{ID: 8, Name: "writer"}}
	setTestSDKCommandClient(t, sdkCommandFake{searchNovel: func(_ context.Context, _ sdk.SearchNovelRequest) (*sdk.NovelListResult, error) {
		return &sdk.NovelListResult{Novels: []sdk.Novel{novel}}, nil
	}})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "novel", "search", "miku", "--ndjson"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	var record map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &record))
	assert.Equal(t, "31", record["id"])
	assert.Equal(t, "novel", record["type"])
	assert.Equal(t, novel.URL, record["url"])
	assert.Equal(t, novel.Title, record["title"])
}
