// Package whoami 提供 pixiv whoami 命令，显示本地当前选中的 Pixiv 账号。
//
// 命令只读取本地账号库，不创建 Pixiv client、不轮换 token，也不触发自动更新检查。
package whoami

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/display"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/account"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/spf13/cobra"
)

// New 构造 `pixiv whoami`；current 是执行时才调用的只读账号端口。
func New(env environment.Env, current func(context.Context) (*account.AccountSummary, error)) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Show the current local Pixiv account",
		Args:  env.ExactArgs(0, "pixiv whoami [--json]"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			account, err := current(cmd.Context())
			if err != nil {
				if sdk.IsReason(err, sdk.Unauthorized) {
					return errors.New(`not logged in; run "pixiv auth login"`)
				}
				return err
			}
			if jsonOut {
				return env.PrintJSON(auth.AccountJSON(*account))
			}
			return printText(env.Out, account)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print JSON")
	env.BindNoInput(cmd)
	environment.SetOffline(cmd)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func printText(out io.Writer, account *account.AccountSummary) error {
	line := display.Identity(account.Username, account.UserID)
	if account.Username != "" && account.Premium != nil && *account.Premium {
		line += " · Premium"
	}
	_, err := fmt.Fprintln(out, line)
	return err
}
