package auth

import (
	"io"

	config "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	pixivaccount "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/account"
)

// Deps 是 Pixiv auth owner 的精确运行依赖。它不暴露根 CLI 或宽泛服务定位器；
// 每个 closure 只对应 auth 命令实际需要的一项能力。交互式提示同样是注入端口，
// 因此整个 CLI 只有 composition root 一处终端判定与提示实现。
type Deps struct {
	Input       io.Reader
	Output      io.Writer
	ErrorOutput io.Writer
	UsageError  func(error) error
	Account     func() (AccountService, error)
	Login       func() (pixivaccount.LoginService, error)
	LoadRuntime func() (config.RuntimeConfig, error)
	WriteBundle func(string, []byte, bool) error

	CanPrompt     func() bool
	PromptInput   func(message, defaultValue string) (string, error)
	PromptSecret  func(message string) (string, error)
	PromptSelect  func(message string, options []string) (string, error)
	PromptConfirm func(message string, defaultValue bool) (bool, error)
}

type controller struct {
	deps   Deps
	in     io.Reader
	out    io.Writer
	errOut io.Writer
}

type proxyOptions struct {
	proxy   string
	noProxy bool
}

type runtimeServices struct {
	Account     AccountService
	Login       pixivaccount.LoginService
	LoadRuntime func() (config.RuntimeConfig, error)
	err         error
}
