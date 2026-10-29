// Package auth 提供 pixiv auth 命令组：账号列表、选择、导入导出、检查与刷新，以及浏览器 OAuth 登录。
//
// 包的组成：[New] 组装命令树，[Support] 携带账号服务、登录服务与配置读取端口。
// [AccountService] 适配账号领域访问。
// 桌面回调与远程登录在 [github.com/FlanChanXwO/pixiv-cli/internal/urlscheme]。
package auth

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/account"
	"github.com/spf13/cobra"
)

// New 构造完整 Pixiv auth 命令组；账号、登录、bundle 和交互都保留在该 owner。
func New(env command.Env, support Support) *cobra.Command {
	return newController(env, support).newAccountCommand()
}

// Support 携带各 auth 叶子命令共用的账号、登录和 bundle 写入端口；输入输出与交互式提问
// 由 command.Env 提供。
type Support struct {
	// Account 延迟打开账号服务；只在执行账号子命令时调用。
	Account func() (AccountService, error)
	// Login 延迟打开登录服务，避免构造命令树时打开数据库。
	Login func() (account.LoginService, error)
	// LoadRuntime 每次调用读取最新的运行时配置。
	LoadRuntime func() (settings.RuntimeConfig, error)
	// WriteBundle 写出含凭据的 bundle；force 为 false 时不得覆盖已有文件。
	WriteBundle func(string, []byte, bool) error
}
