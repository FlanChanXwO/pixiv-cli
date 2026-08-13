package pixiv_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	pixivmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestTask11RegistersTypedReadTools(t *testing.T) {
	session, closeSession := newTestSession(t, &fakeDownloads{})
	defer closeSession()

	want := []string{
		"illust_series", "illust_comments", "novel_detail", "novel_content", "novel_series", "novel_comments",
		"bookmark_tags", "bookmark_detail", "user_novel_bookmarks", "user_followers", "related_users", "blocked_users",
	}
	got := make([]string, 0, len(want))
	for tool, err := range session.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("list tools: %v", err)
		}
		if slices.Contains(want, tool.Name) {
			got = append(got, tool.Name)
		}
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("typed read tools=%v, want=%v", got, want)
	}
}

func TestTask11UserDetailUsesApplicationPooledService(t *testing.T) {
	var pooled bool
	client := openWireClient(t, &fakeSDKClient{userID: 42})
	ports := pixivmcpserver.SDKPorts{
		Open: func(pixivmcpserver.Account) (*pixiv.Client, error) {
			return client, nil
		},
		Pooled: func(context.Context, pixivmcpserver.Account, func(context.Context, *pixiv.Client) (bool, error)) error {
			pooled = true
			return errors.New("application pool failure")
		},
	}
	session, closeSession := newSDKTestSessionWithPorts(t, &fakeAPI{}, ports, pixivmcpserver.Account{})
	defer closeSession()

	result := callTool(t, session, "user_detail", map[string]any{"user_id": 42})
	if !pooled || !result.IsError || !resultHasText(result, "application pool failure") {
		t.Fatalf("pooled=%v result=%+v", pooled, result)
	}
}

func TestTask12SearchBookmarkRangeUsesSearchWorkflow(t *testing.T) {
	var pooled bool
	fake := &fakeSDKClient{searchIllust: func(_ context.Context, _ pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		low := testSDKIllust(51, "low", 1)
		low.TotalBookmarks = 2
		high := testSDKIllust(52, "high", 1)
		high.TotalBookmarks = 20
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{low, high}}, nil
	}}
	sdkClient := openWireClient(t, fake)
	ports := pixivmcpserver.SDKPorts{
		Open: func(pixivmcpserver.Account) (*pixiv.Client, error) {
			return sdkClient, nil
		},
		Pooled: func(ctx context.Context, account pixivmcpserver.Account, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			pooled = true
			_, err := attempt(ctx, sdkClient)
			return err
		},
	}
	session, closeSession := newSDKTestSessionWithPorts(t, &fakeAPI{}, ports, pixivmcpserver.Account{})
	defer closeSession()

	result := callTool(t, session, "search_illust", map[string]any{"word": "miku", "bookmark_min": 10, "limit": 1})
	if !pooled || result.IsError {
		t.Fatalf("pooled=%v result=%+v", pooled, result)
	}
	var out map[string]any
	decodeStructured(t, result, &out)
	if _, ok := out["filter"]; !ok {
		t.Fatalf("search output lacks bookmark filter metadata: %#v", out)
	}
	if resultHasText(result, "51") || !resultHasText(result, "Retrieved 1 records.") {
		t.Fatalf("search result=%+v", result)
	}
}

