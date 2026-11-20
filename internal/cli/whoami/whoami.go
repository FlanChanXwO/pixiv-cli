// Package whoami 提供 pixiv whoami 命令，显示本地当前选中的 Pixiv 账号。
//
// 命令只读取本地账号库，不创建 Pixiv client、不轮换 token，也不触发自动更新检查。
package whoami

import (
	"context"
	"fmt"
	"io"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/display"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/spf13/cobra"
)

// New 构造 `pixiv whoami`；current 是执行时才调用的只读账号端口。
func New(env environment.Env, current func(context.Context) (*auth.AccountSummary, error)) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "help.show_the_current_local_pixiv_account",
		Args:  env.ExactArgs(0, "pixiv whoami [--json]"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			account, err := current(cmd.Context())
			if err != nil {
				if sdk.ReasonOf(err) == sdk.Unauthorized {
					return i18n.Errorf("error.not_logged_in_run_pixiv_auth_login")
				}
				return err
			}
			if jsonOut {
				return env.PrintJSON(auth.AccountJSON(*account))
			}
			return printText(env.Out, env.Language, account)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "flag.print_json")
	env.BindNoInput(cmd)
	environment.SetOffline(cmd)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func printText(out io.Writer, language i18n.Locale, account *auth.AccountSummary) error {
	line := display.Identity(account.Username, account.UserID)
	if account.Username != "" && account.Premium != nil && *account.Premium {
		line += " · " + language.Text("account.premium")
	}
	_, err := fmt.Fprintln(out, line)
	return err
}
