package pixiv_test

import (
	"context"
	"errors"
	"testing"

	pixivmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

type bookmarkAggregateOutput struct {
	Records []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	} `json:"records"`
	Pagination struct {
		Page     int  `json:"page"`
		Returned int  `json:"returned"`
		HasMore  bool `json:"has_more"`
		NextPage *int `json:"next_page"`
	} `json:"pagination"`
}

type bookmarkTagsAggregateOutput struct {
	Tags []struct {
		Name        string `json:"name"`
		Count       int    `json:"count"`
		ContentType string `json:"content_type"`
	} `json:"bookmark_tags"`
	Pagination struct {
		Returned int `json:"returned"`
	} `json:"pagination"`
}

func TestBookmarkListAllUsesArtworkThenNovelWithOneLogicalPageBudget(t *testing.T) {
	client := &fakeSDKClient{
		bookmarks:      []pixiv.Artwork{testSDKIllust(11, "saved artwork", 90)},
		novelBookmarks: []pixiv.Novel{{ID: 22, Title: "saved novel", User: pixiv.User{ID: 90, Name: "writer"}}},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	first := callTool(t, session, "bookmark_list_all", map[string]any{"user_id": 90, "limit": 1})
	if first.IsError {
		t.Fatalf("bookmark_list_all first page returned MCP error: %+v", first)
	}
	var firstOut bookmarkAggregateOutput
	decodeStructured(t, first, &firstOut)
	if len(firstOut.Records) != 1 || firstOut.Records[0].ID != "11" || firstOut.Records[0].Type != "illustration" || !firstOut.Pagination.HasMore || firstOut.Pagination.NextPage == nil || *firstOut.Pagination.NextPage != 2 {
		t.Fatalf("first aggregate page=%+v, want artwork page with novel continuation", firstOut)
	}

	second := callTool(t, session, "bookmark_list_all", map[string]any{"user_id": 90, "page": 2, "limit": 1})
	if second.IsError {
		t.Fatalf("bookmark_list_all second page returned MCP error: %+v", second)
	}
	var secondOut bookmarkAggregateOutput
	decodeStructured(t, second, &secondOut)
	if len(secondOut.Records) != 1 || secondOut.Records[0].ID != "22" || secondOut.Records[0].Type != "novel" || secondOut.Pagination.Returned != 1 || secondOut.Pagination.HasMore {
		t.Fatalf("second aggregate page=%+v, want one novel without duplicate artwork", secondOut)
	}

	if len(client.bookmarksRequests) == 0 || len(client.novelBookmarksRequests) == 0 {
		t.Fatalf("aggregate requests missing artwork=%v novel=%v", client.bookmarksRequests, client.novelBookmarksRequests)
	}
}

func TestBookmarkListAllReplaysWithoutLeakingPartialRecords(t *testing.T) {
	streamErr := errors.New("novel bookmark stream failed")
	client := &fakeSDKClient{
		bookmarks:         []pixiv.Artwork{testSDKIllust(31, "saved artwork", 90)},
		novelBookmarks:    []pixiv.Novel{{ID: 32, Title: "saved novel", User: pixiv.User{ID: 90, Name: "writer"}}},
		novelBookmarksErr: streamErr,
	}
	ports, account := newTestSDKPorts(t, client)
	ports.Execute = func(ctx context.Context, _ pixivmcpserver.Account, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
		_, err := attempt(ctx, mustOpenWireClient(t, client))
		if err == nil {
			return nil
		}
		client.mu.Lock()
		client.novelBookmarksErr = nil
		client.mu.Unlock()
		_, err = attempt(ctx, mustOpenWireClient(t, client))
		return err
	}
	session, closeSession := newSDKTestSessionWithPorts(t, &fakeAPI{}, ports, account)
	defer closeSession()

	result := callTool(t, session, "bookmark_list_all", map[string]any{"user_id": 90, "limit": 2})
	if result.IsError {
		t.Fatalf("bookmark_list_all replay returned MCP error: %+v", result)
	}
	var out bookmarkAggregateOutput
	decodeStructured(t, result, &out)
	if len(out.Records) != 2 || out.Records[0].ID != "31" || out.Records[1].ID != "32" {
		t.Fatalf("replayed aggregate output=%+v, want only successful artwork+novel page", out)
	}
}

func TestBookmarkListAllFailureDoesNotExposePartialRecords(t *testing.T) {
	streamErr := errors.New("novel bookmark stream failed")
	client := &fakeSDKClient{
		bookmarks:         []pixiv.Artwork{testSDKIllust(41, "saved artwork", 90)},
		novelBookmarks:    []pixiv.Novel{{ID: 42, Title: "saved novel", User: pixiv.User{ID: 90, Name: "writer"}}},
		novelBookmarksErr: streamErr,
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "bookmark_list_all", map[string]any{"user_id": 90, "limit": 2})
	if !result.IsError || !resultHasText(result, "Error: pixiv:UserNovelBookmarks: upstream_error") {
		t.Fatalf("bookmark_list_all failure=%+v, want structured stream error", result)
	}
	var out bookmarkAggregateOutput
	decodeStructured(t, result, &out)
	if len(out.Records) != 0 {
		t.Fatalf("failed aggregate output=%+v, want no partial records", out)
	}
}

func TestBookmarkTagsAllPreservesSameNameCountsAndContentType(t *testing.T) {
	client := &fakeSDKClient{
		bookmarkTagsPage:      sdk.Page[pixiv.BookmarkTag]{Items: []pixiv.BookmarkTag{{Name: "same", Count: 3}}},
		novelBookmarkTagsPage: sdk.Page[pixiv.BookmarkTag]{Items: []pixiv.BookmarkTag{{Name: "same", Count: 7}}},
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "bookmark_tags_all", map[string]any{"user_id": 90, "limit": 0})
	if result.IsError {
		t.Fatalf("bookmark_tags_all returned MCP error: %+v", result)
	}
	var out bookmarkTagsAggregateOutput
	decodeStructured(t, result, &out)
	if len(out.Tags) != 2 || out.Pagination.Returned != 2 {
		t.Fatalf("aggregate tags output=%+v, want two typed tags", out)
	}
	if out.Tags[0].Name != "same" || out.Tags[0].Count != 3 || out.Tags[0].ContentType != "artwork" ||
		out.Tags[1].Name != "same" || out.Tags[1].Count != 7 || out.Tags[1].ContentType != "novel" {
		t.Fatalf("aggregate tags=%+v, want artwork then novel with independent counts", out.Tags)
	}
}

func TestBookmarkTagsAllFailureDoesNotExposePartialTags(t *testing.T) {
	streamErr := errors.New("novel bookmark tags failed")
	client := &fakeSDKClient{
		bookmarkTagsPage:      sdk.Page[pixiv.BookmarkTag]{Items: []pixiv.BookmarkTag{{Name: "art", Count: 3}}},
		novelBookmarkTagsPage: sdk.Page[pixiv.BookmarkTag]{Items: []pixiv.BookmarkTag{{Name: "novel", Count: 7}}},
		novelBookmarkTagsErr:  streamErr,
	}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "bookmark_tags_all", map[string]any{"user_id": 90, "limit": 2})
	if !result.IsError || !resultHasText(result, "Error: pixiv:UserNovelBookmarkTags: upstream_error") {
		t.Fatalf("bookmark_tags_all failure=%+v, want structured stream error", result)
	}
	var out bookmarkTagsAggregateOutput
	decodeStructured(t, result, &out)
	if len(out.Tags) != 0 {
		t.Fatalf("failed aggregate tags output=%+v, want no partial tags", out)
	}
}

func mustOpenWireClient(t *testing.T, fake *fakeSDKClient) *pixiv.Client {
	t.Helper()
	return openWireClient(t, fake)
}
