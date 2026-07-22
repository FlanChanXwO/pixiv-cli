package cli

import (
	"context"
	"fmt"
	"io"
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/parse"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/spf13/cobra"
)

type userListOptions struct {
	commandOptions
	listOptions
	restrict   string
	tag        string
	illustType string
}

type mutationOptions struct {
	commandOptions
	restrict string
	tags     []string
}

func (a app) newUserCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "user", Short: "Query a Pixiv user"}
	cmd.AddCommand(a.newUserDetailCommand(), a.newUserArtworksCommand(), a.newUserBookmarksCommand(), a.newUserFollowingCommand())
	return cmd
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
	client, err := services.SDK.OpenOperation(cmd.Context(), request)
	if err != nil {
		return err
	}
	result, err := client.UserDetail(cmd.Context(), sdk.UserDetailRequest{UserID: userID})
	if err != nil {
		return err
	}
	if result == nil {
		return fmt.Errorf("pixiv sdk returned an empty user detail result")
	}
	if jsonOut {
		return a.printJSON(result)
	}
	printUserDetail(a.out, *result)
	return nil
}

// printUserDetail 只展示人可读且有值的文本字段；固定计数保留零值，以免把公开的
// “没有作品/关注”误报为字段缺失。完整机器可读字段由 --json 原样输出 SDK 模型。
func printUserDetail(out io.Writer, result sdk.UserDetailResult) {
	fmt.Fprintf(out, "user id: %d\n", result.User.ID)
	if result.User.Name != "" {
		fmt.Fprintf(out, "name: %s\n", result.User.Name)
	}
	if result.User.Account != "" {
		fmt.Fprintf(out, "account: %s\n", result.User.Account)
	}
	if result.User.Comment != "" {
		fmt.Fprintf(out, "comment: %s\n", result.User.Comment)
	}
	if result.Profile.Webpage != nil {
		if webpage := publicWebpage(*result.Profile.Webpage); webpage != "" {
			fmt.Fprintf(out, "webpage: %s\n", webpage)
		}
	}
	if result.Profile.Region != "" {
		fmt.Fprintf(out, "region: %s\n", result.Profile.Region)
	}
	if result.Profile.CountryCode != "" {
		fmt.Fprintf(out, "country: %s\n", result.Profile.CountryCode)
	}
	if result.Profile.Job != "" {
		fmt.Fprintf(out, "job: %s\n", result.Profile.Job)
	}
	fmt.Fprintf(out, "artworks: %d\n", result.Profile.TotalIllusts)
	fmt.Fprintf(out, "manga: %d\n", result.Profile.TotalManga)
	fmt.Fprintf(out, "novels: %d\n", result.Profile.TotalNovels)
	fmt.Fprintf(out, "following: %d\n", result.Profile.TotalFollowUsers)
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
			fmt.Fprintf(out, "%s: %s\n", field.name, field.value)
		}
	}
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
	opts := userListOptions{illustType: string(sdk.IllustTypeIllust)}
	cmd := &cobra.Command{Use: "artworks [USER_ID]", Short: "List a user's artworks", Args: requireMaxArgs(1, "pixiv user artworks [options] [USER_ID]"), RunE: func(cmd *cobra.Command, args []string) error {
		return a.runUserArtworks(cmd, args, opts)
	}}
	a.bindCommonFlags(cmd, &opts.commandOptions)
	bindListFlags(cmd, &opts.listOptions)
	cmd.Flags().StringVar(&opts.illustType, "type", opts.illustType, "illust type: illust, manga, ugoira")
	return cmd
}

func (a app) newUserBookmarksCommand() *cobra.Command {
	opts := userListOptions{restrict: string(sdk.RestrictPublic)}
	cmd := &cobra.Command{Use: "bookmarks [USER_ID]", Short: "List a user's bookmarks", Args: requireMaxArgs(1, "pixiv user bookmarks [options] [USER_ID]"), RunE: func(cmd *cobra.Command, args []string) error {
		return a.runUserBookmarks(cmd, args, opts)
	}}
	a.bindCommonFlags(cmd, &opts.commandOptions)
	bindListFlags(cmd, &opts.listOptions)
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "bookmark visibility (public or private)")
	cmd.Flags().StringVar(&opts.tag, "tag", "", "filter by bookmark tag")
	return cmd
}

func (a app) newUserFollowingCommand() *cobra.Command {
	opts := userListOptions{restrict: string(sdk.RestrictPublic)}
	cmd := &cobra.Command{Use: "following [USER_ID]", Short: "List users followed by a user", Args: requireMaxArgs(1, "pixiv user following [options] [USER_ID]"), RunE: func(cmd *cobra.Command, args []string) error {
		return a.runUserFollowing(cmd, args, opts)
	}}
	a.bindCommonFlags(cmd, &opts.commandOptions)
	bindListFlags(cmd, &opts.listOptions)
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "follow visibility (public or private)")
	return cmd
}

