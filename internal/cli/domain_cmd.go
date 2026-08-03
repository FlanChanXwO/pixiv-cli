package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/parse"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

type userListOptions struct {
	commandOptions
	ndjsonOutputOptions
	listOptions
	restrict   string
	tag        string
	illustType string
}

type userSearchOptions struct {
	commandOptions
	ndjsonOutputOptions
	listOptions
}

type mutationOptions struct {
	commandOptions
	restrict string
	tags     []string
	onError  string
}

func (a app) newUserCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "user", Short: "Query a Pixiv user"}
	cmd.AddCommand(a.newUserSearchCommand(), a.newUserDetailCommand(), a.newUserArtworksCommand(), a.newUserBookmarksCommand(), a.newUserFollowingCommand())
	return cmd
}

func (a app) newUserSearchCommand() *cobra.Command {
	opts := userSearchOptions{}
	cmd := &cobra.Command{Use: "search WORD", Short: "Search users", Example: "pixiv user search \"miku\" --json", Args: requireMinArgs(1, "pixiv user search [options] WORD"), RunE: func(cmd *cobra.Command, args []string) error {
		return a.runUserSearch(cmd, args, opts)
	}}
	a.bindCommonFlags(cmd, &opts.commandOptions)
	bindNDJSONFlag(cmd, &opts.ndjsonOutputOptions)
	bindListFlags(cmd, &opts.listOptions)
	return cmd
}

