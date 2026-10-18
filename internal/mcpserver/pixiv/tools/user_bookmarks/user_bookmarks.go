// Package user_bookmarks 实现 pixiv_user_bookmarks tool。
package user_bookmarks

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/filters"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 pixiv_user_bookmarks。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "pixiv_user_bookmarks", InputSchema: inputSchema(), Description: "Browse a user's bookmarked artworks or novels.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)},
		OutputSchema: records.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Records, error) {
		return handle(ctx, app, input)
	})
}

// In 要求显式选择目标类型；输出仍保留原操作的字段与语义。
type In struct {
	Kind         string                `json:"kind" jsonschema:"content type"`
	UserID       int64                 `json:"user_id,omitempty" jsonschema:"optional user ID; defaults to the authenticated user"`
	Restrict     string                `json:"restrict,omitempty" jsonschema:"public or private"`
	Tag          string                `json:"tag,omitempty"`
	IllustFilter *filters.IllustFilter `json:"illust_filter,omitempty"`
	runtime.PageLimitIn
}

func handle(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Records, error) {
	plan, err := runtime.ParseListPlan(in.PageLimitIn)
	if err != nil {
		return outputs.Error(err)
	}
	if in.Kind == "novel" && in.IllustFilter != nil {
		return outputs.Error(errors.New("illust_filter requires kind=artwork"))
	}
	if in.Kind == "novel" {
		userID, err := runtime.ResolveUserID(app, ctx, in.UserID)
		if err != nil {
			return outputs.Error(err)
		}
		items, more, err := runtime.CollectWith(ctx, app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
			result, err := client.UserNovelBookmarks(ctx, pixiv.UserNovelBookmarksRequest{UserID: userID, Restrict: pixiv.Restrict(in.Restrict), Tag: in.Tag, Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		})
		if err != nil {
			return outputs.Error(err)
		}
		recordItems, err := records.FromNovels(items)
		if err != nil {
			return outputs.Error(err)
		}
		out := outputs.Records{Records: recordItems, Pagination: runtime.ListPagination(plan, in.Limit, len(items), more)}
		return outputs.Result(out, false), out, nil
	}

	userID := in.UserID
	ctx, err = filters.WithIllustFilter(ctx, in.IllustFilter)
	if err != nil {
		return outputs.Error(err)
	}
	userID, err = runtime.ResolveUserID(app, ctx, userID)
	if err != nil {
		return outputs.Error(err)
	}
	items, more, err := runtime.CollectWith(ctx, app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.UserArtworkBookmarks(ctx, pixiv.UserArtworkBookmarksRequest{UserID: userID, Restrict: pixiv.Restrict(in.Restrict), Tag: in.Tag, Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	})
	if err != nil {
		return outputs.Error(err)
	}
	recordItems, err := records.FromArtworks(items)
	if err != nil {
		return outputs.Error(err)
	}
	out := outputs.Records{Records: recordItems, Pagination: runtime.ListPagination(plan, in.Limit, len(items), more)}
	return outputs.Result(out, false), out, nil
}

func inputSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[In](nil)
	if err != nil {
		panic(err)
	}
	schema.Properties["kind"].Enum = []any{"artwork", "novel"}
	return schema
}
