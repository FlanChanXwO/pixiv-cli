// Package detail owns the Pixiv entity detail command and terminal presenter.
package detail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	requirements "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/text"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

// Request 是 detail 一次执行解析出的传输覆写值；它不持有 client 或资源。
type Request struct {
	UserID             int64
	HTTPSProxyOverride *string
}

// Options 是 detail 自己声明的 flags。它只保存 Cobra 解析结果，不创建资源。
type Options struct {
	Proxy   string
	NoProxy bool
	JSON    bool

	typ     string
	content bool
}

// Dependencies 是 detail owner 的最小执行端口。资源 factory 由 composition root
// 在输入验证后通过 BuildRequest/Pooled 注入；detail 不导入旧 CLI resource graph。
type Dependencies struct {
	Input        io.Reader
	Output       io.Writer
	UsageError   func(error) error
	BuildRequest func(*cobra.Command, Options) (Request, error)
	JSONOut      func(*bool) (bool, error)
	Pooled       func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error
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

func (d Dependencies) bindCommonFlags(cmd *cobra.Command, opts *Options) {
	cmd.Flags().BoolVarP(&opts.JSON, "json", "j", false, "print JSON")
	cmd.Flags().StringVar(&opts.Proxy, "proxy", "", "proxy URL (http, https, socks5, or socks5h) for this command")
	cmd.Flags().BoolVar(&opts.NoProxy, "no-proxy", false, "clear the configured proxy for this command")
}

func (d Dependencies) bindTextValue(cmd *cobra.Command) {
	pipeline.Bind(cmd, pipeline.InputSpec{
		Codec:        pipeline.TextValue,
		MinArgs:      1,
		MaxArgs:      1,
		FillPosition: 0,
		Reader:       d.Input,
		UsageError:   d.usage,
	})
}

func (d Dependencies) exactArgs(count int, usage string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) != count {
			return fmt.Errorf("usage: %s", usage)
		}
		return nil
	}
}

func readDetail[T any](d Dependencies, ctx context.Context, request Request, invoke func(context.Context, *pixiv.Client) (T, error)) (T, error) {
	var zero T
	if d.Pooled == nil {
		return zero, errors.New("pixiv pooled operation is not configured")
	}
	var result T
	err := d.Pooled(ctx, request, func(ctx context.Context, client *pixiv.Client) (bool, error) {
		var err error
		result, err = invoke(ctx, client)
		return false, err
	})
	if err != nil {
		return zero, err
	}
	return result, nil
}

func (d Dependencies) writeJSON(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var out bytes.Buffer
	if err := json.Indent(&out, body, "", "  "); err != nil {
		return err
	}
	_, err = io.WriteString(d.Output, out.String()+"\n")
	return err
}

// New builds the actual `pixiv detail` command.
func New(data Dependencies) *cobra.Command {
	a := command{data: data}
	options := Options{typ: "artwork"}
	cmd := &cobra.Command{
		Use:   "detail ID_OR_URL",
		Short: "Show one artwork, novel, or user",
		Args:  data.exactArgs(1, "pixiv detail [options] ID_OR_URL"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd, args[0], options)
		},
	}
	data.bindCommonFlags(cmd, &options)
	data.bindTextValue(cmd)
	cmd.Flags().StringVarP(&options.typ, "type", "t", options.typ, "entity type: artwork, novel, user")
	cmd.Flags().BoolVar(&options.content, "content", false, "for novels, read structured novel content instead of metadata")
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) run(cmd *cobra.Command, arg string, opts Options) error {
	entity, err := resolveEntity(opts.typ)
	if err != nil {
		return err
	}
	if opts.content && entity != "novel" {
		return errors.New("--content is only supported when --type novel")
	}
	id, err := parseEntityIDOrURL(arg, entity)
	if err != nil {
		return err
	}
	if a.data.BuildRequest == nil {
		return errors.New("pixiv detail request builder is not configured")
	}
	request, err := a.data.BuildRequest(cmd, opts)
	if err != nil {
		return err
	}
	var jsonOverride *bool
	if cmd.Flags().Changed("json") {
		jsonOverride = &opts.JSON
	}
	if a.data.JSONOut == nil {
		return errors.New("pixiv detail JSON output resolver is not configured")
	}
	jsonOut, err := a.data.JSONOut(jsonOverride)
	if err != nil {
		return err
	}
	switch entity {
	case "artwork":
		result, err := readDetail(a.data, cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) (pixiv.Artwork, error) {
			return client.Artwork(ctx, pixiv.ArtworkRequest{ArtworkID: id})
		})
		if err != nil {
			return err
		}
		if jsonOut {
			return a.data.writeJSON(pixiv.ToArtworkDTO(result))
		}
		return printArtwork(a.data.Output, result)
	case "novel":
		if opts.content {
			result, err := readDetail(a.data, cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) (pixiv.NovelContent, error) {
				return client.NovelContent(ctx, pixiv.NovelContentRequest{NovelID: id})
			})
			if err != nil {
				return err
			}
			if jsonOut {
				return a.data.writeJSON(pixiv.ToNovelContentDTO(result))
			}
			return printNovelContent(a.data.Output, result)
		}
		result, err := readDetail(a.data, cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) (pixiv.Novel, error) {
			return client.Novel(ctx, pixiv.NovelRequest{NovelID: id})
		})
		if err != nil {
			return err
		}
		if jsonOut {
			return a.data.writeJSON(pixiv.ToNovelDTO(result))
		}
		return printNovel(a.data.Output, result)
	case "user":
		result, err := readDetail(a.data, cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) (pixiv.UserDetail, error) {
			return client.User(ctx, pixiv.UserRequest{UserID: id})
		})
		if err != nil {
			return err
		}
		if jsonOut {
			return a.data.writeJSON(pixiv.ToUserDetailDTO(result))
		}
		return printUser(a.data.Output, result)
	default:
		return errors.New("type must be one of artwork, novel, user")
	}
}

