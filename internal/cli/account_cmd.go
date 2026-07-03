package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/FlanChanXwO/pixiv-mcp-server/internal/cli/state"
	"github.com/FlanChanXwO/pixiv-mcp-server/internal/config"
	"github.com/FlanChanXwO/pixiv-mcp-server/internal/utils"
	"github.com/spf13/cobra"
)

type accountOut struct {
	Name     string `json:"name"`
	Default  bool   `json:"default"`
	UserID   int64  `json:"user_id,omitempty"`
	HasToken bool   `json:"has_token"`
}

type accountListOut struct {
	DefaultAccount string       `json:"default_account,omitempty"`
	Accounts       []accountOut `json:"accounts"`
}

type accountAddOptions struct {
	token   string
	jsonOut bool
}

type accountRemoveOptions struct {
	jsonOut bool
	yes     bool
}

func (a app) newAccountCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage local Pixiv authentication",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		a.newAccountAddCommand(),
		a.newAccountLoginCommand(),
		a.newAccountListCommand(),
		a.newAccountRemoveCommand(),
		a.newAccountUseCommand(),
		a.newAccountCheckCommand(),
	)
	return cmd
}

func (a app) newAccountAddCommand() *cobra.Command {
	opts := accountAddOptions{}
	cmd := &cobra.Command{
		Use:     "add [NAME]",
		Short:   "Add or replace an account",
		Example: "pixiv auth add main --token YOUR_REFRESH_TOKEN",
		Args:    requireMaxArgs(1, "pixiv auth add [NAME] [--token TOKEN]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.accountAdd(args, opts)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.token, "token", "", "Pixiv refresh token or cookie with refresh_token")
	flags.BoolVar(&opts.jsonOut, "json", false, "print JSON")
	return cmd
}

func (a app) accountAdd(args []string, opts accountAddOptions) error {
	name, err := a.resolveAccountNameArg(args, "account name")
	if err != nil {
		return err
	}
	if err := state.ValidateAccountName(name); err != nil {
		return err
	}
	tokenInput, err := a.resolveRefreshTokenInput(opts.token)
	if err != nil {
		return err
	}
	token, parsedCookie := utils.ParsePixivWebRefreshTokenInput(tokenInput)
	if token == "" {
		if parsedCookie {
			return errors.New("cookie does not contain refresh_token")
		}
		return errors.New("refresh token cannot be empty")
	}

	path, err := state.AuthFilePath()
	if err != nil {
		return err
	}
	store, err := state.LoadAuthStore(path)
	if err != nil {
		return err
	}
	userID := int64(0)
	if _, acct, ok := store.Get(name); ok {
		userID = acct.UserID
	}
	store.Upsert(state.Account{Name: name, RefreshToken: token, UserID: userID})
	if store.DefaultAccount == "" {
		store.DefaultAccount = name
	}
	if err := state.SaveAuthStore(path, store); err != nil {
		return err
	}
	out := accountOut{Name: name, Default: store.DefaultAccount == name, HasToken: true, UserID: userID}
	if opts.jsonOut {
		return a.printJSON(out)
	}
	fmt.Fprintf(a.out, "account %q saved\n", name)
	if out.Default {
		fmt.Fprintf(a.out, "default account: %s\n", name)
	}
	return nil
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
	path, err := state.AuthFilePath()
	if err != nil {
		return err
	}
	store, err := state.LoadAuthStore(path)
	if err != nil {
		return err
	}
	out := accountListOut{DefaultAccount: store.DefaultAccount}
	for _, acct := range store.Accounts {
		out.Accounts = append(out.Accounts, accountOut{
			Name:     acct.Name,
			Default:  acct.Name == store.DefaultAccount,
			UserID:   acct.UserID,
			HasToken: acct.RefreshToken != "",
		})
	}
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
		fmt.Fprintf(a.out, "%s %s token:%s", marker, acct.Name, textBool(acct.HasToken))
		if acct.UserID != 0 {
			fmt.Fprintf(a.out, " user_id:%d", acct.UserID)
		}
		fmt.Fprintln(a.out)
	}
	return nil
}

func (a app) newAccountRemoveCommand() *cobra.Command {
	opts := accountRemoveOptions{}
	cmd := &cobra.Command{
		Use:   "remove [NAME]",
		Short: "Remove an account",
		Args:  requireMaxArgs(1, "pixiv auth remove [NAME] [--yes]"),
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
	path, err := state.AuthFilePath()
	if err != nil {
		return err
	}
	store, err := state.LoadAuthStore(path)
	if err != nil {
		return err
	}
	name, err := a.resolveExistingAccountName(store, args, "Select account to remove")
	if err != nil {
		return err
	}
	if canPrompt(a) && !opts.yes {
		ok, err := promptConfirm(a, fmt.Sprintf("Remove account %q?", name), false)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("account removal canceled")
		}
	}
	if !store.Remove(name) {
		return fmt.Errorf("account %q not found", name)
	}
	if err := state.SaveAuthStore(path, store); err != nil {
		return err
	}
	if opts.jsonOut {
		return a.printJSON(map[string]string{"removed": name, "default_account": store.DefaultAccount})
	}
	fmt.Fprintf(a.out, "account %q removed\n", name)
	if store.DefaultAccount != "" {
		fmt.Fprintf(a.out, "default account: %s\n", store.DefaultAccount)
	}
	return nil
}

func (a app) newAccountUseCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "use [NAME]",
		Short: "Set the default account",
		Args:  requireMaxArgs(1, "pixiv auth use [NAME]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.accountUse(args, jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print JSON")
	return cmd
}

