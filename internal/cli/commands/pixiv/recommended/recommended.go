// Package recommended owns personalized Pixiv recommendation output.
package recommended

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	requirements "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/internal/listing"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	record "github.com/FlanChanXwO/pixiv-cli/internal/shared/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type options struct {
	CommandOptions
	ndjson      bool
	limit       int
	page        int
	typ         string
	contentType string
}

// Request 是 recommended 一次执行解析出的传输覆写值；它不持有 client 或资源。
type Request struct {
	UserID             int64
	HTTPSProxyOverride *string
}

type CommandOptions struct {
	Proxy   string
	NoProxy bool
	JSON    bool
}

// Dependencies 是推荐 command 的最小执行端口。SDK 资源只经公开 client 和
// composition root 注入的 Pooled 回调进入该 owner。
type Dependencies struct {
	Input      io.Reader
	Output     io.Writer
	UsageError func(error) error
	JSONOut    func(*bool) (bool, error)
	Pooled     func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error
}

type command struct {
	data Dependencies
}

func (d Dependencies) usage(err error) error {
	if err == nil || d.UsageError == nil {
		return err
	}
	return d.UsageError(err)
}

func (d Dependencies) maxArgs(count int, usage string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) > count {
			return fmt.Errorf("usage: %s", usage)
		}
		return nil
	}
}

func (d Dependencies) bindCommonFlags(cmd *cobra.Command, opts *CommandOptions) {
	cmd.Flags().BoolVarP(&opts.JSON, "json", "j", false, "print JSON")
	cmd.Flags().StringVar(&opts.Proxy, "proxy", "", "proxy URL (http, https, socks5, or socks5h) for this command")
	cmd.Flags().BoolVar(&opts.NoProxy, "no-proxy", false, "clear the configured proxy for this command")
}

func (d Dependencies) bindTextValue(cmd *cobra.Command) {
	pipeline.Bind(cmd, pipeline.InputSpec{Codec: pipeline.TextValue, MinArgs: 0, MaxArgs: 1, FillPosition: 0, Reader: d.Input, UsageError: d.usage})
}

func (d Dependencies) request(cmd *cobra.Command, opts CommandOptions) (Request, error) {
	if cmd.Flags().Changed("proxy") && cmd.Flags().Changed("no-proxy") {
		return Request{}, errors.New("use either --proxy or --no-proxy, not both")
	}
	request := Request{}
	if cmd.Flags().Changed("no-proxy") && opts.NoProxy {
		empty := ""
		request.HTTPSProxyOverride = &empty
	} else if cmd.Flags().Changed("proxy") {
		request.HTTPSProxyOverride = &opts.Proxy
	}
	return request, nil
}

func (d Dependencies) jsonOverride(cmd *cobra.Command, opts CommandOptions) *bool {
	if !cmd.Flags().Changed("json") {
		return nil
	}
	value := opts.JSON
	return &value
}

func (d Dependencies) jsonOut(override *bool) (bool, error) {
	if d.JSONOut == nil {
		return false, errors.New("pixiv recommended JSON output resolver is not configured")
	}
	return d.JSONOut(override)
}

func (d Dependencies) shouldAutoNDJSON(cmd *cobra.Command, ndjson, jsonOut bool) bool {
	if ndjson || jsonOut || cmd.Flags().Changed("json") {
		return ndjson
	}
	file, ok := d.Output.(interface{ Fd() uintptr })
	return ok && !term.IsTerminal(int(file.Fd()))
}

