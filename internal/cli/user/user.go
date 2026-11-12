// Package user 提供 pixiv user 命令组：用户搜索、详情、作品、收藏、关注关系与屏蔽列表。
//
// 包的组成：[New] 组装命令组，[RunSearch] 共享用户搜索；列表输出在 lists.go。
package user

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/display"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	legacysdk "github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/target"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

type listOptions struct {
	environment.Options
	ndjson     bool
	limit      int
	page       int
	restrict   string
	tag        string
	illustType string
}

type commandRunner struct {
	env        environment.Env
	executeSDK session.SDKExecutor
}

// New 构造 `pixiv user` 命令组。follow 命令由组合根注入，本包不直接依赖同级的 follow 包。
func New(env environment.Env, executeSDK session.SDKExecutor, follow func() *cobra.Command) *cobra.Command {
	a := commandRunner{env: env, executeSDK: executeSDK}
	cmd := &cobra.Command{Use: "user", Short: "Query a Pixiv user"}
	cmd.AddCommand(
		a.newSearchCommand(),
		a.newDetailCommand(),
		a.newArtworksCommand(),
		a.newNovelsCommand(),
		a.newBookmarksCommand(),
		a.newFollowingCommand(),
		a.newFollowersCommand(),
		a.newRelatedCommand(),
		a.newBlockedCommand(),
	)
	cmd.AddCommand(follow())
	a.env.BindNoInput(cmd)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

// SearchOptions 携带兼容入口已解析的用户搜索 flag，让 `pixiv search --type user`
// 复用这里的执行逻辑。
type SearchOptions struct {
	environment.Options
	NDJSON bool
	Limit  int
	Page   int
}

// RunSearch 按 SearchOptions 执行用户搜索；兼容入口调用它避免复制搜索逻辑。
func RunSearch(cmd *cobra.Command, env environment.Env, executeSDK session.SDKExecutor, args []string, opts SearchOptions) error {
	return commandRunner{env: env, executeSDK: executeSDK}.runSearch(cmd, args, listOptions{
		Options: opts.Options,
		ndjson:  opts.NDJSON,
		limit:   opts.Limit,
		page:    opts.Page,
	})
}

func (a commandRunner) newSearchCommand() *cobra.Command {
	opts := listOptions{}
	cmd := &cobra.Command{
		Use:     "search WORD",
		Short:   "Search users",
		Example: "pixiv user search \"miku\" --json",
		Args:    a.env.MinArgs(1, "pixiv user search [options] WORD"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runSearch(cmd, args, opts)
		},
	}
	a.bindListCommand(cmd, &opts)
	a.env.BindTextValue(cmd, 1, -1, 0)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a commandRunner) newDetailCommand() *cobra.Command {
	var opts environment.Options
	cmd := &cobra.Command{
		Use:   "detail USER_ID",
		Short: "Show one user's complete profile",
		Args:  a.env.ExactArgs(1, "pixiv user detail [options] USER_ID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runDetail(cmd, args[0], opts)
		},
	}
	a.env.BindCommonFlags(cmd, &opts)
	a.env.BindTextValue(cmd, 1, 1, 0)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a commandRunner) newArtworksCommand() *cobra.Command {
	opts := listOptions{illustType: string(pixiv.ContentTypeIllust)}
	cmd := &cobra.Command{
		Use:   "artworks [USER_ID]",
		Short: "List a user's artworks",
		Args:  a.env.MaxArgs(1, "pixiv user artworks [options] [USER_ID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runArtworks(cmd, args, opts)
		},
	}
	a.bindListCommand(cmd, &opts)
	cmd.Flags().StringVarP(&opts.illustType, "type", "t", opts.illustType, "illust type: illust, manga, ugoira")
	a.env.BindTextValue(cmd, 0, 1, 0)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a commandRunner) newBookmarksCommand() *cobra.Command {
	opts := listOptions{restrict: string(pixiv.RestrictPublic)}
	cmd := &cobra.Command{
		Use:   "bookmarks [USER_ID]",
		Short: "List a user's bookmarks",
		Args:  a.env.MaxArgs(1, "pixiv user bookmarks [options] [USER_ID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runBookmarks(cmd, args, opts)
		},
	}
	a.bindListCommand(cmd, &opts)
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "bookmark visibility (public or private)")
	cmd.Flags().StringVar(&opts.tag, "tag", "", "filter by bookmark tag")
	a.env.BindTextValue(cmd, 0, 1, 0)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a commandRunner) newFollowingCommand() *cobra.Command {
	opts := listOptions{restrict: string(pixiv.RestrictPublic)}
	cmd := &cobra.Command{
		Use:   "following [USER_ID]",
		Short: "List users followed by a user",
		Args:  a.env.MaxArgs(1, "pixiv user following [options] [USER_ID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runFollowing(cmd, args, opts)
		},
	}
	a.bindListCommand(cmd, &opts)
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "follow visibility (public or private)")
	a.env.BindTextValue(cmd, 0, 1, 0)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a commandRunner) newNovelsCommand() *cobra.Command {
	opts := listOptions{}
	cmd := &cobra.Command{
		Use:   "novels [USER_ID]",
		Short: "List a user's novels",
		Args:  a.env.MaxArgs(1, "pixiv user novels [options] [USER_ID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runNovels(cmd, args, opts)
		},
	}
	a.bindListCommand(cmd, &opts)
	a.env.BindTextValue(cmd, 0, 1, 0)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a commandRunner) newFollowersCommand() *cobra.Command {
	opts := listOptions{restrict: string(pixiv.RestrictPublic)}
	cmd := &cobra.Command{
		Use:   "followers [USER_ID]",
		Short: "List a user's followers",
		Args:  a.env.MaxArgs(1, "pixiv user followers [options] [USER_ID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runFollowers(cmd, args, opts)
		},
	}
	a.bindListCommand(cmd, &opts)
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "follow visibility (public or private)")
	a.env.BindTextValue(cmd, 0, 1, 0)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a commandRunner) newRelatedCommand() *cobra.Command {
	opts := listOptions{}
	cmd := &cobra.Command{
		Use:   "related USER_ID",
		Short: "List users related to a user",
		Args:  a.env.ExactArgs(1, "pixiv user related [options] USER_ID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runRelated(cmd, args[0], opts)
		},
	}
	a.bindListCommand(cmd, &opts)
	a.env.BindTextValue(cmd, 1, 1, 0)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a commandRunner) newBlockedCommand() *cobra.Command {
	opts := listOptions{}
	cmd := &cobra.Command{
		Use:   "blocked [USER_ID]",
		Short: "List users blocked by a user",
		Args:  a.env.MaxArgs(1, "pixiv user blocked [options] [USER_ID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runBlocked(cmd, args, opts)
		},
	}
	a.bindListCommand(cmd, &opts)
	a.env.BindTextValue(cmd, 0, 1, 0)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a commandRunner) bindListCommand(cmd *cobra.Command, opts *listOptions) {
	a.env.BindCommonFlags(cmd, &opts.Options)
	environment.BindNDJSONFlag(cmd, &opts.ndjson)
	environment.BindListFlags(cmd, &opts.limit, &opts.page)
}

