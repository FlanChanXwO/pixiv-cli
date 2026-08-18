// Package fanbox 注册 FANBOX 命令组及其产品内 owner。
package fanbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	requirements "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	configapp "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	fanboxapp "github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox"
	fanboxaccount "github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/account"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/spf13/cobra"
)

// CommandSet 是 composition root 传入的真实 leaf owner 集合。group 只负责
// 固定 FANBOX help/order，不反向导入 leaf，避免产品 group 成为依赖桥。
type CommandSet struct {
	Auth     *cobra.Command
	Posts    []*cobra.Command
	Download *cobra.Command
	MCP      *cobra.Command
}

// FANBOX 命令共享的依赖能力与防御性校验。
type BrowserProvider interface {
	ReadSession(context.Context, string, string) (string, error)
}

// Data contains only the capabilities FANBOX command owners require during a
// single CLI execution. Resource construction remains in the root run graph.
type Data struct {
	Reader io.Reader
	Writer io.Writer

	WrapUsage      func(error) error
	ServiceFactory func() (*fanboxapp.Facade, error)
	// AccountServiceFactory supplies auth-only account management. The product
	// Facade owns client lifecycle; auth commands do not need to open a client
	// through that Facade.
	AccountServiceFactory func() (*fanboxaccount.Service, error)
	Browser               BrowserProvider
	Runtime               func() (configapp.RuntimeConfig, error)
	CanPromptFn           func() bool
	PromptSecretFn        func(string) (string, error)
	PromptConfirmFn       func(string, bool) (bool, error)

	// RunMCPServer 启动 FANBOX MCP stdio server。composition root 注入真实
	// mcpserver wiring；命令 owner 不直接依赖 protocol/infrastructure。
	RunMCPServer func(*cobra.Command, *fanboxapp.Facade, *string) error
}

func (d Data) UsageError(err error) error {
	if err == nil {
		return nil
	}
	if d.WrapUsage == nil {
		panic("fanbox command usage error wrapper is not configured")
	}
	return d.WrapUsage(err)
}

// RequireExactArgs、RequireMinArgs 与 RequireMaxArgs 返回普通错误：位置参数数量
// 错误按命令失败报告为 exit code 1；usage exit code 2 只保留给 unknown option
// 与显式输入契约违规。
func (d Data) RequireExactArgs(count int, usage string) cobra.PositionalArgs {
	return requireArgs(usage, func(length int) bool { return length == count })
}

func (d Data) RequireMinArgs(count int, usage string) cobra.PositionalArgs {
	return requireArgs(usage, func(length int) bool { return length >= count })
}

func (d Data) RequireMaxArgs(count int, usage string) cobra.PositionalArgs {
	return requireArgs(usage, func(length int) bool { return length <= count })
}

func requireArgs(usage string, valid func(int) bool) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if !valid(len(args)) {
			return fmt.Errorf("usage: %s", usage)
		}
		return nil
	}
}

func (d Data) FanboxService() (*fanboxapp.Facade, error) {
	if d.ServiceFactory == nil {
		panic("fanbox service factory is not configured")
	}
	return d.ServiceFactory()
}

// AccountService returns the auth-only account leaf and turns an unavailable
// local account store into the command-level error auth already reports.
func (d Data) AccountService() (*fanboxaccount.Service, error) {
	if d.AccountServiceFactory == nil {
		return nil, errors.New("fanbox is not available: cannot open the local account store")
	}
	service, err := d.AccountServiceFactory()
	if err != nil {
		return nil, err
	}
	if service == nil {
		return nil, errors.New("fanbox is not available: cannot open the local account store")
	}
	return service, nil
}

// Client opens one FANBOX client lease for the current command, applying the
// group's native proxy override. The caller owns closing the returned lease.
func (d Data) Client(cmd *cobra.Command) (*lifecycle.Lease[*fanbox.Client], error) {
	service, err := d.FanboxService()
	if err != nil {
		return nil, err
	}
	proxyOverride, err := ProxyOverride(cmd)
	if err != nil {
		return nil, err
	}
	return service.Open(cmd.Context(), fanboxapp.OpenRequest{ProxyOverride: proxyOverride})
}

