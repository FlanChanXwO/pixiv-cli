// Package fanbox 注册 FANBOX 命令。
package fanbox

import (
	"context"
	"io"

	configapp "github.com/FlanChanXwO/pixiv-cli/internal/application/config"
	fanboxapp "github.com/FlanChanXwO/pixiv-cli/internal/application/fanbox"
	"github.com/spf13/cobra"
)

type Host interface {
	Input() io.Reader
	Output() io.Writer
	PrintJSON(any) error
	UsageError(error) error
	RequireExactArgs(int, string) cobra.PositionalArgs
	RequireMinArgs(int, string) cobra.PositionalArgs
	RequireMaxArgs(int, string) cobra.PositionalArgs
	FanboxService() (*fanboxapp.Service, error)
	FanboxBrowserProvider() BrowserProvider
	FanboxRuntimeConfig() (configapp.RuntimeConfig, error)
	CanPrompt() bool
	PromptConfirm(string, bool) (bool, error)
}

// BrowserProvider 只返回经过 cookie 查询边界校验的 FANBOXSESSID；实现不得
// 把浏览器数据库路径或 session secret 写入输出。
type BrowserProvider interface {
	ReadSession(context.Context, string, string) (string, error)
}

func Register(root *cobra.Command, host Host) {
	root.AddCommand(NewCommand(host))
}

// controller 只保存 command package 需要的输入输出快照；所有运行时服务、
// 浏览器 cookie 和交互能力均由 Host 注入，避免子包依赖根 CLI。
type controller struct {
	Host
	in  io.Reader
	out io.Writer
}

type proxyOptions struct {
	proxy   string
	noProxy bool
}

// NewCommand 构造完整 FANBOX 命令树。Host 在一次 CLI Run 内保持同一 Runtime。
func NewCommand(host Host) *cobra.Command {
	return controller{Host: host, in: host.Input(), out: host.Output()}.newFanboxCommand()
}
