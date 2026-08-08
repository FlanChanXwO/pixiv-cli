package fanbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	fanboxapp "github.com/FlanChanXwO/pixiv-cli/internal/application/fanbox"
	fanboxmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

// fanboxAccountOut 是 FANBOX 账号的安全输出摘要；SessionID 永不进入输出。
type fanboxAccountOut struct {
	UserID      int64  `json:"user_id"`
	DisplayName string `json:"display_name,omitempty"`
	CreatorID   string `json:"creator_id,omitempty"`
	Default     bool   `json:"default"`
}

type fanboxAccountListOut struct {
	DefaultUserID int64              `json:"default_user_id,omitempty"`
	Accounts      []fanboxAccountOut `json:"accounts"`
}

func fanboxAccountOutFrom(userID int64, displayName, creatorID string, def bool) fanboxAccountOut {
	return fanboxAccountOut{UserID: userID, DisplayName: displayName, CreatorID: creatorID, Default: def}
}

type fanboxAuthImportOptions struct {
	stdin       bool
	fromBrowser string
	profile     string
	setDefault  bool
	jsonOut     bool
}

type fanboxAuthUseOptions struct {
	auto    bool
	jsonOut bool
}

type fanboxAuthRemoveOptions struct {
	jsonOut bool
	yes     bool
}

// fanboxListOptions 是 FANBOX 只读列表命令的选项。FANBOX 客户端不消费 pixiv 代理
// 配置，因此不绑定 --proxy/--no-proxy，避免接受却被静默忽略的 flag。
type fanboxListOptions struct {
	jsonOut bool
	ndjson  bool
	listOptions
	kind string
}

