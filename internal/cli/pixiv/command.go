// Package pixiv 注册 Pixiv 数据与关系命令。
package pixiv

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pixiv"
	clioutput "github.com/FlanChanXwO/pixiv-cli/internal/cli/output"
	"github.com/spf13/cobra"
)

// Host 是 Pixiv 命令所需的最小 composition seam。命令包只消费应用服务和
// CLI 输出/参数端口，不反向导入 internal/cli 或 bootstrap 的具体实现。
type Host interface {
	Input() io.Reader
	Output() io.Writer
	ErrorOutput() io.Writer
	PrintJSON(any) error
	UsageError(error) error
	RequireExactArgs(int, string) cobra.PositionalArgs
	RequireMinArgs(int, string) cobra.PositionalArgs
	RequireMaxArgs(int, string) cobra.PositionalArgs
	SDKService() pixivapp.SDKService
}

type controller struct {
	host   Host
	in     io.Reader
	out    io.Writer
	errOut io.Writer
}

type runtimeServices struct {
	SDK pixivapp.SDKService
}

type commandOptions struct {
	proxyOptions
	jsonOut bool
}

type proxyOptions struct {
	proxy   string
	noProxy bool
}

func newController(host Host) controller {
	a := controller{host: host}
	if host != nil {
		a.in = host.Input()
		a.out = host.Output()
		a.errOut = host.ErrorOutput()
	}
	return a
}

// NewCommands 返回 Pixiv 数据命令。它们直接挂到根命令，因此不会再产生
// "pixiv pixiv" 的重复前缀。
func NewCommands(host Host) []*cobra.Command {
	a := newController(host)
	return []*cobra.Command{
		a.newSearchCommand(),
		a.newNovelCommand(),
		a.newDetailCommand(),
		a.newRankingCommand(),
		a.newRecommendedCommand(),
		a.newTimelineCommand(),
		a.newMyPixivCommand(),
		a.newUserCommand(),
		a.newBookmarkCommand(),
		a.newFollowCommand(),
	}
}

func Register(root *cobra.Command, host Host) {
	root.AddCommand(NewCommands(host)...)
}

func (a controller) services() runtimeServices {
	if a.host == nil {
		return runtimeServices{}
	}
	return runtimeServices{SDK: a.host.SDKService()}
}

func (a controller) usageError(err error) error {
	if err == nil {
		return nil
	}
	if a.host == nil {
		return err
	}
	return a.host.UsageError(err)
}

func (a controller) requireExactArgs(count int, usage string) cobra.PositionalArgs {
	if a.host != nil {
		return a.host.RequireExactArgs(count, usage)
	}
	return localExactArgs(count, usage)
}

func (a controller) requireMinArgs(count int, usage string) cobra.PositionalArgs {
	if a.host != nil {
		return a.host.RequireMinArgs(count, usage)
	}
	return localMinArgs(count, usage)
}

func (a controller) requireMaxArgs(count int, usage string) cobra.PositionalArgs {
	if a.host != nil {
		return a.host.RequireMaxArgs(count, usage)
	}
	return localMaxArgs(count, usage)
}

func localExactArgs(count int, usage string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) != count {
			return fmt.Errorf("usage: %s", usage)
		}
		return nil
	}
}

func localMinArgs(count int, usage string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) < count {
			return fmt.Errorf("usage: %s", usage)
		}
		return nil
	}
}

func localMaxArgs(count int, usage string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) > count {
			return fmt.Errorf("usage: %s", usage)
		}
		return nil
	}
}

func (a controller) bindCommonFlags(cmd *cobra.Command, opts *commandOptions) {
	cmd.Flags().BoolVar(&opts.jsonOut, "json", false, "print JSON")
	a.bindProxyFlags(cmd, &opts.proxyOptions)
}

// bindActionFlags 只注册动作所需的传输参数；动作结果走 NDJSON 管道，不接受
// 与逐条诊断契约冲突的 --json 选项。
func (a controller) bindActionFlags(cmd *cobra.Command, opts *commandOptions) {
	a.bindProxyFlags(cmd, &opts.proxyOptions)
}

func (a controller) bindProxyFlags(cmd *cobra.Command, opts *proxyOptions) {
	flags := cmd.Flags()
	flags.StringVar(&opts.proxy, "proxy", "", "proxy URL (http, https, socks5, or socks5h) for this command")
	flags.BoolVar(&opts.noProxy, "no-proxy", false, "clear the configured proxy for this command")
}

func (a controller) clientRequest(cmd *cobra.Command, opts commandOptions, needsAuth bool) (application.ClientRequest, error) {
	req := application.ClientRequest{NeedsAuth: needsAuth}
	proxyOverride, err := proxyOverrideFromFlags(cmd, opts.proxyOptions)
	if err != nil {
		return application.ClientRequest{}, err
	}
	req.HTTPSProxyOverride = proxyOverride
	if flag := cmd.Flags().Lookup("sleep-request"); flag != nil && flag.Changed {
		value, err := time.ParseDuration(flag.Value.String())
		if err != nil {
			return application.ClientRequest{}, fmt.Errorf("invalid --sleep-request: %w", err)
		}
		if value < 0 {
			return application.ClientRequest{}, errors.New("--sleep-request must not be negative")
		}
		req.RequestIntervalOverride = &value
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
		return nil, errors.New("use either --proxy or --no-proxy, not both")
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

func (a controller) printJSON(value any) error {
	if a.host != nil {
		return a.host.PrintJSON(value)
	}
	body, err := clioutput.MarshalJSONValue(value, false)
	if err != nil {
		return err
	}
	var out bytes.Buffer
	if err := json.Indent(&out, body, "", "  "); err != nil {
		return err
	}
	_, err = io.WriteString(a.out, out.String()+"\n")
	return err
}
