// Pixiv auth account commands: list, import/export, selection, checks, refresh,
// and pool state. Account domain access is adapted in account_service.go.

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	inputcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/input"
	"github.com/spf13/cobra"
)

type accountImportOptions struct {
	command.ProxyOptions
	jsonOut bool
}

type authImportInput struct {
	token  string
	bundle []byte
}

// authImportInputs carries only the transient result of the auth-specific
// stdin classifier. Keeping it outside Cobra's context preserves the context
// identity passed to the account service and avoids leaking input state into
// unrelated command helpers.
var authImportInputs sync.Map // map[*cobra.Command]authImportInput

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
	command.ProxyOptions
	jsonOut bool
}

type accountRefreshOptions struct {
	command.ProxyOptions
	all     bool
	jsonOut bool
}

type accountPoolChangeOptions struct {
	all     bool
	jsonOut bool
}

func (a controller) newAccountCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage local Pixiv authentication",
		Args:  a.requireExactArgs(0, "pixiv auth <command>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		a.newAccountImportCommand(),
		a.newAccountLoginCommand(),
		a.newAccountURLCallbackCommand(),
		a.newAccountURLHandlerInstallCommand(),
		a.newAccountListCommand(),
		a.newAccountRemoveCommand(),
		a.newAccountUseCommand(),
		a.newAccountCheckCommand(),
		a.newAccountRefreshCommand(),
		a.newAccountExportCommand(),
		a.newAccountPoolCommand(),
	)
	a.bindNoInput(cmd)
	command.Bind(cmd, command.Execution{})
	return cmd
}

func (a controller) newAccountExportCommand() *cobra.Command {
	opts := accountExportOptions{}
	cmd := &cobra.Command{
		Use:   "export [UID]",
		Short: "Export stored authentication",
		Args:  a.requireMaxArgs(1, "pixiv auth export [UID]"),
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
			services := a.services()
			if err := services.require(); err != nil {
				return err
			}
			result, err := services.Account.export(accountExportRequest{UserID: userID, All: opts.all})
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("output") {
				if strings.TrimSpace(opts.output) == "" {
					return errors.New("--output requires a path")
				}
				if a.support.WriteBundle == nil {
					return errors.New("auth export bundle writer is not configured")
				}
				if err := a.support.WriteBundle(opts.output, result.Bundle, opts.force); err != nil {
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
	cmd.Flags().BoolVar(&opts.all, "all", false, "export all stored accounts; cannot be combined with UID")
	cmd.Flags().StringVar(&opts.output, "output", "", "write a versioned authentication bundle to PATH")
	cmd.Flags().BoolVar(&opts.force, "force", false, "replace an existing output file; requires --output")
	a.bindTextValue(cmd, 0, 1, 0, func(_ *cobra.Command, _ []string) bool { return !opts.all })
	command.Bind(cmd, command.AuthExport())
	return cmd
}

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

func (a controller) newAccountListCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List accounts",
		Args:  a.requireExactArgs(0, "pixiv auth list [--json]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.accountList(cmd.Context(), jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print JSON")
	a.bindNoInput(cmd)
	command.Bind(cmd, command.AuthAccount())
	return cmd
}

func (a controller) accountList(ctx context.Context, jsonOut bool) error {
	services := a.services()
	if err := services.require(); err != nil {
		return err
	}
	result, err := services.Account.listWithContext(ctx)
	if err != nil {
		return err
	}
	out := result
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
		credential := "-"
		if acct.HasToken {
			credential = "✓"
		}
		fmt.Fprintf(a.out, "%s %s uid:%d", marker, credential, acct.UserID)
		if acct.Username != "" {
			fmt.Fprintf(a.out, " username:%s", acct.Username)
		}
		if acct.Schedulable != nil {
			fmt.Fprintf(a.out, " schedulable:%t eligible:%t", *acct.Schedulable, *acct.Eligible)
			if acct.PoolFrozenUntil != nil {
				fmt.Fprintf(a.out, " frozen_until:%s", acct.PoolFrozenUntil.UTC().Format(time.RFC3339))
			}
		}
		fmt.Fprintln(a.out)
	}
	return nil
}

func (a controller) newAccountPoolCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pool",
		Short: "Manage account pool scheduling",
		Args:  a.requireExactArgs(0, "pixiv auth pool <command>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		a.newAccountPoolStatusCommand(),
		a.newAccountPoolChangeCommand("enable", true),
		a.newAccountPoolChangeCommand("disable", false),
	)
	a.bindNoInput(cmd)
	command.Bind(cmd, command.Execution{})
	return cmd
}

