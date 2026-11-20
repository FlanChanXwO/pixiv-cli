package auth

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	fanboxcli "github.com/FlanChanXwO/pixiv-cli/internal/cli/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/internal/display"
	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
	"github.com/spf13/cobra"
)

// accountOut 定义 FANBOX 账号的安全输出摘要；SessionID 永不进入输出。
type accountOut struct {
	UserID      int64  `json:"user_id"`
	DisplayName string `json:"display_name,omitempty"`
	CreatorID   string `json:"creator_id,omitempty"`
	Default     bool   `json:"default"`
}

type accountListOut struct {
	DefaultUserID int64        `json:"default_user_id,omitempty"`
	Accounts      []accountOut `json:"accounts"`
}

func accountOutFrom(userID int64, displayName, creatorID string, isDefault bool) accountOut {
	return accountOut{UserID: userID, DisplayName: displayName, CreatorID: creatorID, Default: isDefault}
}

func (a commandRunner) newImportCommand() *cobra.Command {
	opts := importOptions{}
	cmd := &cobra.Command{
		Use:     "import",
		Short:   "help.import_a_fanbox_session",
		Example: "pixiv fanbox auth import",
		Args:    a.env.ExactArgs(0, "pixiv fanbox auth import [--from-browser BROWSER] [--profile ID] [--default]"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runImport(cmd, opts)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.fromBrowser, "from-browser", "", "flag.read_the_fanboxsessid_value_from_a_browser_profile")
	flags.StringVar(&opts.profile, "profile", "", "flag.browser_profile_identifier_when_the_browser_has_multiple")
	flags.BoolVar(&opts.setDefault, "default", false, "flag.set_the_imported_account_as_the_default_fanbox")
	flags.BoolVar(&opts.jsonOut, "json", false, "flag.print_json")
	a.env.BindNoInput(cmd)
	return cmd
}

func (a commandRunner) runImport(cmd *cobra.Command, opts importOptions) error {
	if opts.fromBrowser != "" {
		value, err := a.browser.ReadSession(cmd.Context(), opts.fromBrowser, opts.profile)
		if err != nil {
			return err
		}
		return a.importSession(cmd, value, opts.setDefault, opts.jsonOut)
	}
	var value string
	var err error
	if a.env.CanPrompt() {
		value, err = a.env.PromptSecret(a.env.Text("prompt.fanboxsessid"))
	} else {
		value, err = readSessionInput(a.env.In)
	}
	if err != nil {
		return err
	}
	return a.importSession(cmd, value, opts.setDefault, opts.jsonOut)
}

// readSessionInput 读取完整 stdin，只消费管道输出常见的一个末尾行结束符；session
// 值的其他字节保持 opaque，不能用 TrimSpace 改写。
func readSessionInput(r io.Reader) (string, error) {
	input, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	value := string(input)
	if strings.HasSuffix(value, "\r\n") {
		value = strings.TrimSuffix(value, "\r\n")
	} else {
		value = strings.TrimSuffix(value, "\n")
	}
	if strings.ContainsAny(value, "\r\n") {
		return "", i18n.Errorf("error.fanbox_session_input_must_contain_exactly_one_line")
	}
	if value == "" {
		return "", i18n.Errorf("error.fanbox_session_value_cannot_be_empty")
	}
	return value, nil
}

func (a commandRunner) importSession(cmd *cobra.Command, value string, setDefault, jsonOut bool) error {
	service, err := a.accounts(cmd.Context())
	if err != nil {
		return err
	}
	proxyOverride, err := fanboxcli.ProxyOverride(cmd)
	if err != nil {
		return err
	}
	account, err := service.ImportSession(cmd.Context(), value, setDefault, proxyOverride)
	if err != nil {
		return err
	}
	if jsonOut {
		return a.env.PrintJSON(accountOutFrom(account.UserID, account.DisplayName, account.CreatorID, account.Default))
	}
	_, err = fmt.Fprintf(a.env.Out, a.env.Text("output.imported"), display.Identity(account.DisplayName, account.UserID))
	return err
}

func (a commandRunner) newListCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "help.list_fanbox_accounts",
		Args:  a.env.ExactArgs(0, "pixiv fanbox auth list [--json]"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runList(cmd, jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "flag.print_json")
	a.env.BindNoInput(cmd)
	return cmd
}

func (a commandRunner) runList(cmd *cobra.Command, jsonOut bool) error {
	if _, err := fanboxcli.ProxyOverride(cmd); err != nil {
		return err
	}
	service, err := a.accounts(cmd.Context())
	if err != nil {
		return err
	}
	accounts, err := service.ListAccounts(cmd.Context())
	if err != nil {
		return err
	}
	out := accountListOut{Accounts: make([]accountOut, 0, len(accounts))}
	for _, account := range accounts {
		out.Accounts = append(out.Accounts, accountOutFrom(account.UserID, account.DisplayName, account.CreatorID, account.Default))
		if account.Default {
			out.DefaultUserID = account.UserID
		}
	}
	if jsonOut {
		return a.env.PrintJSON(out)
	}
	if len(out.Accounts) == 0 {
		_, err := fmt.Fprintln(a.env.Err, a.env.Text("output.no_results"))
		return err
	}
	for _, account := range out.Accounts {
		marker := " "
		if account.Default {
			marker = "*"
		}
		line := marker + " " + display.Identity(account.DisplayName, account.UserID)
		if account.CreatorID != "" {
			line += "  " + a.env.Text("fanbox.creator") + display.SafeLine(account.CreatorID)
		}
		if _, err := fmt.Fprintln(a.env.Out, line); err != nil {
			return err
		}
	}
	return nil
}

func (a commandRunner) newUseCommand() *cobra.Command {
	opts := useOptions{}
	cmd := &cobra.Command{
		Use:   "use [UID]",
		Short: "help.set_the_default_fanbox_account",
		Args:  a.env.MaxArgs(1, "pixiv fanbox auth use [UID] | --auto"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runUse(cmd.Context(), args, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.auto, "auto", false, "flag.clear_the_explicit_default_and_use_the_first")
	cmd.Flags().BoolVar(&opts.jsonOut, "json", false, "flag.print_json")
	a.env.BindTextValue(cmd, 0, 1, 0)
	return cmd
}

func (a commandRunner) runUse(ctx context.Context, args []string, opts useOptions) error {
	if opts.auto && len(args) != 0 {
		return i18n.Errorf("error.auto_cannot_be_combined_with_a_uid")
	}
	service, err := a.accounts(ctx)
	if err != nil {
		return err
	}
	if opts.auto {
		if err := service.UseAuto(); err != nil {
			return err
		}
		if opts.jsonOut {
			return a.env.PrintJSON(map[string]any{"default_user_id": 0, "auto": true})
		}
		_, err := fmt.Fprintln(a.env.Out, a.env.Text("output.now_using_the_first_account_automatically"))
		return err
	}
	if len(args) != 1 {
		return i18n.Errorf("error.usage_pixiv_fanbox_auth_use_uid_auto")
	}
	userID, err := parseUID(args[0])
	if err != nil {
		return err
	}
	if err := service.UseAccount(ctx, userID); err != nil {
		return err
	}
	if opts.jsonOut {
		return a.env.PrintJSON(map[string]any{"default_user_id": userID, "auto": false})
	}
	summary, err := service.Status(ctx)
	if err != nil {
		return i18n.Errorf("error.default_account_changed_read_selected_account_identity", err)
	}
	_, err = fmt.Fprintf(a.env.Out, a.env.Text("output.now_using"), display.Identity(summary.DisplayName, summary.UserID))
	return err
}

func (a commandRunner) newRemoveCommand() *cobra.Command {
	opts := removeOptions{}
	cmd := &cobra.Command{
		Use:   "remove UID",
		Short: "help.remove_a_fanbox_account",
		Args:  a.env.ExactArgs(1, "pixiv fanbox auth remove UID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runRemove(cmd.Context(), args, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.jsonOut, "json", false, "flag.print_json")
	cmd.Flags().BoolVar(&opts.yes, "yes", false, "flag.skip_confirmation_in_interactive_terminals")
	a.env.BindTextValue(cmd, 1, 1, 0)
	return cmd
}

func (a commandRunner) runRemove(ctx context.Context, args []string, opts removeOptions) error {
	userID, err := parseUID(args[0])
	if err != nil {
		return err
	}
	service, err := a.accounts(ctx)
	if err != nil {
		return err
	}
	if a.env.CanPrompt() && !opts.yes {
		ok, err := a.env.PromptConfirm(fmt.Sprintf(a.env.Text("format.remove_fanbox_uid"), userID), false)
		if err != nil {
			return err
		}
		if !ok {
			return i18n.Errorf("error.fanbox_account_removal_canceled")
		}
	}
	identity := ""
	if !opts.jsonOut {
		identity, err = fanboxIdentity(ctx, service, userID)
		if err != nil {
			return err
		}
	}
	if err := service.RemoveAccount(ctx, userID); err != nil {
		return err
	}
	if opts.jsonOut {
		return a.env.PrintJSON(map[string]int64{"removed_user_id": userID})
	}
	_, err = fmt.Fprintf(a.env.Out, a.env.Text("output.removed"), identity)
	return err
}

func (a commandRunner) newStatusCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "status [UID]",
		Short: "help.show_the_default_fanbox_account",
		Args:  a.env.MaxArgs(1, "pixiv fanbox auth status [UID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runStatus(cmd, args, jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "flag.print_json")
	a.env.BindTextValue(cmd, 0, 1, 0)
	return cmd
}

func (a commandRunner) runStatus(cmd *cobra.Command, args []string, jsonOut bool) error {
	if _, err := fanboxcli.ProxyOverride(cmd); err != nil {
		return err
	}
	service, err := a.accounts(cmd.Context())
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	if len(args) == 1 {
		userID, err := parseUID(args[0])
		if err != nil {
			return err
		}
		accounts, err := service.ListAccounts(ctx)
		if err != nil {
			return err
		}
		for _, account := range accounts {
			if account.UserID == userID {
				return a.printSummary(account, jsonOut)
			}
		}
		return i18n.Errorf("error.fanbox_account_uid_not_found", userID)
	}
	summary, err := service.Status(ctx)
	if err != nil {
		return err
	}
	return a.printSummary(*summary, jsonOut)
}

func (a commandRunner) printSummary(summary AccountSummary, jsonOut bool) error {
	if jsonOut {
		return a.env.PrintJSON(accountOutFrom(summary.UserID, summary.DisplayName, summary.CreatorID, summary.Default))
	}
	line := display.Identity(summary.DisplayName, summary.UserID)
	if summary.CreatorID != "" {
		line += " · " + a.env.Text("fanbox.creator") + display.SafeLine(summary.CreatorID)
	}
	_, err := fmt.Fprintln(a.env.Out, line)
	return err
}

func parseUID(raw string) (int64, error) {
	userID, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || userID <= 0 {
		return 0, i18n.Errorf("error.fanbox_uid_must_be_a_positive_integer")
	}
	return userID, nil
}

func fanboxIdentity(ctx context.Context, service *Accounts, userID int64) (string, error) {
	accounts, err := service.ListAccounts(ctx)
	if err != nil {
		return "", err
	}
	for _, acct := range accounts {
		if acct.UserID == userID {
			return display.Identity(acct.DisplayName, userID), nil
		}
	}
	return display.Identity("", userID), nil
}
