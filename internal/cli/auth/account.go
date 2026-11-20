package auth

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/display"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
	"github.com/spf13/cobra"
)

type accountImportOptions struct {
	environment.ProxyOptions
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
	environment.ProxyOptions
	jsonOut bool
}

type accountRefreshOptions struct {
	environment.ProxyOptions
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
		Short: "help.manage_local_pixiv_authentication",
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
	environment.Bind(cmd, environment.Execution{})
	return cmd
}

func (a controller) newAccountExportCommand() *cobra.Command {
	opts := accountExportOptions{}
	cmd := &cobra.Command{
		Use:   "export [UID]",
		Short: "help.export_stored_authentication",
		Args:  a.env.MaxArgs(1, "pixiv auth export [UID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.force && !cmd.Flags().Changed("output") {
				return i18n.Errorf("error.force_requires_output")
			}
			if opts.all && len(args) != 0 {
				return i18n.Errorf("error.all_cannot_be_combined_with_a_uid")
			}
			userID := int64(0)
			if len(args) == 1 {
				parsed, err := parseAuthExportUID(args[0])
				if err != nil {
					return err
				}
				userID = parsed
			}
			service, err := a.accountService(cmd.Context())
			if err != nil {
				return err
			}
			result, err := service.export(accountExportRequest{UserID: userID, All: opts.all})
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("output") {
				if strings.TrimSpace(opts.output) == "" {
					return i18n.Errorf("error.output_requires_a_path")
				}
				if err := a.support.WriteBundle(opts.output, result.Bundle, opts.force); err != nil {
					return err
				}
				return writeAuthExportStdout(a.env.Out, []byte(fmt.Sprintf(a.env.Text("format.exported_accounts_to"), result.AccountCount, display.SafeLine(opts.output))))
			}
			if opts.all {
				return writeAuthExportStdout(a.env.Out, result.Bundle)
			}
			return writeAuthExportStdout(a.env.Out, []byte(result.RefreshToken+"\n"))
		},
	}
	cmd.Flags().BoolVar(&opts.all, "all", false, "flag.export_all_stored_accounts_cannot_be_combined_with")
	cmd.Flags().StringVar(&opts.output, "output", "", "flag.write_a_versioned_authentication_bundle_to_path")
	cmd.Flags().BoolVar(&opts.force, "force", false, "flag.replace_an_existing_output_file_requires_output")
	a.env.BindTextValueWhen(cmd, 0, 1, 0, func(_ *cobra.Command, _ []string) bool { return !opts.all })
	environment.Bind(cmd, environment.Execution{})
	return cmd
}

func (a controller) newAccountListCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "help.list_accounts",
		Args:  a.env.ExactArgs(0, "pixiv auth list [--json]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.accountList(cmd.Context(), jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "flag.print_json")
	a.env.BindNoInput(cmd)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a controller) accountList(ctx context.Context, jsonOut bool) error {
	service, err := a.accountService(ctx)
	if err != nil {
		return err
	}
	result, err := service.listWithContext(ctx)
	if err != nil {
		return err
	}
	out := result
	if jsonOut {
		return a.env.PrintJSON(out)
	}
	if len(out.Accounts) == 0 {
		_, err := fmt.Fprintln(a.env.Err, a.env.Text("output.no_results"))
		return err
	}
	for _, acct := range out.Accounts {
		if err := printAccount(a.env.Out, a.env.Language, acct); err != nil {
			return err
		}
	}
	return nil
}

func printAccount(out io.Writer, language i18n.Locale, acct accountOut) error {
	marker := " "
	if acct.Default {
		marker = "*"
	}
	line := marker + " " + display.Identity(acct.Username, acct.UserID)
	if valueOrFalse(acct.PremiumStatus) {
		line += "  " + language.Text("account.premium")
	}
	if valueOrFalse(acct.Schedulable) {
		line += "  " + language.Text("account.pool")
	}
	if acct.PoolFrozenUntil != nil {
		line += "  " + language.Text("account.frozen") + acct.PoolFrozenUntil.UTC().Format(time.RFC3339)
	}
	_, err := fmt.Fprintln(out, line)
	return err
}

func accountIdentity(list accountListOut, userID int64) string {
	for _, acct := range list.Accounts {
		if acct.UserID == userID {
			return display.Identity(acct.Username, userID)
		}
	}
	return display.Identity("", userID)
}

func (a controller) newAccountPoolCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pool",
		Short: "help.manage_account_pool_scheduling",
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
	environment.Bind(cmd, environment.Execution{})
	return cmd
}