func (a app) accountUse(args []string, jsonOut bool) error {
	path, err := state.AuthFilePath()
	if err != nil {
		return err
	}
	store, err := state.LoadAuthStore(path)
	if err != nil {
		return err
	}
	name, err := a.resolveExistingAccountName(store, args, "Select default account")
	if err != nil {
		return err
	}
	if !store.Has(name) {
		return fmt.Errorf("account %q not found", name)
	}
	store.DefaultAccount = name
	if err := state.SaveAuthStore(path, store); err != nil {
		return err
	}
	if jsonOut {
		return a.printJSON(map[string]string{"default_account": name})
	}
	fmt.Fprintf(a.out, "default account: %s\n", name)
	return nil
}

func (a app) newAccountCheckCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "check [NAME]",
		Short: "Validate an account token",
		Args:  requireMaxArgs(1, "pixiv auth check [NAME]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return a.accountCheck(name, jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print JSON")
	return cmd
}

func (a app) accountCheck(name string, jsonOut bool) error {
	settings, err := config.LoadSettingsState()
	if err != nil {
		return err
	}
	cfg, err := settings.Runtime()
	if err != nil {
		return err
	}
	path, err := state.AuthFilePath()
	if err != nil {
		return err
	}
	store, err := state.LoadAuthStore(path)
	if err != nil {
		return err
	}
	selectedName := ""
	if strings.TrimSpace(name) != "" {
		selectedName = strings.TrimSpace(name)
		_, acct, ok := state.SelectAuthAccount(store, selectedName)
		if !ok {
			return fmt.Errorf("account %q not found", selectedName)
		}
		cfg.RefreshToken = acct.RefreshToken
	} else if token := config.RefreshTokenFromEnv(); token != "" {
		cfg.RefreshToken = token
	} else if chosen, acct, ok := state.SelectAuthAccount(store, ""); ok {
		selectedName = chosen
		cfg.RefreshToken = acct.RefreshToken
	}
	if cfg.RefreshToken == "" {
		return errors.New("no account or PIXIV_REFRESH_TOKEN to check")
	}
	client, err := newPixivClient(clientConfig{RuntimeConfig: cfg})
	if err != nil {
		return err
	}
	if err := client.Refresh(context.Background()); err != nil {
		return err
	}
	userID := client.UserID()
	if selectedName != "" {
		if idx, acct, ok := store.Get(selectedName); ok {
			acct.UserID = userID
			store.Accounts[idx] = acct
			if err := state.SaveAuthStore(path, store); err != nil {
				return err
			}
		}
	}
	out := accountOut{Name: selectedName, Default: selectedName != "" && selectedName == store.DefaultAccount, UserID: userID, HasToken: true}
	if jsonOut {
		return a.printJSON(out)
	}
	if selectedName == "" {
		fmt.Fprintf(a.out, "token ok, user_id:%d\n", userID)
		return nil
	}
	fmt.Fprintf(a.out, "account %q ok, user_id:%d\n", selectedName, userID)
	return nil
}

func (a app) resolveAccountNameArg(args []string, promptLabel string) (string, error) {
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		return strings.TrimSpace(args[0]), nil
	}
	if !canPrompt(a) {
		return "", errors.New("account name is required")
	}
	return promptInput(a, promptLabel, "")
}

func (a app) resolveExistingAccountName(store state.AuthStore, args []string, promptLabel string) (string, error) {
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		return strings.TrimSpace(args[0]), nil
	}
	if !canPrompt(a) {
		return "", errors.New("account name is required")
	}
	if len(store.Accounts) == 0 {
		return "", errors.New("no accounts")
	}
	return promptSelect(a, promptLabel, store.Names())
}

func (a app) resolveRefreshTokenInput(tokenFlag string) (string, error) {
	if strings.TrimSpace(tokenFlag) != "" {
		return tokenFlag, nil
	}
	if canPrompt(a) {
		return promptSecret(a, "Refresh token")
	}
	writePrompt(a.errOut, "refresh token: ")
	return readTokenLine(a.in)
}

func readTokenLine(r io.Reader) (string, error) {
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
