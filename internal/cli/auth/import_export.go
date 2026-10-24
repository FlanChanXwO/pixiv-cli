// Auth import/export plumbing: secret output, UID parsing, and token input.
// Account command construction lives in account.go.

package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	inputcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/input"
	"github.com/spf13/cobra"
)

type authExportStdoutError struct{ cause error }

func (e authExportStdoutError) Error() string { return "write stdout failed" }
func (e authExportStdoutError) Unwrap() error { return e.cause }

// writeAuthExportStdout 把 auth export 输出写到指定 writer，且任何写入错误都
// 以稳定类别返回，绝不把 token 或路径带进错误文本。
func writeAuthExportStdout(writer io.Writer, body []byte) error {
	written, err := writer.Write(body)
	if err != nil {
		return authExportStdoutError{cause: err}
	}
	if written != len(body) {
		return authExportStdoutError{cause: io.ErrShortWrite}
	}
	return nil
}

// parseAuthExportUID 不回显原始输入：调用者可能误把 token 或私有路径放在 UID 位。
func parseAuthExportUID(raw string) (int64, error) {
	userID, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || userID <= 0 {
		return 0, errors.New("uid must be a positive integer")
	}
	return userID, nil
}

func (a controller) newAccountImportCommand() *cobra.Command {
	opts := accountImportOptions{}
	cmd := &cobra.Command{
		Use:     "import [REFRESH_TOKEN]",
		Short:   "Import or replace an account",
		Example: "pixiv auth import YOUR_REFRESH_TOKEN",
		Args:    a.requireMaxArgs(1, "pixiv auth import [REFRESH_TOKEN]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.accountImport(cmd, args, opts)
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.jsonOut, "json", false, "print JSON")
	a.bindProxyFlags(cmd, &opts.ProxyOptions)
	a.bindAuthImportInput(cmd)
	a.bindNoInput(cmd)
	command.Bind(cmd, command.AuthAccount())
	return cmd
}

func (a controller) accountImport(cmd *cobra.Command, args []string, opts accountImportOptions) error {
	if input, ok := authImportInputFrom(cmd); ok && len(input.bundle) > 0 {
		services := a.services()
		if err := services.require(); err != nil {
			return err
		}
		result, err := services.Account.importBundle(cmd.Context(), input.bundle)
		if err != nil {
			return err
		}
		if opts.jsonOut {
			return a.printJSON(result)
		}
		for _, account := range result.Accounts {
			fmt.Fprintf(a.out, "%s uid:%d\n", account.Status, account.UserID)
		}
		fmt.Fprintf(a.out, "default uid: %d\n", result.DefaultUserID)
		return nil
	}
	proxyOverride, err := proxyOverrideFromFlags(cmd, opts.ProxyOptions)
	if err != nil {
		return err
	}
	tokenInput, err := a.resolveRefreshTokenInput(cmd, args)
	if err != nil {
		return err
	}
	services := a.services()
	if err := services.require(); err != nil {
		return err
	}
	result, err := services.Account.importAccount(cmd.Context(), accountImportRequest{
		TokenInput:         tokenInput,
		HTTPSProxyOverride: proxyOverride,
	})
	if err != nil {
		return err
	}
	if opts.jsonOut {
		return a.printJSON(result)
	}
	fmt.Fprintf(a.out, "%s uid:%d\n", result.Status, result.UserID)
	if result.Username != "" {
		fmt.Fprintf(a.out, "username:%s\n", result.Username)
	}
	return nil
}

func (a controller) bindAuthImportInput(cmd *cobra.Command) {
	validate := cmd.Args
	if validate == nil {
		validate = cobra.ArbitraryArgs
	}
	cmd.Args = func(ctxCmd *cobra.Command, args []string) error {
		if len(args) == 0 && !a.canPrompt() {
			if _, err := proxyOverrideFromFlags(ctxCmd, accountImportOptions{}.ProxyOptions); err != nil {
				return err
			}
			body, err := io.ReadAll(a.in)
			if err != nil {
				return fmt.Errorf("read auth import stdin: %w", err)
			}
			first, hasValue := inputcmd.FirstNonWhitespace(body)
			if hasValue && first == '{' {
				if !json.Valid(body) {
					return errors.New("invalid auth export bundle JSON")
				}
				if cmd.Flags().Changed("proxy") || cmd.Flags().Changed("no-proxy") {
					return errors.New("bundle import cannot be combined with --proxy or --no-proxy")
				}
				setAuthImportInput(ctxCmd, authImportInput{bundle: body})
				inputcmd.MarkSkipAutomaticUpdate(ctxCmd)
				command.Override(ctxCmd, command.AuthBundleImport())
			} else {
				token, err := readRefreshTokenInput(bytes.NewReader(body))
				if err != nil {
					return err
				}
				setAuthImportInput(ctxCmd, authImportInput{token: token})
			}
		}
		return validate(ctxCmd, args)
	}
}

func setAuthImportInput(command *cobra.Command, input authImportInput) {
	if command == nil {
		return
	}
	authImportInputs.Store(command.Root(), input)
}

func authImportInputFrom(command *cobra.Command) (authImportInput, bool) {
	if command == nil {
		return authImportInput{}, false
	}
	input, ok := authImportInputs.Load(command.Root())
	if !ok {
		return authImportInput{}, false
	}
	return input.(authImportInput), true
}

// ClearInputState releases the auth-specific classifier result after one CLI
// execution. Embedded callers should defer this alongside inputcmd.Clear.
func ClearInputState(command *cobra.Command) {
	if command == nil {
		return
	}
	authImportInputs.Delete(command.Root())
}

func (a controller) resolveRefreshTokenInput(cmd *cobra.Command, args []string) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}
	if input, ok := authImportInputFrom(cmd); ok && input.token != "" {
		return input.token, nil
	}
	if a.canPrompt() {
		return a.promptSecret("Refresh token")
	}
	if _, err := fmt.Fprint(a.errOut, "refresh token: "); err != nil {
		return "", err
	}
	return readRefreshTokenInput(a.in)
}

// readRefreshTokenInput 读取完整 stdin，只消费管道输出常见的一个末尾行结束符；
// token 的其他字节保持 opaque，不能用 TrimSpace 改写。
func readRefreshTokenInput(r io.Reader) (string, error) {
	input, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	token := string(input)
	if strings.HasSuffix(token, "\r\n") {
		token = strings.TrimSuffix(token, "\r\n")
	} else {
		token = strings.TrimSuffix(token, "\n")
	}
	if strings.ContainsAny(token, "\r\n") {
		return "", errors.New("refresh token input must contain exactly one line")
	}
	if token == "" {
		return "", errors.New("refresh token cannot be empty")
	}
	return token, nil
}
