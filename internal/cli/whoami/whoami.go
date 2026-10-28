// Package whoami 提供 pixiv whoami 命令，显示本地当前选中的 Pixiv 账号。
//
// 命令只读取本地账号库，不创建 Pixiv client、不轮换 token，也不触发自动更新检查。
package whoami

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"

	authcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	pixivaccount "github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount"
	"github.com/FlanChanXwO/pixiv-cli/internal/plaintext"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/spf13/cobra"
)

// New 构造 `pixiv whoami`；current 是执行时才调用的只读账号端口。
func New(env command.Env, current func(context.Context) (*pixivaccount.AccountSummary, error)) *cobra.Command {
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
				return env.PrintJSON(authcommands.AccountJSON(*account))
			}
			return printText(env.Out, account)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print JSON")
	env.BindNoInput(cmd)
	command.SetOffline(cmd)
	command.Bind(cmd, command.Normal())
	return cmd
}

func printText(out io.Writer, account *pixivaccount.AccountSummary) error {
	line := strconv.FormatInt(account.UserID, 10)
	if account.Username != "" {
		line = plaintext.SafeLine(account.Username) + " (" + line + ")"
		if account.Premium != nil && *account.Premium {
			line += " · Premium"
		}
	}
	_, err := fmt.Fprintln(out, line)
	return err
}
