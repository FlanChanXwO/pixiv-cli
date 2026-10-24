// Package auth owns local FANBOX authentication commands and secret prompts.
package auth

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/fanboxcmd"
	"github.com/FlanChanXwO/pixiv-cli/internal/fanboxaccount"
	"github.com/spf13/cobra"
)

type accountStore interface {
	ImportSession(context.Context, string, bool, *string) (fanboxaccount.AccountSummary, error)
	ListAccounts(context.Context) ([]fanboxaccount.AccountSummary, error)
	Status(context.Context) (*fanboxaccount.AccountSummary, error)
	UseAccount(context.Context, int64) error
	UseAuto() error
	RemoveAccount(context.Context, int64) error
}

type commandRunner struct {
	env      command.Env
	accounts func() (*fanboxaccount.Service, error)
	browser  fanboxcmd.BrowserProvider
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

// New builds the actual `pixiv fanbox auth` command group.
func New(env command.Env, accounts func() (*fanboxaccount.Service, error), browser fanboxcmd.BrowserProvider) *cobra.Command {
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
	command.Bind(cmd, command.Execution{})
	for _, child := range cmd.Commands() {
		command.Bind(child, command.FanboxAuth())
	}
	return cmd
}
