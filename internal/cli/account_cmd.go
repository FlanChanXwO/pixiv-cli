package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/FlanChanXwO/pixiv-mcp-server/pkg/config"
	"github.com/FlanChanXwO/pixiv-mcp-server/pkg/pixivutil"
)

type accountOut struct {
	Name     string `json:"name"`
	Default  bool   `json:"default"`
	UserID   int64  `json:"user_id,omitempty"`
	HasToken bool   `json:"has_token"`
}

type accountListOut struct {
	DefaultProfile string       `json:"default_profile,omitempty"`
	Accounts       []accountOut `json:"accounts"`
}

func (a app) runAccount(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		a.printAccountHelp(a.out)
		return nil
	}
	switch args[0] {
	case "add":
		return a.accountAdd(args[1:])
	case "login":
		return a.accountLogin(args[1:])
	case "list":
		return a.accountList(args[1:])
	case "remove":
		return a.accountRemove(args[1:])
	case "use":
		return a.accountUse(args[1:])
	case "check":
		return a.accountCheck(args[1:])
	default:
		return fmt.Errorf("unknown account command %q", args[0])
	}
}

func (a app) printAccountHelp(w io.Writer) {
	fmt.Fprint(w, `Usage: pixiv account <command> [options]

Commands:
  add NAME       Add or replace a profile
  login NAME     Login with local browser OAuth flow
  list           List profiles
  remove NAME    Remove a profile
  use NAME       Set the default profile
  check [NAME]   Validate a profile token
`)
}

func (a app) accountAdd(args []string) error {
	var token string
	var jsonOut bool
	fs := flag.NewFlagSet("account add", flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	fs.StringVar(&token, "token", "", "Pixiv refresh token or cookie with refresh_token")
	fs.BoolVar(&jsonOut, "json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: pixiv account add [--token TOKEN] NAME")
	}
	name := fs.Arg(0)
	if err := validateProfileName(name); err != nil {
		return err
	}
	if token == "" {
		fmt.Fprint(a.errOut, "refresh token: ")
		read, err := readTokenLine(a.in)
		if err != nil {
			return err
		}
		token = read
	}
	parsed, parsedCookie := pixivutil.ParseRefreshTokenInput(token)
	if parsed == "" {
		if parsedCookie {
			return errors.New("cookie does not contain refresh_token")
		}
		return errors.New("refresh token cannot be empty")
	}
	path, err := configPath()
	if err != nil {
		return err
	}
	store, err := loadAccountStore(path)
	if err != nil {
		return err
	}
	store.Accounts[name] = account{RefreshToken: parsed}
	if store.DefaultProfile == "" {
		store.DefaultProfile = name
	}
	if err := saveAccountStore(path, store); err != nil {
		return err
	}
	out := accountOut{Name: name, Default: store.DefaultProfile == name, HasToken: true}
	if jsonOut {
		return a.printJSON(out)
	}
	fmt.Fprintf(a.out, "account %q saved\n", name)
	if out.Default {
		fmt.Fprintf(a.out, "default profile: %s\n", name)
	}
	return nil
}

func (a app) accountList(args []string) error {
	var jsonOut bool
	fs := flag.NewFlagSet("account list", flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	fs.BoolVar(&jsonOut, "json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: pixiv account list [--json]")
	}
	path, err := configPath()
	if err != nil {
		return err
	}
	store, err := loadAccountStore(path)
	if err != nil {
		return err
	}
	out := accountListOut{DefaultProfile: store.DefaultProfile}
	for _, name := range profileNames(store) {
		acct := store.Accounts[name]
		out.Accounts = append(out.Accounts, accountOut{
			Name:     name,
			Default:  store.DefaultProfile == name,
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

func (a app) accountRemove(args []string) error {
	var jsonOut bool
	fs := flag.NewFlagSet("account remove", flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	fs.BoolVar(&jsonOut, "json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: pixiv account remove NAME")
	}
	name := fs.Arg(0)
	path, err := configPath()
	if err != nil {
		return err
	}
	store, err := loadAccountStore(path)
	if err != nil {
		return err
	}
	if _, ok := store.Accounts[name]; !ok {
		return fmt.Errorf("profile %q not found", name)
	}
	delete(store.Accounts, name)
	if store.DefaultProfile == name {
		store.DefaultProfile = ""
		names := profileNames(store)
		if len(names) > 0 {
			store.DefaultProfile = names[0]
		}
	}
	if err := saveAccountStore(path, store); err != nil {
		return err
	}
	if jsonOut {
		return a.printJSON(map[string]string{"removed": name, "default_profile": store.DefaultProfile})
	}
	fmt.Fprintf(a.out, "account %q removed\n", name)
	if store.DefaultProfile != "" {
		fmt.Fprintf(a.out, "default profile: %s\n", store.DefaultProfile)
	}
	return nil
}

func (a app) accountUse(args []string) error {
	var jsonOut bool
	fs := flag.NewFlagSet("account use", flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	fs.BoolVar(&jsonOut, "json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: pixiv account use NAME")
	}
	name := fs.Arg(0)
	path, err := configPath()
	if err != nil {
		return err
	}
	store, err := loadAccountStore(path)
	if err != nil {
		return err
	}
	if _, ok := store.Accounts[name]; !ok {
		return fmt.Errorf("profile %q not found", name)
	}
	store.DefaultProfile = name
	if err := saveAccountStore(path, store); err != nil {
		return err
	}
	if jsonOut {
		return a.printJSON(map[string]string{"default_profile": name})
	}
	fmt.Fprintf(a.out, "default profile: %s\n", name)
	return nil
}

func (a app) accountCheck(args []string) error {
	var jsonOut bool
	fs := flag.NewFlagSet("account check", flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	fs.BoolVar(&jsonOut, "json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return errors.New("usage: pixiv account check [NAME]")
	}
	name := ""
	if fs.NArg() == 1 {
		name = fs.Arg(0)
	}
	path, err := configPath()
	if err != nil {
		return err
	}
	store, err := loadAccountStore(path)
	if err != nil {
		return err
	}
	selected, acct, ok := selectAccount(store, name)
	cfg := config.LoadFromEnv()
	if ok {
		cfg.RefreshToken = acct.RefreshToken
	} else if name != "" {
		return fmt.Errorf("profile %q not found", name)
	} else if cfg.RefreshToken == "" {
		return errors.New("no profile or PIXIV_REFRESH_TOKEN to check")
	}
	client, err := newPixivClient(cfg)
	if err != nil {
		return err
	}
	if err := client.Refresh(context.Background()); err != nil {
		return err
	}
	userID := client.UserID()
	if ok {
		acct.UserID = userID
		store.Accounts[selected] = acct
		if err := saveAccountStore(path, store); err != nil {
			return err
		}
	}
	out := accountOut{Name: selected, Default: selected != "" && selected == store.DefaultProfile, UserID: userID, HasToken: true}
	if jsonOut {
		return a.printJSON(out)
	}
	if selected == "" {
		fmt.Fprintf(a.out, "token ok, user_id:%d\n", userID)
		return nil
	}
	fmt.Fprintf(a.out, "account %q ok, user_id:%d\n", selected, userID)
	return nil
}

func readTokenLine(r io.Reader) (string, error) {
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
