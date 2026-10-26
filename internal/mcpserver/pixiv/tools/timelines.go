// pixiv timelines tools.

package tools

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/filters"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	record "github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerTimelineFollowing 注册 pixiv_timeline_following。
func (t pixivTools) registerTimelineFollowing(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_timeline_following", Description: "Browse artworks or novels from followed users.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)},
		InputSchema:  schemas.List(map[string]any{"illust_filter": filters.IllustFilterSchema(), "novel_filter": filters.NovelFilterSchema(), "restrict": schemas.EnumString("Visibility; omitted defaults to public.", "public", "private"), "kind": schemas.EnumString("Required content type.", "artwork", "novel")}, "kind"),
		OutputSchema: schemas.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input timelineFollowingIn) (*mcp.CallToolResult, outputs.Records, error) {
		return t.timelineFollowingHandler(ctx, input)
	})
}

// timelineFollowingIn 要求显式选择目标类型；输出仍保留原操作的字段与语义。
type timelineFollowingIn struct {
	Kind string `json:"kind"`

	IllustFilter *filters.IllustFilter `json:"illust_filter,omitempty"`
	NovelFilter  *filters.NovelFilter  `json:"novel_filter,omitempty"`
	Restrict     string                `json:"restrict,omitempty"`
	toolhost.PageLimitIn
}

func (t pixivTools) timelineFollowingHandler(ctx context.Context, in timelineFollowingIn) (*mcp.CallToolResult, outputs.Records, error) {
	plan, err := toolhost.ParseListPlan(in.PageLimitIn)
	if err != nil {
		return outputs.Error(err)
	}
	if in.Restrict == "" {
		in.Restrict = string(pixiv.RestrictPublic)
	}
	if in.Kind != "novel" && in.NovelFilter != nil {
		return outputs.Error(errors.New("novel_filter requires kind=novel"))
	}
	if in.Kind == "novel" && in.IllustFilter != nil {
		return outputs.Error(errors.New("illust_filter is not valid for novels"))
	}
	if in.Kind == "novel" {
		ctx, err = filters.WithNovelFilter(ctx, in.NovelFilter)
		if err != nil {
			return outputs.Error(err)
		}
		items, more, err := toolhost.CollectWith(ctx, t.app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
			result, err := client.FollowingNovels(ctx, pixiv.FollowingNovelsRequest{Restrict: pixiv.Restrict(in.Restrict), Cursor: cursor})
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
		return outputs.Result(out), out, nil
	}

	ctx, err = filters.WithIllustFilter(ctx, in.IllustFilter)
	if err != nil {
		return outputs.Error(err)
	}
	items, more, err := toolhost.CollectWith(ctx, t.app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.FollowingArtworks(ctx, pixiv.FollowingArtworksRequest{Restrict: pixiv.Restrict(in.Restrict), Cursor: cursor})
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
	return outputs.Result(out), out, nil
}

// registerTimelineLatest 注册 pixiv_timeline_latest。
func (t pixivTools) registerTimelineLatest(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_timeline_latest", Description: "Browse latest illustrations, manga or novels.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)},
		InputSchema:  schemas.List(map[string]any{"illust_filter": filters.IllustFilterSchema(), "novel_filter": filters.NovelFilterSchema(), "content_type": schemas.EnumString("Required content type.", "illust", "manga", "novel")}, "content_type"),
		OutputSchema: schemas.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input timelineLatestIn) (*mcp.CallToolResult, outputs.Records, error) {
		return t.timelineLatestHandler(ctx, input)
	})
}

// timelineLatestIn 要求显式选择目标类型；输出仍保留原操作的字段与语义。
type timelineLatestIn struct {
	ContentType pixiv.SearchContentType `json:"content_type"`

	IllustFilter *filters.IllustFilter `json:"illust_filter,omitempty"`
	NovelFilter  *filters.NovelFilter  `json:"novel_filter,omitempty"`
	toolhost.PageLimitIn
}

func (t pixivTools) timelineLatestHandler(ctx context.Context, in timelineLatestIn) (*mcp.CallToolResult, outputs.Records, error) {
	plan, err := toolhost.ParseListPlan(in.PageLimitIn)
	if err != nil {
		return outputs.Error(err)
	}
	if in.ContentType != "novel" && in.NovelFilter != nil {
		return outputs.Error(errors.New("novel_filter requires content_type=novel"))
	}
	if in.ContentType == "novel" && in.IllustFilter != nil {
		return outputs.Error(errors.New("illust_filter is not valid for novels"))
	}
	if in.ContentType == "novel" {
		ctx, err = filters.WithNovelFilter(ctx, in.NovelFilter)
		if err != nil {
			return outputs.Error(err)
		}
		items, more, err := toolhost.CollectWith(ctx, t.app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
			result, err := client.LatestNovels(ctx, pixiv.LatestNovelsRequest{Cursor: cursor})
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
		return outputs.Result(out), out, nil
	}

	if in.ContentType != pixiv.SearchContentTypeIllust && in.ContentType != pixiv.SearchContentTypeManga {
		return outputs.Error(errors.New("content_type must be one of: illust, manga"))
	}
	ctx, err = filters.WithIllustFilter(ctx, in.IllustFilter)
	if err != nil {
		return outputs.Error(err)
	}
	items, more, err := toolhost.CollectWith(ctx, t.app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.LatestArtworks(ctx, pixiv.LatestArtworksRequest{ContentType: in.ContentType, Cursor: cursor})
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
	return outputs.Result(out), out, nil
}
