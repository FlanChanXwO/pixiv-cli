package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/files"
	"github.com/spf13/cobra"
)

type accountOut struct {
	UserID   int64  `json:"user_id,omitempty"`
	Username string `json:"username,omitempty"`
	Default  bool   `json:"default"`
	HasToken bool   `json:"has_token"`
	Warning  string `json:"warning,omitempty"`
}

type accountListOut struct {
	DefaultUserID int64        `json:"default_user_id,omitempty"`
	Accounts      []accountOut `json:"accounts"`
}

type accountImportOptions struct {
	proxyOptions
	jsonOut bool
	file    string
}

type accountExportOptions struct {
	all    bool
	output string
	force  bool
}

type accountRemoveOptions struct {
	jsonOut bool
	yes     bool
}

type accountCheckOptions struct {
	proxyOptions
	jsonOut bool
}

func (a app) newAccountCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage local Pixiv authentication",
		Args:  requireExactArgs(0, "pixiv auth <command>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		a.newAccountImportCommand(),
		a.newAccountLoginCommand(),
		a.newAccountListCommand(),
		a.newAccountRemoveCommand(),
		a.newAccountUseCommand(),
		a.newAccountCheckCommand(),
		a.newAccountExportCommand(),
	)
	return cmd
}

func (a app) newAccountExportCommand() *cobra.Command {
	opts := accountExportOptions{}
	cmd := &cobra.Command{
		Use:   "export [UID]",
		Short: "Export stored authentication",
		Args:  requireMaxArgs(1, "pixiv auth export [UID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.force && !cmd.Flags().Changed("output") {
				return errors.New("--force requires --output")
			}
			if opts.all && len(args) != 0 {
				return errors.New("--all cannot be combined with a UID")
			}
			userID := int64(0)
			if len(args) == 1 {
				parsed, err := parseAuthExportUID(args[0])
				if err != nil {
					return err
				}
				userID = parsed
			}
			result, err := a.services().Account.Export(application.AccountExportRequest{UserID: userID, All: opts.all})
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("output") {
				if strings.TrimSpace(opts.output) == "" {
					return errors.New("--output requires a path")
				}
				if err := files.WriteSecretFile(opts.output, result.Bundle, opts.force); err != nil {
					return err
				}
				return writeAuthExportStdout(a.out, []byte(fmt.Sprintf("output: %s\naccounts: %d\n", opts.output, result.AccountCount)))
			}
			if opts.all {
				return writeAuthExportStdout(a.out, result.Bundle)
			}
			return writeAuthExportStdout(a.out, []byte(result.RefreshToken+"\n"))
		},
	}
	cmd.Flags().BoolVar(&opts.all, "all", false, "export all stored accounts")
	cmd.Flags().StringVar(&opts.output, "output", "", "write a versioned authentication bundle to PATH")
	cmd.Flags().BoolVar(&opts.force, "force", false, "replace an existing output file")
	return cmd
}

type authExportStdoutError struct{ cause error }

func (e authExportStdoutError) Error() string { return "write stdout failed" }
func (e authExportStdoutError) Unwrap() error { return e.cause }

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

