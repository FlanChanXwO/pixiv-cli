package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/bootstrap"
	"github.com/FlanChanXwO/pixiv-cli/internal/buildinfo"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/spf13/cobra"
)

type app struct {
	in     io.Reader
	out    io.Writer
	errOut io.Writer
}

type commandOptions struct {
	proxyOptions
	profile          string
	uid              string
	refreshToken     string
	downloadPath     string
	filenameTemplate string
	jsonOut          bool
}

type proxyOptions struct {
	proxy   string
	noProxy bool
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

func runMCP(ctx context.Context, errOut io.Writer, proxyOverride *string) error {
	return bootstrap.RunMCP(ctx, errOut, proxyOverride)
}

func (a app) services() application.Services {
	logger := slog.New(slog.NewTextHandler(a.errOut, nil))
	return newCLIServices(logger)
}

func (a app) newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "pixiv",
		Short:         "Pixiv CLI and MCP server",
		Version:       buildinfo.Current().Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.SetVersionTemplate("pixiv {{.Version}}\n")
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
		a.newVersionCommand(),
		a.newUpdateCommand(),
	)
	return cmd
}

func (a app) newMCPCommand() *cobra.Command {
	var opts proxyOptions
	cmd := &cobra.Command{
		Use:     "mcp",
		Short:   "Run the MCP stdio server",
		Example: "pixiv mcp",
		Args:    requireExactArgs(0, "pixiv mcp"),
		RunE: func(cmd *cobra.Command, args []string) error {
			proxyOverride, err := proxyOverrideFromFlags(cmd, opts)
			if err != nil {
				return err
			}
			return runMCPServer(context.Background(), a.errOut, proxyOverride)
		},
	}
	a.bindProxyFlags(cmd, &opts)
	return cmd
}

func (a app) bindCommonFlags(cmd *cobra.Command, opts *commandOptions) {
	flags := cmd.Flags()
	flags.StringVar(&opts.uid, "uid", "", "Pixiv UID from auth.json")
	flags.StringVar(&opts.profile, "profile", "", "deprecated alias for --uid")
	_ = flags.MarkDeprecated("profile", "use --uid instead")
	flags.StringVar(&opts.refreshToken, "refresh-token", "", "Pixiv refresh token or cookie with refresh_token")
	flags.StringVar(&opts.downloadPath, "download-path", "", "download directory")
	flags.StringVar(&opts.filenameTemplate, "filename-template", "", "filename template")
	flags.BoolVar(&opts.jsonOut, "json", false, "print JSON")
	a.bindProxyFlags(cmd, &opts.proxyOptions)
}

func (a app) bindProxyFlags(cmd *cobra.Command, opts *proxyOptions) {
	flags := cmd.Flags()
	flags.StringVar(&opts.proxy, "proxy", "", "HTTP(S) proxy URL for this command")
	flags.BoolVar(&opts.noProxy, "no-proxy", false, "clear HTTP(S) proxy for this command")
}

func (a app) clientRequest(cmd *cobra.Command, opts commandOptions, needsAuth bool) (application.ClientRequest, error) {
	userID := int64(0)
	if cmd.Flags().Changed("uid") && cmd.Flags().Changed("profile") {
		return application.ClientRequest{}, fmt.Errorf("use either --uid or deprecated --profile, not both")
	}
	rawUID := opts.uid
	if rawUID == "" {
		rawUID = opts.profile
	}
	if rawUID != "" {
		parsed, err := application.ParseUID(rawUID)
		if err != nil {
			return application.ClientRequest{}, err
		}
		userID = parsed
	}
	req := application.ClientRequest{
		UserID:       userID,
		RefreshToken: opts.refreshToken,
		NeedsAuth:    needsAuth,
	}
	proxyOverride, err := proxyOverrideFromFlags(cmd, opts.proxyOptions)
	if err != nil {
		return application.ClientRequest{}, err
	}
	req.HTTPSProxyOverride = proxyOverride
	if cmd.Flags().Changed("download-path") {
		req.DownloadPathOverride = &opts.downloadPath
	}
	if cmd.Flags().Changed("filename-template") {
		req.FilenameTemplateOverride = &opts.filenameTemplate
	}
	if cmd.Flags().Changed("json") {
		req.JSONOverride = &opts.jsonOut
	}
	return req, nil
}

func proxyOverrideFromFlags(cmd *cobra.Command, opts proxyOptions) (*string, error) {
	proxyChanged := cmd.Flags().Changed("proxy")
	noProxyChanged := cmd.Flags().Changed("no-proxy")
	if proxyChanged && noProxyChanged {
		return nil, fmt.Errorf("use either --proxy or --no-proxy, not both")
	}
	if noProxyChanged && opts.noProxy {
		empty := ""
		return &empty, nil
	}
	if proxyChanged {
		return &opts.proxy, nil
	}
	return nil, nil
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
