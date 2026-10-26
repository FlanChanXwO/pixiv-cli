// pixiv related tools.

package tools

import (
	"context"
	"errors"
	"fmt"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/filters"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	record "github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerBlockedUsers 注册 pixiv_blocked_users。
func (t pixivTools) registerBlockedUsers(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_blocked_users", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "View users blocked by the authenticated account through the App API.", InputSchema: schemas.List(map[string]any{
		"user_id":  schemas.PositiveInteger("Optional positive Pixiv user ID; defaults to the authenticated user."),
		"restrict": schemas.EnumString("Compatibility visibility field.", "public", "private"),
	}), OutputSchema: schemas.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input blockedUsersIn) (*mcp.CallToolResult, outputs.Records, error) {
		return t.blockedUsersHandler(ctx, input)
	})
}

type blockedUsersIn struct {
	UserID   int64  `json:"user_id,omitempty" jsonschema:"optional user ID; defaults to authenticated user"`
	Restrict string `json:"restrict,omitempty" jsonschema:"public or private"`
	toolhost.PageLimitIn
}

func (t pixivTools) blockedUsersHandler(ctx context.Context, in blockedUsersIn) (*mcp.CallToolResult, outputs.Records, error) {
	return t.listUserRelations(ctx, "blocked", in.UserID, in.Restrict, in.PageLimitIn, in.Limit)
}

// registerFollowUser 注册 pixiv_follow_user。
func (t pixivTools) registerFollowUser(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_follow_user", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Follow a Pixiv user."}, func(ctx context.Context, request *mcp.CallToolRequest, input followUserIn) (*mcp.CallToolResult, outputs.Mutation, error) {
		return t.followUserHandler(ctx, input)
	})
}

type followUserIn struct {
	UserID   int64  `json:"user_id" jsonschema:"user ID"`
	Restrict string `json:"restrict,omitempty" jsonschema:"public or private"`
}

func (t pixivTools) followUserHandler(ctx context.Context, in followUserIn) (*mcp.CallToolResult, outputs.Mutation, error) {
	out := outputs.Mutation{Action: "follow_user", UserID: in.UserID, Text: fmt.Sprintf("Followed user %d.", in.UserID)}
	return outputs.RunMutation(out, func() error {
		return toolhost.Write(t.app, ctx, func(ctx context.Context, client *pixiv.Client) error {
			return client.FollowUser(ctx, pixiv.FollowUserRequest{UserID: in.UserID, Restrict: pixiv.Restrict(in.Restrict)})
		})
	})
}

// registerMypixiv 注册 pixiv_mypixiv。
func (t pixivTools) registerMypixiv(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_mypixiv", Description: "Browse MyPixiv artworks, novels or users for the authenticated account.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)},
		InputSchema:  schemas.List(map[string]any{"illust_filter": filters.IllustFilterSchema(), "novel_filter": filters.NovelFilterSchema(), "user_filter": filters.UserFilterSchema(), "kind": schemas.EnumString("Required content type.", "artwork", "novel", "user")}, "kind"),
		OutputSchema: schemas.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input mypixivIn) (*mcp.CallToolResult, outputs.Records, error) {
		return t.mypixivHandler(ctx, input)
	})
}

// mypixivIn 要求显式选择目标类型；输出仍保留原操作的字段与语义。
type mypixivIn struct {
	Kind string `json:"kind"`

	IllustFilter *filters.IllustFilter `json:"illust_filter,omitempty"`
	NovelFilter  *filters.NovelFilter  `json:"novel_filter,omitempty"`
	UserFilter   *filters.UserFilter   `json:"user_filter,omitempty"`
	toolhost.PageLimitIn
}

