// Package user owns the Pixiv user command group.
package user

import (
	"context"
	"errors"
	"fmt"
	"strings"

	deps "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivdeps"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv/follow"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv/internal/listing"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/requirements"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/parse"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

type listOptions struct {
	deps.CommandOptions
	ndjson     bool
	limit      int
	page       int
	restrict   string
	tag        string
	illustType string
}

type command struct {
	data deps.Data
}

// New builds the actual `pixiv user` command group.
func New(data deps.Data) *cobra.Command {
	a := command{data: data}
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
		follow.New(data),
	)
	data.BindNoInput(cmd)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

// NewSearch builds the canonical user-search command for the compatible
// `pixiv user search` route and root `pixiv search --type user` execution.
func NewSearch(data deps.Data) *cobra.Command {
	return command{data: data}.newSearchCommand()
}

// SearchOptions carries the flags a compatible route already parsed for the
// canonical user search. It exists so `pixiv search --type user` runs this
// owner's execution instead of a second copy.
type SearchOptions struct {
	deps.CommandOptions
	NDJSON bool
	Limit  int
	Page   int
}

// RunSearch executes the canonical user search for a compatible route.
func RunSearch(cmd *cobra.Command, data deps.Data, args []string, opts SearchOptions) error {
	return command{data: data}.runSearch(cmd, args, listOptions{
		CommandOptions: opts.CommandOptions,
		ndjson:         opts.NDJSON,
		limit:          opts.Limit,
		page:           opts.Page,
	})
}

func (a command) newSearchCommand() *cobra.Command {
	opts := listOptions{}
	cmd := &cobra.Command{
		Use:     "search WORD",
		Short:   "Search users",
		Example: "pixiv user search \"miku\" --json",
		Args:    a.data.MinArgs(1, "pixiv user search [options] WORD"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runSearch(cmd, args, opts)
		},
	}
	a.bindListCommand(cmd, &opts)
	a.data.BindTextValue(cmd, 1, -1, 0)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) newDetailCommand() *cobra.Command {
	var opts deps.CommandOptions
	cmd := &cobra.Command{
		Use:   "detail USER_ID",
		Short: "Show one user's complete profile",
		Args:  a.data.ExactArgs(1, "pixiv user detail [options] USER_ID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runDetail(cmd, args[0], opts)
		},
	}
	a.data.BindCommonFlags(cmd, &opts)
	a.data.BindTextValue(cmd, 1, 1, 0)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) newArtworksCommand() *cobra.Command {
	opts := listOptions{illustType: string(pixiv.ArtworkKindIllustration)}
	cmd := &cobra.Command{
		Use:   "artworks [USER_ID]",
		Short: "List a user's artworks",
		Args:  a.data.MaxArgs(1, "pixiv user artworks [options] [USER_ID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runArtworks(cmd, args, opts)
		},
	}
	a.bindListCommand(cmd, &opts)
	cmd.Flags().StringVarP(&opts.illustType, "type", "t", opts.illustType, "illust type: illust, manga, ugoira")
	a.data.BindTextValue(cmd, 0, 1, 0)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) newBookmarksCommand() *cobra.Command {
	opts := listOptions{restrict: string(pixiv.RestrictPublic)}
	cmd := &cobra.Command{
		Use:   "bookmarks [USER_ID]",
		Short: "List a user's bookmarks",
		Args:  a.data.MaxArgs(1, "pixiv user bookmarks [options] [USER_ID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runBookmarks(cmd, args, opts)
		},
	}
	a.bindListCommand(cmd, &opts)
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "bookmark visibility (public or private)")
	cmd.Flags().StringVar(&opts.tag, "tag", "", "filter by bookmark tag")
	a.data.BindTextValue(cmd, 0, 1, 0)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) newFollowingCommand() *cobra.Command {
	opts := listOptions{restrict: string(pixiv.RestrictPublic)}
	cmd := &cobra.Command{
		Use:   "following [USER_ID]",
		Short: "List users followed by a user",
		Args:  a.data.MaxArgs(1, "pixiv user following [options] [USER_ID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runFollowing(cmd, args, opts)
		},
	}
	a.bindListCommand(cmd, &opts)
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "follow visibility (public or private)")
	a.data.BindTextValue(cmd, 0, 1, 0)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) newNovelsCommand() *cobra.Command {
	opts := listOptions{}
	cmd := &cobra.Command{
		Use:   "novels [USER_ID]",
		Short: "List a user's novels",
		Args:  a.data.MaxArgs(1, "pixiv user novels [options] [USER_ID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runNovels(cmd, args, opts)
		},
	}
	a.bindListCommand(cmd, &opts)
	a.data.BindTextValue(cmd, 0, 1, 0)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) newFollowersCommand() *cobra.Command {
	opts := listOptions{restrict: string(pixiv.RestrictPublic)}
	cmd := &cobra.Command{
		Use:   "followers [USER_ID]",
		Short: "List a user's followers",
		Args:  a.data.MaxArgs(1, "pixiv user followers [options] [USER_ID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runFollowers(cmd, args, opts)
		},
	}
	a.bindListCommand(cmd, &opts)
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "follow visibility (public or private)")
	a.data.BindTextValue(cmd, 0, 1, 0)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) newRelatedCommand() *cobra.Command {
	opts := listOptions{}
	cmd := &cobra.Command{
		Use:   "related USER_ID",
		Short: "List users related to a user",
		Args:  a.data.ExactArgs(1, "pixiv user related [options] USER_ID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runRelated(cmd, args[0], opts)
		},
	}
	a.bindListCommand(cmd, &opts)
	a.data.BindTextValue(cmd, 1, 1, 0)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) newBlockedCommand() *cobra.Command {
	opts := listOptions{}
	cmd := &cobra.Command{
		Use:   "blocked [USER_ID]",
		Short: "List users blocked by a user",
		Args:  a.data.MaxArgs(1, "pixiv user blocked [options] [USER_ID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runBlocked(cmd, args, opts)
		},
	}
	a.bindListCommand(cmd, &opts)
	a.data.BindTextValue(cmd, 0, 1, 0)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) bindListCommand(cmd *cobra.Command, opts *listOptions) {
	a.data.BindCommonFlags(cmd, &opts.CommandOptions)
	listing.BindNDJSONFlag(cmd, &opts.ndjson)
	listing.BindListFlags(cmd, &opts.limit, &opts.page)
}

