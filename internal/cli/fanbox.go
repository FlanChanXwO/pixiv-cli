// FANBOX command group. Leaf owners construct their own children; this file
// only fixes the group's help text, flags, and order.

package cli

import (
	commands "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/input"
	"github.com/spf13/cobra"
)

// newFanboxCommand assembles the FANBOX product group.
func newFanboxCommand(env commands.Env, children []*cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fanbox",
		Short: "Browse and download Pixiv FANBOX content",
		Args:  env.ExactArgs(0, "pixiv fanbox <command>"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	flags := cmd.PersistentFlags()
	flags.String("proxy", "", "native FANBOX proxy URL (HTTP or HTTPS CONNECT)")
	flags.Bool("no-proxy", false, "use a direct native FANBOX connection for this command")
	for _, child := range children {
		if child != nil {
			cmd.AddCommand(child)
		}
	}
	input.Bind(cmd, input.InputSpec{Codec: input.NoInput, MinArgs: 0, MaxArgs: 0, Reader: env.In, UsageError: env.UsageError})
	commands.Bind(cmd, commands.Execution{})
	return cmd
}
