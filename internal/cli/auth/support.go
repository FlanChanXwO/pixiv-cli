package auth

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/input"
	"github.com/spf13/cobra"
)

func (a controller) canPrompt() bool { return a.env.CanPrompt() }

func (a controller) promptInput(message, defaultValue string) (string, error) {
	return a.env.PromptInput(message, defaultValue)
}

func (a controller) promptSecret(message string) (string, error) {
	return a.env.PromptSecret(message)
}

func (a controller) promptSelect(message string, options []string) (string, error) {
	return a.env.PromptSelect(message, options)
}

func (a controller) promptConfirm(message string, defaultValue bool) (bool, error) {
	return a.env.PromptConfirm(message, defaultValue)
}

// requireExactArgs 与 requireMaxArgs 返回普通错误：位置参数数量错误按命令失败
// 报告为 exit code 1，只有 unknown option 与显式输入契约违规才使用 usage exit
// code 2。
func (a controller) requireExactArgs(count int, usage string) cobra.PositionalArgs {
	return a.env.ExactArgs(count, usage)
}

func (a controller) requireMaxArgs(count int, usage string) cobra.PositionalArgs {
	return a.env.MaxArgs(count, usage)
}

func (a controller) usageError(err error) error { return a.env.UsageError(err) }

func (a controller) bindTextValue(cmd *cobra.Command, minArgs, maxArgs, fillPosition int, enabled func(*cobra.Command, []string) bool) {
	input.Bind(cmd, input.InputSpec{
		Codec:        input.TextValue,
		MinArgs:      minArgs,
		MaxArgs:      maxArgs,
		FillPosition: fillPosition,
		Reader:       a.in,
		UsageError:   a.usageError,
		Enabled:      enabled,
	})
}

func (a controller) bindNoInput(cmd *cobra.Command) {
	input.Bind(cmd, input.InputSpec{
		Codec:      input.NoInput,
		MinArgs:    0,
		MaxArgs:    0,
		Reader:     a.in,
		UsageError: a.usageError,
	})
}

func (a controller) services() runtimeServices {
	services := runtimeServices{}
	if a.support.Account != nil {
		services.Account, services.err = a.support.Account()
	}
	if services.err == nil && a.support.Login != nil {
		services.Login, services.err = a.support.Login()
	}
	services.LoadRuntime = a.support.LoadRuntime
	return services
}

func (s runtimeServices) require() error {
	return s.err
}

func (a controller) printJSON(value any) error { return a.env.PrintJSON(value) }

func (a controller) bindProxyFlags(cmd *cobra.Command, options *command.ProxyOptions) {
	a.env.BindProxyFlags(cmd, options)
}

func proxyOverrideFromFlags(cmd *cobra.Command, options command.ProxyOptions) (*string, error) {
	return command.Env{}.ProxyOverride(cmd, options)
}
