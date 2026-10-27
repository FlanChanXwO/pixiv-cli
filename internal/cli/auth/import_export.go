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
	// stdin is classified once in Args; RunE reads the result from this command's own state.
	var input authImportInput
	cmd := &cobra.Command{
		Use:     "import [REFRESH_TOKEN]",
		Short:   "Import or replace an account",
		Example: "pixiv auth import YOUR_REFRESH_TOKEN",
		Args:    a.env.MaxArgs(1, "pixiv auth import [REFRESH_TOKEN]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.accountImport(cmd, args, opts, input)
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.jsonOut, "json", false, "print JSON")
	a.env.BindProxyFlags(cmd, &opts.ProxyOptions)
	a.bindAuthImportInput(cmd, &input)
	a.env.BindNoInput(cmd)
	command.Bind(cmd, command.AuthAccount())
	return cmd
}

func (a controller) accountImport(cmd *cobra.Command, args []string, opts accountImportOptions, input authImportInput) error {
	if len(input.bundle) > 0 {
		services, err := a.services()
		if err != nil {
			return err
		}
		result, err := services.Account.importBundle(cmd.Context(), input.bundle)
		if err != nil {
			return err
		}
		if opts.jsonOut {
			return a.env.PrintJSON(result)
		}
		for _, account := range result.Accounts {
			fmt.Fprintf(a.env.Out, "%s uid:%d\n", account.Status, account.UserID)
		}
		fmt.Fprintf(a.env.Out, "default uid: %d\n", result.DefaultUserID)
		return nil
	}
	proxyOverride, err := command.ProxyOverride(cmd, opts.ProxyOptions)
	if err != nil {
		return err
	}
	tokenInput, err := a.resolveRefreshTokenInput(args, input)
	if err != nil {
		return err
	}
	services, err := a.services()
	if err != nil {
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
		return a.env.PrintJSON(result)
	}
	fmt.Fprintf(a.env.Out, "%s uid:%d\n", result.Status, result.UserID)
	if result.Username != "" {
		fmt.Fprintf(a.env.Out, "username:%s\n", result.Username)
	}
	return nil
}

func (a controller) bindAuthImportInput(cmd *cobra.Command, input *authImportInput) {
	validate := cmd.Args
	cmd.Args = func(ctxCmd *cobra.Command, args []string) error {
		if len(args) == 0 && !a.env.CanPrompt() {
			if _, err := command.ProxyOverride(ctxCmd, accountImportOptions{}.ProxyOptions); err != nil {
				return err
			}
			body, err := io.ReadAll(a.env.In)
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
				*input = authImportInput{bundle: body}
				command.SetOffline(ctxCmd)
				command.Bind(ctxCmd, command.AuthBundleImport())
			} else {
				token, err := readRefreshTokenInput(bytes.NewReader(body))
				if err != nil {
					return err
				}
				*input = authImportInput{token: token}
			}
		}
		return validate(ctxCmd, args)
	}
}

func (a controller) resolveRefreshTokenInput(args []string, input authImportInput) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}
	if input.token != "" {
		return input.token, nil
	}
	if a.env.CanPrompt() {
		return a.env.PromptSecret("Refresh token")
	}
	if _, err := fmt.Fprint(a.env.Err, "refresh token: "); err != nil {
		return "", err
	}
	return readRefreshTokenInput(a.env.In)
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
