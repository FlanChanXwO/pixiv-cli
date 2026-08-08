// Package download 注册 Pixiv download 命令。
package download

import (
	"io"

	downloadapp "github.com/FlanChanXwO/pixiv-cli/internal/application/download"
	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pixiv"
	"github.com/spf13/cobra"
)

// Host 是下载命令所需的最小 composition seam。
type Host interface {
	Input() io.Reader
	Output() io.Writer
	ErrorOutput() io.Writer
	UsageError(error) error
	SDKService() pixivapp.SDKService
	DownloadService() downloadapp.DownloadService
}

// NewCommand 构造单一 download 叶子命令。
func NewCommand(host Host) *cobra.Command {
	return newController(host).newDownloadCommand()
}

func Register(root *cobra.Command, host Host) {
	root.AddCommand(NewCommand(host))
}
