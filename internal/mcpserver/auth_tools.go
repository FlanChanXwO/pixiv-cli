package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/utils/credentials"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (a *App) refreshToken(ctx context.Context, _ *mcp.CallToolRequest, _ emptyIn) (*mcp.CallToolResult, textOut, error) {
	client, release, err := a.openSDKMutable(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return toolTextError(ctx, err, "Refresh canceled.")
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return toolTextError(ctx, err, "Refresh failed: operation timed out.")
		}
		var sdkErr *sdk.Error
		if errors.As(err, &sdkErr) {
			return toolTextError(ctx, err, "Refresh failed: could not initialize the Pixiv SDK: "+sdkErr.Error())
		}
		return toolTextError(ctx, err, "Refresh failed: could not initialize the Pixiv SDK. Check the local configuration or proxy settings.")
	}
	defer release()
	account, err := client.Refresh(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return toolTextError(ctx, err, "Refresh canceled.")
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return toolTextError(ctx, err, "Refresh failed: operation timed out.")
		}
		if errors.Is(err, sdk.ErrUnauthorized) {
			return toolTextError(ctx, err, "Error: no refresh token is configured. Use set_refresh_token to set one first.")
		}
		var sdkErr *sdk.Error
		if errors.As(err, &sdkErr) {
			return toolTextError(ctx, err, "Refresh failed: "+sdkErr.Error())
		}
		return toolTextError(ctx, err, "Refresh failed. Check whether the refresh token is valid and verify the network or proxy settings.")
	}
	a.sdkRequest.UserID = account.UserID
	return toolText(fmt.Sprintf("Refresh successful.\n%s\n\nYou can now use Pixiv API features.", authIdentityText(*account)))
}

type setRefreshTokenIn struct {
	RefreshToken string `json:"refresh_token" jsonschema:"Pixiv refresh token"`
}

func (a *App) setRefreshToken(ctx context.Context, _ *mcp.CallToolRequest, in setRefreshTokenIn) (*mcp.CallToolResult, textOut, error) {
	token, err := credentials.ValidateRefreshTokenInput(in.RefreshToken)
	if err != nil {
		return toolTextError(ctx, err, "Error: "+err.Error())
	}
	if token == "" {
		return toolTextError(ctx, errLegacyValidation, "Error: refresh token must not be empty.")
	}
	client, release, err := a.openSDKMutable(ctx)
	if err != nil {
		return toolTextError(ctx, err, "The refresh token was set for this session, but authentication failed: "+err.Error())
	}
	defer release()
	account, err := client.ImportAccount(ctx, token)
	if err != nil {
		return toolTextError(ctx, err, fmt.Sprintf("The refresh token was set for this session, but authentication failed: %v\n\nCheck that the token is valid, then retry with refresh_token.", err))
	}
	if err := client.SelectAccount(account.UserID); err != nil {
		return toolTextError(ctx, err, "The refresh token was set for this session and authenticated, but the authenticated account could not be selected: "+err.Error())
	}
	a.sdkRequest.UserID = account.UserID
	return toolText(fmt.Sprintf("The refresh token was set for this session and authenticated.\n%s\n\nYou can now use all Pixiv features.", authIdentityText(*account)))
}

func authIdentityText(account sdk.Account) string {
	identity := fmt.Sprintf("User ID: %d", account.UserID)
	if username := strings.TrimSpace(account.Username); username != "" {
		identity += "\nUsername: " + username
	}
	return identity
}