func (a app) runUserSearch(cmd *cobra.Command, args []string, opts userSearchOptions) error {
	plan, err := parseListPlan(cmd, opts.listOptions)
	if err != nil {
		return err
	}
	request, jsonOverride, err := a.sdkRequest(cmd, opts.commandOptions)
	if err != nil {
		return err
	}
	services := a.services()
	if opts.ndjson && cmd.Flags().Changed("json") {
		return newUsageError(fmt.Errorf("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = services.SDK.JSONOut(jsonOverride)
		if err != nil {
			return err
		}
	}
	word := strings.Join(args, " ")
	return services.SDK.RunPooledOperation(cmd.Context(), request, func(ctx context.Context, client application.SDKClient) (bool, error) {
		fetch := func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
			result, searchErr := client.SearchUsers(ctx, pixiv.SearchUsersRequest{Word: word, Cursor: cursor})
			if searchErr != nil {
				return nil, sdk.Cursor{}, searchErr
			}
			return result.Items, result.Next, nil
		}
		committed := false
		if opts.ndjson {
			encoder := json.NewEncoder(a.out)
			err := pageItems(ctx, plan, fetch, func(items []pixiv.UserPreview) error {
				for _, item := range items {
					record, err := application.RecordFromUserPreview(item)
					if err != nil {
						return err
					}
					committed = true
					if err := encoder.Encode(record); err != nil {
						return err
					}
				}
				return nil
			})
			return committed, err
		}
		if jsonOut {
			spool, err := newJSONArraySpool("user_previews")
			if err != nil {
				return false, err
			}
			defer spool.Close()
			if err := pageItems(ctx, plan, fetch, func(items []pixiv.UserPreview) error { return appendJSONArray(spool, items) }); err != nil {
				return false, err
			}
			committed = true
			err = spool.Commit(a.out)
			return committed, err
		}
		headingWritten := false
		err := pageItems(ctx, plan, fetch, func(items []pixiv.UserPreview) error {
			if !headingWritten {
				committed = true
				if _, err := fmt.Fprintln(a.out, fmt.Sprintf("users for %q", word)); err != nil {
					return err
				}
				headingWritten = true
			}
			if len(items) > 0 {
				committed = true
			}
			if err := printUserSearchPreviews(a.out, items); err != nil {
				return err
			}
			return nil
		})
		return committed, err
	})
}

func printUserSearchPreviews(out io.Writer, items []pixiv.UserPreview) error {
	for _, item := range items {
		line := fmt.Sprintf("%d %s", item.User.ID, safeTextLine(item.User.Name))
		if item.User.Account != "" {
			line += " (@" + safeTextLine(item.User.Account) + ")"
		}
		if item.User.Comment != "" {
			line += " — " + safeTextLine(item.User.Comment)
		}
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}
	return nil
}

func (a app) newUserDetailCommand() *cobra.Command {
	var opts commandOptions
	cmd := &cobra.Command{Use: "detail USER_ID", Short: "Show one user's complete profile", Args: requireExactArgs(1, "pixiv user detail [options] USER_ID"), RunE: func(cmd *cobra.Command, args []string) error {
		return a.runUserDetail(cmd, args[0], opts)
	}}
	a.bindCommonFlags(cmd, &opts)
	return cmd
}

func (a app) runUserDetail(cmd *cobra.Command, arg string, opts commandOptions) error {
	userID, err := parse.PositiveInt64(arg, "user_id")
	if err != nil {
		return err
	}
	services := a.services()
	request, jsonOverride, err := a.sdkRequest(cmd, opts)
	if err != nil {
		return err
	}
	jsonOut, err := services.SDK.JSONOut(jsonOverride)
	if err != nil {
		return err
	}
	return services.SDK.RunPooledOperation(cmd.Context(), request, func(ctx context.Context, client application.SDKClient) (bool, error) {
		result, err := client.User(ctx, pixiv.UserRequest{UserID: userID})
		if err != nil {
			return false, err
		}
		if jsonOut {
			// 下游写入开始即禁止因伪装成 429 的 writer error 重放请求。
			committed := true
			err = a.printJSON(result)
			return committed, err
		} else {
			committed := true
			err = printUserDetail(a.out, result)
			return committed, err
		}
	})
}

// printUserDetail 只展示人可读且有值的文本字段；固定计数保留零值，以免把公开的
// “没有作品/关注”误报为字段缺失。完整机器可读字段由 --json 原样输出 SDK 模型。
func printUserDetail(out io.Writer, result pixiv.UserDetail) error {
	lines := []string{fmt.Sprintf("user id: %d", result.User.ID)}
	if result.User.Name != "" {
		lines = append(lines, fmt.Sprintf("name: %s", result.User.Name))
	}
	if result.User.Account != "" {
		lines = append(lines, fmt.Sprintf("account: %s", result.User.Account))
	}
	if result.User.Comment != "" {
		lines = append(lines, fmt.Sprintf("comment: %s", result.User.Comment))
	}
	if webpage := publicWebpage(result.Profile.Webpage); webpage != "" {
		lines = append(lines, fmt.Sprintf("webpage: %s", webpage))
	}
	if result.Profile.Region != "" {
		lines = append(lines, fmt.Sprintf("region: %s", result.Profile.Region))
	}
	if result.Profile.CountryCode != "" {
		lines = append(lines, fmt.Sprintf("country: %s", result.Profile.CountryCode))
	}
	if result.Profile.Job != "" {
		lines = append(lines, fmt.Sprintf("job: %s", result.Profile.Job))
	}
	lines = append(lines,
		fmt.Sprintf("artworks: %d", result.Profile.TotalIllusts),
		fmt.Sprintf("manga: %d", result.Profile.TotalManga),
		fmt.Sprintf("novels: %d", result.Profile.TotalNovels),
		fmt.Sprintf("following: %d", result.Profile.TotalFollowUsers),
	)
	for _, field := range []struct {
		name  string
		value string
	}{
		{"workspace pc", result.Workspace.PC},
		{"workspace monitor", result.Workspace.Monitor},
		{"workspace tool", result.Workspace.Tool},
		{"workspace scanner", result.Workspace.Scanner},
		{"workspace tablet", result.Workspace.Tablet},
		{"workspace mouse", result.Workspace.Mouse},
		{"workspace printer", result.Workspace.Printer},
		{"workspace desktop", result.Workspace.Desktop},
		{"workspace music", result.Workspace.Music},
		{"workspace desk", result.Workspace.Desk},
		{"workspace chair", result.Workspace.Chair},
		{"workspace comment", result.Workspace.Comment},
	} {
		if field.value != "" {
			lines = append(lines, fmt.Sprintf("%s: %s", field.name, field.value))
		}
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}
	return nil
}

// publicWebpage 将可显示的个人主页限定为有主机的 HTTP(S) 地址，并移除可能含有
// 私密信息的 userinfo、query 和 fragment。机器接口仍由 --json 原样返回完整 SDK 模型。
func publicWebpage(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return parsed.String()
}

func (a app) newUserArtworksCommand() *cobra.Command {
	opts := userListOptions{illustType: string(pixiv.ArtworkKindIllustration)}
	cmd := &cobra.Command{Use: "artworks [USER_ID]", Short: "List a user's artworks", Args: requireMaxArgs(1, "pixiv user artworks [options] [USER_ID]"), RunE: func(cmd *cobra.Command, args []string) error {
		return a.runUserArtworks(cmd, args, opts)
	}}
	a.bindCommonFlags(cmd, &opts.commandOptions)
	bindNDJSONFlag(cmd, &opts.ndjsonOutputOptions)
	bindListFlags(cmd, &opts.listOptions)
	cmd.Flags().StringVar(&opts.illustType, "type", opts.illustType, "illust type: illust, manga, ugoira")
	return cmd
}

func (a app) newUserBookmarksCommand() *cobra.Command {
	opts := userListOptions{restrict: string(pixiv.RestrictPublic)}
	cmd := &cobra.Command{Use: "bookmarks [USER_ID]", Short: "List a user's bookmarks", Args: requireMaxArgs(1, "pixiv user bookmarks [options] [USER_ID]"), RunE: func(cmd *cobra.Command, args []string) error {
		return a.runUserBookmarks(cmd, args, opts)
	}}
	a.bindCommonFlags(cmd, &opts.commandOptions)
	bindNDJSONFlag(cmd, &opts.ndjsonOutputOptions)
	bindListFlags(cmd, &opts.listOptions)
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "bookmark visibility (public or private)")
	cmd.Flags().StringVar(&opts.tag, "tag", "", "filter by bookmark tag")
	return cmd
}

