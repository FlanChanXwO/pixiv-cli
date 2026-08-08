// Package runtime 提供 CLI 根控制器使用的运行期 seam。
package runtime

import "github.com/FlanChanXwO/pixiv-cli/internal/bootstrap"

// Factory 是一次 CLI Run 的 runtime 构造函数。测试和嵌入调用方可替换它，
// 而命令子包不需要反向依赖 internal/cli 根包。
type Factory func() (*bootstrap.Runtime, error)

// DefaultFactory 返回生产 runtime 构造器；真正执行命令时才会调用。
func DefaultFactory() Factory {
	return func() (*bootstrap.Runtime, error) {
		return bootstrap.NewRuntime(bootstrap.RuntimeOptions{})
	}
}