// New builds the actual `pixiv recommended` command.
func New(data Dependencies) *cobra.Command {
	a := command{data: data}
	opts := options{contentType: "all"}
	cmd := &cobra.Command{
		Use:   "recommended [KIND]",
		Short: "Show personalized recommendations",
		Args:  data.maxArgs(1, "pixiv recommended [KIND] [options]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			kind, err := resolveKind(cmd, args, &opts, a.data.usage)
			if err != nil {
				return err
			}
			return a.run(cmd, kind, opts)
		},
	}
	data.bindCommonFlags(cmd, &opts.CommandOptions)
	listing.BindNDJSONFlag(cmd, &opts.ndjson)
	listing.BindListFlags(cmd, &opts.limit, &opts.page)
	cmd.Flags().StringVarP(&opts.typ, "type", "t", opts.typ, "entity type: artwork, novel, user, all")
	cmd.Flags().StringVar(&opts.contentType, "content-type", opts.contentType, "artwork subtype: all, illust, manga")
	data.bindTextValue(cmd)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

// resolveKind 保留 KIND 位置参数与 --type 的既有互斥、取值域和 --content-type
// 组合规则；两种入口最终解析为同一组内部 kind。
func resolveKind(cmd *cobra.Command, args []string, opts *options, usage func(error) error) (string, error) {
	kind := opts.typ
	if len(args) == 1 {
		if cmd.Flags().Changed("type") {
			return "", usage(errors.New("KIND cannot be combined with --type"))
		}
		kind = args[0]
	}
	if kind == "" {
		return "", errors.New("recommended requires KIND or --type")
	}
	if cmd.Flags().Changed("type") {
		if kind != "artwork" && kind != "novel" && kind != "user" && kind != "all" {
			return "", usage(errors.New("type must be one of artwork, novel, user, all"))
		}
		if kind != "artwork" && cmd.Flags().Changed("content-type") {
			return "", usage(errors.New("--content-type is only supported when --type artwork"))
		}
		if opts.contentType != "all" && opts.contentType != "illust" && opts.contentType != "manga" {
			return "", usage(errors.New("content-type must be one of all, illust, manga"))
		}
	}
	if kind == "artwork" {
		if opts.contentType == "manga" {
			return "manga", nil
		}
		return "illust", nil
	}
	return kind, nil
}

func (a command) run(cmd *cobra.Command, kind string, opts options) error {
	if !validKind(kind) {
		return errors.New("recommendation kind must be one of: all, illust, manga, novel, user")
	}
	plan, err := listing.ParsePlan(cmd, opts.limit, opts.page)
	if err != nil {
		return err
	}
	request, err := a.data.request(cmd, opts.CommandOptions)
	if err != nil {
		return err
	}
	jsonOverride := a.data.jsonOverride(cmd, opts.CommandOptions)
	if opts.ndjson && cmd.Flags().Changed("json") {
		return a.data.usage(errors.New("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = a.data.jsonOut(jsonOverride)
		if err != nil {
			return err
		}
	}
	ndjson := a.data.shouldAutoNDJSON(cmd, opts.ndjson, jsonOut)
	if kind == "all" {
		if ndjson {
			return a.data.Pooled(cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) (bool, error) {
				return a.runAllNDJSON(ctx, client, plan)
			})
		}
		return a.data.Pooled(cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) (bool, error) {
			return a.runAll(ctx, client, plan, jsonOut)
		})
	}
	return a.runOne(cmd.Context(), listing.Request(request), plan, jsonOut, ndjson, kind)
}

func validKind(kind string) bool {
	return kind == "all" || kind == "illust" || kind == "manga" || kind == "novel" || kind == "user"
}

func (a command) runOne(ctx context.Context, request listing.Request, plan listing.Plan, jsonOut, ndjson bool, kind string) error {
	if kind == "illust" || kind == "manga" {
		jsonKey := "illusts"
		if kind == "manga" {
			jsonKey = "manga"
		}
		fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
			result, err := client.RecommendedArtworks(ctx, pixiv.RecommendedArtworksRequest{Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}
		return a.runner().RunPooledIllustListWithKey(ctx, request, plan, jsonOut, ndjson, jsonKey, func() string {
			return fmt.Sprintf("recommended %s", kind)
		}, fetch, func(items []pixiv.Artwork, start int) error { return printArtworks(a.data.Output, items) })
	}
	if kind == "novel" {
		fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
			result, err := client.RecommendedNovels(ctx, pixiv.RecommendedNovelsRequest{Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}
		return a.runner().RunPooledNovelList(ctx, request, plan, jsonOut, ndjson, "recommended novels", fetch,
			func(items []pixiv.Novel) error { return printNovels(a.data.Output, items) })
	}
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
		result, err := client.RecommendedUsers(ctx, pixiv.RecommendedUsersRequest{Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return a.runner().RunPooledUserList(ctx, request, plan, jsonOut, ndjson,
		func() string { return "recommended users" }, fetch,
		func(items []pixiv.UserPreview) error { return printUserPreviews(a.data.Output, items) })
}

// runAllNDJSON 保留 all 的既有类别顺序。每写出一条记录即标记提交，因此账号池只会
// 在任何下游可见输出之前重放 429。
func (a command) runAllNDJSON(ctx context.Context, client *pixiv.Client, plan listing.Plan) (bool, error) {
	encoder := json.NewEncoder(a.data.Output)
	committed := false
	writeArtworks := func(items []pixiv.Artwork) error {
		for _, item := range items {
			record, err := record.RecordFromArtworkDTO(pixiv.ToArtworkDTO(item))
			if err != nil {
				return err
			}
			committed = true
			if err := encoder.Encode(record); err != nil {
				return err
			}
		}
		return nil
	}
	fetchArtworks := func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.RecommendedArtworks(ctx, pixiv.RecommendedArtworksRequest{Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	if err := listing.PageItems(ctx, plan, fetchArtworks, writeArtworks); err != nil {
		return committed, err
	}
	if err := listing.PageItems(ctx, plan, fetchArtworks, writeArtworks); err != nil {
		return committed, err
	}
	if err := listing.PageItems(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
		result, err := client.RecommendedNovels(ctx, pixiv.RecommendedNovelsRequest{Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}, func(items []pixiv.Novel) error {
		for _, item := range items {
			record, err := record.RecordFromNovelDTO(pixiv.ToNovelDTO(item))
			if err != nil {
				return err
			}
			committed = true
			if err := encoder.Encode(record); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return committed, err
	}
	err := listing.PageItems(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
		result, err := client.RecommendedUsers(ctx, pixiv.RecommendedUsersRequest{Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}, func(items []pixiv.UserPreview) error {
		for _, item := range items {
			record, err := record.RecordFromUserPreviewDTO(pixiv.ToUserPreviewDTO(item))
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

// runner is resolved only after Cobra has accepted the command input and the
// root has prepared the command's declared resources.
func (a command) runner() listing.Runner {
	return listing.New(a.data.Output, func(ctx context.Context, request listing.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
		return a.data.Pooled(ctx, Request(request), attempt)
	})
}

func printArtworks(out io.Writer, items []pixiv.Artwork) error {
	for _, item := range items {
		if _, err := fmt.Fprintf(out, "https://www.pixiv.net/artworks/%d\n", item.ID); err != nil {
			return err
		}
		tags := make([]string, 0, len(item.Tags))
		for _, tag := range item.Tags {
			tags = append(tags, tag.Name)
		}
		if _, err := fmt.Fprintf(out, "%d %q by %s bookmarks:%d views:%d tags:%s\n", item.ID, item.Title, item.User.Name, item.TotalBookmarks, item.TotalViews, strings.Join(tags, ",")); err != nil {
			return err
		}
	}
	return nil
}

func printNovels(out io.Writer, items []pixiv.Novel) error {
	for _, item := range items {
		if _, err := fmt.Fprintf(out, "%d %s — %s\n", item.ID, item.Title, item.User.Name); err != nil {
			return err
		}
	}
	return nil
}

func printUserPreviews(out io.Writer, users []pixiv.UserPreview) error {
	for _, item := range users {
		if _, err := fmt.Fprintf(out, "%d %s\n", item.User.ID, item.User.Name); err != nil {
			return err
		}
	}
	return nil
}