func TestTask11TypedToolsMapRequestsAndPreserveEnvelopes(t *testing.T) {
	total := int64(3)
	client := &fakeSDKClient{
		userID:            900,
		artworkSeriesPage: sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(11, "series-artwork", 1)}},
		novelDetailResult: pixiv.Novel{ID: 12, Title: "novel-detail", User: pixiv.User{ID: 2}},
		novelContentHTML: `<!DOCTYPE html><html><body>` +
			`<h1 class="title">novel-content</h1>` +
			`<p class="noveltext">complete body</p>` +
			`<figure class="novel_image"><img src="https://i.pximg.net/novel/12/image"></figure>` +
			`</body></html>`,
		novelSeriesResult: pixiv.NovelSeriesResult{
			Series: pixiv.NovelSeries{ID: 13, Title: "novel-series", User: pixiv.User{ID: 3}},
			Novels: sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 14, Title: "series-novel", User: pixiv.User{ID: 3}}}},
		},
		artworkCommentsResult: pixiv.CommentPage{
			Page:          sdk.Page[pixiv.Comment]{Items: []pixiv.Comment{{ID: 21, Comment: "artwork comment", User: pixiv.User{ID: 4}}}},
			Total:         &total,
			AccessControl: &pixiv.CommentAccessControl{CanComment: true},
		},
		novelCommentsResult: pixiv.CommentPage{
			Page: sdk.Page[pixiv.Comment]{Items: []pixiv.Comment{{ID: 22, Comment: "novel comment", User: pixiv.User{ID: 5}}}},
		},
		bookmarkTagsPage:     sdk.Page[pixiv.BookmarkTag]{Items: []pixiv.BookmarkTag{{Name: "blue", Count: 4}}},
		bookmarkDetailResult: pixiv.ArtworkBookmarkDetail{Restrict: pixiv.RestrictPrivate, Tags: []string{"blue"}},
		novelBookmarksPage:   sdk.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 31, Title: "bookmark-novel", User: pixiv.User{ID: 6}}}},
		followersPage:        sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 41, Name: "follower"}}}},
		relatedPage:          sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 42, Name: "related"}}}},
		blockedPage:          sdk.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{{User: pixiv.User{ID: 43, Name: "blocked"}}}},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	seriesResult := callTool(t, session, "illust_series", map[string]any{"series_id": 10})
	if seriesResult.IsError || client.artworkSeriesRequest.SeriesID != 10 {
		t.Fatalf("illust series result=%+v request=%+v", seriesResult, client.artworkSeriesRequest)
	}
	var artworkSeriesOut outputs.Records
	decodeStructured(t, seriesResult, &artworkSeriesOut)
	if len(artworkSeriesOut.Records) != 1 || artworkSeriesOut.Records[0].ID() != "11" {
		t.Fatalf("illust series records=%+v", artworkSeriesOut.Records)
	}

	novelDetail := callTool(t, session, "novel_detail", map[string]any{"novel_id": 12})
	if novelDetail.IsError || client.novelRequest.NovelID != 12 {
		t.Fatalf("novel detail result=%+v request=%+v", novelDetail, client.novelRequest)
	}
	var novelOut outputs.NovelDetail
	decodeStructured(t, novelDetail, &novelOut)
	if len(novelOut.Records) != 1 || novelOut.Records[0].ID() != "12" {
		t.Fatalf("novel detail records=%+v", novelOut.Records)
	}

	novelContent := callTool(t, session, "novel_content", map[string]any{"novel_id": 12})
	if novelContent.IsError || client.novelContentRequest.NovelID != 12 {
		t.Fatalf("novel content result=%+v request=%+v", novelContent, client.novelContentRequest)
	}
	var contentOut outputs.NovelContent
	decodeStructured(t, novelContent, &contentOut)
	derivedRef, err := sdk.NewResourceRef("pixiv", []byte(`{"k":"novel_image","id":12}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(contentOut.Content.Blocks) != 2 || contentOut.Content.Blocks[0].Text != "complete body" || contentOut.Content.Blocks[1].Image == nil || contentOut.Content.Blocks[1].Image.Resource == nil || contentOut.Content.Blocks[1].Image.Resource.Ref != derivedRef.String() {
		t.Fatalf("novel content=%+v", contentOut)
	}
	rawContent, err := json.Marshal(contentOut)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rawContent), "https://private.example") || strings.Contains(strings.ToLower(string(rawContent)), "cookie") || strings.Contains(strings.ToLower(string(rawContent)), "header") {
		t.Fatalf("novel content leaked resource transport data: %s", rawContent)
	}

	artworkComments := callTool(t, session, "illust_comments", map[string]any{"id": 20})
	if artworkComments.IsError || client.artworkCommentsRequest.ArtworkID != 20 {
		t.Fatalf("artwork comments result=%+v request=%+v", artworkComments, client.artworkCommentsRequest)
	}
	var commentsOut outputs.Comments
	decodeStructured(t, artworkComments, &commentsOut)
	if len(commentsOut.Comments) != 1 || commentsOut.Comments[0].ID != 21 || commentsOut.Total == nil || *commentsOut.Total != total || commentsOut.AccessControl == nil || !commentsOut.AccessControl.CanComment {
		t.Fatalf("artwork comments=%+v", commentsOut)
	}

	novelComments := callTool(t, session, "novel_comments", map[string]any{"id": 22})
	if novelComments.IsError || client.novelCommentsRequest.NovelID != 22 {
		t.Fatalf("novel comments result=%+v request=%+v", novelComments, client.novelCommentsRequest)
	}

	novelSeries := callTool(t, session, "novel_series", map[string]any{"series_id": 13})
	if novelSeries.IsError || client.novelSeriesRequest.SeriesID != 13 {
		t.Fatalf("novel series result=%+v request=%+v", novelSeries, client.novelSeriesRequest)
	}
	var novelSeriesOut outputs.NovelSeries
	decodeStructured(t, novelSeries, &novelSeriesOut)
	if novelSeriesOut.Series.ID != 13 || len(novelSeriesOut.Records) != 1 || novelSeriesOut.Records[0].ID() != "14" {
		t.Fatalf("novel series=%+v", novelSeriesOut)
	}

	bookmarkTags := callTool(t, session, "bookmark_tags", map[string]any{"user_id": 90, "restrict": "private"})
	if bookmarkTags.IsError || client.bookmarkTagsRequest.UserID != 90 || client.bookmarkTagsRequest.Restrict != pixiv.RestrictPrivate {
		t.Fatalf("bookmark tags result=%+v request=%+v", bookmarkTags, client.bookmarkTagsRequest)
	}
	var tagsOut outputs.BookmarkTags
	decodeStructured(t, bookmarkTags, &tagsOut)
	if len(tagsOut.Tags) != 1 || tagsOut.Tags[0].Name != "blue" {
		t.Fatalf("bookmark tags=%+v", tagsOut)
	}

	bookmarkDetail := callTool(t, session, "bookmark_detail", map[string]any{"illust_id": 11})
	if bookmarkDetail.IsError || client.bookmarkDetailRequest.ArtworkID != 11 {
		t.Fatalf("bookmark detail result=%+v request=%+v", bookmarkDetail, client.bookmarkDetailRequest)
	}
	var detailOut outputs.BookmarkDetail
	decodeStructured(t, bookmarkDetail, &detailOut)
	if !detailOut.Bookmarked || detailOut.Restrict != string(pixiv.RestrictPrivate) || !slices.Equal(detailOut.Tags, []string{"blue"}) {
		t.Fatalf("bookmark detail=%+v", detailOut)
	}

	novelBookmarks := callTool(t, session, "user_novel_bookmarks", map[string]any{"user_id": 91, "restrict": "public", "tag": "blue"})
	if novelBookmarks.IsError || client.novelBookmarksRequest.UserID != 91 || client.novelBookmarksRequest.Tag != "blue" {
		t.Fatalf("novel bookmarks result=%+v request=%+v", novelBookmarks, client.novelBookmarksRequest)
	}
	var novelBookmarksOut outputs.Records
	decodeStructured(t, novelBookmarks, &novelBookmarksOut)
	if len(novelBookmarksOut.Records) != 1 || novelBookmarksOut.Records[0].ID() != "31" {
		t.Fatalf("novel bookmarks=%+v", novelBookmarksOut)
	}

	followers := callTool(t, session, "user_followers", map[string]any{"user_id": 92, "restrict": "private"})
	if followers.IsError || client.followersRequest.UserID != 92 || client.followersRequest.Restrict != pixiv.RestrictPrivate {
		t.Fatalf("followers result=%+v request=%+v", followers, client.followersRequest)
	}
	related := callTool(t, session, "related_users", map[string]any{"user_id": 93})
	if related.IsError || client.relatedRequest.UserID != 93 {
		t.Fatalf("related result=%+v request=%+v", related, client.relatedRequest)
	}
	blocked := callTool(t, session, "blocked_users", map[string]any{"user_id": 94})
	if blocked.IsError || client.blockedRequest.UserID != 94 {
		t.Fatalf("blocked result=%+v request=%+v", blocked, client.blockedRequest)
	}
}