func resolveEntity(value string) (string, error) {
	switch value {
	case "artwork", "novel", "user":
		return value, nil
	default:
		return "", errors.New("type must be one of artwork, novel, user")
	}
}

func parseEntityIDOrURL(arg, entity string) (int64, error) {
	value := strings.TrimSpace(arg)
	if id, err := strconv.ParseInt(value, 10, 64); err == nil && id > 0 {
		return id, nil
	}
	ref, err := pixiv.ParseURL(value)
	if err != nil {
		return 0, errors.New("argument must be an entity ID or a supported Pixiv URL")
	}
	want := map[string]pixiv.ReferenceKind{
		"artwork": pixiv.ReferenceKindArtwork,
		"novel":   pixiv.ReferenceKindNovel,
		"user":    pixiv.ReferenceKindUser,
	}
	if ref.Kind != want[entity] {
		return 0, fmt.Errorf("URL does not name a supported Pixiv %s", entity)
	}
	return ref.ID, nil
}

func printArtwork(out io.Writer, item pixiv.Artwork) error {
	tags := make([]string, 0, len(item.Tags))
	for _, tag := range item.Tags {
		tags = append(tags, tag.Name)
	}
	url := ""
	if item.ID > 0 {
		url = fmt.Sprintf("https://www.pixiv.net/artworks/%d", item.ID)
	}
	for _, line := range []string{
		fmt.Sprintf("url: %s\n", url), fmt.Sprintf("id: %d\n", item.ID), fmt.Sprintf("title: %s\n", item.Title),
		fmt.Sprintf("author: %s (%d)\n", item.User.Name, item.User.ID), fmt.Sprintf("type: %s\n", string(item.Kind)),
		fmt.Sprintf("page_count: %d\n", item.PageCount), fmt.Sprintf("bookmarks: %d\n", item.TotalBookmarks),
		fmt.Sprintf("views: %d\n", item.TotalViews), fmt.Sprintf("tags: %s\n", strings.Join(tags, ",")),
	} {
		if _, err := io.WriteString(out, line); err != nil {
			return err
		}
	}
	if caption := text.HTMLPlainText(item.Caption); caption != "" {
		if _, err := fmt.Fprintf(out, "caption:\n%s\n", caption); err != nil {
			return err
		}
	}
	return nil
}

func printNovel(out io.Writer, item pixiv.Novel) error {
	_, err := fmt.Fprintf(out, "%d %s — %s\n", item.ID, item.Title, item.User.Name)
	return err
}

func printNovelContent(out io.Writer, content pixiv.NovelContent) error {
	if _, err := fmt.Fprintf(out, "novel %d: %s\n", content.NovelID, content.Title); err != nil {
		return err
	}
	for _, block := range content.Blocks {
		switch block.Kind {
		case pixiv.NovelBlockParagraph, pixiv.NovelBlockHeader:
			if _, err := fmt.Fprintln(out, block.Text); err != nil {
				return err
			}
		case pixiv.NovelBlockImage:
			if block.Image != nil {
				if _, err := fmt.Fprintf(out, "[image resource %s] %s\n", block.Image.Resource.Ref.String(), block.Image.Caption); err != nil {
					return err
				}
			}
		case pixiv.NovelBlockFile:
			if block.File != nil {
				if _, err := fmt.Fprintf(out, "[file %s resource %s] %s\n", block.File.Filename, block.File.Resource.Ref.String(), block.File.Caption); err != nil {
					return err
				}
			}
		case pixiv.NovelBlockUnknown:
			if block.Unknown != nil {
				if _, err := fmt.Fprintf(out, "[unknown block %s]\n", block.Unknown.RawType); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func printUser(out io.Writer, result pixiv.UserDetail) error {
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
		{"workspace pc", result.Workspace.PC}, {"workspace monitor", result.Workspace.Monitor},
		{"workspace tool", result.Workspace.Tool}, {"workspace scanner", result.Workspace.Scanner},
		{"workspace tablet", result.Workspace.Tablet}, {"workspace mouse", result.Workspace.Mouse},
		{"workspace printer", result.Workspace.Printer}, {"workspace desktop", result.Workspace.Desktop},
		{"workspace music", result.Workspace.Music}, {"workspace desk", result.Workspace.Desk},
		{"workspace chair", result.Workspace.Chair}, {"workspace comment", result.Workspace.Comment},
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
