package pixiv_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// 内容读取类 tool 的 owner 契约：系列、小说详情/正文、评论的请求映射与输出形状。

func TestIllustSeriesMapsRequestAndReturnsRecords(t *testing.T) {
	client := &fakeSDKClient{
		artworkSeriesPage: sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(11, "series-artwork", 1)}},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "illust_series", map[string]any{"series_id": 10})
	if result.IsError || client.artworkSeriesRequest.SeriesID != 10 {
		t.Fatalf("illust series result=%+v request=%+v", result, client.artworkSeriesRequest)
	}
	var out outputs.Records
	decodeStructured(t, result, &out)
	if len(out.Records) != 1 || out.Records[0].ID() != "11" {
		t.Fatalf("illust series records=%+v", out.Records)
	}
}

func TestNovelDetailMapsRequestAndReturnsRecords(t *testing.T) {
	client := &fakeSDKClient{
		novelDetailResult: pixiv.Novel{ID: 12, Title: "novel-detail", User: pixiv.User{ID: 2}},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "novel_detail", map[string]any{"novel_id": 12})
	if result.IsError || client.novelRequest.NovelID != 12 {
		t.Fatalf("novel detail result=%+v request=%+v", result, client.novelRequest)
	}
	var out outputs.NovelDetail
	decodeStructured(t, result, &out)
	if len(out.Records) != 1 || out.Records[0].ID() != "12" {
		t.Fatalf("novel detail records=%+v", out.Records)
	}
}

func TestNovelContentReturnsBlocksWithOpaqueResourceRefs(t *testing.T) {
	client := &fakeSDKClient{
		novelContentHTML: `<!DOCTYPE html><html><body>` +
			`<h1 class="title">novel-content</h1>` +
			`<p class="noveltext">complete body</p>` +
			`<figure class="novel_image"><img src="https://i.pximg.net/novel/12/image"></figure>` +
			`</body></html>`,
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "novel_content", map[string]any{"novel_id": 12})
	if result.IsError || client.novelContentRequest.NovelID != 12 {
		t.Fatalf("novel content result=%+v request=%+v", result, client.novelContentRequest)
	}
	var out outputs.NovelContent
	decodeStructured(t, result, &out)
	derivedRef, err := sdk.NewResourceRef("pixiv", []byte(`{"k":"novel_image","id":12}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Content.Blocks) != 2 || out.Content.Blocks[0].Text != "complete body" || out.Content.Blocks[1].Image == nil || out.Content.Blocks[1].Image.Resource == nil || out.Content.Blocks[1].Image.Resource.Ref != derivedRef.String() {
		t.Fatalf("novel content=%+v", out)
	}
	rawContent, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rawContent), "https://private.example") || strings.Contains(strings.ToLower(string(rawContent)), "cookie") || strings.Contains(strings.ToLower(string(rawContent)), "header") {
		t.Fatalf("novel content leaked resource transport data: %s", rawContent)
	}
}

func TestNovelSeriesMapsRequestAndReturnsRecords(t *testing.T) {
	client := &fakeSDKClient{
		novelSeriesResult: pixiv.NovelSeriesResult{
			Series: pixiv.NovelSeries{ID: 13, Title: "novel-series", User: pixiv.User{ID: 3}},
			Novels: sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 14, Title: "series-novel", User: pixiv.User{ID: 3}}}},
		},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "novel_series", map[string]any{"series_id": 13})
	if result.IsError || client.novelSeriesRequest.SeriesID != 13 {
		t.Fatalf("novel series result=%+v request=%+v", result, client.novelSeriesRequest)
	}
	var out outputs.NovelSeries
	decodeStructured(t, result, &out)
	if out.Series.ID != 13 || len(out.Records) != 1 || out.Records[0].ID() != "14" {
		t.Fatalf("novel series=%+v", out)
	}
}

func TestIllustCommentsMapsRequestAndPreservesEnvelope(t *testing.T) {
	total := int64(3)
	client := &fakeSDKClient{
		artworkCommentsResult: pixiv.CommentPage{
			Page:          sdk.Page[pixiv.Comment]{Items: []pixiv.Comment{{ID: 21, Comment: "artwork comment", User: pixiv.User{ID: 4}}}},
			Total:         &total,
			AccessControl: &pixiv.CommentAccessControl{CanComment: true},
		},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "illust_comments", map[string]any{"id": 20})
	if result.IsError || client.artworkCommentsRequest.ArtworkID != 20 {
		t.Fatalf("artwork comments result=%+v request=%+v", result, client.artworkCommentsRequest)
	}
	var out outputs.Comments
	decodeStructured(t, result, &out)
	if len(out.Comments) != 1 || out.Comments[0].ID != 21 || out.Total == nil || *out.Total != total || out.AccessControl == nil || !out.AccessControl.CanComment {
		t.Fatalf("artwork comments=%+v", out)
	}
}

func TestNovelCommentsMapsRequestAndReturnsRecords(t *testing.T) {
	client := &fakeSDKClient{
		novelCommentsResult: pixiv.CommentPage{
			Page: sdk.Page[pixiv.Comment]{Items: []pixiv.Comment{{ID: 22, Comment: "novel comment", User: pixiv.User{ID: 5}}}},
		},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "novel_comments", map[string]any{"id": 22})
	if result.IsError || client.novelCommentsRequest.NovelID != 22 {
		t.Fatalf("novel comments result=%+v request=%+v", result, client.novelCommentsRequest)
	}
}