func (a app) newAccountImportCommand() *cobra.Command {
	opts := accountImportOptions{}
	cmd := &cobra.Command{
		Use:     "import [REFRESH_TOKEN]",
		Short:   "Import or replace an account",
		Example: "pixiv auth import YOUR_REFRESH_TOKEN",
		Args:    requireMaxArgs(1, "pixiv auth import [REFRESH_TOKEN]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.accountImport(cmd, args, opts)
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.jsonOut, "json", false, "print JSON")
	flags.StringVar(&opts.file, "file", "", "restore an authentication export bundle from PATH or stdin with -")
	a.bindProxyFlags(cmd, &opts.proxyOptions)
	return cmd
}

func (a app) accountImport(cmd *cobra.Command, args []string, opts accountImportOptions) error {
	if cmd.Flags().Changed("file") {
		if len(args) != 0 || cmd.Flags().Changed("proxy") || cmd.Flags().Changed("no-proxy") {
			return errors.New("--file cannot be combined with a refresh token, --proxy, or --no-proxy")
		}
		body, err := readAuthBundleInput(a.in, opts.file)
		if err != nil {
			return err
		}
		result, err := a.services().Account.ImportBundle(body)
		if err != nil {
			return err
		}
		if opts.jsonOut {
			return a.printJSON(result)
		}
		for _, account := range result.Added {
			fmt.Fprintf(a.out, "added uid:%d\n", account.UserID)
		}
		for _, account := range result.Updated {
			fmt.Fprintf(a.out, "updated uid:%d\n", account.UserID)
		}
		fmt.Fprintf(a.out, "default uid: %d\n", result.DefaultUserID)
		return nil
	}
	proxyOverride, err := proxyOverrideFromFlags(cmd, opts.proxyOptions)
	if err != nil {
		return err
	}
	tokenInput, err := a.resolveRefreshTokenInput(args)
	if err != nil {
		return err
	}
	services := a.services()
	result, err := services.Account.Import(cmd.Context(), application.AccountImportRequest{
		TokenInput:         tokenInput,
		HTTPSProxyOverride: proxyOverride,
	})
	if err != nil {
		return err
	}
	out := accountOutFromResult(result)
	if opts.jsonOut {
		return a.printJSON(out)
	}
	fmt.Fprintf(a.out, "account uid:%d saved\n", out.UserID)
	if out.Username != "" {
		fmt.Fprintf(a.out, "username:%s\n", out.Username)
	}
	if out.Default {
		fmt.Fprintf(a.out, "default uid: %d\n", out.UserID)
	}
	if out.Warning != "" {
		fmt.Fprintf(a.errOut, "warning: %s\n", out.Warning)
	}
	return nil
}

func readAuthBundleInput(stdin io.Reader, path string) ([]byte, error) {
	if path == "-" {
		body, err := io.ReadAll(stdin)
		if err != nil {
			return nil, errors.New("cannot read authentication export bundle from stdin")
		}
		return body, nil
	}
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("--file requires a path or -")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, classifyAuthBundleFileReadError(path, err)
	}
	return body, nil
}

type authBundleFileReadCategory string

const (
	authBundleFileReadNotFound         authBundleFileReadCategory = "not_found"
	authBundleFileReadPermissionDenied authBundleFileReadCategory = "permission_denied"
	authBundleFileReadIsDirectory      authBundleFileReadCategory = "is_directory"
	authBundleFileReadIO               authBundleFileReadCategory = "io"
)

type authBundleFileReadError struct {
	category authBundleFileReadCategory
	cause    error
}

func (e authBundleFileReadError) Error() string {
	return "read authentication export bundle failed: " + string(e.category)
}

func (e authBundleFileReadError) Unwrap() error { return e.cause }

func (e authBundleFileReadError) Category() string { return string(e.category) }

func classifyAuthBundleFileReadError(path string, err error) error {
	category := authBundleFileReadIO
	switch {
	case errors.Is(err, os.ErrNotExist):
		category = authBundleFileReadNotFound
	case errors.Is(err, os.ErrPermission):
		category = authBundleFileReadPermissionDenied
	default:
		if info, statErr := os.Stat(path); statErr == nil && info.IsDir() {
			category = authBundleFileReadIsDirectory
		}
	}
	return authBundleFileReadError{category: category, cause: err}
}

func (a app) newAccountListCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List accounts",
		Args:  requireExactArgs(0, "pixiv auth list [--json]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.accountList(jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print JSON")
	return cmd
}

func (a app) accountList(jsonOut bool) error {
	services := a.services()
	result, err := services.Account.List()
	if err != nil {
		return err
	}
	out := accountListOutFromResult(result)
	if jsonOut {
		return a.printJSON(out)
	}
	if len(out.Accounts) == 0 {
		fmt.Fprintln(a.out, "no accounts")
		return nil
	}
	for _, acct := range out.Accounts {
		marker := " "
		if acct.Default {
			marker = "*"
		}
		fmt.Fprintf(a.out, "%s uid:%d token:%s", marker, acct.UserID, textBool(acct.HasToken))
		if acct.Username != "" {
			fmt.Fprintf(a.out, " username:%s", acct.Username)
		}
		fmt.Fprintln(a.out)
	}
	return nil
}

func (a app) newAccountRemoveCommand() *cobra.Command {
	opts := accountRemoveOptions{}
	cmd := &cobra.Command{
		Use:   "remove [UID]",
		Short: "Remove an account",
		Args:  requireMaxArgs(1, "pixiv auth remove [UID] [--yes]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.accountRemove(args, opts)
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.jsonOut, "json", false, "print JSON")
	flags.BoolVar(&opts.yes, "yes", false, "skip confirmation in interactive terminals")
	return cmd
}

func (a app) accountRemove(args []string, opts accountRemoveOptions) error {
	services := a.services()
	list, err := services.Account.List()
	if err != nil {
		return err
	}
	userID, err := a.resolveExistingUID(list, args, "Select account to remove")
	if err != nil {
		return err
	}
	if canPrompt(a) && !opts.yes {
		ok, err := promptConfirm(a, fmt.Sprintf("Remove uid %d?", userID), false)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("account removal canceled")
		}
	}
	_, defaultUserID, err := services.Account.Remove(userID)
	if err != nil {
		return err
	}
	if opts.jsonOut {
		return a.printJSON(map[string]int64{"removed_user_id": userID, "default_user_id": defaultUserID})
	}
	fmt.Fprintf(a.out, "account uid:%d removed\n", userID)
	if defaultUserID != 0 {
		fmt.Fprintf(a.out, "default uid: %d\n", defaultUserID)
	}
	return nil
}

