package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/utils"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (a *App) refreshToken(ctx context.Context, _ *mcp.CallToolRequest, _ emptyIn) (*mcp.CallToolResult, textOut, error) {
	client, release, err := a.openSDKMutable(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return toolTextError(ctx, err, "Token刷新已取消。")
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return toolTextError(ctx, err, "Token刷新失败：操作超时。")
		}
		var sdkErr *sdk.Error
		if errors.As(err, &sdkErr) {
			return toolTextError(ctx, err, "Token刷新失败：无法初始化 Pixiv SDK："+sdkErr.Error()+"。")
		}
		return toolTextError(ctx, err, "Token刷新失败：无法初始化 Pixiv SDK。请检查本地配置或代理设置。")
	}
	defer release()
	account, err := client.Refresh(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return toolTextError(ctx, err, "Token刷新已取消。")
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return toolTextError(ctx, err, "Token刷新失败：操作超时。")
		}
		if errors.Is(err, sdk.ErrUnauthorized) {
			return toolTextError(ctx, err, "错误：未设置 refresh token。请先使用 set_refresh_token 工具设置 token。")
		}
		var sdkErr *sdk.Error
		if errors.As(err, &sdkErr) {
			return toolTextError(ctx, err, "Token刷新失败："+sdkErr.Error()+"。")
		}
		return toolTextError(ctx, err, "Token刷新失败。请检查 refresh token 是否有效，以及网络连接或代理设置。")
	}
	a.sdkRequest.UserID = account.UserID
	return toolText(fmt.Sprintf("Token刷新成功！%s。现在可以正常使用Pixiv API功能了。", authIdentityText(*account)))
}

type setRefreshTokenIn struct {
	RefreshToken string `json:"refresh_token" jsonschema:"Pixiv refresh token"`
}

func (a *App) setRefreshToken(ctx context.Context, _ *mcp.CallToolRequest, in setRefreshTokenIn) (*mcp.CallToolResult, textOut, error) {
	token, err := utils.ValidateRefreshTokenInput(in.RefreshToken)
	if err != nil {
		return toolTextError(ctx, err, "错误："+err.Error())
	}
	if token == "" {
		return toolTextError(ctx, errLegacyValidation, "错误：refresh token 不能为空。")
	}
	client, release, err := a.openSDKMutable(ctx)
	if err != nil {
		return toolTextError(ctx, err, "Refresh token 已在当前会话设置，但认证失败: "+err.Error())
	}
	defer release()
	account, err := client.ImportAccount(ctx, token)
	if err != nil {
		return toolTextError(ctx, err, fmt.Sprintf("Refresh token 已在当前会话设置，但认证失败: %v\n\n请检查 token 是否有效，或稍后使用 refresh_token 工具重试认证。", err))
	}
	if err := client.SelectAccount(account.UserID); err != nil {
		return toolTextError(ctx, err, "Refresh token 已在当前会话设置并完成认证，但无法选择认证账号: "+err.Error())
	}
	a.sdkRequest.UserID = account.UserID
	return toolText(fmt.Sprintf("Refresh token 已在当前会话设置并完成认证！\n%s\n\n现在您可以使用所有 Pixiv 功能了。", authIdentityText(*account)))
}

func authIdentityText(account sdk.Account) string {
	identity := fmt.Sprintf("用户 ID: %d", account.UserID)
	if username := strings.TrimSpace(account.Username); username != "" {
		identity += "\n用户名: " + username
	}
	return identity
}