func (a controller) newAccountPoolStatusCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "help.show_account_pool_scheduling_status",
		Args:  a.env.ExactArgs(0, "pixiv auth pool status [--json]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			service, err := a.accountService(cmd.Context())
			if err != nil {
				return err
			}
			result, err := service.poolStatus(cmd.Context())
			if err != nil {
				return err
			}
			out := result
			if jsonOut {
				return a.env.PrintJSON(out)
			}
			state := a.env.Text("state.disabled")
			if out.Enabled {
				state = a.env.Text("state.enabled")
			}
			if _, err := fmt.Fprintf(a.env.Out, a.env.Text("output.account_pool"), state, display.SafeLine(out.Strategy)); err != nil {
				return err
			}
			for _, acct := range out.Accounts {
				if err := printAccount(a.env.Out, a.env.Language, acct); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "flag.print_json")
	a.env.BindNoInput(cmd)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a controller) newAccountPoolChangeCommand(name string, schedulable bool) *cobra.Command {
	opts := accountPoolChangeOptions{}
	cmd := &cobra.Command{
		Use:   name + " [UID...]",
		Short: "help.pool_" + name,
		Args: func(_ *cobra.Command, args []string) error {
			if opts.all && len(args) > 0 {
				return i18n.Errorf("error.all_cannot_be_combined_with_uids")
			}
			if !opts.all && len(args) == 0 {
				return i18n.Errorf("error.at_least_one_uid_is_required_unless_all")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			service, err := a.accountService(cmd.Context())
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
			if err := service.setPool(cmd.Context(), userIDs, schedulable, opts.all); err != nil {
				return err
			}
			if opts.jsonOut {
				return a.env.PrintJSON(struct {
					Schedulable bool    `json:"schedulable"`
					All         bool    `json:"all"`
					UserIDs     []int64 `json:"user_ids,omitempty"`
				}{Schedulable: schedulable, All: opts.all, UserIDs: userIDs})
			}
			accounts := formatUIDs(userIDs)
			if opts.all {
				accounts = a.env.Text("account.all")
			}
			if schedulable {
				_, err = fmt.Fprintf(a.env.Out, a.env.Text("output.added_to_the_account_pool"), accounts)
			} else {
				_, err = fmt.Fprintf(a.env.Out, a.env.Text("output.removed_from_the_account_pool"), accounts)
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&opts.all, "all", false, "flag.apply_to_every_stored_account")
	cmd.Flags().BoolVar(&opts.jsonOut, "json", false, "flag.print_json")
	a.env.BindTextValueWhen(cmd, 0, -1, 0, func(_ *cobra.Command, _ []string) bool { return !opts.all })
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func formatUIDs(userIDs []int64) string {
	values := make([]string, 0, len(userIDs))
	for _, userID := range userIDs {
		values = append(values, strconv.FormatInt(userID, 10))
	}
	return strings.Join(values, ", ")
}

func valueOrFalse(value *bool) bool {
	return value != nil && *value
}

func (a controller) newAccountRemoveCommand() *cobra.Command {
	opts := accountRemoveOptions{}
	cmd := &cobra.Command{
		Use:   "remove [UID]",
		Short: "help.remove_an_account",
		Args:  a.env.MaxArgs(1, "pixiv auth remove [UID] [--yes]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.accountRemove(cmd.Context(), args, opts)
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.jsonOut, "json", false, "flag.print_json")
	flags.BoolVar(&opts.yes, "yes", false, "flag.skip_confirmation_in_interactive_terminals")
	a.env.BindTextValue(cmd, 0, 1, 0)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a controller) accountRemove(ctx context.Context, args []string, opts accountRemoveOptions) error {
	service, err := a.accountService(ctx)
	if err != nil {
		return err
	}
	list, err := service.listWithContext(ctx)
	if err != nil {
		return err
	}
	userID, err := a.resolveExistingUID(list, args, a.env.Text("account.select_remove"))
	if err != nil {
		return err
	}
	if a.env.CanPrompt() && !opts.yes {
		ok, err := a.env.PromptConfirm(fmt.Sprintf(a.env.Text("format.remove_uid"), userID), false)
		if err != nil {
			return err
		}
		if !ok {
			return i18n.Errorf("error.account_removal_canceled")
		}
	}
	_, defaultUserID, err := service.remove(userID)
	if err != nil {
		return err
	}
	if opts.jsonOut {
		return a.env.PrintJSON(map[string]int64{"removed_user_id": userID, "default_user_id": defaultUserID})
	}
	line := a.env.Text("account.removed") + accountIdentity(list, userID)
	if list.defaultUserID() == userID && defaultUserID != 0 {
		line += a.env.Text("account.now_using") + accountIdentity(list, defaultUserID)
	}
	_, err = fmt.Fprintln(a.env.Out, line)
	return err
}

func (a controller) newAccountUseCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "use [UID]",
		Short: "help.set_the_default_account",
		Args:  a.env.MaxArgs(1, "pixiv auth use [UID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.accountUse(cmd.Context(), args, jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "flag.print_json")
	a.env.BindTextValue(cmd, 0, 1, 0)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a controller) accountUse(ctx context.Context, args []string, jsonOut bool) error {
	service, err := a.accountService(ctx)
	if err != nil {
		return err
	}
	list, err := service.listWithContext(ctx)
	if err != nil {
		return err
	}
	userID, err := a.resolveExistingUID(list, args, "Select default account")
	if err != nil {
		return err
	}
	userID, err = service.use(userID)
	if err != nil {
		return err
	}
	if jsonOut {
		return a.env.PrintJSON(map[string]int64{"default_user_id": userID})
	}
	_, err = fmt.Fprintf(a.env.Out, a.env.Text("output.now_using"), accountIdentity(list, userID))
	return err
}

func (a controller) newAccountCheckCommand() *cobra.Command {
	opts := accountCheckOptions{}
	cmd := &cobra.Command{
		Use:   "check [UID]",
		Short: "help.validate_an_account_token",
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
	cmd.Flags().BoolVar(&opts.jsonOut, "json", false, "flag.print_json")
	a.env.BindTextValue(cmd, 0, 1, 0)
	a.env.BindProxyFlags(cmd, &opts.ProxyOptions)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a controller) accountCheck(cmd *cobra.Command, userID int64, opts accountCheckOptions) error {
	proxyOverride, err := environment.ProxyOverride(cmd, opts.ProxyOptions)
	if err != nil {
		return err
	}
	service, err := a.accountService(cmd.Context())
	if err != nil {
		return err
	}
	result, err := service.checkWithRequest(cmd.Context(), accountCheckRequest{
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
	if _, err := fmt.Fprintf(a.env.Out, a.env.Text("output.token_valid_for"), display.Identity(result.Username, result.UserID)); err != nil {
		return err
	}
	return nil
}

func (a controller) newAccountRefreshCommand() *cobra.Command {
	opts := accountRefreshOptions{}
	cmd := &cobra.Command{
		Use:   "refresh [UID]",
		Short: "help.refresh_account_credentials_and_membership_status",
		Args:  a.env.MaxArgs(1, "pixiv auth refresh [UID] [--all]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.all && len(args) != 0 {
				return i18n.Errorf("error.all_cannot_be_combined_with_a_uid")
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
	flags.BoolVar(&opts.all, "all", false, "flag.refresh_every_stored_account")
	flags.BoolVar(&opts.jsonOut, "json", false, "flag.print_json")
	a.env.BindTextValueWhen(cmd, 0, 1, 0, func(_ *cobra.Command, _ []string) bool { return !opts.all })
	a.env.BindProxyFlags(cmd, &opts.ProxyOptions)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a controller) accountRefresh(cmd *cobra.Command, userID int64, opts accountRefreshOptions) error {
	proxyOverride, err := environment.ProxyOverride(cmd, opts.ProxyOptions)
	if err != nil {
		return err
	}
	service, err := a.accountService(cmd.Context())
	if err != nil {
		return err
	}
	userIDs := []int64{userID}
	if opts.all {
		list, err := service.listWithContext(cmd.Context())
		if err != nil {
			return err
		}
		userIDs = make([]int64, 0, len(list.Accounts))
		for _, account := range list.Accounts {
			userIDs = append(userIDs, account.UserID)
		}
		if len(userIDs) == 0 {
			return i18n.Errorf("error.no_accounts")
		}
	}
	results := make([]accountOut, 0, len(userIDs))
	for _, selectedUserID := range userIDs {
		result, err := service.refreshWithRequest(cmd.Context(), accountRefreshRequest{
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
		label := ""
		if valueOrFalse(account.PremiumStatus) {
			label = " · " + a.env.Text("account.premium")
		}
		if _, err := fmt.Fprintf(a.env.Out, a.env.Text("output.refreshed"), display.Identity(account.Username, account.UserID), label); err != nil {
			return err
		}
	}
	return nil
}

func (a controller) resolveExistingUID(list accountListOut, args []string, promptLabel string) (int64, error) {
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		return parseAuthUID(args[0])
	}
	if !a.env.CanPrompt() {
		return 0, i18n.Errorf("error.uid_is_required")
	}
	if len(list.Accounts) == 0 {
		return 0, i18n.Errorf("error.no_accounts")
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
		return 0, i18n.Errorf("error.uid_cannot_be_empty")
	}
	return parseAuthUID(fields[0])
}
