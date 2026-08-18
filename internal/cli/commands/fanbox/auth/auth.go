// Package auth owns local FANBOX authentication commands and secret prompts.
package auth

import (
	"io"

	requirements "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands"
	deps "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/fanbox"
	"github.com/spf13/cobra"
)

type command struct {
	data deps.Data
	in   io.Reader
	out  io.Writer
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
func New(data deps.Data) *cobra.Command {
	a := command{data: data, in: data.Reader, out: data.Writer}
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage local FANBOX authentication",
		Args:  data.RequireExactArgs(0, "pixiv fanbox auth <command>"),
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
	data.BindNoInput(cmd)
	requirements.Bind(cmd, requirements.Execution{})
	for _, child := range cmd.Commands() {
		requirements.Bind(child, requirements.FanboxAuth())
	}
	return cmd
}
