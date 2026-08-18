// Package auth 注册 Pixiv auth 命令；具体 OAuth/login helper 也归本目录所有。
package auth

import (
	"io"

	"github.com/spf13/cobra"
)

// New 构造完整 Pixiv auth 命令组；账号、登录、bundle 和交互都保留在该 owner。
func New(deps Deps) *cobra.Command {
	return controller{deps: deps, in: deps.Input, out: deps.Output, errOut: deps.ErrorOutput}.newAccountCommand()
}

// CanPrompt 和 Prompt* 是 auth owner 提供的终端提示实现。composition root 把它们
// 注入回 auth Deps，并把同一实现共享给尚未完成迁移的 FANBOX auth；auth 命令自身
// 只使用注入端口，不反向依赖根包。
func CanPrompt(in io.Reader, out io.Writer) bool {
	return terminalCanPrompt(in, out)
}

func PromptInput(in io.Reader, out, errOut io.Writer, message, defaultValue string) (string, error) {
	return terminalPromptInput(in, out, errOut, message, defaultValue)
}

func PromptSecret(in io.Reader, out, errOut io.Writer, message string) (string, error) {
	return terminalPromptSecret(in, out, errOut, message)
}

func PromptSelect(in io.Reader, out, errOut io.Writer, message string, options []string) (string, error) {
	return terminalPromptSelect(in, out, errOut, message, options)
}

func PromptConfirm(in io.Reader, out, errOut io.Writer, message string, defaultValue bool) (bool, error) {
	return terminalPromptConfirm(in, out, errOut, message, defaultValue)
}
