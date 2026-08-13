// Package version 注册 version 命令。
package version

import (
	"fmt"
	"io"

	"github.com/FlanChanXwO/pixiv-cli/internal/buildinfo"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/requirements"
	"github.com/spf13/cobra"
)

type Host interface {
	Output() io.Writer
	PrintJSON(any) error
	RequireExactArgs(int, string) cobra.PositionalArgs
}

func Register(root *cobra.Command, host Host) {
	root.AddCommand(NewCommand(host))
}

// NewCommand 只展示当前二进制的元数据，不初始化配置、认证或 Pixiv 服务。
func NewCommand(host Host) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show build version information",
		Args:  host.RequireExactArgs(0, "pixiv version [--json]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			info := buildinfo.Current()
			if jsonOut {
				return host.PrintJSON(info)
			}
			_, err := fmt.Fprintf(host.Output(), "pixiv %s\ncommit: %s\nbuild date: %s\n", info.Version, info.Commit, info.BuildDate)
			return err
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print JSON")
	pipeline.Bind(cmd, pipeline.InputSpec{Codec: pipeline.NoInput, MinArgs: 0, MaxArgs: 0})
	requirements.Bind(cmd, requirements.Version())
	return cmd
}
