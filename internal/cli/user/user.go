// Package user owns the Pixiv user command group.
package user

import (
	"context"
	"errors"
	"fmt"
	"strings"

	commands "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/input"
	pixivcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/resolver"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

type listOptions struct {
	commands.Options
	ndjson     bool
	limit      int
	page       int
	restrict   string
	tag        string
	illustType string
}

type command struct {
	env    commands.Env
	runner pixivcmd.Runner
}

// currentUserID keeps the user commands' typed unauthorized classification
// when the opened account has no identity.
func currentUserID(client *pixiv.Client) (int64, error) {
	if id := client.UserID(); id > 0 {
		return id, nil
	}
	return 0, sdk.NewError("pixiv", "CurrentUser", sdk.Unauthorized,
		sdk.WithDetail("cannot determine current user id"))
}

// New builds the actual `pixiv user` command group. follow is assembled by the
// composition root so this owner does not import a sibling command package.
func New(env commands.Env, runner pixivcmd.Runner, follow func() *cobra.Command) *cobra.Command {
	a := command{env: env, runner: runner}
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
	if follow != nil {
		cmd.AddCommand(follow())
	}
	a.env.BindNoInput(cmd)
	commands.Bind(cmd, commands.PixivData())
	return cmd
}

// NewSearch builds the canonical user-search command for the compatible
// `pixiv user search` route and root `pixiv search --type user` execution.
func NewSearch(env commands.Env, runner pixivcmd.Runner) *cobra.Command {
	return command{env: env, runner: runner}.newSearchCommand()
}

// SearchOptions carries the flags a compatible route already parsed for the
// canonical user search. It exists so `pixiv search --type user` runs this
// owner's execution instead of a second copy.
type SearchOptions struct {
	commands.Options
	NDJSON bool
	Limit  int
	Page   int
}

// RunSearch executes the canonical user search for a compatible route.
func RunSearch(cmd *cobra.Command, env commands.Env, runner pixivcmd.Runner, args []string, opts SearchOptions) error {
	return command{env: env, runner: runner}.runSearch(cmd, args, listOptions{
		Options: opts.Options,
		ndjson:  opts.NDJSON,
		limit:   opts.Limit,
		page:    opts.Page,
	})
}

func (a command) newSearchCommand() *cobra.Command {
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
	commands.Bind(cmd, commands.PixivData())
	return cmd
}

func (a command) newDetailCommand() *cobra.Command {
	var opts commands.Options
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
	commands.Bind(cmd, commands.PixivData())
	return cmd
}

func (a command) newArtworksCommand() *cobra.Command {
	opts := listOptions{illustType: string(pixiv.ArtworkKindIllustration)}
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
	commands.Bind(cmd, commands.PixivData())
	return cmd
}

func (a command) newBookmarksCommand() *cobra.Command {
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
	commands.Bind(cmd, commands.PixivData())
	return cmd
}

func (a command) newFollowingCommand() *cobra.Command {
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
	commands.Bind(cmd, commands.PixivData())
	return cmd
}

func (a command) newNovelsCommand() *cobra.Command {
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
	commands.Bind(cmd, commands.PixivData())
	return cmd
}

func (a command) newFollowersCommand() *cobra.Command {
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
	commands.Bind(cmd, commands.PixivData())
	return cmd
}

func (a command) newRelatedCommand() *cobra.Command {
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
	commands.Bind(cmd, commands.PixivData())
	return cmd
}

func (a command) newBlockedCommand() *cobra.Command {
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
	commands.Bind(cmd, commands.PixivData())
	return cmd
}

func (a command) bindListCommand(cmd *cobra.Command, opts *listOptions) {
	a.env.BindCommonFlags(cmd, &opts.Options)
	pixivcmd.BindNDJSONFlag(cmd, &opts.ndjson)
	pixivcmd.BindListFlags(cmd, &opts.limit, &opts.page)
}

// resolve 统一列表命令的分页、传输覆盖与输出模式判定。autoNDJSON 保持各命令既有
// 的自动 NDJSON 行为，不在此处统一放宽或收紧。
func (a command) resolve(cmd *cobra.Command, opts listOptions, autoNDJSON bool) (pixivcmd.Plan, pixivcmd.Request, bool, bool, error) {
	plan, err := pixivcmd.ParsePlan(cmd, opts.limit, opts.page)
	if err != nil {
		return pixivcmd.Plan{}, pixivcmd.Request{}, false, false, err
	}
	request, err := pixivcmd.BuildRequest(cmd, opts.Options)
	if err != nil {
		return pixivcmd.Plan{}, pixivcmd.Request{}, false, false, err
	}
	if opts.ndjson && cmd.Flags().Changed("json") {
		return pixivcmd.Plan{}, pixivcmd.Request{}, false, false, a.env.UsageError(errors.New("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = a.env.JSONOut(a.env.JSONOverride(cmd, opts.Options))
		if err != nil {
			return pixivcmd.Plan{}, pixivcmd.Request{}, false, false, err
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
	return input.PositiveInt64(args[0], "user_id")
}

func resolveUserID(ctx context.Context, value, operation string) (int64, error) {
	target, err := resolver.Resolve(ctx, resolver.Input{Value: value}, resolver.Contract{
		Operation:   operation,
		DefaultType: "user",
		Types: []resolver.TypeSpec{{
			Name:              "user",
			ResultKind:        resolver.ResultKindUser,
			BareReferenceKind: pixiv.ReferenceKindUser,
		}},
	})
	if err != nil {
		return 0, fmt.Errorf("user_id: %w", err)
	}
	return target.ID, nil
}

func requestedResolvedUserID(ctx context.Context, args []string, operation string) (int64, error) {
	if len(args) != 1 {
		return 0, nil
	}
	return resolveUserID(ctx, args[0], operation)
}

func validateArtworkType(value string) error {
	switch pixiv.ArtworkKind(value) {
	case "", pixiv.ArtworkKindIllustration, pixiv.ArtworkKind("illust"), pixiv.ArtworkKindManga, pixiv.ArtworkKindUgoira:
		return nil
	default:
		return sdk.NewError("pixiv", "user artworks", sdk.InvalidArgument,
			sdk.WithDetail("type must be one of illustration, illust, manga, or ugoira"))
	}
}

func validateRelationshipRestrict(value string) error {
	switch pixiv.Restrict(value) {
	case "", pixiv.RestrictPublic, pixiv.RestrictPrivate:
		return nil
	default:
		return sdk.NewError("pixiv", "user relationships", sdk.InvalidArgument,
			sdk.WithDetail("restrict must be public or private"))
	}
}

func (a command) runSearch(cmd *cobra.Command, args []string, opts listOptions) error {
	plan, request, jsonOut, ndjson, err := a.resolve(cmd, opts, true)
	if err != nil {
		return err
	}
	word := strings.Join(args, " ")
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
		result, searchErr := client.SearchUsers(ctx, pixiv.SearchUsersRequest{Word: word, Cursor: cursor})
		if searchErr != nil {
			return nil, sdk.Cursor{}, searchErr
		}
		return result.Items, result.Next, nil
	}
	return a.runner.RunUsers(cmd.Context(), request, plan, pixivcmd.FormatOf(jsonOut, ndjson), pixivcmd.UserList{Heading: func() string {
		return fmt.Sprintf("users for %q", word)
	}, Fetch: fetch, Print: func(items []pixiv.UserPreview) error { return printUserSearchPreviews(a.env.Out, items) }})
}

func (a command) runDetail(cmd *cobra.Command, arg string, opts commands.Options) error {
	userID, err := resolveUserID(cmd.Context(), arg, "user detail")
	if err != nil {
		return err
	}
	request, err := pixivcmd.BuildRequest(cmd, opts)
	if err != nil {
		return err
	}
	jsonOut, err := a.env.JSONOut(a.env.JSONOverride(cmd, opts))
	if err != nil {
		return err
	}
	result, err := pixivcmd.Read(a.runner, cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) (pixiv.UserDetail, error) {
		return client.User(ctx, pixiv.UserRequest{UserID: userID})
	})
	if err != nil {
		return err
	}
	if jsonOut {
		return a.env.PrintJSON(result)
	}
	return printUserDetail(a.env.Out, result)
}

func (a command) runArtworks(cmd *cobra.Command, args []string, opts listOptions) error {
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
	userID := requested
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		if userID == 0 {
			var identityErr error
			userID, identityErr = currentUserID(client)
			if identityErr != nil {
				return nil, sdk.Cursor{}, identityErr
			}
		}
		result, err := client.UserArtworks(ctx, pixiv.UserArtworksRequest{UserID: userID, Kind: pixiv.ArtworkKind(opts.illustType), Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return a.runner.RunArtworks(cmd.Context(), request, plan, pixivcmd.FormatOf(jsonOut, ndjson), pixivcmd.ArtworkList{Heading: func() string {
		return fmt.Sprintf("artworks by %d", userID)
	}, Fetch: fetch, Print: func(items []pixiv.Artwork, start int) error { return pixivcmd.PrintArtworksSafe(a.env.Out, items) }})
}

func (a command) runBookmarks(cmd *cobra.Command, args []string, opts listOptions) error {
	requested, err := requestedUserID(args)
	if err != nil {
		return err
	}
	plan, request, jsonOut, ndjson, err := a.resolve(cmd, opts, true)
	if err != nil {
		return err
	}
	userID := requested
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		if userID == 0 {
			var identityErr error
			userID, identityErr = currentUserID(client)
			if identityErr != nil {
				return nil, sdk.Cursor{}, identityErr
			}
		}
		result, err := client.UserArtworkBookmarks(ctx, pixiv.UserArtworkBookmarksRequest{UserID: userID, Restrict: pixiv.Restrict(opts.restrict), Tag: opts.tag, Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return a.runner.RunArtworks(cmd.Context(), request, plan, pixivcmd.FormatOf(jsonOut, ndjson), pixivcmd.ArtworkList{Heading: func() string {
		return fmt.Sprintf("bookmarks by %d", userID)
	}, Fetch: fetch, Print: func(items []pixiv.Artwork, start int) error { return pixivcmd.PrintArtworksSafe(a.env.Out, items) }})
}

// runFollowing 保持既有输出模式：该命令不参与自动 NDJSON 判定。
func (a command) runFollowing(cmd *cobra.Command, args []string, opts listOptions) error {
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
	userID := requested
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
		if userID == 0 {
			var identityErr error
			userID, identityErr = currentUserID(client)
			if identityErr != nil {
				return nil, sdk.Cursor{}, identityErr
			}
		}
		result, err := client.UserFollowing(ctx, pixiv.UserFollowingRequest{UserID: userID, Restrict: pixiv.Restrict(opts.restrict), Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return a.runner.RunUsers(cmd.Context(), request, plan, pixivcmd.FormatOf(jsonOut, ndjson), pixivcmd.UserList{Heading: func() string {
		return fmt.Sprintf("users followed by %d", userID)
	}, Fetch: fetch, Print: func(items []pixiv.UserPreview) error { return pixivcmd.PrintUserPreviewsSafe(a.env.Out, items) }})
}

func (a command) runNovels(cmd *cobra.Command, args []string, opts listOptions) error {
	requested, err := requestedResolvedUserID(cmd.Context(), args, "user novels")
	if err != nil {
		return err
	}
	plan, request, jsonOut, ndjson, err := a.resolve(cmd, opts, true)
	if err != nil {
		return err
	}
	userID := requested
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
		if userID == 0 {
			var identityErr error
			userID, identityErr = currentUserID(client)
			if identityErr != nil {
				return nil, sdk.Cursor{}, identityErr
			}
		}
		result, err := client.UserNovels(ctx, pixiv.UserNovelsRequest{UserID: userID, Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return a.runner.RunNovels(cmd.Context(), request, plan, pixivcmd.FormatOf(jsonOut, ndjson), pixivcmd.NovelList{Heading: fmt.Sprintf("novels by %d", userID), Fetch: fetch, Print: func(items []pixiv.Novel) error { return pixivcmd.PrintNovelsSafe(a.env.Out, items) }})
}

func (a command) runFollowers(cmd *cobra.Command, args []string, opts listOptions) error {
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
	userID := requested
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
		if userID == 0 {
			var identityErr error
			userID, identityErr = currentUserID(client)
			if identityErr != nil {
				return nil, sdk.Cursor{}, identityErr
			}
		}
		result, err := client.UserFollowers(ctx, pixiv.UserFollowersRequest{UserID: userID, Restrict: pixiv.Restrict(opts.restrict), Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return a.runner.RunUsers(cmd.Context(), request, plan, pixivcmd.FormatOf(jsonOut, ndjson), pixivcmd.UserList{Heading: func() string {
		return fmt.Sprintf("followers of %d", userID)
	}, Fetch: fetch, Print: func(items []pixiv.UserPreview) error { return pixivcmd.PrintUserPreviewsSafe(a.env.Out, items) }})
}

func (a command) runRelated(cmd *cobra.Command, arg string, opts listOptions) error {
	userID, err := resolveUserID(cmd.Context(), arg, "user related")
	if err != nil {
		return err
	}
	plan, request, jsonOut, ndjson, err := a.resolve(cmd, opts, true)
	if err != nil {
		return err
	}
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
		result, err := client.RelatedUsers(ctx, pixiv.RelatedUsersRequest{UserID: userID, Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return a.runner.RunUsers(cmd.Context(), request, plan, pixivcmd.FormatOf(jsonOut, ndjson), pixivcmd.UserList{Heading: func() string {
		return fmt.Sprintf("users related to %d", userID)
	}, Fetch: fetch, Print: func(items []pixiv.UserPreview) error { return pixivcmd.PrintUserPreviewsSafe(a.env.Out, items) }})
}

func (a command) runBlocked(cmd *cobra.Command, args []string, opts listOptions) error {
	requested, err := requestedResolvedUserID(cmd.Context(), args, "user blocked")
	if err != nil {
		return err
	}
	plan, request, jsonOut, ndjson, err := a.resolve(cmd, opts, true)
	if err != nil {
		return err
	}
	userID := requested
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
		if userID == 0 {
			var identityErr error
			userID, identityErr = currentUserID(client)
			if identityErr != nil {
				return nil, sdk.Cursor{}, identityErr
			}
		}
		result, err := client.UserBlockedUsers(ctx, pixiv.UserBlockedUsersRequest{UserID: userID, Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return a.runner.RunUsers(cmd.Context(), request, plan, pixivcmd.FormatOf(jsonOut, ndjson), pixivcmd.UserList{Heading: func() string {
		return fmt.Sprintf("blocked users of %d", userID)
	}, Fetch: fetch, Print: func(items []pixiv.UserPreview) error { return pixivcmd.PrintUserPreviewsSafe(a.env.Out, items) }})
}
