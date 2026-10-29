package cli

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/spf13/cobra"
)

// newFanboxCommand 组装 FANBOX 产品组，只固定组的帮助文本、持久 flag 和子命令顺序。
func newFanboxCommand(env environment.Env, children []*cobra.Command) *cobra.Command {
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
	cmd.AddCommand(children...)
	env.BindNoInput(cmd)
	environment.Bind(cmd, environment.Execution{})
	return cmd
}
