package pixiv_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// 收藏/关注 mutation tool 的 owner 契约：结构化成功结果。
func TestSDKMutationTypedErrorIsMCPError(t *testing.T) {
	client := &fakeSDKClient{addBookmarkErr: &sdk.Error{
		Product:    "pixiv",
		Operation:  "AddBookmark",
		Reason:     sdk.UpstreamError,
		HTTPStatus: http.StatusBadGateway,
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "add_bookmark", map[string]any{"illust_id": 41})
	if !result.IsError {
		t.Fatalf("typed SDK mutation failure must be an MCP error: %+v", result)
	}
	var out outputs.Mutation
	decodeStructured(t, result, &out)
	if out.Success || !strings.Contains(out.Text, "upstream_error") {
		t.Fatalf("structured mutation error = %+v", out)
	}
}

func TestSDKMutationToolsReturnStructuredSuccess(t *testing.T) {
	client := &fakeSDKClient{}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	for _, test := range []struct {
		name     string
		args     map[string]any
		want     string
		wantText string
	}{
		{"add_bookmark", map[string]any{"illust_id": 9, "restrict": "private", "tags": []string{"one"}}, "add_bookmark", "Bookmarked artwork 9."},
		{"remove_bookmark", map[string]any{"illust_id": 9}, "remove_bookmark", "Removed bookmark from artwork 9."},
		{"follow_user", map[string]any{"user_id": 8, "restrict": "private"}, "follow_user", "Followed user 8."},
		{"unfollow_user", map[string]any{"user_id": 8}, "unfollow_user", "Unfollowed user 8."},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := callTool(t, session, test.name, test.args)
			var out outputs.Mutation
			decodeStructured(t, result, &out)
			if !out.Success || out.Action != test.want || out.Text != test.wantText {
				t.Fatalf("mutation output = %+v", out)
			}
		})
	}
	if client.addBookmarkRequest.ArtworkID != 9 || client.addBookmarkRequest.Restrict != pixiv.RestrictPrivate || !slices.Equal(client.addBookmarkRequest.Tags, []string{"one"}) {
		t.Fatalf("add bookmark request = %+v", client.addBookmarkRequest)
	}
	if client.removeBookmarkRequest.ArtworkID != 9 || client.followUserRequest.UserID != 8 || client.followUserRequest.Restrict != pixiv.RestrictPrivate || client.unfollowUserRequest.UserID != 8 {
		t.Fatalf("mutation requests = remove=%+v follow=%+v unfollow=%+v", client.removeBookmarkRequest, client.followUserRequest, client.unfollowUserRequest)
	}
}