func (t pixivTools) mypixivHandler(ctx context.Context, in mypixivIn) (*mcp.CallToolResult, outputs.Records, error) {
	plan, err := toolhost.ParseListPlan(in.PageLimitIn)
	if err != nil {
		return outputs.Error(err)
	}
	if in.Kind != "novel" && in.NovelFilter != nil {
		return outputs.Error(errors.New("novel_filter requires kind=novel"))
	}
	if in.Kind == "novel" && in.IllustFilter != nil {
		return outputs.Error(errors.New("illust_filter is not valid for novels"))
	}
	if in.Kind != "user" && in.UserFilter != nil {
		return outputs.Error(errors.New("user_filter requires kind=user"))
	}
	if in.Kind == "user" && (in.IllustFilter != nil || in.NovelFilter != nil) {
		return outputs.Error(errors.New("user records only accept user_filter"))
	}
	if in.Kind == "novel" {
		ctx, err = filters.WithNovelFilter(ctx, in.NovelFilter)
		if err != nil {
			return outputs.Error(err)
		}
		items, more, err := toolhost.CollectWith(ctx, t.app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
			result, err := client.MyPixivNovels(ctx, pixiv.MyPixivNovelsRequest{Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		})
		if err != nil {
			return outputs.Error(err)
		}
		recordItems, err := record.FromNovels(items)
		if err != nil {
			return outputs.Error(err)
		}
		out := outputs.Records{Records: recordItems, Pagination: toolhost.ListPagination(plan, in.Limit, len(items), more)}
		return outputs.Result(out, false), out, nil
	}
	if in.Kind == "user" {
		ctx, err = filters.WithUserFilter(ctx, in.UserFilter)
		if err != nil {
			return outputs.Error(err)
		}
		items, more, err := toolhost.CollectWith(ctx, t.app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
			result, err := client.MyPixivUsers(ctx, pixiv.MyPixivUsersRequest{Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		})
		if err != nil {
			return outputs.Error(err)
		}
		recordItems, err := record.FromUserPreviews(items)
		if err != nil {
			return outputs.Error(err)
		}
		out := outputs.Records{Records: recordItems, Pagination: toolhost.ListPagination(plan, in.Limit, len(items), more)}
		return outputs.Result(out, false), out, nil
	}

	ctx, err = filters.WithIllustFilter(ctx, in.IllustFilter)
	if err != nil {
		return outputs.Error(err)
	}
	items, more, err := toolhost.CollectWith(ctx, t.app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.MyPixivArtworks(ctx, pixiv.MyPixivArtworksRequest{Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	})
	if err != nil {
		return outputs.Error(err)
	}
	recordItems, err := record.FromArtworks(items)
	if err != nil {
		return outputs.Error(err)
	}
	out := outputs.Records{Records: recordItems, Pagination: toolhost.ListPagination(plan, in.Limit, len(items), more)}
	return outputs.Result(out, false), out, nil
}

// registerUnfollowUser 注册 pixiv_unfollow_user。
func (t pixivTools) registerUnfollowUser(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_unfollow_user", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: new(true), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Unfollow a Pixiv user."}, func(ctx context.Context, request *mcp.CallToolRequest, input unfollowUserIn) (*mcp.CallToolResult, outputs.Mutation, error) {
		return t.unfollowUserHandler(ctx, input)
	})
}

type unfollowUserIn struct {
	UserID int64 `json:"user_id" jsonschema:"user ID"`
}

func (t pixivTools) unfollowUserHandler(ctx context.Context, in unfollowUserIn) (*mcp.CallToolResult, outputs.Mutation, error) {
	out := outputs.Mutation{Action: "unfollow_user", UserID: in.UserID, Text: fmt.Sprintf("Unfollowed user %d.", in.UserID)}
	return outputs.RunMutation(out, func() error {
		return toolhost.Write(t.app, ctx, func(ctx context.Context, client *pixiv.Client) error {
			return client.UnfollowUser(ctx, pixiv.UnfollowUserRequest{UserID: in.UserID})
		})
	})
}

// registerUserRelations 注册 pixiv_user_relations。
func (t pixivTools) registerUserRelations(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_user_relations", Description: "Browse followers, following or related users; restrict applies only to followers/following.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)},
		InputSchema:  schemas.List(map[string]any{"relation": schemas.EnumString("User relation.", "followers", "following", "related"), "user_id": schemas.PositiveInteger("Optional user ID; defaults to authenticated user."), "restrict": schemas.EnumString("Followers/following visibility; ignored for related users.", "public", "private"), "user_filter": filters.UserFilterSchema()}, "relation"),
		OutputSchema: schemas.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input userRelationsIn) (*mcp.CallToolResult, outputs.Records, error) {
		return t.userRelationsHandler(ctx, input)
	})
}

// userRelationsIn 要求显式选择目标类型；输出仍保留原操作的字段与语义。
type userRelationsIn struct {
	Relation   string              `json:"relation"`
	UserID     int64               `json:"user_id,omitempty" jsonschema:"optional user ID; defaults to authenticated user"`
	Restrict   string              `json:"restrict,omitempty" jsonschema:"public or private"`
	UserFilter *filters.UserFilter `json:"user_filter,omitempty"`
	toolhost.PageLimitIn
}

func (t pixivTools) userRelationsHandler(ctx context.Context, in userRelationsIn) (*mcp.CallToolResult, outputs.Records, error) {
	if in.Relation == "followers" && in.UserFilter != nil {
		return outputs.Error(errors.New("user_filter is only supported for following and related users"))
	}
	if _, err := toolhost.ParseListPlan(in.PageLimitIn); err != nil {
		return outputs.Error(err)
	}
	ctx, err := filters.WithUserFilter(ctx, in.UserFilter)
	if err != nil {
		return outputs.Error(err)
	}
	return t.listUserRelations(ctx, in.Relation, in.UserID, in.Restrict, in.PageLimitIn, in.Limit)
}

// ListUserRelations 在账号池重放边界内收集 followers/following/related/blocked 用户列表并
// 输出统一 records envelope。
func (t pixivTools) listUserRelations(ctx context.Context, kind string, userID int64, restrict string, input toolhost.PageLimitIn, limit *int) (*mcp.CallToolResult, outputs.Records, error) {
	userID, err := toolhost.ResolveUserID(t.app, ctx, userID)
	if err != nil {
		return outputs.Error(err)
	}
	plan, err := toolhost.ParseListPlan(input)
	if err != nil {
		return outputs.Error(err)
	}
	items, more, err := toolhost.CollectWith(ctx, t.app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
		var result sdk.Page[pixiv.UserPreview]
		var err error
		switch kind {
		case "followers":
			result, err = client.UserFollowers(ctx, pixiv.UserFollowersRequest{UserID: userID, Restrict: pixiv.Restrict(restrict), Cursor: cursor})
		case "following":
			result, err = client.UserFollowing(ctx, pixiv.UserFollowingRequest{UserID: userID, Restrict: pixiv.Restrict(restrict), Cursor: cursor})
		case "related":
			result, err = client.RelatedUsers(ctx, pixiv.RelatedUsersRequest{UserID: userID, Cursor: cursor})
		case "blocked":
			result, err = client.UserBlockedUsers(ctx, pixiv.UserBlockedUsersRequest{UserID: userID, Cursor: cursor})
		default:
			return nil, sdk.Cursor{}, errors.New("unknown user relation")
		}
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	})
	if err != nil {
		return outputs.Error(err)
	}
	recordItems, err := record.FromUserPreviews(items)
	if err != nil {
		return outputs.Error(err)
	}
	out := outputs.Records{Records: recordItems, Pagination: toolhost.ListPagination(plan, limit, len(items), more)}
	return outputs.Result(out, false), out, nil
}
