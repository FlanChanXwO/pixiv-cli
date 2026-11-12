package tools

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (t pixivTools) myPixivArtworks(ctx context.Context, in mypixivIn, plan pagination.PagePlan) (*mcp.CallToolResult, outputs.Records, error) {
	items, more, err := toolhost.CollectSDKWith(ctx, t.app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]*pixiv.Artwork, sdk.Cursor, error) {
		page, err := client.MyPixivArtworks(ctx, pixiv.MyPixivOptions{Cursor: cursor})
		return page.Items, page.Cursor, err
	})
	if err != nil {
		return outputs.Error(err)
	}
	records, err := record.FromSDKArtworks(items)
	if err != nil {
		return outputs.Error(err)
	}
	out := outputs.Records{Records: records, Pagination: toolhost.ListPagination(plan, in.Limit, len(items), more)}
	return outputs.Result(out), out, nil
}