func (a controller) newFanboxCommand() *cobra.Command {
	var proxy proxyOptions
	cmd := &cobra.Command{
		Use:   "fanbox",
		Short: "Browse and download Pixiv FANBOX content",
		Args:  a.RequireExactArgs(0, "pixiv fanbox <command>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	flags := cmd.PersistentFlags()
	flags.StringVar(&proxy.proxy, "proxy", "", "native FANBOX proxy URL (HTTP or HTTPS CONNECT)")
	flags.BoolVar(&proxy.noProxy, "no-proxy", false, "use a direct native FANBOX connection for this command")
	cmd.AddCommand(
		a.newFanboxAuthCommand(),
		a.newFanboxCreatorsCommand(),
		a.newFanboxPostsCommand(),
		a.newFanboxTagsCommand(),
		a.newFanboxHomeCommand(),
		a.newFanboxSupportingCommand(),
		a.newFanboxPostCommand(),
		a.newFanboxDownloadCommand(),
		a.newFanboxMCPCommand(),
	)
	return cmd
}

func (a controller) fanboxService() (*fanboxapp.Service, error) {
	service, err := a.FanboxService()
	if err != nil {
		return nil, err
	}
	if service == nil {
		return nil, errors.New("fanbox is not available: cannot open the local account store")
	}
	return service, nil
}

func (a controller) fanboxClient(cmd *cobra.Command) (*fanbox.Client, error) {
	service, err := a.fanboxService()
	if err != nil {
		return nil, err
	}
	proxyOverride, err := fanboxProxyOverrideFromCommand(cmd)
	if err != nil {
		return nil, err
	}
	return service.OpenClientWithProxy(cmd.Context(), proxyOverride)
}

func fanboxProxyOverrideFromCommand(cmd *cobra.Command) (*string, error) {
	if cmd == nil {
		return nil, nil
	}
	var options proxyOptions
	if flag := cmd.Flags().Lookup("proxy"); flag != nil {
		options.proxy = flag.Value.String()
	}
	if flag := cmd.Flags().Lookup("no-proxy"); flag != nil {
		options.noProxy = flag.Value.String() == "true"
	}
	return fanboxProxyOverrideFromFlags(cmd, options)
}

func fanboxProxyOverrideFromFlags(cmd *cobra.Command, options proxyOptions) (*string, error) {
	proxyChanged := cmd.Flags().Changed("proxy")
	noProxyChanged := cmd.Flags().Changed("no-proxy")
	if proxyChanged && noProxyChanged {
		return nil, errors.New("use either --proxy or --no-proxy, not both")
	}
	if noProxyChanged && options.noProxy {
		empty := ""
		return &empty, nil
	}
	if proxyChanged {
		return &options.proxy, nil
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// auth subtree
// ---------------------------------------------------------------------------

func (a controller) newFanboxAuthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage local FANBOX authentication",
		Args:  a.RequireExactArgs(0, "pixiv fanbox auth <command>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		a.newFanboxAuthImportCommand(),
		a.newFanboxAuthListCommand(),
		a.newFanboxAuthUseCommand(),
		a.newFanboxAuthRemoveCommand(),
		a.newFanboxAuthStatusCommand(),
	)
	return cmd
}

func (a controller) newFanboxAuthImportCommand() *cobra.Command {
	opts := fanboxAuthImportOptions{}
	cmd := &cobra.Command{
		Use:     "import",
		Short:   "Import a FANBOX session",
		Example: "pixiv fanbox auth import --stdin",
		Args:    a.RequireExactArgs(0, "pixiv fanbox auth import --stdin | --from-browser BROWSER [--profile ID] [--default]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.fanboxAuthImport(cmd, opts)
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.stdin, "stdin", false, "read the FANBOXSESSID value from stdin")
	flags.StringVar(&opts.fromBrowser, "from-browser", "", "read the FANBOXSESSID value from a browser profile")
	flags.StringVar(&opts.profile, "profile", "", "browser profile identifier when the browser has multiple profiles")
	flags.BoolVar(&opts.setDefault, "default", false, "set the imported account as the default FANBOX account")
	flags.BoolVar(&opts.jsonOut, "json", false, "print JSON")
	return cmd
}

func (a controller) fanboxAuthImport(cmd *cobra.Command, opts fanboxAuthImportOptions) error {
	if opts.stdin == (opts.fromBrowser != "") {
		return errors.New("use exactly one of --stdin or --from-browser")
	}
	if opts.fromBrowser != "" {
		value, err := a.FanboxBrowserProvider().ReadSession(cmd.Context(), opts.fromBrowser, opts.profile)
		if err != nil {
			return err
		}
		return a.fanboxImportSession(cmd, value, opts.setDefault, opts.jsonOut)
	}
	value, err := readFanboxSessionInput(a.in)
	if err != nil {
		return err
	}
	return a.fanboxImportSession(cmd, value, opts.setDefault, opts.jsonOut)
}

// readFanboxSessionInput 读取完整 stdin，只消费管道输出常见的一个末尾行结束符；
// session 值的其他字节保持 opaque，不能用 TrimSpace 改写。
func readFanboxSessionInput(r io.Reader) (string, error) {
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

func (a controller) fanboxImportSession(cmd *cobra.Command, value string, setDefault, jsonOut bool) error {
	service, err := a.fanboxService()
	if err != nil {
		return err
	}
	proxyOverride, err := fanboxProxyOverrideFromCommand(cmd)
	if err != nil {
		return err
	}
	account, err := service.ImportSessionWithProxy(cmd.Context(), value, setDefault, proxyOverride)
	if err != nil {
		return err
	}
	if jsonOut {
		return a.printJSON(fanboxAccountOutFrom(account.UserID, account.DisplayName, account.CreatorID, account.Default))
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

func (a controller) newFanboxAuthListCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List FANBOX accounts",
		Args:  a.RequireExactArgs(0, "pixiv fanbox auth list [--json]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.fanboxAuthList(jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print JSON")
	return cmd
}

func (a controller) fanboxAuthList(jsonOut bool) error {
	service, err := a.fanboxService()
	if err != nil {
		return err
	}
	accounts, err := service.ListAccounts(context.Background())
	if err != nil {
		return err
	}
	out := fanboxAccountListOut{Accounts: make([]fanboxAccountOut, 0, len(accounts))}
	for _, acct := range accounts {
		out.Accounts = append(out.Accounts, fanboxAccountOutFrom(acct.UserID, acct.DisplayName, acct.CreatorID, acct.Default))
		if acct.Default {
			out.DefaultUserID = acct.UserID
		}
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
		fmt.Fprintf(a.out, "%s uid:%d", marker, acct.UserID)
		if acct.DisplayName != "" {
			fmt.Fprintf(a.out, " display:%s", acct.DisplayName)
		}
		if acct.CreatorID != "" {
			fmt.Fprintf(a.out, " creator:%s", acct.CreatorID)
		}
		fmt.Fprintln(a.out)
	}
	return nil
}

func (a controller) newFanboxAuthUseCommand() *cobra.Command {
	opts := fanboxAuthUseOptions{}
	cmd := &cobra.Command{
		Use:   "use [UID]",
		Short: "Set the default FANBOX account",
		Args:  a.RequireMaxArgs(1, "pixiv fanbox auth use [UID] | --auto"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.fanboxAuthUse(args, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.auto, "auto", false, "clear the explicit default and use the first stored account")
	cmd.Flags().BoolVar(&opts.jsonOut, "json", false, "print JSON")
	return cmd
}

func (a controller) fanboxAuthUse(args []string, opts fanboxAuthUseOptions) error {
	if opts.auto && len(args) != 0 {
		return errors.New("--auto cannot be combined with a UID")
	}
	service, err := a.fanboxService()
	if err != nil {
		return err
	}
	if opts.auto {
		if err := service.UseAuto(); err != nil {
			return err
		}
		if opts.jsonOut {
			return a.printJSON(map[string]any{"default_user_id": 0, "auto": true})
		}
		fmt.Fprintln(a.out, "default uid: auto")
		return nil
	}
	if len(args) != 1 {
		return errors.New("usage: pixiv fanbox auth use UID | --auto")
	}
	userID, err := parseFanboxUID(args[0])
	if err != nil {
		return err
	}
	if err := service.UseAccount(context.Background(), userID); err != nil {
		return err
	}
	if opts.jsonOut {
		return a.printJSON(map[string]any{"default_user_id": userID, "auto": false})
	}
	fmt.Fprintf(a.out, "default uid: %d\n", userID)
	return nil
}

func (a controller) newFanboxAuthRemoveCommand() *cobra.Command {
	opts := fanboxAuthRemoveOptions{}
	cmd := &cobra.Command{
		Use:   "remove UID",
		Short: "Remove a FANBOX account",
		Args:  a.RequireExactArgs(1, "pixiv fanbox auth remove UID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.fanboxAuthRemove(args, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.jsonOut, "json", false, "print JSON")
	cmd.Flags().BoolVar(&opts.yes, "yes", false, "skip confirmation in interactive terminals")
	return cmd
}

func (a controller) fanboxAuthRemove(args []string, opts fanboxAuthRemoveOptions) error {
	userID, err := parseFanboxUID(args[0])
	if err != nil {
		return err
	}
	service, err := a.fanboxService()
	if err != nil {
		return err
	}
	if a.CanPrompt() && !opts.yes {
		ok, err := a.PromptConfirm(fmt.Sprintf("Remove fanbox uid %d?", userID), false)
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
		return a.printJSON(map[string]int64{"removed_user_id": userID})
	}
	fmt.Fprintf(a.out, "account uid:%d removed\n", userID)
	return nil
}

func (a controller) newFanboxAuthStatusCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "status [UID]",
		Short: "Show the default FANBOX account",
		Args:  a.RequireMaxArgs(1, "pixiv fanbox auth status [UID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.fanboxAuthStatus(args, jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print JSON")
	return cmd
}

func (a controller) fanboxAuthStatus(args []string, jsonOut bool) error {
	service, err := a.fanboxService()
	if err != nil {
		return err
	}
	ctx := context.Background()
	if len(args) == 1 {
		userID, err := parseFanboxUID(args[0])
		if err != nil {
			return err
		}
		accounts, err := service.ListAccounts(ctx)
		if err != nil {
			return err
		}
		for _, acct := range accounts {
			if acct.UserID == userID {
				return a.printFanboxAccountSummary(acct, jsonOut)
			}
		}
		return fmt.Errorf("fanbox account uid:%d not found", userID)
	}
	summary, err := service.Status(ctx)
	if err != nil {
		return err
	}
	return a.printFanboxAccountSummary(*summary, jsonOut)
}

func (a controller) printFanboxAccountSummary(summary fanboxapp.AccountSummary, jsonOut bool) error {
	if jsonOut {
		return a.printJSON(fanboxAccountOutFrom(summary.UserID, summary.DisplayName, summary.CreatorID, summary.Default))
	}
	fmt.Fprintf(a.out, "uid:%d\n", summary.UserID)
	if summary.DisplayName != "" {
		fmt.Fprintf(a.out, "display:%s\n", summary.DisplayName)
	}
	if summary.CreatorID != "" {
		fmt.Fprintf(a.out, "creator:%s\n", summary.CreatorID)
	}
	fmt.Fprintf(a.out, "default:%s\n", fanboxTextBool(summary.Default))
	return nil
}

func (a controller) printJSON(value any) error {
	return a.Host.PrintJSON(value)
}

func fanboxTextBool(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func parseFanboxUID(raw string) (int64, error) {
	userID, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || userID <= 0 {
		return 0, errors.New("fanbox uid must be a positive integer")
	}
	return userID, nil
}

// ---------------------------------------------------------------------------
// read commands
// ---------------------------------------------------------------------------

func bindFanboxListFlags(cmd *cobra.Command, opts *fanboxListOptions) {
	flags := cmd.Flags()
	flags.BoolVar(&opts.jsonOut, "json", false, "print JSON")
	flags.BoolVar(&opts.ndjson, "ndjson", false, "print one item as JSON per line")
	flags.SetOutput(cmd.ErrOrStderr())
	flags.IntVar(&opts.listOptions.limit, "limit", 0, "maximum results; omitted returns one upstream batch; 0 returns all results")
	flags.IntVar(&opts.listOptions.page, "page", 0, "1-based logical page (requires --limit > 0)")
}

// bindFanboxSingleFlags 只给单实体命令暴露 --json/--ndjson，不接受会被静默忽略的
// --limit/--page。
func bindFanboxSingleFlags(cmd *cobra.Command, opts *fanboxListOptions) {
	flags := cmd.Flags()
	flags.BoolVar(&opts.jsonOut, "json", false, "print JSON")
	flags.BoolVar(&opts.ndjson, "ndjson", false, "print one item as JSON per line")
}

func (a controller) fanboxJSONOut(cmd *cobra.Command, opts fanboxListOptions) (bool, error) {
	if opts.ndjson && cmd.Flags().Changed("json") {
		return false, a.UsageError(fmt.Errorf("--ndjson cannot be used with --json"))
	}
	return !opts.ndjson && opts.jsonOut, nil
}

func (a controller) newFanboxCreatorsCommand() *cobra.Command {
	opts := fanboxListOptions{kind: string(fanbox.CreatorListSupporting)}
	cmd := &cobra.Command{
		Use:   "creators",
		Short: "List supporting or following FANBOX creators",
		Args:  a.RequireExactArgs(0, "pixiv fanbox creators --kind supporting|following"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.fanboxCreators(cmd, opts)
		},
	}
	bindFanboxListFlags(cmd, &opts)
	cmd.Flags().StringVar(&opts.kind, "kind", opts.kind, "creator list kind: supporting or following")
	return cmd
}

func (a controller) fanboxCreators(cmd *cobra.Command, opts fanboxListOptions) error {
	kind := fanbox.CreatorListKind(opts.kind)
	if kind != fanbox.CreatorListSupporting && kind != fanbox.CreatorListFollowing {
		return fmt.Errorf("kind must be one of: supporting, following")
	}
	plan, err := parseListPlan(cmd, opts.listOptions)
	if err != nil {
		return err
	}
	jsonOut, err := a.fanboxJSONOut(cmd, opts)
	if err != nil {
		return err
	}
	client, err := a.fanboxClient(cmd)
	if err != nil {
		return err
	}
	defer client.CloseIdleConnections()
	fetch := func(ctx context.Context, cursor sdk.Cursor) (sdk.Page[fanbox.CreatorSummary], error) {
		return client.Creators(ctx, fanbox.CreatorsRequest{Kind: kind, Cursor: cursor})
	}
	return runFanboxList(a.out, cmd.Context(), plan, "creators", jsonOut, opts.ndjson, func(items []fanbox.CreatorSummary) error {
		return printFanboxCreators(a.out, items)
	}, fetch)
}

func printFanboxCreators(out io.Writer, creators []fanbox.CreatorSummary) error {
	for _, creator := range creators {
		fmt.Fprintf(out, "id:%s", creator.ID)
		if creator.Name != "" {
			fmt.Fprintf(out, " name:%s", creator.Name)
		}
		fmt.Fprintln(out)
	}
	return nil
}

func (a controller) newFanboxPostsCommand() *cobra.Command {
	opts := fanboxListOptions{}
	cmd := &cobra.Command{
		Use:   "posts SOURCE",
		Short: "List posts from a creator, tag, post, or FANBOX URL",
		Args:  a.RequireExactArgs(1, "pixiv fanbox posts SOURCE"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.fanboxPosts(cmd, args, opts)
		},
	}
	bindFanboxListFlags(cmd, &opts)
	return cmd
}

func (a controller) fanboxPosts(cmd *cobra.Command, args []string, opts fanboxListOptions) error {
	plan, err := parseListPlan(cmd, opts.listOptions)
	if err != nil {
		return err
	}
	jsonOut, err := a.fanboxJSONOut(cmd, opts)
	if err != nil {
		return err
	}
	client, err := a.fanboxClient(cmd)
	if err != nil {
		return err
	}
	defer client.CloseIdleConnections()
	fetch, err := a.fanboxPostsFetch(cmd.Context(), client, args[0])
	if err != nil {
		return err
	}
	return runFanboxList(a.out, cmd.Context(), plan, "posts", jsonOut, opts.ndjson, func(items []fanbox.Post) error {
		return printFanboxPosts(a.out, items)
	}, fetch)
}

// fanboxPostsFetch 把 creator/post/URL 源解析为可分页的帖子获取函数。URL 经
// ResolveURL 分类；纯数字源按 post ID 处理；其余按 creator ID 处理。
func (a controller) fanboxPostsFetch(ctx context.Context, client *fanbox.Client, source string) (func(context.Context, sdk.Cursor) (sdk.Page[fanbox.Post], error), error) {
	switch {
	case strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://"):
		ref, err := client.ResolveURL(ctx, fanbox.ResolveURLRequest{RawURL: source})
		if err != nil {
			return nil, err
		}
		switch ref.Kind {
		case fanbox.ReferenceKindPost:
			postID := ref.PostID
			return singleFanboxPostFetch(client, postID), nil
		case fanbox.ReferenceKindTag:
			creatorID, tag := ref.CreatorID, ref.Tag
			return func(ctx context.Context, cursor sdk.Cursor) (sdk.Page[fanbox.Post], error) {
				return client.TaggedPosts(ctx, fanbox.TaggedPostsRequest{CreatorID: creatorID, Tag: tag, Cursor: cursor})
			}, nil
		default:
			creatorID := ref.CreatorID
			return func(ctx context.Context, cursor sdk.Cursor) (sdk.Page[fanbox.Post], error) {
				return client.CreatorPosts(ctx, fanbox.CreatorPostsRequest{CreatorID: creatorID, Cursor: cursor})
			}, nil
		}
	case fanboxIsNumericID(source):
		return singleFanboxPostFetch(client, source), nil
	default:
		creatorID := source
		return func(ctx context.Context, cursor sdk.Cursor) (sdk.Page[fanbox.Post], error) {
			return client.CreatorPosts(ctx, fanbox.CreatorPostsRequest{CreatorID: creatorID, Cursor: cursor})
		}, nil
	}
}

func singleFanboxPostFetch(client *fanbox.Client, postID string) func(context.Context, sdk.Cursor) (sdk.Page[fanbox.Post], error) {
	return func(ctx context.Context, cursor sdk.Cursor) (sdk.Page[fanbox.Post], error) {
		if !cursor.IsZero() {
			return sdk.Page[fanbox.Post]{Items: []fanbox.Post{}}, nil
		}
		post, err := client.Post(ctx, fanbox.PostRequest{PostID: postID})
		if err != nil {
			return sdk.Page[fanbox.Post]{}, err
		}
		return sdk.Page[fanbox.Post]{Items: []fanbox.Post{post}}, nil
	}
}

func fanboxIsNumericID(source string) bool {
	if source == "" {
		return false
	}
	for _, r := range source {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (a controller) newFanboxTagsCommand() *cobra.Command {
	opts := fanboxListOptions{}
	cmd := &cobra.Command{
		Use:   "tags CREATOR",
		Short: "List tags used by a FANBOX creator",
		Args:  a.RequireExactArgs(1, "pixiv fanbox tags CREATOR"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.fanboxTags(cmd, args, opts)
		},
	}
	bindFanboxSingleFlags(cmd, &opts)
	return cmd
}

func (a controller) fanboxTags(cmd *cobra.Command, args []string, opts fanboxListOptions) error {
	jsonOut, err := a.fanboxJSONOut(cmd, opts)
	if err != nil {
		return err
	}
	client, err := a.fanboxClient(cmd)
	if err != nil {
		return err
	}
	defer client.CloseIdleConnections()
	tags, err := client.CreatorTags(cmd.Context(), fanbox.CreatorTagsRequest{CreatorID: args[0]})
	if err != nil {
		return err
	}
	out := make([]fanboxTagOut, 0, len(tags))
	for _, tag := range tags {
		out = append(out, fanboxTagOut{Name: tag.Name, URL: tag.URL})
	}
	if jsonOut {
		return a.printJSON(struct {
			Tags []fanboxTagOut `json:"tags"`
		}{Tags: out})
	}
	if opts.ndjson {
		encoder := json.NewEncoder(a.out)
		for _, tag := range out {
			if err := encoder.Encode(tag); err != nil {
				return err
			}
		}
		return nil
	}
	for _, tag := range out {
		fmt.Fprintf(a.out, "tag:%s\n", tag.Name)
	}
	return nil
}

type fanboxTagOut struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

func (a controller) newFanboxHomeCommand() *cobra.Command {
	opts := fanboxListOptions{}
	cmd := &cobra.Command{
		Use:   "home",
		Short: "Browse the FANBOX home feed",
		Args:  a.RequireExactArgs(0, "pixiv fanbox home"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.fanboxHome(cmd, opts)
		},
	}
	bindFanboxListFlags(cmd, &opts)
	return cmd
}

func (a controller) fanboxHome(cmd *cobra.Command, opts fanboxListOptions) error {
	plan, err := parseListPlan(cmd, opts.listOptions)
	if err != nil {
		return err
	}
	jsonOut, err := a.fanboxJSONOut(cmd, opts)
	if err != nil {
		return err
	}
	client, err := a.fanboxClient(cmd)
	if err != nil {
		return err
	}
	defer client.CloseIdleConnections()
	fetch := func(ctx context.Context, cursor sdk.Cursor) (sdk.Page[fanbox.Post], error) {
		return client.Home(ctx, fanbox.HomeRequest{Cursor: cursor})
	}
	return runFanboxList(a.out, cmd.Context(), plan, "posts", jsonOut, opts.ndjson, func(items []fanbox.Post) error {
		return printFanboxPosts(a.out, items)
	}, fetch)
}

func (a controller) newFanboxSupportingCommand() *cobra.Command {
	opts := fanboxListOptions{}
	cmd := &cobra.Command{
		Use:   "supporting",
		Short: "Browse posts from supporting creators",
		Args:  a.RequireExactArgs(0, "pixiv fanbox supporting"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.fanboxSupporting(cmd, opts)
		},
	}
	bindFanboxListFlags(cmd, &opts)
	return cmd
}

func (a controller) fanboxSupporting(cmd *cobra.Command, opts fanboxListOptions) error {
	plan, err := parseListPlan(cmd, opts.listOptions)
	if err != nil {
		return err
	}
	jsonOut, err := a.fanboxJSONOut(cmd, opts)
	if err != nil {
		return err
	}
	client, err := a.fanboxClient(cmd)
	if err != nil {
		return err
	}
	defer client.CloseIdleConnections()
	fetch := func(ctx context.Context, cursor sdk.Cursor) (sdk.Page[fanbox.Post], error) {
		return client.Supporting(ctx, fanbox.SupportingRequest{Cursor: cursor})
	}
	return runFanboxList(a.out, cmd.Context(), plan, "posts", jsonOut, opts.ndjson, func(items []fanbox.Post) error {
		return printFanboxPosts(a.out, items)
	}, fetch)
}

func (a controller) newFanboxPostCommand() *cobra.Command {
	opts := fanboxListOptions{}
	cmd := &cobra.Command{
		Use:   "post POST_ID",
		Short: "Show one FANBOX post",
		Args:  a.RequireExactArgs(1, "pixiv fanbox post POST_ID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.fanboxPost(cmd, args, opts)
		},
	}
	bindFanboxSingleFlags(cmd, &opts)
	return cmd
}

func (a controller) fanboxPost(cmd *cobra.Command, args []string, opts fanboxListOptions) error {
	jsonOut, err := a.fanboxJSONOut(cmd, opts)
	if err != nil {
		return err
	}
	client, err := a.fanboxClient(cmd)
	if err != nil {
		return err
	}
	defer client.CloseIdleConnections()
	post, err := client.Post(cmd.Context(), fanbox.PostRequest{PostID: args[0]})
	if err != nil {
		return err
	}
	out := fanboxPostOutFrom(post)
	if jsonOut {
		return a.printJSON(out)
	}
	if opts.ndjson {
		return json.NewEncoder(a.out).Encode(out)
	}
	return printFanboxPosts(a.out, []fanbox.Post{post})
}

type fanboxPostOut struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	PublishedAt  string `json:"published_at"`
	CreatorID    string `json:"creator_id"`
	FeeRequired  int    `json:"fee_required,omitempty"`
	IsRestricted bool   `json:"is_restricted"`
	IsPinned     bool   `json:"is_pinned,omitempty"`
	CommentCount int    `json:"comment_count,omitempty"`
}

func fanboxPostOutFrom(post fanbox.Post) fanboxPostOut {
	published := ""
	if !post.PublishedAt.IsZero() {
		published = post.PublishedAt.UTC().Format(time.RFC3339)
	}
	return fanboxPostOut{
		ID:           post.ID,
		Title:        post.Title,
		PublishedAt:  published,
		CreatorID:    post.CreatorID,
		FeeRequired:  post.FeeRequired,
		IsRestricted: post.IsRestricted,
		IsPinned:     post.IsPinned,
		CommentCount: post.CommentCount,
	}
}

func printFanboxPosts(out io.Writer, posts []fanbox.Post) error {
	for _, post := range posts {
		fmt.Fprintf(out, "id:%s title:%s published:%s restricted:%s\n",
			post.ID, post.Title, fanboxPostOutFrom(post).PublishedAt, fanboxTextBool(post.IsRestricted))
	}
	return nil
}

// traverseFanboxPages 跟随 sdk.Cursor 分页并交给 consume。成功空结果保持 non-nil；
// oneBatch 语义与 pixiv 列表一致（跳过空批直到首个非空逻辑结果或真正结束）。
func traverseFanboxPages[T any](ctx context.Context, plan listPlan, fetch func(context.Context, sdk.Cursor) (sdk.Page[T], error), consume func([]T) error) error {
	cursor := sdk.Cursor{}
	seen := make(map[string]struct{})
	returned := 0
	skip := plan.skip
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		key := cursor.String()
		if _, exists := seen[key]; exists {
			return errors.New("pagination cursor repeated")
		}
		seen[key] = struct{}{}
		page, err := fetch(ctx, cursor)
		if err != nil {
			return err
		}
		items := page.Items
		if skip >= len(items) {
			skip -= len(items)
			items = nil
		} else if skip > 0 {
			items = items[skip:]
			skip = 0
		}
		if plan.limit > 0 {
			remaining := plan.limit - returned
			if len(items) > remaining {
				items = items[:remaining]
			}
		}
		if len(items) > 0 {
			if err := consume(items); err != nil {
				return err
			}
			returned += len(items)
		}
		if plan.limit > 0 && returned >= plan.limit {
			return nil
		}
		if plan.oneBatch && (returned > 0 || page.Next.IsZero()) {
			return nil
		}
		if page.Next.IsZero() {
			return nil
		}
		cursor = page.Next
	}
}

// runFanboxList 统一 FANBOX 列表命令的 JSON / NDJSON / 文本输出与分页。
func runFanboxList[T any](out io.Writer, ctx context.Context, plan listPlan, jsonKey string, jsonOut, ndjson bool, printText func([]T) error, fetch func(context.Context, sdk.Cursor) (sdk.Page[T], error)) error {
	if jsonOut {
		spool, err := newJSONArraySpool(jsonKey)
		if err != nil {
			return err
		}
		defer spool.Close()
		if err := traverseFanboxPages(ctx, plan, fetch, func(items []T) error {
			return appendJSONArray(spool, items)
		}); err != nil {
			return err
		}
		return spool.Commit(out)
	}
	if ndjson {
		encoder := json.NewEncoder(out)
		return traverseFanboxPages(ctx, plan, fetch, func(items []T) error {
			for _, item := range items {
				if err := encoder.Encode(item); err != nil {
					return err
				}
			}
			return nil
		})
	}
	return traverseFanboxPages(ctx, plan, fetch, printText)
}

// ---------------------------------------------------------------------------
// download
// ---------------------------------------------------------------------------

type fanboxDownloadOptions struct{}

func (a controller) newFanboxDownloadCommand() *cobra.Command {
	opts := fanboxDownloadOptions{}
	cmd := &cobra.Command{
		Use:   "download SOURCE...",
		Short: "Download posts and their assets from FANBOX",
		Args:  a.RequireMinArgs(1, "pixiv fanbox download SOURCE..."),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.fanboxDownload(cmd, args, opts)
		},
	}
	return cmd
}

func (a controller) fanboxDownload(cmd *cobra.Command, args []string, _ fanboxDownloadOptions) error {
	runtime, err := a.FanboxRuntimeConfig()
	if err != nil {
		return err
	}
	baseDir := filepath.Join(runtime.DownloadPath, "fanbox")
	client, err := a.fanboxClient(cmd)
	if err != nil {
		return err
	}
	defer client.CloseIdleConnections()
	seen := make(map[string]struct{})
	plan := listPlan{limit: 0}
	for _, source := range args {
		fetch, err := a.fanboxPostsFetch(cmd.Context(), client, source)
		if err != nil {
			return err
		}
		if err := traverseFanboxPages(cmd.Context(), plan, fetch, func(posts []fanbox.Post) error {
			for _, post := range posts {
				if err := a.fanboxSavePostAssets(cmd.Context(), client, baseDir, post, seen); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func (a controller) fanboxSavePostAssets(ctx context.Context, client *fanbox.Client, baseDir string, post fanbox.Post, seen map[string]struct{}) error {
	if post.Body == nil {
		return nil
	}
	dir := filepath.Join(baseDir, post.CreatorID, post.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, asset := range post.Body.Assets {
		if asset.Resource.Ref.IsZero() {
			continue
		}
		path := filepath.Join(dir, fanboxAssetFilename(asset))
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		if _, err := client.SaveResource(ctx, asset.Resource.Ref, sdk.SaveOptions{Path: path}); err != nil {
			return err
		}
		fmt.Fprintf(a.out, "saved: %s\n", path)
	}
	return nil
}

func fanboxAssetFilename(asset fanbox.Asset) string {
	if asset.Name != "" {
		return asset.Name
	}
	ext := fanboxAssetExtension(asset.Resource.URL)
	if asset.ID != "" {
		return asset.ID + ext
	}
	if ext != "" {
		return "asset" + ext
	}
	return "asset"
}

func fanboxAssetExtension(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(parsed.Path))
	if len(ext) > 1 && len(ext) <= 12 {
		return ext
	}
	return ""
}

// ---------------------------------------------------------------------------
// mcp
// ---------------------------------------------------------------------------

func (a controller) newFanboxMCPCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "mcp",
		Short:   "Run the FANBOX MCP stdio server",
		Example: "pixiv fanbox mcp",
		Args:    a.RequireExactArgs(0, "pixiv fanbox mcp"),
		RunE: func(cmd *cobra.Command, args []string) error {
			service, err := a.fanboxService()
			if err != nil {
				return err
			}
			proxyOverride, err := fanboxProxyOverrideFromCommand(cmd)
			if err != nil {
				return err
			}
			server := fanboxmcpserver.NewWithProxy(service, proxyOverride)
			return server.Run(cmd.Context(), &mcp.StdioTransport{})
		},
	}
	return cmd
}
