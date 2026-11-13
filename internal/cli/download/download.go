// Package download 提供 pixiv download 命令，把作品图片或 ugoira 保存到本地。
//
// 包的组成：[New] 组装命令树，[Runtime] 提供下载路径与命名模板。
package download

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
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
func New(env environment.Env, exec session.SDKExecutor, runtime func() (Runtime, error)) *cobra.Command {
	return controller{env: env, execute: exec, runtime: runtime}.newDownloadCommand()
}

// RuntimeFrom 把运行时配置投影成下载命令使用的路径与命名模板。
func RuntimeFrom(load func() (config.Config, error)) func() (Runtime, error) {
	return func() (Runtime, error) {
		runtime, err := load()
		if err != nil {
			return Runtime{}, err
		}
		return Runtime{DownloadPath: runtime.Download.Path, FilenameTemplate: runtime.Download.FilenameTemplate, DirectoryTemplate: runtime.Download.DirectoryTemplate}, nil
	}
}
