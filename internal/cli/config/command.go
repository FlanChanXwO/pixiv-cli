// Package config 注册 config 命令。
package config

import (
	"io"

	configapp "github.com/FlanChanXwO/pixiv-cli/internal/application/config"
	"github.com/spf13/cobra"
)

type Host interface {
	Output() io.Writer
	ErrorOutput() io.Writer
	RequireExactArgs(int, string) cobra.PositionalArgs
	ConfigService() configapp.ConfigService
	ConfigPath() (string, error)
}

func Register(root *cobra.Command, host Host) {
	root.AddCommand(NewCommand(host))
}

// NewCommand 只负责将 config 命令注册到同目录实现。
func NewCommand(host Host) *cobra.Command {
	return newConfigCommand(host)
}
