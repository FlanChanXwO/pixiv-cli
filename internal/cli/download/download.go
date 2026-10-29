// Package download 提供 pixiv download 命令，把作品图片或 ugoira 保存到本地。
//
// 包的组成：[New] 组装命令树，[Runtime] 提供下载路径与命名模板。
// [ProgressRenderer] 输出终端进度。
package download

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/spf13/cobra"
)

// Runtime 保存 download 命令从一次执行快照取得的路径和命名模板。
type Runtime struct {
	DownloadPath      string
	FilenameTemplate  string
	DirectoryTemplate string
}

// New 构造单一 download 叶子命令。
func New(env environment.Env, exec session.Executor, runtime func() (Runtime, error), download func() downloader.DownloadService) *cobra.Command {
	return controller{env: env, runner: session.NewRunner(env.Out, exec), runtime: runtime, download: download}.newDownloadCommand()
}

// RuntimeFrom 把运行时配置投影成下载命令使用的路径与命名模板。
func RuntimeFrom(load func() (settings.RuntimeConfig, error)) func() (Runtime, error) {
	return func() (Runtime, error) {
		runtime, err := load()
		if err != nil {
			return Runtime{}, err
		}
		return Runtime{DownloadPath: runtime.DownloadPath, FilenameTemplate: runtime.FilenameTemplate, DirectoryTemplate: runtime.DirectoryTemplate}, nil
	}
}
