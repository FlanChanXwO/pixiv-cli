package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/bootstrap"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
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
	application.AuthenticatedPixivClient
	application.ArtworkClient
	application.DownloadClient
}

var (
	runMCPServer   = runMCP
	newCLIServices = bootstrap.NewServices
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
	return bootstrap.RunMCP(ctx, errOut)
}

func (a app) services() application.Services {
	logger := slog.New(slog.NewTextHandler(a.errOut, nil))
	return newCLIServices(logger)
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

func (a app) clientRequest(cmd *cobra.Command, opts commandOptions, needsAuth bool) application.ClientRequest {
	req := application.ClientRequest{
		Profile:      opts.profile,
		RefreshToken: opts.refreshToken,
		NeedsAuth:    needsAuth,
	}
	if cmd.Flags().Changed("download-path") {
		req.DownloadPathOverride = &opts.downloadPath
	}
	if cmd.Flags().Changed("filename-template") {
		req.FilenameTemplateOverride = &opts.filenameTemplate
	}
	if cmd.Flags().Changed("json") {
		req.JSONOverride = &opts.jsonOut
	}
	return req
}

func (a app) printJSON(v any) error {
	encoder := json.NewEncoder(a.out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
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
