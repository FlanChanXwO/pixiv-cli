// Package download owns the Pixiv download command and its terminal presenter.
package download

import (
	"context"
	"io"

	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

// Runtime 是 download 命令从一次执行 snapshot 取用的窄配置值。
type Runtime struct {
	DownloadPath      string
	FilenameTemplate  string
	DirectoryTemplate string
}

// Deps 是 download command 所需的窄运行依赖；root 仅在构造命令时注入它们。
type Deps struct {
	Input       io.Reader
	Output      io.Writer
	ErrorOutput io.Writer
	UsageError  func(error) error
	// Open 为一次 operation 打开独立认证快照的 public SDK client。
	Open func(CommandRequest) (*pixiv.Client, error)
	// Pooled 在账号池安全重放边界内执行一次下载读取。
	Pooled func(context.Context, CommandRequest, func(context.Context, *pixiv.Client) (bool, error)) error
	// Runtime 返回一次执行固定使用的窄配置值。
	Runtime  func() (Runtime, error)
	Download func() downloader.DownloadService
}

// New 构造单一 download 叶子命令。
func New(deps Deps) *cobra.Command {
	return newController(deps).newDownloadCommand()
}
