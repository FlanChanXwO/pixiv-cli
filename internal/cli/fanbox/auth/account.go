package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	fanboxapp "github.com/FlanChanXwO/pixiv-cli/internal/account/fanbox"
	deps "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/fanboxdeps"
	"github.com/spf13/cobra"
)

// accountOut 是 FANBOX 账号的安全输出摘要；SessionID 永不进入输出。
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

func (a command) newImportCommand() *cobra.Command {
	opts := importOptions{}
	cmd := &cobra.Command{
		Use:     "import",
		Short:   "Import a FANBOX session",
		Example: "pixiv fanbox auth import",
		Args:    a.data.RequireExactArgs(0, "pixiv fanbox auth import [--from-browser BROWSER] [--profile ID] [--default]"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runImport(cmd, opts)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.fromBrowser, "from-browser", "", "read the FANBOXSESSID value from a browser profile")
	flags.StringVar(&opts.profile, "profile", "", "browser profile identifier when the browser has multiple profiles")
	flags.BoolVar(&opts.setDefault, "default", false, "set the imported account as the default FANBOX account")
	flags.BoolVar(&opts.jsonOut, "json", false, "print JSON")
	a.data.BindNoInput(cmd)
	return cmd
}

func (a command) runImport(cmd *cobra.Command, opts importOptions) error {
	if opts.fromBrowser != "" {
		value, err := a.data.FanboxBrowserProvider().ReadSession(cmd.Context(), opts.fromBrowser, opts.profile)
		if err != nil {
			return err
		}
		return a.importSession(cmd, value, opts.setDefault, opts.jsonOut)
	}
	var value string
	var err error
	if a.data.CanPrompt() {
		value, err = a.data.PromptSecret("FANBOXSESSID")
	} else {
		value, err = readSessionInput(a.in)
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
		return "", errors.New("FANBOX session input must contain exactly one line")
	}
	if value == "" {
		return "", errors.New("FANBOX session value cannot be empty")
	}
	return value, nil
}

func (a command) importSession(cmd *cobra.Command, value string, setDefault, jsonOut bool) error {
	service, err := a.data.Service()
	if err != nil {
		return err
	}
	proxyOverride, err := deps.ProxyOverride(cmd)
	if err != nil {
		return err
	}
	account, err := service.ImportSessionWithProxy(cmd.Context(), value, setDefault, proxyOverride)
	if err != nil {
		return err
	}
	if jsonOut {
		return a.data.PrintJSON(accountOutFrom(account.UserID, account.DisplayName, account.CreatorID, account.Default))
	}
	fmt.Fprintf(a.out, "imported uid:%d\n", account.UserID)
	if account.DisplayName != "" {
		fmt.Fprintf(a.out, "display:%s\n", account.DisplayName)
	}
	if account.CreatorID != "" {
		fmt.Fprintf(a.out, "creator:%s\n", account.CreatorID)
	}
	return nil
}

func (a command) newListCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List FANBOX accounts",
		Args:  a.data.RequireExactArgs(0, "pixiv fanbox auth list [--json]"),
		RunE: func(_ *cobra.Command, _ []string) error {
			return a.runList(jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print JSON")
	a.data.BindNoInput(cmd)
	return cmd
}

func (a command) runList(jsonOut bool) error {
	service, err := a.data.Service()
	if err != nil {
		return err
	}
	accounts, err := service.ListAccounts(context.Background())
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
		return a.data.PrintJSON(out)
	}
	if len(out.Accounts) == 0 {
		fmt.Fprintln(a.out, "no accounts")
		return nil
	}
	for _, account := range out.Accounts {
		marker := " "
		if account.Default {
			marker = "*"
		}
		fmt.Fprintf(a.out, "%s uid:%d", marker, account.UserID)
		if account.DisplayName != "" {
			fmt.Fprintf(a.out, " display:%s", account.DisplayName)
		}
		if account.CreatorID != "" {
			fmt.Fprintf(a.out, " creator:%s", account.CreatorID)
		}
		fmt.Fprintln(a.out)
	}
	return nil
}

func (a command) newUseCommand() *cobra.Command {
	opts := useOptions{}
	cmd := &cobra.Command{
		Use:   "use [UID]",
		Short: "Set the default FANBOX account",
		Args:  a.data.RequireMaxArgs(1, "pixiv fanbox auth use [UID] | --auto"),
		RunE: func(_ *cobra.Command, args []string) error {
			return a.runUse(args, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.auto, "auto", false, "clear the explicit default and use the first stored account")
	cmd.Flags().BoolVar(&opts.jsonOut, "json", false, "print JSON")
	a.data.BindTextValue(cmd, 0, 1, 0)
	return cmd
}

func (a command) runUse(args []string, opts useOptions) error {
	if opts.auto && len(args) != 0 {
		return errors.New("--auto cannot be combined with a UID")
	}
	service, err := a.data.Service()
	if err != nil {
		return err
	}
	if opts.auto {
		if err := service.UseAuto(); err != nil {
			return err
		}
		if opts.jsonOut {
			return a.data.PrintJSON(map[string]any{"default_user_id": 0, "auto": true})
		}
		fmt.Fprintln(a.out, "default uid: auto")
		return nil
	}
	if len(args) != 1 {
		return errors.New("usage: pixiv fanbox auth use UID | --auto")
	}
	userID, err := parseUID(args[0])
	if err != nil {
		return err
	}
	if err := service.UseAccount(context.Background(), userID); err != nil {
		return err
	}
	if opts.jsonOut {
		return a.data.PrintJSON(map[string]any{"default_user_id": userID, "auto": false})
	}
	fmt.Fprintf(a.out, "default uid: %d\n", userID)
	return nil
}

func (a command) newRemoveCommand() *cobra.Command {
	opts := removeOptions{}
	cmd := &cobra.Command{
		Use:   "remove UID",
		Short: "Remove a FANBOX account",
		Args:  a.data.RequireExactArgs(1, "pixiv fanbox auth remove UID"),
		RunE: func(_ *cobra.Command, args []string) error {
			return a.runRemove(args, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.jsonOut, "json", false, "print JSON")
	cmd.Flags().BoolVar(&opts.yes, "yes", false, "skip confirmation in interactive terminals")
	a.data.BindTextValue(cmd, 1, 1, 0)
	return cmd
}

func (a command) runRemove(args []string, opts removeOptions) error {
	userID, err := parseUID(args[0])
	if err != nil {
		return err
	}
	service, err := a.data.Service()
	if err != nil {
		return err
	}
	if a.data.CanPrompt() && !opts.yes {
		ok, err := a.data.PromptConfirm(fmt.Sprintf("Remove fanbox uid %d?", userID), false)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("fanbox account removal canceled")
		}
	}
	if err := service.RemoveAccount(context.Background(), userID); err != nil {
		return err
	}
	if opts.jsonOut {
		return a.data.PrintJSON(map[string]int64{"removed_user_id": userID})
	}
	fmt.Fprintf(a.out, "account uid:%d removed\n", userID)
	return nil
}

func (a command) newStatusCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "status [UID]",
		Short: "Show the default FANBOX account",
		Args:  a.data.RequireMaxArgs(1, "pixiv fanbox auth status [UID]"),
		RunE: func(_ *cobra.Command, args []string) error {
			return a.runStatus(args, jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print JSON")
	a.data.BindTextValue(cmd, 0, 1, 0)
	return cmd
}

func (a command) runStatus(args []string, jsonOut bool) error {
	service, err := a.data.Service()
	if err != nil {
		return err
	}
	ctx := context.Background()
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
		return fmt.Errorf("fanbox account uid:%d not found", userID)
	}
	summary, err := service.Status(ctx)
	if err != nil {
		return err
	}
	return a.printSummary(*summary, jsonOut)
}

func (a command) printSummary(summary fanboxapp.AccountSummary, jsonOut bool) error {
	if jsonOut {
		return a.data.PrintJSON(accountOutFrom(summary.UserID, summary.DisplayName, summary.CreatorID, summary.Default))
	}
	fmt.Fprintf(a.out, "uid:%d\n", summary.UserID)
	if summary.DisplayName != "" {
		fmt.Fprintf(a.out, "display:%s\n", summary.DisplayName)
	}
	if summary.CreatorID != "" {
		fmt.Fprintf(a.out, "creator:%s\n", summary.CreatorID)
	}
	fmt.Fprintf(a.out, "default:%s\n", textBool(summary.Default))
	return nil
}

func parseUID(raw string) (int64, error) {
	userID, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || userID <= 0 {
		return 0, errors.New("fanbox uid must be a positive integer")
	}
	return userID, nil
}

func textBool(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
