// Package trending_tags_illust 实现 trending_tags_illust tool。
package trending_tags_illust

import (
	"context"
	"fmt"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 trending_tags_illust。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "trending_tags_illust", Description: "Get currently trending illustration tags."}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.TrendingTags, error) {
		return handleTrendingTags(ctx, app, input)
	})
}

type In struct{}

func handleTrendingTags(ctx context.Context, app *runtime.App, _ In) (*mcp.CallToolResult, outputs.TrendingTags, error) {
	result, err := runtime.Read(app, ctx, func(ctx context.Context, client *pixiv.Client) ([]pixiv.TrendingTag, error) {
		return client.TrendingArtworkTags(ctx, pixiv.TrendingArtworkTagsRequest{})
	})
	if err != nil {
		return outputs.TrendingTagsError(err)
	}
	out := outputs.TrendingTags{Tags: []pixiv.TrendingTagDTO{}}
	if len(result) == 0 {
		out.Text = "Could not retrieve trending tags."
		return outputs.TrendingTagsResult(out, false), out, nil
	}
	lines := make([]string, 0, len(result))
	for _, tag := range result {
		out.Tags = append(out.Tags, pixiv.ToTrendingTagDTO(tag))
		translated := tag.TranslatedName
		if translated == "" {
			translated = "none"
		}
		lines = append(lines, fmt.Sprintf("- %s (translation: %s)", tag.Tag, translated))
	}
	out.Text = "Trending tags:\n" + strings.Join(lines, "\n")
	return outputs.TrendingTagsResult(out, false), out, nil
}
