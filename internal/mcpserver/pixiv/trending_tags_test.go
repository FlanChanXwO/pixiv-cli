package pixiv_test

import (
	"context"
	"errors"
	"testing"

	pixivmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// trending_tags_illust 的 owner 契约：无输入、成功输出形状与 SDK 错误分类。

func TestTrendingTagsIllustReturnsTagsAndText(t *testing.T) {
	client := &fakeSDKClient{trendingTags: []pixiv.TrendingTag{
		{Tag: "miku", TranslatedName: "Hatsune Miku", Artwork: testSDKIllust(101, "miku-art", 1)},
		{Tag: "frieren", TranslatedName: "", Artwork: testSDKIllust(102, "frieren-art", 2)},
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "trending_tags_illust", map[string]any{})
	if result.IsError {
		t.Fatalf("trending_tags_illust returned error: %+v", result)
	}
	var out outputs.TrendingTags
	decodeStructured(t, result, &out)
	if len(out.Tags) != 2 || out.Tags[0].Tag != "miku" || out.Tags[0].TranslatedName != "Hatsune Miku" {
		t.Fatalf("trending tags=%+v", out.Tags)
	}
	if !resultHasText(result, "Trending tags:") || !resultHasText(result, "- miku (translation: Hatsune Miku)") || !resultHasText(result, "- frieren (translation: none)") {
		t.Fatalf("trending text=%+v", result.Content)
	}
}

func TestTrendingTagsIllustEmptyResultHasDistinctText(t *testing.T) {
	session, closeSession := newSDKTestSession(t, &fakeSDKClient{})
	defer closeSession()

	result := callTool(t, session, "trending_tags_illust", map[string]any{})
	if result.IsError {
		t.Fatalf("empty trending result must not be an error: %+v", result)
	}
	var out outputs.TrendingTags
	decodeStructured(t, result, &out)
	if len(out.Tags) != 0 || !resultHasText(result, "Could not retrieve trending tags.") {
		t.Fatalf("empty trending output=%+v", out)
	}
}

func TestTrendingTagsIllustSDKErrorIsStructured(t *testing.T) {
	client := openWireClient(t, &fakeSDKClient{trendingTags: []pixiv.TrendingTag{{Tag: "never", TranslatedName: ""}}})
	ports := pixivmcpserver.SDKPorts{
		Open: func(pixivmcpserver.Account) (*pixiv.Client, error) {
			return client, nil
		},
		Pooled: func(context.Context, pixivmcpserver.Account, func(context.Context, *pixiv.Client) (bool, error)) error {
			return errors.New("trending pool failure")
		},
	}
	session, closeSession := newSDKTestSessionWithPorts(t, &fakeAPI{}, ports, pixivmcpserver.Account{})
	defer closeSession()

	result := callTool(t, session, "trending_tags_illust", map[string]any{})
	if !result.IsError || !resultHasText(result, "Error: trending pool failure") {
		t.Fatalf("trending SDK failure result=%+v", result)
	}
	var out outputs.TrendingTags
	decodeStructured(t, result, &out)
	if len(out.Tags) != 0 {
		t.Fatalf("structured error must carry empty tags: %+v", out)
	}
}
