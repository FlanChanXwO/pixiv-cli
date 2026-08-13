// Package deps defines the narrow runtime inputs shared by FANBOX commands.
package deps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	fanboxapp "github.com/FlanChanXwO/pixiv-cli/internal/account/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	configapp "github.com/FlanChanXwO/pixiv-cli/internal/storage/config"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/spf13/cobra"
)

// BrowserProvider reads a validated FANBOX browser session without exposing
// browser database paths or the session value to command output.
type BrowserProvider interface {
	ReadSession(context.Context, string, string) (string, error)
}

// Data contains only the capabilities FANBOX command owners require during a
// single CLI execution. Resource construction remains in the root run graph.
type Data struct {
	Reader io.Reader
	Writer io.Writer

	WrapUsage       func(error) error
	ServiceFactory  func() (*fanboxapp.Service, error)
	Browser         BrowserProvider
	Runtime         func() (configapp.RuntimeConfig, error)
	CanPromptFn     func() bool
	PromptSecretFn  func(string) (string, error)
	PromptConfirmFn func(string, bool) (bool, error)

	// RunMCPServer 启动 FANBOX MCP stdio server。composition root 注入真实
	// mcpserver wiring；命令 owner 不直接依赖 protocol/infrastructure。
	RunMCPServer func(*cobra.Command, *fanboxapp.Service, *string) error
}

func (d Data) Input() io.Reader  { return d.Reader }
func (d Data) Output() io.Writer { return d.Writer }

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
	return func(_ *cobra.Command, args []string) error {
		if len(args) != count {
			return fmt.Errorf("usage: %s", usage)
		}
		return nil
	}
}

func (d Data) RequireMinArgs(count int, usage string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) < count {
			return fmt.Errorf("usage: %s", usage)
		}
		return nil
	}
}

func (d Data) RequireMaxArgs(count int, usage string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) > count {
			return fmt.Errorf("usage: %s", usage)
		}
		return nil
	}
}

func (d Data) FanboxService() (*fanboxapp.Service, error) {
	if d.ServiceFactory == nil {
		panic("fanbox service factory is not configured")
	}
	return d.ServiceFactory()
}

// Service returns the FANBOX service and turns an unavailable local account
// store into the command-level error every FANBOX command already reports.
func (d Data) Service() (*fanboxapp.Service, error) {
	service, err := d.FanboxService()
	if err != nil {
		return nil, err
	}
	if service == nil {
		return nil, errors.New("fanbox is not available: cannot open the local account store")
	}
	return service, nil
}

// Client opens one FANBOX client for the current command, applying the group's
// native proxy override. Callers own closing idle connections.
func (d Data) Client(cmd *cobra.Command) (*fanbox.Client, error) {
	service, err := d.Service()
	if err != nil {
		return nil, err
	}
	proxyOverride, err := ProxyOverride(cmd)
	if err != nil {
		return nil, err
	}
	return service.OpenClientWithProxy(cmd.Context(), proxyOverride)
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
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var out bytes.Buffer
	if err := json.Indent(&out, body, "", "  "); err != nil {
		return err
	}
	_, err = io.WriteString(d.Writer, out.String()+"\n")
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