// UseClient keeps the explicit lease boundary local to one command callback
// and preserves both callback and close errors.
func (d Data) UseClient(cmd *cobra.Command, use func(*fanbox.Client) error) error {
	lease, err := d.Client(cmd)
	if err != nil {
		return err
	}
	if lease == nil {
		return errors.New("fanbox client lease is nil")
	}
	useErr := use(lease.Value())
	return errors.Join(useErr, lease.Close())
}

// ProxyOverride reads the FANBOX group's persistent --proxy/--no-proxy flags.
func ProxyOverride(cmd *cobra.Command) (*string, error) {
	if cmd == nil {
		return nil, nil
	}
	proxyFlag := cmd.Flags().Lookup("proxy")
	noProxyFlag := cmd.Flags().Lookup("no-proxy")
	proxyChanged := cmd.Flags().Changed("proxy")
	noProxyChanged := cmd.Flags().Changed("no-proxy")
	if proxyChanged && noProxyChanged {
		return nil, errors.New("use either --proxy or --no-proxy, not both")
	}
	if noProxyChanged && noProxyFlag != nil && noProxyFlag.Value.String() == "true" {
		empty := ""
		return &empty, nil
	}
	if proxyChanged && proxyFlag != nil {
		value := proxyFlag.Value.String()
		return &value, nil
	}
	return nil, nil
}

func (d Data) FanboxBrowserProvider() BrowserProvider {
	if d.Browser == nil {
		panic("fanbox browser provider is not configured")
	}
	return d.Browser
}

func (d Data) FanboxRuntimeConfig() (configapp.RuntimeConfig, error) {
	if d.Runtime == nil {
		panic("fanbox runtime config factory is not configured")
	}
	return d.Runtime()
}

func (d Data) CanPrompt() bool {
	if d.CanPromptFn == nil {
		panic("fanbox prompt capability is not configured")
	}
	return d.CanPromptFn()
}

func (d Data) PromptSecret(message string) (string, error) {
	if d.PromptSecretFn == nil {
		panic("fanbox secret prompt is not configured")
	}
	return d.PromptSecretFn(message)
}

func (d Data) PromptConfirm(message string, defaultValue bool) (bool, error) {
	if d.PromptConfirmFn == nil {
		panic("fanbox confirmation prompt is not configured")
	}
	return d.PromptConfirmFn(message, defaultValue)
}

func (d Data) PrintJSON(value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = io.WriteString(d.Writer, string(body)+"\n")
	return err
}

func (d Data) BindTextValue(cmd *cobra.Command, minArgs, maxArgs, fillPosition int) {
	pipeline.Bind(cmd, pipeline.InputSpec{
		Codec:        pipeline.TextValue,
		MinArgs:      minArgs,
		MaxArgs:      maxArgs,
		FillPosition: fillPosition,
		Reader:       d.Reader,
		UsageError:   d.UsageError,
	})
}

func (d Data) BindNoInput(cmd *cobra.Command) {
	pipeline.Bind(cmd, pipeline.InputSpec{
		Codec:      pipeline.NoInput,
		MinArgs:    0,
		MaxArgs:    0,
		Reader:     d.Reader,
		UsageError: d.UsageError,
	})
}

// New 构造 FANBOX 产品命令组；资源与 MCP stdio 生命周期由 root host 注入。
func New(data Data, children CommandSet) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fanbox",
		Short: "Browse and download Pixiv FANBOX content",
		Args:  data.RequireExactArgs(0, "pixiv fanbox <command>"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	flags := cmd.PersistentFlags()
	flags.String("proxy", "", "native FANBOX proxy URL (HTTP or HTTPS CONNECT)")
	flags.Bool("no-proxy", false, "use a direct native FANBOX connection for this command")
	childrenList := make([]*cobra.Command, 0, 3+len(children.Posts))
	childrenList = append(childrenList, children.Auth)
	childrenList = append(childrenList, children.Posts...)
	childrenList = append(childrenList, children.Download, children.MCP)
	for _, child := range childrenList {
		if child != nil {
			cmd.AddCommand(child)
		}
	}
	data.BindNoInput(cmd)
	requirements.Bind(cmd, requirements.Execution{})
	return cmd
}
