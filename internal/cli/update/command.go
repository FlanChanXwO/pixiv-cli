// Package update 注册 update 命令；更新实现仍归 internal/update。
package update

import (
	"io"

	configapp "github.com/FlanChanXwO/pixiv-cli/internal/application/config"
	updateapp "github.com/FlanChanXwO/pixiv-cli/internal/update"
	"github.com/spf13/cobra"
)

type Host interface {
	Output() io.Writer
	ErrorOutput() io.Writer
	PrintJSON(any) error
	RequireExactArgs(int, string) cobra.PositionalArgs
	LoadUpdateRuntimeConfig() (configapp.RuntimeConfig, error)
	NewUpdateCoordinator(string, io.Writer, io.Writer) (*updateapp.UpdateCoordinator, error)
}

// AutomaticCheckHost 是成功命令后的只读更新检查所需的最小依赖端口。
type AutomaticCheckHost interface {
	ErrorOutput() io.Writer
	LoadAutomaticUpdateRuntimeConfig() (configapp.RuntimeConfig, error)
	NewAutomaticUpdateChecker(string) (*updateapp.AutomaticUpdateChecker, error)
}

func Register(root *cobra.Command, host Host) {
	root.AddCommand(NewCommand(host))
}

// NewCommand 只负责将 update 命令注册到同目录实现。
func NewCommand(host Host) *cobra.Command {
	return newUpdateCommand(host)
}