func (a controller) newAccountPoolStatusCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show account pool scheduling status",
		Args:  a.requireExactArgs(0, "pixiv auth pool status [--json]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			services := a.services()
			if err := services.require(); err != nil {
				return err
			}
			result, err := services.Account.poolStatus(cmd.Context())
			if err != nil {
				return err
			}
			out := result
			if jsonOut {
				return a.printJSON(out)
			}
			fmt.Fprintf(a.out, "enabled:%t strategy:%s\n", out.Enabled, out.Strategy)
			if out.EarliestFrozenUntil != nil {
				fmt.Fprintf(a.out, "earliest_frozen_until:%s\n", out.EarliestFrozenUntil.UTC().Format(time.RFC3339))
			}
			for _, account := range out.Accounts {
				fmt.Fprintf(a.out, "uid:%d schedulable:%t eligible:%t", account.UserID, valueOrFalse(account.Schedulable), valueOrFalse(account.Eligible))
				if account.PoolFrozenUntil != nil {
					fmt.Fprintf(a.out, " frozen_until:%s", account.PoolFrozenUntil.UTC().Format(time.RFC3339))
				}
				fmt.Fprintln(a.out)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print JSON")
	a.bindNoInput(cmd)
	command.Bind(cmd, command.AuthAccount())
	return cmd
}

func (a controller) newAccountPoolChangeCommand(name string, schedulable bool) *cobra.Command {
	opts := accountPoolChangeOptions{}
	cmd := &cobra.Command{
		Use:   name + " [UID...]",
		Short: fmt.Sprintf("%s accounts in the pool", strings.Title(name)),
		Args: func(_ *cobra.Command, args []string) error {
			if opts.all && len(args) > 0 {
				return errors.New("--all cannot be combined with UIDs")
			}
			if !opts.all && len(args) == 0 {
				return errors.New("at least one UID is required unless --all is used")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			services := a.services()
			if err := services.require(); err != nil {
				return err
			}
			userIDs := make([]int64, 0, len(args))
			for _, raw := range args {
				userID, err := parseAuthUID(raw)
				if err != nil {
					return err
				}
				userIDs = append(userIDs, userID)
			}
			if err := services.Account.setPool(cmd.Context(), userIDs, schedulable, opts.all); err != nil {
				return err
			}
			if opts.jsonOut {
				return a.printJSON(struct {
					Schedulable bool    `json:"schedulable"`
					All         bool    `json:"all"`
					UserIDs     []int64 `json:"user_ids,omitempty"`
				}{Schedulable: schedulable, All: opts.all, UserIDs: userIDs})
			}
			if opts.all {
				fmt.Fprintf(a.out, "schedulable:%t all\n", schedulable)
			} else {
				fmt.Fprintf(a.out, "schedulable:%t uid:%s\n", schedulable, formatUIDs(userIDs))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&opts.all, "all", false, "apply to every stored account")
	cmd.Flags().BoolVar(&opts.jsonOut, "json", false, "print JSON")
	a.bindTextValue(cmd, 0, -1, 0, func(_ *cobra.Command, _ []string) bool { return !opts.all })
	command.Bind(cmd, command.AuthAccount())
	return cmd
}

func formatUIDs(userIDs []int64) string {
	values := make([]string, 0, len(userIDs))
	for _, userID := range userIDs {
		values = append(values, strconv.FormatInt(userID, 10))
	}
	return strings.Join(values, ",")
}

func valueOrFalse(value *bool) bool {
	return value != nil && *value
}

func (a controller) newAccountRemoveCommand() *cobra.Command {
	opts := accountRemoveOptions{}
	cmd := &cobra.Command{
		Use:   "remove [UID]",
		Short: "Remove an account",
		Args:  a.requireMaxArgs(1, "pixiv auth remove [UID] [--yes]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.accountRemove(cmd.Context(), args, opts)
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.jsonOut, "json", false, "print JSON")
	flags.BoolVar(&opts.yes, "yes", false, "skip confirmation in interactive terminals")
	a.bindTextValue(cmd, 0, 1, 0, nil)
	command.Bind(cmd, command.AuthAccount())
	return cmd
}

func (a controller) accountRemove(ctx context.Context, args []string, opts accountRemoveOptions) error {
	services := a.services()
	if err := services.require(); err != nil {
		return err
	}
	list, err := services.Account.listWithContext(ctx)
	if err != nil {
		return err
	}
	userID, err := a.resolveExistingUID(list, args, "Select account to remove")
	if err != nil {
		return err
	}
	if a.canPrompt() && !opts.yes {
		ok, err := a.promptConfirm(fmt.Sprintf("Remove uid %d?", userID), false)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("account removal canceled")
		}
	}
	_, defaultUserID, err := services.Account.remove(userID)
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

func (a controller) newAccountUseCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "use [UID]",
		Short: "Set the default account",
		Args:  a.requireMaxArgs(1, "pixiv auth use [UID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.accountUse(cmd.Context(), args, jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print JSON")
	a.bindTextValue(cmd, 0, 1, 0, nil)
	command.Bind(cmd, command.AuthAccount())
	return cmd
}

func (a controller) accountUse(ctx context.Context, args []string, jsonOut bool) error {
	services := a.services()
	if err := services.require(); err != nil {
		return err
	}
	list, err := services.Account.listWithContext(ctx)
	if err != nil {
		return err
	}
	userID, err := a.resolveExistingUID(list, args, "Select default account")
	if err != nil {
		return err
	}
	userID, err = services.Account.use(userID)
	if err != nil {
		return err
	}
	if jsonOut {
		return a.printJSON(map[string]int64{"default_user_id": userID})
	}
	fmt.Fprintf(a.out, "default uid: %d\n", userID)
	return nil
}

func (a controller) newAccountCheckCommand() *cobra.Command {
	opts := accountCheckOptions{}
	cmd := &cobra.Command{
		Use:   "check [UID]",
		Short: "Validate an account token",
		Args:  a.requireMaxArgs(1, "pixiv auth check [UID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			userID := int64(0)
			if len(args) == 1 {
				var err error
				userID, err = parseAuthUID(args[0])
				if err != nil {
					return err
				}
			}
			return a.accountCheck(cmd, userID, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.jsonOut, "json", false, "print JSON")
	a.bindTextValue(cmd, 0, 1, 0, nil)
	a.bindProxyFlags(cmd, &opts.ProxyOptions)
	command.Bind(cmd, command.AuthAccount())
	return cmd
}

func (a controller) accountCheck(cmd *cobra.Command, userID int64, opts accountCheckOptions) error {
	proxyOverride, err := proxyOverrideFromFlags(cmd, opts.ProxyOptions)
	if err != nil {
		return err
	}
	services := a.services()
	if err := services.require(); err != nil {
		return err
	}
	result, err := services.Account.checkWithRequest(cmd.Context(), accountCheckRequest{
		UserID:             userID,
		HTTPSProxyOverride: proxyOverride,
	})
	if err != nil {
		return err
	}
	out := result
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

func (a controller) newAccountRefreshCommand() *cobra.Command {
	opts := accountRefreshOptions{}
	cmd := &cobra.Command{
		Use:   "refresh [UID]",
		Short: "Refresh account credentials and membership status",
		Args:  a.requireMaxArgs(1, "pixiv auth refresh [UID] [--all]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.all && len(args) != 0 {
				return errors.New("--all cannot be combined with a UID")
			}
			userID := int64(0)
			if len(args) == 1 {
				var err error
				userID, err = parseAuthUID(args[0])
				if err != nil {
					return err
				}
			}
			return a.accountRefresh(cmd, userID, opts)
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.all, "all", false, "refresh every stored account")
	flags.BoolVar(&opts.jsonOut, "json", false, "print JSON")
	a.bindTextValue(cmd, 0, 1, 0, func(_ *cobra.Command, _ []string) bool { return !opts.all })
	a.bindProxyFlags(cmd, &opts.ProxyOptions)
	command.Bind(cmd, command.AuthAccount())
	return cmd
}

func (a controller) accountRefresh(cmd *cobra.Command, userID int64, opts accountRefreshOptions) error {
	proxyOverride, err := proxyOverrideFromFlags(cmd, opts.ProxyOptions)
	if err != nil {
		return err
	}
	services := a.services()
	if err := services.require(); err != nil {
		return err
	}
	userIDs := []int64{userID}
	if opts.all {
		list, err := services.Account.listWithContext(cmd.Context())
		if err != nil {
			return err
		}
		userIDs = make([]int64, 0, len(list.Accounts))
		for _, account := range list.Accounts {
			userIDs = append(userIDs, account.UserID)
		}
		if len(userIDs) == 0 {
			return errors.New("no accounts")
		}
	}
	results := make([]accountOut, 0, len(userIDs))
	for _, selectedUserID := range userIDs {
		result, err := services.Account.refreshWithRequest(cmd.Context(), accountRefreshRequest{
			UserID:             selectedUserID,
			HTTPSProxyOverride: proxyOverride,
		})
		if err != nil {
			return err
		}
		results = append(results, result)
	}
	if opts.jsonOut {
		return a.printJSON(struct {
			Accounts []accountOut `json:"accounts"`
		}{Accounts: results})
	}
	for _, account := range results {
		premium := "unknown"
		if account.PremiumStatus != nil {
			premium = command.TextBool(*account.PremiumStatus)
		}
		fmt.Fprintf(a.out, "✓ refreshed uid:%d premium:%s\n", account.UserID, premium)
	}
	return nil
}

func (a controller) resolveExistingUID(list accountListOut, args []string, promptLabel string) (int64, error) {
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		return parseAuthUID(args[0])
	}
	if !a.canPrompt() {
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
	selected, err := a.promptSelect(promptLabel, options)
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(selected)
	if len(fields) == 0 {
		return 0, errors.New("uid cannot be empty")
	}
	return parseAuthUID(fields[0])
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
