package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	"github.com/spf13/cobra"
)

func (a controller) canPrompt() bool {
	if a.deps.CanPrompt == nil {
		panic("pixiv auth prompt capability is not configured")
	}
	return a.deps.CanPrompt()
}

func (a controller) promptInput(message, defaultValue string) (string, error) {
	if a.deps.PromptInput == nil {
		panic("pixiv auth input prompt is not configured")
	}
	return a.deps.PromptInput(message, defaultValue)
}

func (a controller) promptSecret(message string) (string, error) {
	if a.deps.PromptSecret == nil {
		panic("pixiv auth secret prompt is not configured")
	}
	return a.deps.PromptSecret(message)
}

func (a controller) promptSelect(message string, options []string) (string, error) {
	if a.deps.PromptSelect == nil {
		panic("pixiv auth select prompt is not configured")
	}
	return a.deps.PromptSelect(message, options)
}

func (a controller) promptConfirm(message string, defaultValue bool) (bool, error) {
	if a.deps.PromptConfirm == nil {
		panic("pixiv auth confirmation prompt is not configured")
	}
	return a.deps.PromptConfirm(message, defaultValue)
}

// requireExactArgs 与 requireMaxArgs 返回普通错误：位置参数数量错误按命令失败
// 报告为 exit code 1，只有 unknown option 与显式输入契约违规才使用 usage exit
// code 2。
func (a controller) requireExactArgs(count int, usage string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) != count {
			return errors.New("usage: " + usage)
		}
		return nil
	}
}

func (a controller) requireMaxArgs(count int, usage string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) > count {
			return errors.New("usage: " + usage)
		}
		return nil
	}
}

func (a controller) usageError(err error) error {
	if err == nil || a.deps.UsageError == nil {
		return err
	}
	return a.deps.UsageError(err)
}

func (a controller) bindTextValue(cmd *cobra.Command, minArgs, maxArgs, fillPosition int, enabled func(*cobra.Command, []string) bool) {
	pipeline.Bind(cmd, pipeline.InputSpec{
		Codec:        pipeline.TextValue,
		MinArgs:      minArgs,
		MaxArgs:      maxArgs,
		FillPosition: fillPosition,
		Reader:       a.in,
		UsageError:   a.usageError,
		Enabled:      enabled,
	})
}

func (a controller) bindNoInput(cmd *cobra.Command) {
	pipeline.Bind(cmd, pipeline.InputSpec{
		Codec:      pipeline.NoInput,
		MinArgs:    0,
		MaxArgs:    0,
		Reader:     a.in,
		UsageError: a.usageError,
	})
}

func (a controller) services() runtimeServices {
	services := runtimeServices{}
	if a.deps.Account != nil {
		services.Account = a.deps.Account()
	}
	if a.deps.Login != nil {
		services.Login = a.deps.Login()
	}
	return services
}

func (a controller) printJSON(value any) error {
	body, err := json.Marshal(value)
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

func (a controller) bindProxyFlags(cmd *cobra.Command, options *proxyOptions) {
	flags := cmd.Flags()
	flags.StringVar(&options.proxy, "proxy", "", "proxy URL (http, https, socks5, or socks5h) for this command")
	flags.BoolVar(&options.noProxy, "no-proxy", false, "clear the configured proxy for this command")
}

func proxyOverrideFromFlags(cmd *cobra.Command, options proxyOptions) (*string, error) {
	proxyChanged := cmd.Flags().Changed("proxy")
	noProxyChanged := cmd.Flags().Changed("no-proxy")
	if proxyChanged && noProxyChanged {
		return nil, errors.New("use either --proxy or --no-proxy, not both")
	}
	if noProxyChanged && options.noProxy {
		empty := ""
		return &empty, nil
	}
	if proxyChanged {
		return &options.proxy, nil
	}
	return nil, nil
}

func textBool(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