func (a app) newUserFollowingCommand() *cobra.Command {
	opts := userListOptions{restrict: string(pixiv.RestrictPublic)}
	cmd := &cobra.Command{Use: "following [USER_ID]", Short: "List users followed by a user", Args: requireMaxArgs(1, "pixiv user following [options] [USER_ID]"), RunE: func(cmd *cobra.Command, args []string) error {
		return a.runUserFollowing(cmd, args, opts)
	}}
	a.bindCommonFlags(cmd, &opts.commandOptions)
	bindNDJSONFlag(cmd, &opts.ndjsonOutputOptions)
	bindListFlags(cmd, &opts.listOptions)
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "follow visibility (public or private)")
	return cmd
}

func (a app) runUserArtworks(cmd *cobra.Command, args []string, options userListOptions) error {
	plan, err := parseListPlan(cmd, options.listOptions)
	if err != nil {
		return err
	}
	var requestedUserID int64
	if len(args) == 1 {
		requestedUserID, err = parse.PositiveInt64(args[0], "user_id")
		if err != nil {
			return err
		}
	}
	services := a.services()
	request, jsonOverride, err := a.sdkRequest(cmd, options.commandOptions)
	if err != nil {
		return err
	}
	if options.ndjson && cmd.Flags().Changed("json") {
		return newUsageError(fmt.Errorf("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !options.ndjson {
		jsonOut, err = services.SDK.JSONOut(jsonOverride)
		if err != nil {
			return err
		}
	}
	options.ndjson = a.shouldAutoNDJSON(cmd, options.ndjson, jsonOut)
	userID := requestedUserID
	fetch := func(client application.SDKClient, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		if userID == 0 {
			var identityErr error
			userID, identityErr = client.CurrentUserID(ctx)
			if identityErr != nil {
				return nil, sdk.Cursor{}, identityErr
			}
		}
		result, err := client.UserArtworks(ctx, pixiv.UserArtworksRequest{UserID: userID, Kind: pixiv.ArtworkKind(options.illustType), Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return a.runPooledIllustListWithHeading(cmd.Context(), request, plan, jsonOut, options.ndjson, func() string {
		return fmt.Sprintf("artworks by %d", userID)
	}, fetch, func(items []pixiv.Artwork, start int) error { return printIllusts(a.out, items, start, false) })
}

func (a app) runUserBookmarks(cmd *cobra.Command, args []string, options userListOptions) error {
	plan, err := parseListPlan(cmd, options.listOptions)
	if err != nil {
		return err
	}
	var requestedUserID int64
	if len(args) == 1 {
		requestedUserID, err = parse.PositiveInt64(args[0], "user_id")
		if err != nil {
			return err
		}
	}
	services := a.services()
	request, jsonOverride, err := a.sdkRequest(cmd, options.commandOptions)
	if err != nil {
		return err
	}
	if options.ndjson && cmd.Flags().Changed("json") {
		return newUsageError(fmt.Errorf("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !options.ndjson {
		jsonOut, err = services.SDK.JSONOut(jsonOverride)
		if err != nil {
			return err
		}
	}
	options.ndjson = a.shouldAutoNDJSON(cmd, options.ndjson, jsonOut)
	userID := requestedUserID
	fetch := func(client application.SDKClient, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		if userID == 0 {
			var identityErr error
			userID, identityErr = client.CurrentUserID(ctx)
			if identityErr != nil {
				return nil, sdk.Cursor{}, identityErr
			}
		}
		result, err := client.UserArtworkBookmarks(ctx, pixiv.UserArtworkBookmarksRequest{UserID: userID, Restrict: pixiv.Restrict(options.restrict), Tag: options.tag, Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return a.runPooledIllustListWithHeading(cmd.Context(), request, plan, jsonOut, options.ndjson, func() string {
		return fmt.Sprintf("bookmarks by %d", userID)
	}, fetch, func(items []pixiv.Artwork, start int) error { return printIllusts(a.out, items, start, false) })
}

func (a app) runUserFollowing(cmd *cobra.Command, args []string, options userListOptions) error {
	plan, err := parseListPlan(cmd, options.listOptions)
	if err != nil {
		return err
	}
	var requestedUserID int64
	if len(args) == 1 {
		requestedUserID, err = parse.PositiveInt64(args[0], "user_id")
		if err != nil {
			return err
		}
	}
	services := a.services()
	request, jsonOverride, err := a.sdkRequest(cmd, options.commandOptions)
	if err != nil {
		return err
	}
	if options.ndjson && cmd.Flags().Changed("json") {
		return newUsageError(fmt.Errorf("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !options.ndjson {
		jsonOut, err = services.SDK.JSONOut(jsonOverride)
		if err != nil {
			return err
		}
	}
	userID := requestedUserID
	fetch := func(client application.SDKClient, ctx context.Context, cursor sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
		if userID == 0 {
			var identityErr error
			userID, identityErr = client.CurrentUserID(ctx)
			if identityErr != nil {
				return nil, sdk.Cursor{}, identityErr
			}
		}
		result, err := client.UserFollowing(ctx, pixiv.UserFollowingRequest{UserID: userID, Restrict: pixiv.Restrict(options.restrict), Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return a.runPooledUserList(cmd.Context(), request, plan, jsonOut, options.ndjson, func() string {
		return fmt.Sprintf("users followed by %d", userID)
	}, fetch, func(items []pixiv.UserPreview) error { return printUserPreviews(a.out, items) })
}

func (a app) newBookmarkCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "bookmark", Short: "Manage illustration bookmarks"}
	cmd.AddCommand(a.newBookmarkAddCommand(), a.newBookmarkRemoveCommand())
	return cmd
}

func (a app) newBookmarkAddCommand() *cobra.Command {
	opts := mutationOptions{restrict: string(pixiv.RestrictPublic)}
	cmd := &cobra.Command{Use: "add [ILLUST_ID]", Short: "Bookmark an illustration", Args: actionInputArgs(a.in, "pixiv bookmark add [options] [ILLUST_ID]"), RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := recordFailureStrategy(opts.onError); err != nil {
			return err
		}
		invoke := a.lazyActionInvoker(cmd, opts.commandOptions, func(client application.SDKClient, ctx context.Context, id int64) error {
			return client.AddBookmark(ctx, pixiv.AddBookmarkRequest{ArtworkID: id, Restrict: pixiv.Restrict(opts.restrict), Tags: opts.tags})
		})
		if len(args) == 0 {
			return a.consumeActionRecords(cmd, "bookmark_add", opts.onError, visualRecordTypes, invoke)
		}
		id, err := parse.PositiveInt64(args[0], "illust_id")
		if err != nil {
			return newUsageError(err)
		}
		return invoke(cmd.Context(), id)
	}}
	a.bindActionFlags(cmd, &opts.commandOptions)
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "bookmark visibility (public or private)")
	cmd.Flags().StringArrayVar(&opts.tags, "tag", nil, "bookmark tag; may be repeated")
	cmd.Flags().StringVar(&opts.onError, "on-error", "skip", "record failure strategy: skip or fail-fast")
	return cmd
}

func (a app) newBookmarkRemoveCommand() *cobra.Command {
	opts := mutationOptions{}
	cmd := &cobra.Command{Use: "remove [ILLUST_ID]", Short: "Remove an illustration bookmark", Args: actionInputArgs(a.in, "pixiv bookmark remove [options] [ILLUST_ID]"), RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := recordFailureStrategy(opts.onError); err != nil {
			return err
		}
		invoke := a.lazyActionInvoker(cmd, opts.commandOptions, func(client application.SDKClient, ctx context.Context, id int64) error {
			return client.RemoveBookmark(ctx, pixiv.RemoveBookmarkRequest{ArtworkID: id})
		})
		if len(args) == 0 {
			return a.consumeActionRecords(cmd, "bookmark_remove", opts.onError, visualRecordTypes, invoke)
		}
		id, err := parse.PositiveInt64(args[0], "illust_id")
		if err != nil {
			return newUsageError(err)
		}
		return invoke(cmd.Context(), id)
	}}
	a.bindActionFlags(cmd, &opts.commandOptions)
	cmd.Flags().StringVar(&opts.onError, "on-error", "skip", "record failure strategy: skip or fail-fast")
	return cmd
}

func (a app) newFollowCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "follow", Short: "Manage followed users"}
	cmd.AddCommand(a.newFollowAddCommand(), a.newFollowRemoveCommand())
	return cmd
}

func (a app) newFollowAddCommand() *cobra.Command {
	opts := mutationOptions{restrict: string(pixiv.RestrictPublic)}
	cmd := &cobra.Command{Use: "add [USER_ID]", Short: "Follow a user", Args: actionInputArgs(a.in, "pixiv follow add [options] [USER_ID]"), RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := recordFailureStrategy(opts.onError); err != nil {
			return err
		}
		invoke := a.lazyActionInvoker(cmd, opts.commandOptions, func(client application.SDKClient, ctx context.Context, id int64) error {
			return client.FollowUser(ctx, pixiv.FollowUserRequest{UserID: id, Restrict: pixiv.Restrict(opts.restrict)})
		})
		if len(args) == 0 {
			return a.consumeActionRecords(cmd, "follow_add", opts.onError, userRecordTypes, invoke)
		}
		id, err := parse.PositiveInt64(args[0], "user_id")
		if err != nil {
			return newUsageError(err)
		}
		return invoke(cmd.Context(), id)
	}}
	a.bindActionFlags(cmd, &opts.commandOptions)
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "follow visibility (public or private)")
	cmd.Flags().StringVar(&opts.onError, "on-error", "skip", "record failure strategy: skip or fail-fast")
	return cmd
}

func (a app) newFollowRemoveCommand() *cobra.Command {
	opts := mutationOptions{}
	cmd := &cobra.Command{Use: "remove [USER_ID]", Short: "Unfollow a user", Args: actionInputArgs(a.in, "pixiv follow remove [options] [USER_ID]"), RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := recordFailureStrategy(opts.onError); err != nil {
			return err
		}
		invoke := a.lazyActionInvoker(cmd, opts.commandOptions, func(client application.SDKClient, ctx context.Context, id int64) error {
			return client.UnfollowUser(ctx, pixiv.UnfollowUserRequest{UserID: id})
		})
		if len(args) == 0 {
			return a.consumeActionRecords(cmd, "follow_remove", opts.onError, userRecordTypes, invoke)
		}
		id, err := parse.PositiveInt64(args[0], "user_id")
		if err != nil {
			return newUsageError(err)
		}
		return invoke(cmd.Context(), id)
	}}
	a.bindActionFlags(cmd, &opts.commandOptions)
	cmd.Flags().StringVar(&opts.onError, "on-error", "skip", "record failure strategy: skip or fail-fast")
	return cmd
}

// lazyActionInvoker 仅在拿到第一条可执行输入后再打开 SDK operation。空 stdin
// 不读取认证配置也不访问网络；写操作始终固定在同一账号快照，绝不进入账号池重放。
func (a app) lazyActionInvoker(cmd *cobra.Command, options commandOptions, invoke func(application.SDKClient, context.Context, int64) error) func(context.Context, int64) error {
	var client application.SDKClient
	return func(ctx context.Context, id int64) error {
		if client == nil {
			request, _, err := a.sdkRequest(cmd, options)
			if err != nil {
				return err
			}
			opened, err := a.services().SDK.OpenOperation(ctx, request)
			if err != nil {
				return err
			}
			client = opened
		}
		return invoke(client, ctx, id)
	}
}