func (a app) resolveUserID(cmd *cobra.Command, args []string, options commandOptions) (sdkUserID int64, client application.SDKClient, err error) {
	if len(args) == 1 {
		id, parseErr := parse.PositiveInt64(args[0], "user_id")
		if parseErr != nil {
			return 0, nil, parseErr
		}
		request, _, requestErr := a.sdkRequest(cmd, options)
		if requestErr != nil {
			return 0, nil, requestErr
		}
		services := a.services()
		client, err = services.SDK.OpenOperation(cmd.Context(), request)
		if err != nil {
			return 0, nil, err
		}
		return id, client, nil
	}
	request, _, err := a.sdkRequest(cmd, options)
	if err != nil {
		return 0, nil, err
	}
	services := a.services()
	client, sdkUserID, err = services.SDK.CurrentUserID(cmd.Context(), request)
	return sdkUserID, client, err
}

func (a app) runUserArtworks(cmd *cobra.Command, args []string, options userListOptions) error {
	plan, err := parseListPlan(cmd, options.listOptions)
	if err != nil {
		return err
	}
	userID, client, err := a.resolveUserID(cmd, args, options.commandOptions)
	if err != nil {
		return err
	}
	services := a.services()
	_, jsonOverride, err := a.sdkRequest(cmd, options.commandOptions)
	if err != nil {
		return err
	}
	jsonOut, err := services.SDK.JSONOut(jsonOverride)
	if err != nil {
		return err
	}
	if !jsonOut {
		fmt.Fprintf(a.out, "artworks by %d\n", userID)
	}
	return a.runIllustList(cmd.Context(), plan, jsonOut, func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
		result, err := client.UserArtworks(ctx, sdk.UserArtworksRequest{UserID: userID, Type: sdk.IllustType(options.illustType), Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		return result.Illusts, result.NextCursor, nil
	}, func(items []sdk.Illust, start int) { printIllusts(a.out, items, start, false) })
}

func (a app) runUserBookmarks(cmd *cobra.Command, args []string, options userListOptions) error {
	plan, err := parseListPlan(cmd, options.listOptions)
	if err != nil {
		return err
	}
	userID, client, err := a.resolveUserID(cmd, args, options.commandOptions)
	if err != nil {
		return err
	}
	services := a.services()
	_, jsonOverride, err := a.sdkRequest(cmd, options.commandOptions)
	if err != nil {
		return err
	}
	jsonOut, err := services.SDK.JSONOut(jsonOverride)
	if err != nil {
		return err
	}
	if !jsonOut {
		fmt.Fprintf(a.out, "bookmarks by %d\n", userID)
	}
	return a.runIllustList(cmd.Context(), plan, jsonOut, func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
		result, err := client.UserBookmarks(ctx, sdk.UserBookmarksRequest{UserID: userID, Restrict: sdk.Restrict(options.restrict), Tag: options.tag, Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		return result.Illusts, result.NextCursor, nil
	}, func(items []sdk.Illust, start int) { printIllusts(a.out, items, start, false) })
}

func (a app) runUserFollowing(cmd *cobra.Command, args []string, options userListOptions) error {
	plan, err := parseListPlan(cmd, options.listOptions)
	if err != nil {
		return err
	}
	userID, client, err := a.resolveUserID(cmd, args, options.commandOptions)
	if err != nil {
		return err
	}
	services := a.services()
	_, jsonOverride, err := a.sdkRequest(cmd, options.commandOptions)
	if err != nil {
		return err
	}
	jsonOut, err := services.SDK.JSONOut(jsonOverride)
	if err != nil {
		return err
	}
	if !jsonOut {
		fmt.Fprintf(a.out, "users followed by %d\n", userID)
	}
	return a.runUserList(cmd.Context(), plan, jsonOut, func(ctx context.Context, cursor sdk.Cursor) ([]sdk.UserPreview, sdk.Cursor, error) {
		result, err := client.UserFollowing(ctx, sdk.UserFollowingRequest{UserID: userID, Restrict: sdk.Restrict(options.restrict), Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		return result.UserPreviews, result.NextCursor, nil
	}, func(items []sdk.UserPreview) {
		for _, item := range items {
			fmt.Fprintf(a.out, "%d %s\n", item.User.ID, item.User.Name)
		}
	})
}

func (a app) newBookmarkCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "bookmark", Short: "Manage illustration bookmarks"}
	cmd.AddCommand(a.newBookmarkAddCommand(), a.newBookmarkRemoveCommand())
	return cmd
}

func (a app) newBookmarkAddCommand() *cobra.Command {
	opts := mutationOptions{restrict: string(sdk.RestrictPublic)}
	cmd := &cobra.Command{Use: "add ILLUST_ID", Short: "Bookmark an illustration", Args: requireExactArgs(1, "pixiv bookmark add [options] ILLUST_ID"), RunE: func(cmd *cobra.Command, args []string) error {
		id, err := parse.PositiveInt64(args[0], "illust_id")
		if err != nil {
			return err
		}
		request, jsonOverride, err := a.sdkRequest(cmd, opts.commandOptions)
		if err != nil {
			return err
		}
		jsonOut, err := a.services().SDK.JSONOut(jsonOverride)
		if err != nil {
			return err
		}
		client, err := a.services().SDK.OpenOperation(cmd.Context(), request)
		if err != nil {
			return err
		}
		if err := client.AddBookmark(cmd.Context(), sdk.AddBookmarkRequest{IllustID: id, Restrict: sdk.Restrict(opts.restrict), Tags: opts.tags}); err != nil {
			return err
		}
		return a.printMutation(jsonOut, "bookmarked", "illust_id", id)
	}}
	a.bindCommonFlags(cmd, &opts.commandOptions)
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "bookmark visibility (public or private)")
	cmd.Flags().StringArrayVar(&opts.tags, "tag", nil, "bookmark tag; may be repeated")
	return cmd
}

func (a app) newBookmarkRemoveCommand() *cobra.Command {
	var opts commandOptions
	cmd := &cobra.Command{Use: "remove ILLUST_ID", Short: "Remove an illustration bookmark", Args: requireExactArgs(1, "pixiv bookmark remove [options] ILLUST_ID"), RunE: func(cmd *cobra.Command, args []string) error {
		id, err := parse.PositiveInt64(args[0], "illust_id")
		if err != nil {
			return err
		}
		request, jsonOverride, err := a.sdkRequest(cmd, opts)
		if err != nil {
			return err
		}
		jsonOut, err := a.services().SDK.JSONOut(jsonOverride)
		if err != nil {
			return err
		}
		client, err := a.services().SDK.OpenOperation(cmd.Context(), request)
		if err != nil {
			return err
		}
		if err := client.RemoveBookmark(cmd.Context(), sdk.RemoveBookmarkRequest{IllustID: id}); err != nil {
			return err
		}
		return a.printMutation(jsonOut, "removed_bookmark", "illust_id", id)
	}}
	a.bindCommonFlags(cmd, &opts)
	return cmd
}

func (a app) newFollowCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "follow", Short: "Manage followed users"}
	cmd.AddCommand(a.newFollowAddCommand(), a.newFollowRemoveCommand())
	return cmd
}