func (a app) newAccountUseCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "use [UID]",
		Short: "Set the default account",
		Args:  requireMaxArgs(1, "pixiv auth use [UID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.accountUse(args, jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print JSON")
	return cmd
}

func (a app) accountUse(args []string, jsonOut bool) error {
	services := a.services()
	list, err := services.Account.List()
	if err != nil {
		return err
	}
	userID, err := a.resolveExistingUID(list, args, "Select default account")
	if err != nil {
		return err
	}
	userID, err = services.Account.Use(userID)
	if err != nil {
		return err
	}
	if jsonOut {
		return a.printJSON(map[string]int64{"default_user_id": userID})
	}
	fmt.Fprintf(a.out, "default uid: %d\n", userID)
	return nil
}

func (a app) newAccountCheckCommand() *cobra.Command {
	opts := accountCheckOptions{}
	cmd := &cobra.Command{
		Use:   "check [UID]",
		Short: "Validate an account token",
		Args:  requireMaxArgs(1, "pixiv auth check [UID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			userID := int64(0)
			if len(args) == 1 {
				var err error
				userID, err = application.ParseUID(args[0])
				if err != nil {
					return err
				}
			}
			return a.accountCheck(cmd, userID, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.jsonOut, "json", false, "print JSON")
	a.bindProxyFlags(cmd, &opts.proxyOptions)
	return cmd
}

func (a app) accountCheck(cmd *cobra.Command, userID int64, opts accountCheckOptions) error {
	proxyOverride, err := proxyOverrideFromFlags(cmd, opts.proxyOptions)
	if err != nil {
		return err
	}
	services := a.services()
	result, err := services.Account.CheckWithRequest(cmd.Context(), application.AccountCheckRequest{
		UserID:             userID,
		HTTPSProxyOverride: proxyOverride,
	})
	if err != nil {
		return err
	}
	out := accountOutFromResult(result)
	if opts.jsonOut {
		return a.printJSON(out)
	}
	if userID == 0 {
		fmt.Fprintf(a.out, "token ok, uid:%d\n", result.UserID)
	} else {
		fmt.Fprintf(a.out, "account uid:%d ok\n", result.UserID)
	}
	if result.Username != "" {
		fmt.Fprintf(a.out, "username:%s\n", result.Username)
	}
	if result.Warning != "" {
		fmt.Fprintf(a.errOut, "warning: %s\n", result.Warning)
	}
	return nil
}

func (a app) resolveExistingUID(list application.AccountListResult, args []string, promptLabel string) (int64, error) {
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		return application.ParseUID(args[0])
	}
	if !canPrompt(a) {
		return 0, errors.New("uid is required")
	}
	if len(list.Accounts) == 0 {
		return 0, errors.New("no accounts")
	}
	options := make([]string, 0, len(list.Accounts))
	for _, acct := range list.Accounts {
		label := fmt.Sprintf("%d", acct.UserID)
		if acct.Username != "" {
			label += " " + acct.Username
		}
		options = append(options, label)
	}
	selected, err := promptSelect(a, promptLabel, options)
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(selected)
	if len(fields) == 0 {
		return 0, errors.New("uid cannot be empty")
	}
	return application.ParseUID(fields[0])
}

func (a app) resolveRefreshTokenInput(args []string) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}
	if canPrompt(a) {
		return promptSecret(a, "Refresh token")
	}
	writePrompt(a.errOut, "refresh token: ")
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

func accountOutFromResult(result application.AccountResult) accountOut {
	return accountOut{
		UserID:   result.UserID,
		Username: result.Username,
		Default:  result.Default,
		HasToken: result.HasToken,
		Warning:  result.Warning,
	}
}

func accountListOutFromResult(result application.AccountListResult) accountListOut {
	out := accountListOut{DefaultUserID: result.DefaultUserID}
	for _, acct := range result.Accounts {
		out.Accounts = append(out.Accounts, accountOutFromResult(acct))
	}
	return out
}