// resolve 统一列表命令的分页、传输覆盖与输出模式判定。autoNDJSON 保持各命令既有
// 的自动 NDJSON 行为，不在此处统一放宽或收紧。
func (a command) resolve(cmd *cobra.Command, opts listOptions, autoNDJSON bool) (listing.Plan, deps.Request, bool, bool, error) {
	plan, err := listing.ParsePlan(cmd, opts.limit, opts.page)
	if err != nil {
		return listing.Plan{}, deps.Request{}, false, false, err
	}
	request, err := a.data.Request(cmd, opts.CommandOptions)
	if err != nil {
		return listing.Plan{}, deps.Request{}, false, false, err
	}
	if opts.ndjson && cmd.Flags().Changed("json") {
		return listing.Plan{}, deps.Request{}, false, false, a.data.Usage(errors.New("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = a.data.JSONOut(a.data.JSONOverride(cmd, opts.CommandOptions))
		if err != nil {
			return listing.Plan{}, deps.Request{}, false, false, err
		}
	}
	ndjson := opts.ndjson
	if autoNDJSON {
		ndjson = a.data.ShouldAutoNDJSON(cmd, opts.ndjson, jsonOut)
	}
	return plan, request, jsonOut, ndjson, nil
}

// requestedUserID 解析可选的 USER_ID；省略时返回 0，由读取阶段用当前账号身份补齐。
func requestedUserID(args []string) (int64, error) {
	if len(args) != 1 {
		return 0, nil
	}
	return parse.PositiveInt64(args[0], "user_id")
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
	return a.runner().RunPooledUserList(cmd.Context(), request, plan, jsonOut, ndjson, func() string {
		return fmt.Sprintf("users for %q", word)
	}, fetch, func(items []pixiv.UserPreview) error { return printUserSearchPreviews(a.data.Output, items) })
}

func (a command) runDetail(cmd *cobra.Command, arg string, opts deps.CommandOptions) error {
	userID, err := parse.PositiveInt64(arg, "user_id")
	if err != nil {
		return err
	}
	request, err := a.data.Request(cmd, opts)
	if err != nil {
		return err
	}
	jsonOut, err := a.data.JSONOut(a.data.JSONOverride(cmd, opts))
	if err != nil {
		return err
	}
	result, err := deps.Read(a.data, cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) (pixiv.UserDetail, error) {
		return client.User(ctx, pixiv.UserRequest{UserID: userID})
	})
	if err != nil {
		return err
	}
	if jsonOut {
		return a.data.WriteJSON(pixiv.ToUserDetailDTO(result))
	}
	return printUserDetail(a.data.Output, result)
}

func (a command) runArtworks(cmd *cobra.Command, args []string, opts listOptions) error {
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
			userID, identityErr = deps.CurrentUserID(client)
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
	return a.runner().RunPooledIllustListWithHeading(cmd.Context(), request, plan, jsonOut, ndjson, func() string {
		return fmt.Sprintf("artworks by %d", userID)
	}, fetch, func(items []pixiv.Artwork, start int) error { return printArtworks(a.data.Output, items) })
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
			userID, identityErr = deps.CurrentUserID(client)
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
	return a.runner().RunPooledIllustListWithHeading(cmd.Context(), request, plan, jsonOut, ndjson, func() string {
		return fmt.Sprintf("bookmarks by %d", userID)
	}, fetch, func(items []pixiv.Artwork, start int) error { return printArtworks(a.data.Output, items) })
}

// runFollowing 保持既有输出模式：该命令不参与自动 NDJSON 判定。
func (a command) runFollowing(cmd *cobra.Command, args []string, opts listOptions) error {
	requested, err := requestedUserID(args)
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
			userID, identityErr = deps.CurrentUserID(client)
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
	return a.runner().RunPooledUserList(cmd.Context(), request, plan, jsonOut, ndjson, func() string {
		return fmt.Sprintf("users followed by %d", userID)
	}, fetch, func(items []pixiv.UserPreview) error { return printUserPreviews(a.data.Output, items) })
}

func (a command) runNovels(cmd *cobra.Command, args []string, opts listOptions) error {
	requested, err := requestedUserID(args)
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
			userID, identityErr = deps.CurrentUserID(client)
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
	return a.runner().RunPooledNovelList(cmd.Context(), request, plan, jsonOut, ndjson, fmt.Sprintf("novels by %d", userID), fetch,
		func(items []pixiv.Novel) error { return printNovels(a.data.Output, items) })
}

func (a command) runFollowers(cmd *cobra.Command, args []string, opts listOptions) error {
	requested, err := requestedUserID(args)
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
			userID, identityErr = deps.CurrentUserID(client)
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
	return a.runner().RunPooledUserList(cmd.Context(), request, plan, jsonOut, ndjson, func() string {
		return fmt.Sprintf("followers of %d", userID)
	}, fetch, func(items []pixiv.UserPreview) error { return printUserPreviews(a.data.Output, items) })
}

func (a command) runRelated(cmd *cobra.Command, arg string, opts listOptions) error {
	userID, err := parse.PositiveInt64(arg, "user_id")
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
	return a.runner().RunPooledUserList(cmd.Context(), request, plan, jsonOut, ndjson, func() string {
		return fmt.Sprintf("users related to %d", userID)
	}, fetch, func(items []pixiv.UserPreview) error { return printUserPreviews(a.data.Output, items) })
}

func (a command) runBlocked(cmd *cobra.Command, args []string, opts listOptions) error {
	requested, err := requestedUserID(args)
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
			userID, identityErr = deps.CurrentUserID(client)
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
	return a.runner().RunPooledUserList(cmd.Context(), request, plan, jsonOut, ndjson, func() string {
		return fmt.Sprintf("blocked users of %d", userID)
	}, fetch, func(items []pixiv.UserPreview) error { return printUserPreviews(a.data.Output, items) })
}

// runner is resolved only after Cobra has accepted the command input and the
// root has prepared the command's declared resources.
func (a command) runner() listing.Runner {
	return listing.New(a.data.Output, a.data)
}
