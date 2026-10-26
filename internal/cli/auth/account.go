// Pixiv auth account commands: list, import/export, selection, checks, refresh,
// and pool state. Account domain access is adapted in account_service.go.

package auth

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
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
		Args:  a.env.ExactArgs(0, "pixiv auth <command>"),
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
	a.env.BindNoInput(cmd)
	command.Bind(cmd, command.Execution{})
	return cmd
}

func (a controller) newAccountExportCommand() *cobra.Command {
	opts := accountExportOptions{}
	cmd := &cobra.Command{
		Use:   "export [UID]",
		Short: "Export stored authentication",
		Args:  a.env.MaxArgs(1, "pixiv auth export [UID]"),
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
			services, err := a.services()
			if err != nil {
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
				if err := a.support.WriteBundle(opts.output, result.Bundle, opts.force); err != nil {
					return err
				}
				return writeAuthExportStdout(a.env.Out, []byte(fmt.Sprintf("output: %s\naccounts: %d\n", opts.output, result.AccountCount)))
			}
			if opts.all {
				return writeAuthExportStdout(a.env.Out, result.Bundle)
			}
			return writeAuthExportStdout(a.env.Out, []byte(result.RefreshToken+"\n"))
		},
	}
	cmd.Flags().BoolVar(&opts.all, "all", false, "export all stored accounts; cannot be combined with UID")
	cmd.Flags().StringVar(&opts.output, "output", "", "write a versioned authentication bundle to PATH")
	cmd.Flags().BoolVar(&opts.force, "force", false, "replace an existing output file; requires --output")
	a.env.BindTextValueWhen(cmd, 0, 1, 0, func(_ *cobra.Command, _ []string) bool { return !opts.all })
	command.Bind(cmd, command.AuthExport())
	return cmd
}

func (a controller) newAccountListCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List accounts",
		Args:  a.env.ExactArgs(0, "pixiv auth list [--json]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.accountList(cmd.Context(), jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print JSON")
	a.env.BindNoInput(cmd)
	command.Bind(cmd, command.AuthAccount())
	return cmd
}

func (a controller) accountList(ctx context.Context, jsonOut bool) error {
	services, err := a.services()
	if err != nil {
		return err
	}
	result, err := services.Account.listWithContext(ctx)
	if err != nil {
		return err
	}
	out := result
	if jsonOut {
		return a.env.PrintJSON(out)
	}
	if len(out.Accounts) == 0 {
		fmt.Fprintln(a.env.Out, "no accounts")
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
		fmt.Fprintf(a.env.Out, "%s %s uid:%d", marker, credential, acct.UserID)
		if acct.Username != "" {
			fmt.Fprintf(a.env.Out, " username:%s", acct.Username)
		}
		if acct.Schedulable != nil {
			fmt.Fprintf(a.env.Out, " schedulable:%t eligible:%t", *acct.Schedulable, *acct.Eligible)
			if acct.PoolFrozenUntil != nil {
				fmt.Fprintf(a.env.Out, " frozen_until:%s", acct.PoolFrozenUntil.UTC().Format(time.RFC3339))
			}
		}
		fmt.Fprintln(a.env.Out)
	}
	return nil
}

func (a controller) newAccountPoolCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pool",
		Short: "Manage account pool scheduling",
		Args:  a.env.ExactArgs(0, "pixiv auth pool <command>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		a.newAccountPoolStatusCommand(),
		a.newAccountPoolChangeCommand("enable", true),
		a.newAccountPoolChangeCommand("disable", false),
	)
	a.env.BindNoInput(cmd)
	command.Bind(cmd, command.Execution{})
	return cmd
}

