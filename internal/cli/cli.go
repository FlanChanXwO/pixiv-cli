package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-mcp-server/internal/cli/mcpapp"
	"github.com/FlanChanXwO/pixiv-mcp-server/internal/cli/state"
	"github.com/FlanChanXwO/pixiv-mcp-server/internal/config"
	"github.com/FlanChanXwO/pixiv-mcp-server/internal/download"
	"github.com/FlanChanXwO/pixiv-mcp-server/internal/pixiv"
	"github.com/FlanChanXwO/pixiv-mcp-server/internal/utils"
	"github.com/spf13/cobra"
)

type app struct {
	in     io.Reader
	out    io.Writer
	errOut io.Writer
}

type commandOptions struct {
	profile          string
	refreshToken     string
	downloadPath     string
	filenameTemplate string
	jsonOut          bool
}

type clientConfig struct {
	config.RuntimeConfig
}

type cliPixivClient interface {
	download.PixivClient
	Refresh(context.Context) error
	UserID() int64
	SearchIllust(context.Context, string, string, string, string, int) (*pixiv.IllustList, error)
	IllustRanking(context.Context, string, string, int) (*pixiv.IllustList, error)
	IllustRecommended(context.Context, int) (*pixiv.IllustList, error)
}

var (
	runMCPServer = runMCP
	newCLIClient = newPixivClient
)

func Run(args []string, in io.Reader, out io.Writer, errOut io.Writer) int {
	if len(args) == 0 {
		args = []string{"pixiv"}
	}
	a := app{in: in, out: out, errOut: errOut}
	cmd := a.newRootCommand()
	cmd.SetIn(in)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs(args[1:])
	return a.exit(cmd.Execute())
}

func (a app) exit(err error) int {
	if err == nil {
		return 0
	}
	fmt.Fprintln(a.errOut, "error:", err)
	return 1
}

func runMCP(ctx context.Context, errOut io.Writer) error {
	return mcpapp.Run(ctx, errOut)
}

func newPixivClient(cfg clientConfig) (cliPixivClient, error) {
	return pixiv.NewSource(pixiv.SourceConfig{
		RefreshToken:       cfg.RefreshToken,
		HTTPSProxy:         cfg.HTTPSProxy,
		WebFallbackEnabled: cfg.WebFallbackEnabled,
	})
}

func (a app) newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "pixiv",
		Short:         "Pixiv CLI and MCP server",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.AddCommand(
		a.newAccountCommand(),
		a.newConfigCommand(),
		a.newSearchCommand(),
		a.newDetailCommand(),
		a.newRankingCommand(),
		a.newRecommendedCommand(),
		a.newDownloadCommand(),
		a.newMCPCommand(),
	)
	return cmd
}

func (a app) newMCPCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "mcp",
		Short:   "Run the MCP stdio server",
		Example: "pixiv mcp",
		Args:    requireExactArgs(0, "pixiv mcp"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPServer(context.Background(), a.errOut)
		},
	}
}

func (a app) bindCommonFlags(cmd *cobra.Command, opts *commandOptions) {
	flags := cmd.Flags()
	flags.StringVar(&opts.profile, "profile", "", "account name")
	flags.StringVar(&opts.refreshToken, "refresh-token", "", "Pixiv refresh token or cookie with refresh_token")
	flags.StringVar(&opts.downloadPath, "download-path", "", "download directory")
	flags.StringVar(&opts.filenameTemplate, "filename-template", "", "filename template")
	flags.BoolVar(&opts.jsonOut, "json", false, "print JSON")
}

func (a app) clientAndConfig(cmd *cobra.Command, opts commandOptions, needsAuth bool) (cliPixivClient, config.RuntimeConfig, bool, error) {
	settings, err := config.LoadSettingsState()
	if err != nil {
		return nil, config.RuntimeConfig{}, false, err
	}
	cfg, err := settings.Runtime()
	if err != nil {
		return nil, config.RuntimeConfig{}, false, err
	}
	if cmd.Flags().Changed("download-path") {
		cfg.DownloadPath = opts.downloadPath
	}
	if cmd.Flags().Changed("filename-template") {
		cfg.FilenameTemplate = opts.filenameTemplate
	}
	jsonOut := cfg.OutputJSON
	if cmd.Flags().Changed("json") {
		jsonOut = opts.jsonOut
	}

	authPath, err := state.AuthFilePath()
	if err != nil {
		return nil, config.RuntimeConfig{}, false, err
	}
	store, err := state.LoadAuthStore(authPath)
	if err != nil {
		return nil, config.RuntimeConfig{}, false, err
	}
	refreshToken, err := resolveRefreshToken(store, opts.profile, opts.refreshToken)
	if err != nil {
		return nil, config.RuntimeConfig{}, false, err
	}
	cfg.RefreshToken = refreshToken
	if needsAuth && cfg.RefreshToken == "" {
		return nil, config.RuntimeConfig{}, false, errors.New("missing refresh token; use PIXIV_REFRESH_TOKEN or pixiv auth add/login")
	}
	client, err := newCLIClient(clientConfig{RuntimeConfig: cfg})
	if err != nil {
		return nil, config.RuntimeConfig{}, false, err
	}
	if needsAuth {
		if err := client.Refresh(context.Background()); err != nil {
			return nil, config.RuntimeConfig{}, false, err
		}
	}
	return client, cfg, jsonOut, nil
}

func resolveRefreshToken(store state.AuthStore, requestedProfile, requestedToken string) (string, error) {
	if strings.TrimSpace(requestedToken) != "" {
		token, parsedCookie := utils.ParsePixivWebRefreshTokenInput(requestedToken)
		if token == "" {
			if parsedCookie {
				return "", errors.New("refresh-token cookie does not contain refresh_token")
			}
			return "", errors.New("refresh-token cannot be empty")
		}
		return token, nil
	}
	if profile := strings.TrimSpace(requestedProfile); profile != "" {
		_, acct, ok := state.SelectAuthAccount(store, profile)
		if !ok {
			return "", fmt.Errorf("account %q not found", profile)
		}
		return acct.RefreshToken, nil
	}
	if token := config.RefreshTokenFromEnv(); token != "" {
		return token, nil
	}
	if _, acct, ok := state.SelectAuthAccount(store, ""); ok {
		return acct.RefreshToken, nil
	}
	return "", nil
}

func (a app) printJSON(v any) error {
	encoder := json.NewEncoder(a.out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}

func parseInt64Arg(value, name string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return id, nil
}

func textBool(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func requireExactArgs(count int, usage string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != count {
			return fmt.Errorf("usage: %s", usage)
		}
		return nil
	}
}

func requireMinArgs(count int, usage string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < count {
			return fmt.Errorf("usage: %s", usage)
		}
		return nil
	}
}

func requireMaxArgs(count int, usage string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) > count {
			return fmt.Errorf("usage: %s", usage)
		}
		return nil
	}
}