// resolve 统一列表命令的分页、传输覆盖与输出模式判定。autoNDJSON 保持各命令既有
// 的自动 NDJSON 行为，不在此处统一放宽或收紧。
func (a commandRunner) resolve(cmd *cobra.Command, opts listOptions, autoNDJSON bool) (pagination.PagePlan, session.ProxyRequest, bool, bool, error) {
	plan, err := environment.ParsePlan(cmd, opts.limit, opts.page)
	if err != nil {
		return pagination.PagePlan{}, session.ProxyRequest{}, false, false, err
	}
	request, err := a.env.Request(cmd, opts.Options.ProxyOptions)
	if err != nil {
		return pagination.PagePlan{}, session.ProxyRequest{}, false, false, err
	}
	if opts.ndjson && cmd.Flags().Changed("json") {
		return pagination.PagePlan{}, session.ProxyRequest{}, false, false, a.env.WrapUsage(errors.New("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = a.env.ResolveJSON(a.env.JSONOverride(cmd, opts.Options))
		if err != nil {
			return pagination.PagePlan{}, session.ProxyRequest{}, false, false, err
		}
	}
	ndjson := opts.ndjson
	if autoNDJSON {
		ndjson = a.env.AutoNDJSON(cmd, opts.ndjson, jsonOut)
	}
	return plan, request, jsonOut, ndjson, nil
}

// requestedUserID 解析可选的 USER_ID；省略时返回 0，由读取阶段用当前账号身份补齐。
func requestedUserID(args []string) (int64, error) {
	if len(args) != 1 {
		return 0, nil
	}
	return environment.PositiveInt64(args[0], "user_id")
}

func resolveUserID(ctx context.Context, value, operation string) (int64, error) {
	resolved, err := target.Resolve(ctx, target.Input{Value: value}, target.Contract{
		Operation:   operation,
		DefaultType: string(target.NamespaceUser),
		Types: []target.TypeSpec{{
			Name:           string(target.NamespaceUser),
			ResultKind:     target.NamespaceUser,
			BareTargetKind: target.NamespaceUser,
		}},
	})
	if err != nil {
		return 0, fmt.Errorf("user_id: %w", err)
	}
	return resolved.ID, nil
}

func requestedResolvedUserID(ctx context.Context, args []string, operation string) (int64, error) {
	if len(args) != 1 {
		return 0, nil
	}
	return resolveUserID(ctx, args[0], operation)
}

func validateArtworkType(value string) error {
	switch pixiv.ContentType(value) {
	case "", pixiv.ContentTypeIllust, pixiv.ContentTypeManga, pixiv.ContentTypeUgoira:
		return nil
	default:
		return &sdk.Error{Operation: "pixiv.user artworks", Reason: sdk.InvalidArgument, Message: "type must be one of illust, manga, or ugoira"}
	}
}

func validateRelationshipRestrict(value string) error {
	switch pixiv.Restrict(value) {
	case "", pixiv.RestrictPublic, pixiv.RestrictPrivate:
		return nil
	default:
		return &sdk.Error{Operation: "pixiv.user relationships", Reason: sdk.InvalidArgument, Message: "restrict must be public or private"}
	}
}

func (a commandRunner) runSearch(cmd *cobra.Command, args []string, opts listOptions) error {
	plan, request, jsonOut, ndjson, err := a.resolve(cmd, opts, true)
	if err != nil {
		return err
	}
	word := strings.Join(args, " ")
	fetch := func(ctx context.Context, _ int64, client *pixiv.Client, cursor sdk.Cursor) (sdk.Page[*pixiv.User], error) {
		return client.SearchUsers(ctx, word, pixiv.UserSearchOptions{Cursor: cursor})
	}
	return a.runUsers(cmd.Context(), request, plan, record.FormatOf(jsonOut, ndjson), fetch)
}

func (a commandRunner) runDetail(cmd *cobra.Command, arg string, opts environment.Options) error {
	userID, err := resolveUserID(cmd.Context(), arg, "user detail")
	if err != nil {
		return err
	}
	request, err := a.env.Request(cmd, opts.ProxyOptions)
	if err != nil {
		return err
	}
	jsonOut, err := a.env.ResolveJSON(a.env.JSONOverride(cmd, opts))
	if err != nil {
		return err
	}
	var result legacysdk.UserDetail
	err = a.executeSDK(cmd.Context(), request, func(ctx context.Context, _ int64, client *pixiv.Client) (bool, error) {
		profile, err := client.User(userID).Profile(ctx)
		if err != nil {
			return false, err
		}
		result, err = record.UserDetailOutput(profile)
		return false, err
	})
	if err != nil {
		return err
	}
	if jsonOut {
		return a.env.PrintJSON(result)
	}
	return display.PrintUserDetail(a.env.Out, result)
}

func (a commandRunner) runArtworks(cmd *cobra.Command, args []string, opts listOptions) error {
	if err := validateArtworkType(opts.illustType); err != nil {
		return err
	}
	requested, err := requestedResolvedUserID(cmd.Context(), args, "user artworks")
	if err != nil {
		return err
	}
	plan, request, jsonOut, ndjson, err := a.resolve(cmd, opts, true)
	if err != nil {
		return err
	}
	fetch := func(ctx context.Context, selected int64, client *pixiv.Client, cursor sdk.Cursor) (sdk.Page[*pixiv.Artwork], error) {
		userID := requested
		if userID == 0 {
			userID = selected
		}
		return client.User(userID).Artworks(ctx, pixiv.UserArtworksOptions{Kind: pixiv.ContentType(opts.illustType), Cursor: cursor})
	}
	return a.runArtworkList(cmd.Context(), request, plan, record.FormatOf(jsonOut, ndjson), fetch)
}

func (a commandRunner) runBookmarks(cmd *cobra.Command, args []string, opts listOptions) error {
	if err := validateRelationshipRestrict(opts.restrict); err != nil {
		return err
	}
	requested, err := requestedUserID(args)
	if err != nil {
		return err
	}
	plan, request, jsonOut, ndjson, err := a.resolve(cmd, opts, true)
	if err != nil {
		return err
	}
	fetch := func(ctx context.Context, selected int64, client *pixiv.Client, cursor sdk.Cursor) (sdk.Page[*pixiv.Artwork], error) {
		userID := requested
		if userID == 0 {
			userID = selected
		}
		return client.User(userID).ArtworkBookmarks(ctx, pixiv.ArtworkBookmarksOptions{Restrict: pixiv.Restrict(opts.restrict), Tag: opts.tag, Cursor: cursor})
	}
	return a.runArtworkList(cmd.Context(), request, plan, record.FormatOf(jsonOut, ndjson), fetch)
}

// runFollowing 保持既有输出模式：该命令不参与自动 NDJSON 判定。
func (a commandRunner) runFollowing(cmd *cobra.Command, args []string, opts listOptions) error {
	if err := validateRelationshipRestrict(opts.restrict); err != nil {
		return err
	}
	requested, err := requestedResolvedUserID(cmd.Context(), args, "user following")
	if err != nil {
		return err
	}
	plan, request, jsonOut, ndjson, err := a.resolve(cmd, opts, false)
	if err != nil {
		return err
	}
	fetch := func(ctx context.Context, selected int64, client *pixiv.Client, cursor sdk.Cursor) (sdk.Page[*pixiv.User], error) {
		userID := requested
		if userID == 0 {
			userID = selected
		}
		return client.User(userID).Following(ctx, pixiv.RelationshipOptions{Restrict: pixiv.Restrict(opts.restrict), Cursor: cursor})
	}
	return a.runUsers(cmd.Context(), request, plan, record.FormatOf(jsonOut, ndjson), fetch)
}

func (a commandRunner) runNovels(cmd *cobra.Command, args []string, opts listOptions) error {
	requested, err := requestedResolvedUserID(cmd.Context(), args, "user novels")
	if err != nil {
		return err
	}
	plan, request, jsonOut, ndjson, err := a.resolve(cmd, opts, true)
	if err != nil {
		return err
	}
	fetch := func(ctx context.Context, selected int64, client *pixiv.Client, cursor sdk.Cursor) (sdk.Page[*pixiv.Novel], error) {
		userID := requested
		if userID == 0 {
			userID = selected
		}
		return client.User(userID).Novels(ctx, pixiv.UserNovelsOptions{Cursor: cursor})
	}
	return a.runNovelList(cmd.Context(), request, plan, record.FormatOf(jsonOut, ndjson), fetch)
}

func (a commandRunner) runFollowers(cmd *cobra.Command, args []string, opts listOptions) error {
	if err := validateRelationshipRestrict(opts.restrict); err != nil {
		return err
	}
	requested, err := requestedResolvedUserID(cmd.Context(), args, "user followers")
	if err != nil {
		return err
	}
	plan, request, jsonOut, ndjson, err := a.resolve(cmd, opts, true)
	if err != nil {
		return err
	}
	fetch := func(ctx context.Context, selected int64, client *pixiv.Client, cursor sdk.Cursor) (sdk.Page[*pixiv.User], error) {
		userID := requested
		if userID == 0 {
			userID = selected
		}
		return client.User(userID).Followers(ctx, pixiv.RelationshipOptions{Restrict: pixiv.Restrict(opts.restrict), Cursor: cursor})
	}
	return a.runUsers(cmd.Context(), request, plan, record.FormatOf(jsonOut, ndjson), fetch)
}

func (a commandRunner) runRelated(cmd *cobra.Command, arg string, opts listOptions) error {
	userID, err := resolveUserID(cmd.Context(), arg, "user related")
	if err != nil {
		return err
	}
	plan, request, jsonOut, ndjson, err := a.resolve(cmd, opts, true)
	if err != nil {
		return err
	}
	fetch := func(ctx context.Context, _ int64, client *pixiv.Client, cursor sdk.Cursor) (sdk.Page[*pixiv.User], error) {
		return client.User(userID).Related(ctx, pixiv.RelatedUsersOptions{Cursor: cursor})
	}
	return a.runUsers(cmd.Context(), request, plan, record.FormatOf(jsonOut, ndjson), fetch)
}

func (a commandRunner) runBlocked(cmd *cobra.Command, args []string, opts listOptions) error {
	requested, err := requestedResolvedUserID(cmd.Context(), args, "user blocked")
	if err != nil {
		return err
	}
	plan, request, jsonOut, ndjson, err := a.resolve(cmd, opts, true)
	if err != nil {
		return err
	}
	fetch := func(ctx context.Context, selected int64, client *pixiv.Client, cursor sdk.Cursor) (sdk.Page[*pixiv.User], error) {
		userID := requested
		if userID == 0 {
			userID = selected
		}
		return client.User(userID).BlockedUsers(ctx, pixiv.BlockedUsersOptions{Cursor: cursor})
	}
	return a.runUsers(cmd.Context(), request, plan, record.FormatOf(jsonOut, ndjson), fetch)
}
