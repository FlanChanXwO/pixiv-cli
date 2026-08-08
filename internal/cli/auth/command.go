// Package auth 注册 Pixiv auth 命令；具体 OAuth/login helper 也归本目录所有。
package auth

import (
	"io"

	"github.com/spf13/cobra"
)

// Register 将 auth 命令挂到 CLI 根命令。
func Register(root *cobra.Command, host Host) {
	root.AddCommand(NewCommand(host))
}

// NewCommand 只负责将 auth 命令注册到同目录实现。
func NewCommand(host Host) *cobra.Command {
	return controller{Host: host, in: host.Input(), out: host.Output(), errOut: host.ErrorOutput()}.newAccountCommand()
}

// CanPrompt/PromptConfirm 是 FANBOX 等其他 command package 复用的交互端口；
// prompt 的测试 seam 仍由 auth package 自己拥有。
func CanPrompt(in io.Reader, out io.Writer) bool {
	return canPrompt(controller{in: in, out: out})
}

func PromptInput(in io.Reader, out, errOut io.Writer, message, defaultValue string) (string, error) {
	return promptInput(controller{in: in, out: out, errOut: errOut}, message, defaultValue)
}

func PromptSecret(in io.Reader, out, errOut io.Writer, message string) (string, error) {
	return promptSecret(controller{in: in, out: out, errOut: errOut}, message)
}

func PromptSelect(in io.Reader, out, errOut io.Writer, message string, options []string) (string, error) {
	return promptSelect(controller{in: in, out: out, errOut: errOut}, message, options)
}

func PromptConfirm(in io.Reader, out, errOut io.Writer, message string, defaultValue bool) (bool, error) {
	return promptConfirm(controller{in: in, out: out, errOut: errOut}, message, defaultValue)
}
