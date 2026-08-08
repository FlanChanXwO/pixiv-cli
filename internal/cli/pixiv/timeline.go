package pixiv

import (
	"context"
	"fmt"

	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/parse"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

// timelineOptions 是关注新作与全站最新命令的共同选项。内容类型由边缘层显式选择，
// 这样不会把插画、漫画和小说的独立 App 分页流错误合并。
type timelineOptions struct {
	commandOptions
	ndjsonOutputOptions
	listOptions
	contentType string
	restrict    string
}

// myPixivOptions 是只读 MyPixiv 命令的共同选项。MyPixiv 的好友关系写操作不在
// 此命令组中，避免把读取工作流与关系变更混在一起。
type myPixivOptions struct {
	commandOptions
	ndjsonOutputOptions
	listOptions
	contentType string
}

func (a controller) newTimelineCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "timeline", Short: "Browse authenticated Pixiv timelines"}
	cmd.AddCommand(a.newTimelineFollowingCommand(), a.newTimelineLatestCommand())
	return cmd
}

func (a controller) newTimelineFollowingCommand() *cobra.Command {
	opts := timelineOptions{restrict: string(pixiv.RestrictPublic)}
	cmd := &cobra.Command{
		Use:   "following",
		Short: "Browse new works from followed users",
		Args:  a.requireExactArgs(0, "pixiv timeline following --type illust|novel"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runTimelineFollowing(cmd, opts)
		},
	}
	a.bindCommonFlags(cmd, &opts.commandOptions)
	bindNDJSONFlag(cmd, &opts.ndjsonOutputOptions)
	bindListFlags(cmd, &opts.listOptions)
	cmd.Flags().StringVar(&opts.contentType, "type", "", "required content type: illust or novel")
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "follow visibility: public or private")
	return cmd
}

func (a controller) newTimelineLatestCommand() *cobra.Command {
	opts := timelineOptions{}
	cmd := &cobra.Command{
		Use:   "latest",
		Short: "Browse Pixiv's latest works",
		Args:  a.requireExactArgs(0, "pixiv timeline latest --type illust|manga|novel"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runTimelineLatest(cmd, opts)
		},
	}
	a.bindCommonFlags(cmd, &opts.commandOptions)
	bindNDJSONFlag(cmd, &opts.ndjsonOutputOptions)
	bindListFlags(cmd, &opts.listOptions)
	cmd.Flags().StringVar(&opts.contentType, "type", "", "required content type: illust, manga, or novel")
	return cmd
}

