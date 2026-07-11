package cli

import (
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/buildinfo"
	"github.com/spf13/cobra"
)

// newVersionCommand 只展示当前二进制的元数据，不初始化配置、认证或 Pixiv 服务。
func (a app) newVersionCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show build version information",
		Args:  requireExactArgs(0, "pixiv version [--json]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			info := buildinfo.Current()
			if jsonOut {
				return a.printJSON(info)
			}
			_, err := fmt.Fprintf(a.out, "pixiv %s\ncommit: %s\nbuild date: %s\n", info.Version, info.Commit, info.BuildDate)
			return err
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print JSON")
	return cmd
}
