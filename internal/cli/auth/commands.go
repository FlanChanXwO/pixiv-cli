package auth

import (
	"io"

	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pixiv"
	"github.com/spf13/cobra"
)

type Host interface {
	Input() io.Reader
	Output() io.Writer
	ErrorOutput() io.Writer
	PrintJSON(any) error
	UsageError(error) error
	RequireExactArgs(int, string) cobra.PositionalArgs
	RequireMinArgs(int, string) cobra.PositionalArgs
	RequireMaxArgs(int, string) cobra.PositionalArgs
	AccountService() pixivapp.AccountService
	LoginService() pixivapp.LoginService
	WriteAuthExportBundle(string, []byte, bool) error
	CanPrompt() bool
	PromptInput(string, string) (string, error)
	PromptSecret(string) (string, error)
	PromptSelect(string, []string) (string, error)
	PromptConfirm(string, bool) (bool, error)
}

type controller struct {
	Host
	in     io.Reader
	out    io.Writer
	errOut io.Writer
}

type proxyOptions struct {
	proxy   string
	noProxy bool
}

type runtimeServices struct {
	Account pixivapp.AccountService
	Login   pixivapp.LoginService
}
