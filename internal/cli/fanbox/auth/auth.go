// Package auth 提供 pixiv fanbox auth 命令组：FANBOX session 的导入、列表、选择、移除与状态检查。
//
// 包的组成：[New] 组装命令树，[BrowserProvider] 抽象浏览器凭据读取。
package auth

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/fanbox/account"
	"github.com/spf13/cobra"
)

type accountStore interface {
	ImportSession(context.Context, string, bool, *string) (account.AccountSummary, error)
	ListAccounts(context.Context) ([]account.AccountSummary, error)
	Status(context.Context) (*account.AccountSummary, error)
	UseAccount(context.Context, int64) error
	UseAuto() error
	RemoveAccount(context.Context, int64) error
}

type commandRunner struct {
	env      environment.Env
	accounts func() (*account.Service, error)
	browser  BrowserProvider
}

type importOptions struct {
	fromBrowser string
	profile     string
	setDefault  bool
	jsonOut     bool
}

type useOptions struct {
	auto    bool
	jsonOut bool
}

type removeOptions struct {
	jsonOut bool
	yes     bool
}

// BrowserProvider 从浏览器 profile 读取一个 FANBOXSESSID；reader 负责选择 profile。
type BrowserProvider interface {
	ReadSession(context.Context, string, string) (string, error)
}

// New 构造 `pixiv fanbox auth` 命令组；account 服务延迟打开，构造命令树不访问账号库。
func New(env environment.Env, accounts func() (*account.Service, error), browser BrowserProvider) *cobra.Command {
	a := commandRunner{env: env, accounts: accounts, browser: browser}
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage local FANBOX authentication",
		Args:  env.ExactArgs(0, "pixiv fanbox auth <command>"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		a.newImportCommand(),
		a.newListCommand(),
		a.newUseCommand(),
		a.newRemoveCommand(),
		a.newStatusCommand(),
	)
	env.BindNoInput(cmd)
	environment.Bind(cmd, environment.Execution{})
	for _, child := range cmd.Commands() {
		environment.Bind(child, environment.FanboxAuth())
	}
	return cmd
}
