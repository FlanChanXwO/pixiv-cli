package mcpapp

import (
	"context"
	"io"
	"log/slog"

	"github.com/FlanChanXwO/pixiv-mcp-server/internal/cli/state"
	"github.com/FlanChanXwO/pixiv-mcp-server/internal/config"
	"github.com/FlanChanXwO/pixiv-mcp-server/internal/download"
	"github.com/FlanChanXwO/pixiv-mcp-server/internal/mcpserver"
	"github.com/FlanChanXwO/pixiv-mcp-server/internal/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func Run(ctx context.Context, errOut io.Writer) error {
	logger := slog.New(slog.NewTextHandler(errOut, &slog.HandlerOptions{Level: slog.LevelInfo}))
	settings, err := config.LoadSettingsState()
	if err != nil {
		return err
	}
	cfg, err := settings.Runtime()
	if err != nil {
		return err
	}
	authPath, err := state.AuthFilePath()
	if err != nil {
		return err
	}
	store, err := state.LoadAuthStore(authPath)
	if err != nil {
		return err
	}
	if cfg.RefreshToken = config.RefreshTokenFromEnv(); cfg.RefreshToken == "" {
		if _, acct, ok := state.SelectAuthAccount(store, ""); ok {
			cfg.RefreshToken = acct.RefreshToken
		}
	}
	client, err := newPixivClient(cfg)
	if err != nil {
		return err
	}
	manager := download.NewManager(client, logger, cfg.DownloadPath, cfg.FilenameTemplate)
	server := mcpserver.New(client, manager, logger)

	if cfg.RefreshToken != "" {
		if err := client.Refresh(ctx); err != nil {
			logger.Warn("auto-authentication failed", "error", err)
		} else {
			logger.Info("auto-authentication successful", "user_id", client.UserID())
		}
	}
	return server.Run(ctx, &mcp.StdioTransport{})
}

func newPixivClient(cfg config.RuntimeConfig) (*pixiv.Source, error) {
	return pixiv.NewSource(pixiv.SourceConfig{
		RefreshToken:       cfg.RefreshToken,
		HTTPSProxy:         cfg.HTTPSProxy,
		WebFallbackEnabled: cfg.WebFallbackEnabled,
	})
}
