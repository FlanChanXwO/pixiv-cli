// Package mypixiv 实现 pixiv_mypixiv tool。
package mypixiv

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/filters"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	record "github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 pixiv_mypixiv。
func Register(app *toolhost.App, server *mcp.Server) {
	toolhost.AddTool(app, server, &mcp.Tool{Name: "pixiv_mypixiv", Description: "Browse MyPixiv artworks, novels or users for the authenticated account.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)},
		InputSchema:  schemas.List(map[string]any{"illust_filter": filters.IllustFilterSchema(), "novel_filter": filters.NovelFilterSchema(), "user_filter": filters.UserFilterSchema(), "kind": schemas.EnumString("Required content type.", "artwork", "novel", "user")}, "kind"),
		OutputSchema: records.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Records, error) {
		return handle(ctx, app, input)
	})
}

// In 要求显式选择目标类型；输出仍保留原操作的字段与语义。
type In struct {
	Kind string `json:"kind"`

	IllustFilter *filters.IllustFilter `json:"illust_filter,omitempty"`
	NovelFilter  *filters.NovelFilter  `json:"novel_filter,omitempty"`
	UserFilter   *filters.UserFilter   `json:"user_filter,omitempty"`
	toolhost.PageLimitIn
}

func handle(ctx context.Context, app *toolhost.App, in In) (*mcp.CallToolResult, outputs.Records, error) {
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
		items, more, err := toolhost.CollectWith(ctx, app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
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
		items, more, err := toolhost.CollectWith(ctx, app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
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
	items, more, err := toolhost.CollectWith(ctx, app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
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
