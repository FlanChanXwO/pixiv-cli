// Package creators 实现 fanbox_creators tool。
package creators

import (
	"context"
	"errors"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/internal/toolhost"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	fanbox "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 fanbox_creators。
func Register(app *toolhost.App, server *mcp.Server) {
	toolhost.AddTool(app, server, &mcp.Tool{Name: "fanbox_creators", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "List supporting or following FANBOX creators."}, func(ctx context.Context, request *mcp.CallToolRequest, input in) (*mcp.CallToolResult, out, error) {
		return handle(ctx, app, input)
	})
}

type in struct {
	Kind string `json:"kind,omitempty" jsonschema:"supporting or following"`
	toolhost.ListIn
}

type creatorSummaryOut struct {
	ID   string                `json:"id"`
	Name string                `json:"name,omitempty"`
	Icon *toolhost.ResourceOut `json:"icon,omitempty"`
}

type out struct {
	Creators   []creatorSummaryOut    `json:"creators"`
	Pagination toolhost.PaginationOut `json:"pagination"`
}

func handle(ctx context.Context, app *toolhost.App, input in) (*mcp.CallToolResult, out, error) {
	out := out{Creators: []creatorSummaryOut{}}
	plan, err := toolhost.ParseListPlan(input.ListIn)
	kind := fanbox.CreatorListKind(input.Kind)
	if input.Kind == "" {
		kind = fanbox.CreatorListSupporting
	}
	if err == nil && kind != fanbox.CreatorListSupporting && kind != fanbox.CreatorListFollowing {
		err = errors.New("kind must be one of: supporting, following")
	}
	if err != nil {
		return toolhost.Result(out, true, "Error: "+err.Error()), out, nil
	}
	lease, openErr := app.OpenClient(ctx)
	if openErr != nil {
		return toolhost.Result(out, true, "Error: "+openErr.Error()), out, nil
	}
	defer lease.Close()
	client := lease.Value()
	items, hasMore, fetchErr := toolhost.CollectPages(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) (sdk.Page[fanbox.CreatorSummary], error) {
		return client.Creators(ctx, fanbox.CreatorsRequest{Kind: kind, Cursor: cursor})
	})
	if fetchErr != nil {
		return toolhost.Result(out, true, "Error: "+fetchErr.Error()), out, nil
	}
	for _, item := range items {
		out.Creators = append(out.Creators, creatorSummaryOut{ID: item.ID, Name: item.Name, Icon: toolhost.ResourceOutFrom(item.Icon.Resource)})
	}
	out.Pagination = toolhost.ListPagination(plan, input.Limit, len(out.Creators), hasMore)
	return toolhost.Result(out, false, fmt.Sprintf("Retrieved %d creators.", len(out.Creators))), out, nil
}
