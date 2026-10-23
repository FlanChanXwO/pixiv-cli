// Package download owns the Pixiv download command and its terminal presenter.
package download

import (
	commands "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/spf13/cobra"
)

// Runtime 是 download 命令从一次执行 snapshot 取用的窄配置值。
type Runtime struct {
	DownloadPath      string
	FilenameTemplate  string
	DirectoryTemplate string
}

// New 构造单一 download 叶子命令。
func New(env commands.Env, exec pixivcmd.Executor, runtime func() (Runtime, error), download func() downloader.DownloadService) *cobra.Command {
	return controller{env: env, runner: pixivcmd.New(env.Out, exec), runtime: runtime, download: download}.newDownloadCommand()
}