func (a controller) newAccountPoolStatusCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show account pool scheduling status",
		Args:  a.env.ExactArgs(0, "pixiv auth pool status [--json]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			services, err := a.services()
			if err != nil {
				return err
			}
			result, err := services.Account.poolStatus(cmd.Context())
			if err != nil {
				return err
			}
			out := result
			if jsonOut {
				return a.env.PrintJSON(out)
			}
			fmt.Fprintf(a.env.Out, "enabled:%t strategy:%s\n", out.Enabled, out.Strategy)
			if out.EarliestFrozenUntil != nil {
				fmt.Fprintf(a.env.Out, "earliest_frozen_until:%s\n", out.EarliestFrozenUntil.UTC().Format(time.RFC3339))
			}
			for _, account := range out.Accounts {
				fmt.Fprintf(a.env.Out, "uid:%d schedulable:%t eligible:%t", account.UserID, valueOrFalse(account.Schedulable), valueOrFalse(account.Eligible))
				if account.PoolFrozenUntil != nil {
					fmt.Fprintf(a.env.Out, " frozen_until:%s", account.PoolFrozenUntil.UTC().Format(time.RFC3339))
				}
				fmt.Fprintln(a.env.Out)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print JSON")
	a.env.BindNoInput(cmd)
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
			services, err := a.services()
			if err != nil {
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
				return a.env.PrintJSON(struct {
					Schedulable bool    `json:"schedulable"`
					All         bool    `json:"all"`
					UserIDs     []int64 `json:"user_ids,omitempty"`
				}{Schedulable: schedulable, All: opts.all, UserIDs: userIDs})
			}
			if opts.all {
				fmt.Fprintf(a.env.Out, "schedulable:%t all\n", schedulable)
			} else {
				fmt.Fprintf(a.env.Out, "schedulable:%t uid:%s\n", schedulable, formatUIDs(userIDs))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&opts.all, "all", false, "apply to every stored account")
	cmd.Flags().BoolVar(&opts.jsonOut, "json", false, "print JSON")
	a.env.BindTextValueWhen(cmd, 0, -1, 0, func(_ *cobra.Command, _ []string) bool { return !opts.all })
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
		Args:  a.env.MaxArgs(1, "pixiv auth remove [UID] [--yes]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.accountRemove(cmd.Context(), args, opts)
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.jsonOut, "json", false, "print JSON")
	flags.BoolVar(&opts.yes, "yes", false, "skip confirmation in interactive terminals")
	a.env.BindTextValueWhen(cmd, 0, 1, 0, nil)
	command.Bind(cmd, command.AuthAccount())
	return cmd
}

func (a controller) accountRemove(ctx context.Context, args []string, opts accountRemoveOptions) error {
	services, err := a.services()
	if err != nil {
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
	if a.env.CanPrompt() && !opts.yes {
		ok, err := a.env.PromptConfirm(fmt.Sprintf("Remove uid %d?", userID), false)
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
		return a.env.PrintJSON(map[string]int64{"removed_user_id": userID, "default_user_id": defaultUserID})
	}
	fmt.Fprintf(a.env.Out, "account uid:%d removed\n", userID)
	if defaultUserID != 0 {
		fmt.Fprintf(a.env.Out, "default uid: %d\n", defaultUserID)
	}
	return nil
}

func (a controller) newAccountUseCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "use [UID]",
		Short: "Set the default account",
		Args:  a.env.MaxArgs(1, "pixiv auth use [UID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.accountUse(cmd.Context(), args, jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print JSON")
	a.env.BindTextValueWhen(cmd, 0, 1, 0, nil)
	command.Bind(cmd, command.AuthAccount())
	return cmd
}

func (a controller) accountUse(ctx context.Context, args []string, jsonOut bool) error {
	services, err := a.services()
	if err != nil {
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
		return a.env.PrintJSON(map[string]int64{"default_user_id": userID})
	}
	fmt.Fprintf(a.env.Out, "default uid: %d\n", userID)
	return nil
}

func (a controller) newAccountCheckCommand() *cobra.Command {
	opts := accountCheckOptions{}
	cmd := &cobra.Command{
		Use:   "check [UID]",
		Short: "Validate an account token",
		Args:  a.env.MaxArgs(1, "pixiv auth check [UID]"),
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
	a.env.BindTextValueWhen(cmd, 0, 1, 0, nil)
	a.env.BindProxyFlags(cmd, &opts.ProxyOptions)
	command.Bind(cmd, command.AuthAccount())
	return cmd
}

func (a controller) accountCheck(cmd *cobra.Command, userID int64, opts accountCheckOptions) error {
	proxyOverride, err := command.ProxyOverride(cmd, opts.ProxyOptions)
	if err != nil {
		return err
	}
	services, err := a.services()
	if err != nil {
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
		return a.env.PrintJSON(out)
	}
	if userID == 0 {
		fmt.Fprintf(a.env.Out, "token ok, uid:%d\n", result.UserID)
	} else {
		fmt.Fprintf(a.env.Out, "account uid:%d ok\n", result.UserID)
	}
	if result.Username != "" {
		fmt.Fprintf(a.env.Out, "username:%s\n", result.Username)
	}
	if result.Warning != "" {
		fmt.Fprintf(a.env.Err, "warning: %s\n", result.Warning)
	}
	return nil
}

func (a controller) newAccountRefreshCommand() *cobra.Command {
	opts := accountRefreshOptions{}
	cmd := &cobra.Command{
		Use:   "refresh [UID]",
		Short: "Refresh account credentials and membership status",
		Args:  a.env.MaxArgs(1, "pixiv auth refresh [UID] [--all]"),
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
	a.env.BindTextValueWhen(cmd, 0, 1, 0, func(_ *cobra.Command, _ []string) bool { return !opts.all })
	a.env.BindProxyFlags(cmd, &opts.ProxyOptions)
	command.Bind(cmd, command.AuthAccount())
	return cmd
}

func (a controller) accountRefresh(cmd *cobra.Command, userID int64, opts accountRefreshOptions) error {
	proxyOverride, err := command.ProxyOverride(cmd, opts.ProxyOptions)
	if err != nil {
		return err
	}
	services, err := a.services()
	if err != nil {
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
		return a.env.PrintJSON(struct {
			Accounts []accountOut `json:"accounts"`
		}{Accounts: results})
	}
	for _, account := range results {
		premium := "unknown"
		if account.PremiumStatus != nil {
			premium = command.TextBool(*account.PremiumStatus)
		}
		fmt.Fprintf(a.env.Out, "✓ refreshed uid:%d premium:%s\n", account.UserID, premium)
	}
	return nil
}

func (a controller) resolveExistingUID(list accountListOut, args []string, promptLabel string) (int64, error) {
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		return parseAuthUID(args[0])
	}
	if !a.env.CanPrompt() {
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
	selected, err := a.env.PromptSelect(promptLabel, options)
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(selected)
	if len(fields) == 0 {
		return 0, errors.New("uid cannot be empty")
	}
	return parseAuthUID(fields[0])
}