func (a controller) runTimelineFollowing(cmd *cobra.Command, opts timelineOptions) error {
	if opts.contentType != "illust" && opts.contentType != "novel" {
		return fmt.Errorf("type must be one of: illust, novel")
	}
	plan, err := parseListPlan(cmd, opts.listOptions)
	if err != nil {
		return err
	}
	services := a.services()
	request, jsonOverride, err := a.sdkRequest(cmd, opts.commandOptions)
	if err != nil {
		return err
	}
	if opts.ndjson && cmd.Flags().Changed("json") {
		return a.usageError(fmt.Errorf("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = services.SDK.JSONOut(jsonOverride)
		if err != nil {
			return err
		}
	}
	opts.ndjson = a.shouldAutoNDJSON(cmd, opts.ndjson, jsonOut)
	if opts.contentType == "illust" {
		fetch := func(client pixivapp.ClientSet, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
			result, err := client.FollowingArtworks(ctx, pixiv.FollowingArtworksRequest{Restrict: pixiv.Restrict(opts.restrict), Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}
		return a.runPooledIllustList(cmd.Context(), request, plan, jsonOut, opts.ndjson, "new artworks from followed users", fetch, func(items []pixiv.Artwork, start int) error { return printIllusts(a.out, items, start, false) })
	}
	fetch := func(client pixivapp.ClientSet, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
		result, err := client.FollowingNovels(ctx, pixiv.FollowingNovelsRequest{Restrict: pixiv.Restrict(opts.restrict), Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return a.runPooledNovelList(cmd.Context(), request, plan, jsonOut, opts.ndjson, "new novels from followed users", fetch, func(items []pixiv.Novel) error { return printNovels(a.out, items) })
}

func (a controller) runTimelineLatest(cmd *cobra.Command, opts timelineOptions) error {
	if opts.contentType != "illust" && opts.contentType != "manga" && opts.contentType != "novel" {
		return fmt.Errorf("type must be one of: illust, manga, novel")
	}
	plan, err := parseListPlan(cmd, opts.listOptions)
	if err != nil {
		return err
	}
	services := a.services()
	request, jsonOverride, err := a.sdkRequest(cmd, opts.commandOptions)
	if err != nil {
		return err
	}
	if opts.ndjson && cmd.Flags().Changed("json") {
		return a.usageError(fmt.Errorf("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = services.SDK.JSONOut(jsonOverride)
		if err != nil {
			return err
		}
	}
	opts.ndjson = a.shouldAutoNDJSON(cmd, opts.ndjson, jsonOut)
	if opts.contentType == "novel" {
		fetch := func(client pixivapp.ClientSet, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
			result, err := client.LatestNovels(ctx, pixiv.LatestNovelsRequest{Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}
		return a.runPooledNovelList(cmd.Context(), request, plan, jsonOut, opts.ndjson, "latest novels", fetch, func(items []pixiv.Novel) error { return printNovels(a.out, items) })
	}
	fetch := func(client pixivapp.ClientSet, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.LatestArtworks(ctx, pixiv.LatestArtworksRequest{ContentType: pixiv.SearchContentType(opts.contentType), Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return a.runPooledIllustList(cmd.Context(), request, plan, jsonOut, opts.ndjson, fmt.Sprintf("latest %s", opts.contentType), fetch, func(items []pixiv.Artwork, start int) error { return printIllusts(a.out, items, start, false) })
}

func (a controller) newMyPixivCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "mypixiv", Short: "Browse authenticated MyPixiv data"}
	cmd.AddCommand(a.newMyPixivUsersCommand(), a.newMyPixivWorksCommand())
	return cmd
}

func (a controller) newMyPixivUsersCommand() *cobra.Command {
	opts := myPixivOptions{}
	cmd := &cobra.Command{
		Use:   "users",
		Short: "List MyPixiv users for the authenticated account",
		Args:  a.requireExactArgs(0, "pixiv mypixiv users"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runMyPixivUsers(cmd, opts)
		},
	}
	a.bindCommonFlags(cmd, &opts.commandOptions)
	bindNDJSONFlag(cmd, &opts.ndjsonOutputOptions)
	bindListFlags(cmd, &opts.listOptions)
	return cmd
}

func (a controller) newMyPixivWorksCommand() *cobra.Command {
	opts := myPixivOptions{}
	cmd := &cobra.Command{
		Use:   "works [USER_ID]",
		Short: "Browse MyPixiv works or one user's works",
		Args:  a.requireMaxArgs(1, "pixiv mypixiv works [USER_ID] --type illust|manga|novel"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runMyPixivWorks(cmd, args, opts)
		},
	}
	a.bindCommonFlags(cmd, &opts.commandOptions)
	bindNDJSONFlag(cmd, &opts.ndjsonOutputOptions)
	bindListFlags(cmd, &opts.listOptions)
	cmd.Flags().StringVar(&opts.contentType, "type", "", "required content type: illust, manga, or novel")
	return cmd
}

func (a controller) runMyPixivUsers(cmd *cobra.Command, opts myPixivOptions) error {
	plan, err := parseListPlan(cmd, opts.listOptions)
	if err != nil {
		return err
	}
	services := a.services()
	request, jsonOverride, err := a.sdkRequest(cmd, opts.commandOptions)
	if err != nil {
		return err
	}
	if opts.ndjson && cmd.Flags().Changed("json") {
		return a.usageError(fmt.Errorf("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = services.SDK.JSONOut(jsonOverride)
		if err != nil {
			return err
		}
	}
	var userID int64
	fetch := func(client pixivapp.ClientSet, ctx context.Context, cursor sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
		if userID == 0 {
			var err error
			userID, err = client.CurrentUserID(ctx)
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
		}
		result, err := client.MyPixivUsers(ctx, pixiv.MyPixivUsersRequest{Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return a.runPooledUserList(cmd.Context(), request, plan, jsonOut, opts.ndjson, func() string {
		return fmt.Sprintf("MyPixiv users for %d", userID)
	}, fetch, func(items []pixiv.UserPreview) error { return printUserPreviews(a.out, items) })
}

func (a controller) runMyPixivWorks(cmd *cobra.Command, args []string, opts myPixivOptions) error {
	var userID int64
	if len(args) == 0 {
		if opts.contentType != "illust" && opts.contentType != "novel" {
			return fmt.Errorf("type without USER_ID must be one of: illust, novel")
		}
	} else {
		if opts.contentType != "illust" && opts.contentType != "manga" && opts.contentType != "novel" {
			return fmt.Errorf("type with USER_ID must be one of: illust, manga, novel")
		}
		var err error
		userID, err = parse.PositiveInt64(args[0], "user_id")
		if err != nil {
			return err
		}
	}
	plan, err := parseListPlan(cmd, opts.listOptions)
	if err != nil {
		return err
	}
	services := a.services()
	request, jsonOverride, err := a.sdkRequest(cmd, opts.commandOptions)
	if err != nil {
		return err
	}
	if opts.ndjson && cmd.Flags().Changed("json") {
		return a.usageError(fmt.Errorf("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = services.SDK.JSONOut(jsonOverride)
		if err != nil {
			return err
		}
	}
	opts.ndjson = a.shouldAutoNDJSON(cmd, opts.ndjson, jsonOut)
	if len(args) == 0 {
		if opts.contentType == "illust" {
			fetch := func(client pixivapp.ClientSet, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
				result, err := client.MyPixivArtworks(ctx, pixiv.MyPixivArtworksRequest{Cursor: cursor})
				if err != nil {
					return nil, sdk.Cursor{}, err
				}
				return result.Items, result.Next, nil
			}
			return a.runPooledIllustList(cmd.Context(), request, plan, jsonOut, opts.ndjson, "MyPixiv artworks", fetch, func(items []pixiv.Artwork, start int) error { return printIllusts(a.out, items, start, false) })
		}
		fetch := func(client pixivapp.ClientSet, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
			result, err := client.MyPixivNovels(ctx, pixiv.MyPixivNovelsRequest{Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}
		return a.runPooledNovelList(cmd.Context(), request, plan, jsonOut, opts.ndjson, "MyPixiv novels", fetch, func(items []pixiv.Novel) error { return printNovels(a.out, items) })
	}
	if opts.contentType == "novel" {
		fetch := func(client pixivapp.ClientSet, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
			result, err := client.UserNovels(ctx, pixiv.UserNovelsRequest{UserID: userID, Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}
		return a.runPooledNovelList(cmd.Context(), request, plan, jsonOut, opts.ndjson, fmt.Sprintf("novels by %d", userID), fetch, func(items []pixiv.Novel) error { return printNovels(a.out, items) })
	}
	fetch := func(client pixivapp.ClientSet, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.UserArtworks(ctx, pixiv.UserArtworksRequest{UserID: userID, Kind: pixiv.ArtworkKind(opts.contentType), Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return a.runPooledIllustList(cmd.Context(), request, plan, jsonOut, opts.ndjson, fmt.Sprintf("artworks by %d", userID), fetch, func(items []pixiv.Artwork, start int) error { return printIllusts(a.out, items, start, false) })
}
