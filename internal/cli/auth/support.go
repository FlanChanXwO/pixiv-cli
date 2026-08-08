package auth

import (
	"errors"

	"github.com/spf13/cobra"
)

func (a controller) canPrompt() bool { return a.Host.CanPrompt() }

func (a controller) promptInput(message, defaultValue string) (string, error) {
	return a.Host.PromptInput(message, defaultValue)
}

func (a controller) promptSecret(message string) (string, error) {
	return a.Host.PromptSecret(message)
}

func (a controller) promptSelect(message string, options []string) (string, error) {
	return a.Host.PromptSelect(message, options)
}

func (a controller) promptConfirm(message string, defaultValue bool) (bool, error) {
	return a.Host.PromptConfirm(message, defaultValue)
}

func (a controller) requireExactArgs(count int, usage string) cobra.PositionalArgs {
	if a.Host != nil {
		return a.RequireExactArgs(count, usage)
	}
	return func(_ *cobra.Command, args []string) error {
		if len(args) != count {
			return errors.New("usage: " + usage)
		}
		return nil
	}
}

func (a controller) services() runtimeServices {
	return runtimeServices{Account: a.AccountService(), Login: a.LoginService()}
}

func (a controller) printJSON(value any) error {
	return a.Host.PrintJSON(value)
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
