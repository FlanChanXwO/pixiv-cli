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

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	inputcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/input"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	"github.com/FlanChanXwO/pixiv-cli/internal/plaintext"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

// Options 是 detail 自己声明的 flags。它只保存 Cobra 解析结果，不创建资源。
type Options struct {
	command.Options
	input  *inputcmd.State
	ndjson bool

	typ     string
	content bool
}

type commandRunner struct {
	env    command.Env
	runner pixivcmd.Runner
}

// New builds the actual `pixiv detail` command.
func New(env command.Env, exec pixivcmd.Executor) *cobra.Command {
	a := commandRunner{env: env, runner: pixivcmd.New(env.Out, exec)}
	options := Options{typ: "artwork"}
	cmd := &cobra.Command{
		Use:   "detail [ID_OR_URL]",
		Short: "Show one artwork, novel, or user",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd, args, options)
		},
	}
	env.BindCommonFlags(cmd, &options.Options)
	cmd.Flags().BoolVar(&options.ndjson, "ndjson", false, "print one canonical record per line")
	options.input = env.BindTextOrRecord(cmd, 1, 1, 0)
	cmd.Flags().StringVarP(&options.typ, "type", "t", options.typ, "entity type: artwork, novel, user")
	cmd.Flags().BoolVar(&options.content, "content", false, "for novels, read structured novel content instead of metadata")
	command.Bind(cmd, command.PixivData())
	return cmd
}

func (a commandRunner) run(cmd *cobra.Command, args []string, opts Options) error {
	if cmd.Flags().Changed("json") && cmd.Flags().Changed("ndjson") {
		return errors.New("--json and --ndjson cannot be used together")
	}
	if opts.input.Mode() == inputcmd.RecordMode {
		return a.runRecords(cmd, opts)
	}
	a.markNDJSON(cmd, opts.ndjson)
	if len(args) != 1 {
		return errors.New("detail requires one ID or URL")
	}
	entity, err := resolveEntity(opts.typ)
	if err != nil {
		return err
	}
	if err := validateContentEntity(opts.content, entity); err != nil {
		return err
	}
	id, err := parseEntityIDOrURL(args[0], entity)
	if err != nil {
		return err
	}
	return a.runOne(cmd, entity, id, opts, false)
}

func validateContentEntity(content bool, entity string) error {
	if content && entity != "novel" {
		return errors.New("--content is only supported when --type novel")
	}
	if content {
		// App v1 的正文 endpoint 已被拒绝；在构造 request 和打开账号池前返回
		// 兼容错误，保证 text/record 两条输入链路都不会触达已排除的 endpoint。
		return sdk.NewError("pixiv", "NovelContent", sdk.ContentUnavailable, sdk.WithDetail("novel content is unsupported by the v1 App API"))
	}
	return nil
}

func (a commandRunner) runRecords(cmd *cobra.Command, opts Options) error {
	jsonOut, err := a.jsonOutput(cmd, opts)
	if err != nil {
		return err
	}
	ndjson := opts.ndjson
	if !cmd.Flags().Changed("json") && !cmd.Flags().Changed("ndjson") && !jsonOut {
		ndjson = !a.env.OutputIsTTY()
	}
	a.markNDJSON(cmd, ndjson)
	worker := a
	humanOutput := !ndjson && !jsonOut
	var humanBuffer bytes.Buffer
	firstHuman := true
	var spool *jsonArraySpool
	if jsonOut && !ndjson {
		spool, err = newJSONArraySpool()
		if err != nil {
			return err
		}
		defer spool.close()
		worker.env.Out = spool
	}
	reader := opts.input.Reader(a.env.In)
	err = inputcmd.ConsumeNDJSONRecords(cmd.Context(), reader, a.env.Err, "detail", true, func(ctx context.Context, input record.Record) error {
		entity, err := entityForRecord(input.Type(), cmd.Flags().Changed("type"), opts.typ)
		if err != nil {
			return err
		}
		if err := validateContentEntity(opts.content, entity); err != nil {
			if sdk.ReasonOf(err) == sdk.ContentUnavailable {
				return inputcmd.FatalRecordPipeline(err)
			}
			return err
		}
		id, err := inputcmd.RequiredRecordID(input)
		if err != nil {
			return err
		}
		if humanOutput {
			humanBuffer.Reset()
			worker.env.Out = &humanBuffer
		}
		if err := worker.runOneWithOutput(ctx, cmd, entity, id, opts, ndjson, jsonOut); err != nil {
			return err
		}
		if !humanOutput {
			return nil
		}
		if !firstHuman {
			if _, err := io.WriteString(a.env.Out, "\n---\n"); err != nil {
				return inputcmd.FatalRecordPipeline(err)
			}
		}
		if _, err := io.Copy(a.env.Out, &humanBuffer); err != nil {
			return inputcmd.FatalRecordPipeline(err)
		}
		firstHuman = false
		return nil
	})
	if err != nil {
		return err
	}
	if spool != nil {
		return spool.commit(a.env.Out)
	}
	return nil
}

func (a commandRunner) jsonOutput(cmd *cobra.Command, opts Options) (bool, error) {
	var override *bool
	if cmd.Flags().Changed("json") {
		override = &opts.JSON
	}
	return a.env.JSONOut(override)
}

