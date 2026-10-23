// Package creator 实现 fanbox_creator tool。
package creator

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/internal/toolhost"
	fanbox "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 fanbox_creator。
func Register(app *toolhost.App, server *mcp.Server) {
	toolhost.AddTool(app, server, &mcp.Tool{Name: "fanbox_creator", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Get one FANBOX creator profile."}, func(ctx context.Context, request *mcp.CallToolRequest, input in) (*mcp.CallToolResult, out, error) {
		return handle(ctx, app, input)
	})
}

type in struct {
	CreatorID string `json:"creator_id" jsonschema:"required FANBOX creator id"`
}

type out struct {
	ID                string                `json:"id"`
	Name              string                `json:"name"`
	HasAdultContent   bool                  `json:"has_adult_content,omitempty"`
	IsFollowing       bool                  `json:"is_following,omitempty"`
	PlanFee           int                   `json:"plan_fee,omitempty"`
	HasSupportingPlan bool                  `json:"has_supporting_plan,omitempty"`
	Icon              *toolhost.ResourceOut `json:"icon,omitempty"`
	Cover             *toolhost.ResourceOut `json:"cover,omitempty"`
}

func handle(ctx context.Context, app *toolhost.App, input in) (*mcp.CallToolResult, out, error) {
	empty := out{}
	if input.CreatorID == "" {
		return toolhost.Result(empty, true, "Error: creator_id is required"), empty, nil
	}
	lease, err := app.OpenClient(ctx)
	if err != nil {
		return toolhost.Result(empty, true, "Error: "+err.Error()), empty, nil
	}
	defer lease.Close()
	client := lease.Value()
	creator, err := client.Creator(ctx, fanbox.CreatorRequest{CreatorID: input.CreatorID})
	if err != nil {
		return toolhost.Result(empty, true, "Error: "+err.Error()), empty, nil
	}
	out := out{
		ID:                creator.ID,
		Name:              creator.Name,
		HasAdultContent:   creator.HasAdultContent,
		IsFollowing:       creator.IsFollowing,
		PlanFee:           creator.PlanFee,
		HasSupportingPlan: creator.HasSupportingPlan,
		Icon:              toolhost.ResourceOutFrom(creator.Icon.Resource),
		Cover:             toolhost.ResourceOutFrom(creator.Cover.Resource),
	}
	return toolhost.Result(out, false, fmt.Sprintf("Retrieved creator %s.", out.ID)), out, nil
}
