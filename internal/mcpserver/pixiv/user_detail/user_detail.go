// Package user_detail 实现 user_detail tool。
package user_detail

import (
	"context"
	"fmt"

	pipeline "github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 user_detail。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "user_detail", Description: "Get a user's complete profile through the authenticated Pixiv SDK.", OutputSchema: records.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.UserDetail, error) {
		return handleUserDetail(ctx, app, input)
	})
}

// In 只接受明确指定的目标用户，避免把请求误解析为当前认证账号。
type In struct {
	UserID int64 `json:"user_id" jsonschema:"required positive Pixiv user ID"`
}

// handleUserDetail 将完整公开 SDK envelope 封装为单条 user record，避免详情 tool
// 成为 records 契约的例外。
func handleUserDetail(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.UserDetail, error) {
	if in.UserID <= 0 {
		return outputs.UserDetailError(fmt.Errorf("user_id must be a positive integer"))
	}
	result, err := runtime.Read(app, ctx, func(ctx context.Context, client *pixiv.Client) (pixiv.UserDetail, error) {
		return client.User(ctx, pixiv.UserRequest{UserID: in.UserID})
	})
	if err != nil {
		return outputs.UserDetailError(err)
	}
	record, err := records.FromUserDetail(result)
	if err != nil {
		return outputs.UserDetailError(err)
	}
	out := outputs.UserDetail{Records: []pipeline.Record{record}}
	return outputs.UserDetailResult(out, false), out, nil
}