func (a commandRunner) runOne(cmd *cobra.Command, entity string, id int64, opts Options, recordMode bool) error {
	jsonOut, err := a.jsonOutput(cmd, opts)
	if err != nil {
		return err
	}
	ndjson := opts.ndjson
	return a.runOneWithOutput(cmd.Context(), cmd, entity, id, opts, ndjson, jsonOut && !recordMode)
}

func (a commandRunner) runOneWithOutput(ctx context.Context, cmd *cobra.Command, entity string, id int64, opts Options, ndjson, jsonOut bool) error {
	request, err := pixivcmd.BuildRequest(cmd, opts.Options)
	if err != nil {
		return err
	}
	switch entity {
	case "artwork":
		result, err := pixivcmd.Read(a.runner, ctx, request, func(ctx context.Context, client *pixiv.Client) (pixiv.Artwork, error) {
			return client.Artwork(ctx, pixiv.ArtworkRequest{ArtworkID: id})
		})
		if err != nil {
			return err
		}
		if ndjson || (jsonOut && opts.input.Mode() == inputcmd.RecordMode) {
			value, err := record.RecordFromArtwork(result)
			if err != nil {
				return err
			}
			return a.writeRecord(value)
		}
		if jsonOut {
			return a.env.PrintJSON(result)
		}
		return printArtwork(a.env.Out, result)
	case "novel":
		if opts.content {
			result, err := pixivcmd.Read(a.runner, ctx, request, func(ctx context.Context, client *pixiv.Client) (pixiv.NovelContent, error) {
				return client.NovelContent(ctx, pixiv.NovelContentRequest{NovelID: id})
			})
			if err != nil {
				return err
			}
			if ndjson || (jsonOut && opts.input.Mode() == inputcmd.RecordMode) {
				value, err := record.RecordFromNovelContent(result)
				if err != nil {
					return err
				}
				return a.writeRecord(value)
			}
			if jsonOut {
				return a.env.PrintJSON(result)
			}
			return printNovelContent(a.env.Out, result)
		}
		result, err := pixivcmd.Read(a.runner, ctx, request, func(ctx context.Context, client *pixiv.Client) (pixiv.Novel, error) {
			return client.Novel(ctx, pixiv.NovelRequest{NovelID: id})
		})
		if err != nil {
			return err
		}
		if ndjson || (jsonOut && opts.input.Mode() == inputcmd.RecordMode) {
			value, err := record.RecordFromNovel(result)
			if err != nil {
				return err
			}
			return a.writeRecord(value)
		}
		if jsonOut {
			return a.env.PrintJSON(result)
		}
		return printNovel(a.env.Out, result)
	case "user":
		result, err := pixivcmd.Read(a.runner, ctx, request, func(ctx context.Context, client *pixiv.Client) (pixiv.UserDetail, error) {
			return client.User(ctx, pixiv.UserRequest{UserID: id})
		})
		if err != nil {
			return err
		}
		if ndjson || (jsonOut && opts.input.Mode() == inputcmd.RecordMode) {
			value, err := record.RecordFromUserDetail(result)
			if err != nil {
				return err
			}
			return a.writeRecord(value)
		}
		if jsonOut {
			return a.env.PrintJSON(result)
		}
		return printUser(a.env.Out, result)
	default:
		return errors.New("type must be one of artwork, novel, user")
	}
}

func (a commandRunner) writeRecord(value record.Record) error {
	if err := json.NewEncoder(a.env.Out).Encode(value); err != nil {
		return inputcmd.FatalRecordPipeline(err)
	}
	return nil
}

func entityForRecord(typ string, explicit bool, requested string) (string, error) {
	inferred, err := resolveRecordEntity(typ)
	if err != nil {
		return "", err
	}
	if !explicit {
		return inferred, nil
	}
	wanted, err := resolveEntity(requested)
	if err != nil {
		return "", err
	}
	if wanted != inferred && !(wanted == "artwork" && inferred == "artwork") {
		return "", fmt.Errorf("record type %q is incompatible with --type %q", typ, requested)
	}
	return wanted, nil
}

func resolveRecordEntity(typ string) (string, error) {
	switch typ {
	case "artwork", "illust", "manga", "ugoira":
		return "artwork", nil
	case "novel":
		return "novel", nil
	case "user":
		return "user", nil
	default:
		return "", fmt.Errorf("unsupported record type %q", typ)
	}
}

func resolveEntity(value string) (string, error) {
	switch value {
	case "artwork", "illust", "manga", "ugoira":
		return "artwork", nil
	case "novel", "user":
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
		return 0, sdk.NewError("pixiv", "detail", sdk.InvalidArgument, sdk.WithDetail("argument must be an entity ID or a supported Pixiv URL"))
	}
	want := map[string]pixiv.ReferenceKind{
		"artwork": pixiv.ReferenceKindArtwork,
		"novel":   pixiv.ReferenceKindNovel,
		"user":    pixiv.ReferenceKindUser,
	}
	if ref.Kind != want[entity] {
		return 0, sdk.NewError("pixiv", "detail", sdk.InvalidArgument, sdk.WithDetail(fmt.Sprintf("URL does not name a supported Pixiv %s", entity)))
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
	if caption := plaintext.HTMLPlainText(item.Caption); caption != "" {
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