func (a app) newFollowAddCommand() *cobra.Command {
	opts := mutationOptions{restrict: string(sdk.RestrictPublic)}
	cmd := &cobra.Command{Use: "add USER_ID", Short: "Follow a user", Args: requireExactArgs(1, "pixiv follow add [options] USER_ID"), RunE: func(cmd *cobra.Command, args []string) error {
		id, err := parse.PositiveInt64(args[0], "user_id")
		if err != nil {
			return err
		}
		request, jsonOverride, err := a.sdkRequest(cmd, opts.commandOptions)
		if err != nil {
			return err
		}
		jsonOut, err := a.services().SDK.JSONOut(jsonOverride)
		if err != nil {
			return err
		}
		client, err := a.services().SDK.OpenOperation(cmd.Context(), request)
		if err != nil {
			return err
		}
		if err := client.FollowUser(cmd.Context(), sdk.FollowUserRequest{UserID: id, Restrict: sdk.Restrict(opts.restrict)}); err != nil {
			return err
		}
		return a.printMutation(jsonOut, "followed", "user_id", id)
	}}
	a.bindCommonFlags(cmd, &opts.commandOptions)
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "follow visibility (public or private)")
	return cmd
}

func (a app) newFollowRemoveCommand() *cobra.Command {
	var opts commandOptions
	cmd := &cobra.Command{Use: "remove USER_ID", Short: "Unfollow a user", Args: requireExactArgs(1, "pixiv follow remove [options] USER_ID"), RunE: func(cmd *cobra.Command, args []string) error {
		id, err := parse.PositiveInt64(args[0], "user_id")
		if err != nil {
			return err
		}
		request, jsonOverride, err := a.sdkRequest(cmd, opts)
		if err != nil {
			return err
		}
		jsonOut, err := a.services().SDK.JSONOut(jsonOverride)
		if err != nil {
			return err
		}
		client, err := a.services().SDK.OpenOperation(cmd.Context(), request)
		if err != nil {
			return err
		}
		if err := client.UnfollowUser(cmd.Context(), sdk.UnfollowUserRequest{UserID: id}); err != nil {
			return err
		}
		return a.printMutation(jsonOut, "unfollowed", "user_id", id)
	}}
	a.bindCommonFlags(cmd, &opts)
	return cmd
}

func (a app) printMutation(jsonOut bool, result, idKey string, id int64) error {
	if jsonOut {
		return a.printJSON(map[string]any{"result": result, idKey: id})
	}
	fmt.Fprintf(a.out, "%s %d\n", result, id)
	return nil
}
