package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-mcp-server/internal/download"
	"github.com/FlanChanXwO/pixiv-mcp-server/internal/mcpserver"
	"github.com/FlanChanXwO/pixiv-mcp-server/internal/pixiv"
	"github.com/FlanChanXwO/pixiv-mcp-server/pkg/config"
	"github.com/FlanChanXwO/pixiv-mcp-server/pkg/pixivutil"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type app struct {
	args   []string
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

var runMCPServer = runMCP

func Run(args []string, in io.Reader, out io.Writer, errOut io.Writer) int {
	if len(args) == 0 {
		args = []string{"pixiv"}
	}
	a := app{args: args, in: in, out: out, errOut: errOut}
	if len(args) == 1 {
		a.printHelp(out)
		return 0
	}
	cmd := args[1]
	if cmd == "-h" || cmd == "--help" || cmd == "help" {
		a.printHelp(out)
		return 0
	}
	if cmd == "mcp" {
		if len(args) > 2 && (args[2] == "-h" || args[2] == "--help") {
			fmt.Fprintln(out, "Usage: pixiv mcp\n\nRun the Pixiv MCP stdio server.")
			return 0
		}
		return a.exit(runMCPServer(context.Background(), errOut))
	}

	var err error
	switch cmd {
	case "account":
		err = a.runAccount(args[2:])
	case "search":
		err = a.runSearch(args[2:])
	case "detail":
		err = a.runDetail(args[2:])
	case "ranking":
		err = a.runRanking(args[2:])
	case "recommended":
		err = a.runRecommended(args[2:])
	case "download":
		err = a.runDownload(args[2:])
	default:
		err = fmt.Errorf("unknown command %q", cmd)
	}
	return a.exit(err)
}

func (a app) exit(err error) int {
	if err == nil {
		return 0
	}
	fmt.Fprintln(a.errOut, "error:", err)
	return 1
}

func (a app) printHelp(w io.Writer) {
	fmt.Fprint(w, `Usage: pixiv <command> [options]

Commands:
  account      Manage local Pixiv refresh-token profiles
  search       Search illustrations
  detail       Show one illustration
  ranking      Show illustration ranking
  recommended  Show personalized recommendations
  download     Download illustrations
  mcp          Run the MCP stdio server

Use "pixiv <command> --help" for command options.
`)
}

func runMCP(ctx context.Context, errOut io.Writer) error {
	logger := slog.New(slog.NewTextHandler(errOut, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := config.LoadFromEnv()
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

func newPixivClient(cfg config.Config) (*pixiv.Client, error) {
	httpClient := &http.Client{Transport: http.DefaultTransport}
	if cfg.HTTPSProxy != "" {
		proxyURL, err := url.Parse(cfg.HTTPSProxy)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL %q: %w", cfg.HTTPSProxy, err)
		}
		httpClient.Transport = &http.Transport{Proxy: http.ProxyURL(proxyURL)}
	}
	return pixiv.New(cfg.RefreshToken, pixiv.WithHTTPClient(httpClient)), nil
}

func (a app) flagSet(name string, opts *commandOptions) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	fs.StringVar(&opts.profile, "profile", "", "profile name")
	fs.StringVar(&opts.refreshToken, "refresh-token", "", "Pixiv refresh token or cookie with refresh_token")
	fs.StringVar(&opts.downloadPath, "download-path", "", "download directory")
	fs.StringVar(&opts.filenameTemplate, "filename-template", "", "filename template")
	fs.BoolVar(&opts.jsonOut, "json", false, "print JSON")
	return fs
}

func (a app) clientAndConfig(opts commandOptions, needsAuth bool) (*pixiv.Client, config.Config, error) {
	cfg := config.LoadFromEnv()
	path, err := configPath()
	if err != nil {
		return nil, cfg, err
	}
	store, err := loadAccountStore(path)
	if err != nil {
		return nil, cfg, err
	}
	if opts.profile != "" || cfg.RefreshToken == "" {
		if name, acct, ok := selectAccount(store, opts.profile); ok {
			cfg.RefreshToken = acct.RefreshToken
			_ = name
		} else if opts.profile != "" {
			return nil, cfg, fmt.Errorf("profile %q not found", opts.profile)
		}
	}
	if opts.refreshToken != "" {
		token, parsedCookie := pixivutil.ParseRefreshTokenInput(opts.refreshToken)
		if token == "" {
			if parsedCookie {
				return nil, cfg, errors.New("refresh-token cookie does not contain refresh_token")
			}
			return nil, cfg, errors.New("refresh-token cannot be empty")
		}
		cfg.RefreshToken = token
	}
	if opts.downloadPath != "" {
		cfg.DownloadPath = opts.downloadPath
	}
	if opts.filenameTemplate != "" {
		cfg.FilenameTemplate = opts.filenameTemplate
	}
	if needsAuth && cfg.RefreshToken == "" {
		return nil, cfg, errors.New("missing refresh token; use PIXIV_REFRESH_TOKEN or pixiv account add")
	}
	client, err := newPixivClient(cfg)
	if err != nil {
		return nil, cfg, err
	}
	if needsAuth {
		if err := client.Refresh(context.Background()); err != nil {
			return nil, cfg, err
		}
	}
	return client, cfg, nil
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
