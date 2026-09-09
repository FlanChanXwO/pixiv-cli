// Package timeline_novel_following 实现 timeline_novel_following tool。
package timeline_novel_following

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/filters"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 timeline_novel_following。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "timeline_novel_following", Description: "Browse new novels from followed users through the App API.", InputSchema: novelFollowingInputSchema(), OutputSchema: records.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Records, error) {
		return handleNovelFollow(ctx, app, input)
	})
}

type In struct {
	Restrict    string               `json:"restrict,omitempty" jsonschema:"public or private"`
	NovelFilter *filters.NovelFilter `json:"novel_filter,omitempty"`
	runtime.PageLimitIn
}

func novelFollowingInputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"restrict":     map[string]any{"type": "string", "enum": []string{"public", "private"}, "description": "Visibility scope; omitted defaults to public."},
			"novel_filter": filters.NovelFilterSchema(),
			"page":         map[string]any{"type": "integer", "minimum": 1, "description": "1-based logical page; requires a positive limit."},
			"limit":        map[string]any{"type": "integer", "minimum": 0, "description": "Maximum logical results; 0 returns all; omitted reads one upstream batch."},
		},
	}
}

// handleNovelFollow 读取当前认证账号关注作者的小说新作；这是 App OAuth-only 流，
// 不用匿名 Web 搜索结果替代。
func handleNovelFollow(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Records, error) {
	if in.Restrict == "" {
		in.Restrict = string(pixiv.RestrictPublic)
	}
	plan, err := runtime.ParseListPlan(in.PageLimitIn)
	if err != nil {
		return outputs.Error(err)
	}
	ctx, err = filters.WithNovelFilter(ctx, in.NovelFilter)
	if err != nil {
		return outputs.Error(err)
	}
	items, more, err := runtime.CollectWith(ctx, app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
		result, err := client.FollowingNovels(ctx, pixiv.FollowingNovelsRequest{Restrict: pixiv.Restrict(in.Restrict), Cursor: cursor})
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
