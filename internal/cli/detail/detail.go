// Package detail 提供 pixiv detail 命令，输出作品、小说、用户等实体的详情。
//
// 包的组成：[New] 组装命令树，[Options] 记录实体类型与输出选择；JSON 暂存在 output.go。
package detail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/display"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/target"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

// Options 保存 detail 自己声明的 flag，只承接 Cobra 解析结果，不创建资源。
type Options struct {
	environment.Options
	input  *environment.State
	ndjson bool

	typ     string
	content bool
}

type commandRunner struct {
	env    environment.Env
	runner session.Runner
}

// New 构造 `pixiv detail` 命令；实体类型由 --type 或 URL 决定。
func New(env environment.Env, exec session.Executor) *cobra.Command {
	a := commandRunner{env: env, runner: session.NewRunner(env.Out, exec)}
	options := Options{typ: string(target.NamespaceArtwork)}
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
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a commandRunner) run(cmd *cobra.Command, args []string, opts Options) error {
	if cmd.Flags().Changed("json") && cmd.Flags().Changed("ndjson") {
		return errors.New("--json and --ndjson cannot be used together")
	}
	if opts.input.Mode() == environment.RecordMode {
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
	if content && entity != string(target.NamespaceNovel) {
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
	err = record.ConsumeNDJSONRecords(cmd.Context(), reader, a.env.Err, "detail", true, func(ctx context.Context, item record.Record) error {
		entity, err := entityForRecord(item.Type(), cmd.Flags().Changed("type"), opts.typ)
		if err != nil {
			return err
		}
		if err := validateContentEntity(opts.content, entity); err != nil {
			if sdk.ReasonOf(err) == sdk.ContentUnavailable {
				return record.FatalRecordPipeline(err)
			}
			return err
		}
		id, err := record.RequiredRecordID(item)
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
				return record.FatalRecordPipeline(err)
			}
		}
		if _, err := io.Copy(a.env.Out, &humanBuffer); err != nil {
			return record.FatalRecordPipeline(err)
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
	return a.env.ResolveJSON(override)
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
	request, err := a.env.Request(cmd, opts.Options.ProxyOptions)
	if err != nil {
		return err
	}
	switch entity {
	case string(target.NamespaceArtwork):
		result, err := session.Read(a.runner, ctx, request, func(ctx context.Context, client *pixiv.Client) (pixiv.Artwork, error) {
			return client.Artwork(ctx, pixiv.ArtworkRequest{ArtworkID: id})
		})
		if err != nil {
			return err
		}
		if ndjson || (jsonOut && opts.input.Mode() == environment.RecordMode) {
			value, err := record.RecordFromArtwork(result)
			if err != nil {
				return err
			}
			return a.writeRecord(value)
		}
		if jsonOut {
			return a.env.PrintJSON(result)
		}
		return display.PrintArtworkDetail(a.env.Out, result)
	case string(target.NamespaceNovel):
		if opts.content {
			result, err := session.Read(a.runner, ctx, request, func(ctx context.Context, client *pixiv.Client) (pixiv.NovelContent, error) {
				return client.NovelContent(ctx, pixiv.NovelContentRequest{NovelID: id})
			})
			if err != nil {
				return err
			}
			if ndjson || (jsonOut && opts.input.Mode() == environment.RecordMode) {
				value, err := record.RecordFromNovelContent(result)
				if err != nil {
					return err
				}
				return a.writeRecord(value)
			}
			if jsonOut {
				return a.env.PrintJSON(result)
			}
			return display.PrintNovelContent(a.env.Out, result)
		}
		result, err := session.Read(a.runner, ctx, request, func(ctx context.Context, client *pixiv.Client) (pixiv.Novel, error) {
			return client.Novel(ctx, pixiv.NovelRequest{NovelID: id})
		})
		if err != nil {
			return err
		}
		if ndjson || (jsonOut && opts.input.Mode() == environment.RecordMode) {
			value, err := record.RecordFromNovel(result)
			if err != nil {
				return err
			}
			return a.writeRecord(value)
		}
		if jsonOut {
			return a.env.PrintJSON(result)
		}
		return display.PrintNovelDetail(a.env.Out, result)
	case string(target.NamespaceUser):
		result, err := session.Read(a.runner, ctx, request, func(ctx context.Context, client *pixiv.Client) (pixiv.UserDetail, error) {
			return client.User(ctx, pixiv.UserRequest{UserID: id})
		})
		if err != nil {
			return err
		}
		if ndjson || (jsonOut && opts.input.Mode() == environment.RecordMode) {
			value, err := record.RecordFromUserDetail(result)
			if err != nil {
				return err
			}
			return a.writeRecord(value)
		}
		if jsonOut {
			return a.env.PrintJSON(result)
		}
		return display.PrintUserDetail(a.env.Out, result)
	default:
		return errors.New("type must be one of artwork, novel, user")
	}
}

func (a commandRunner) writeRecord(value record.Record) error {
	if err := json.NewEncoder(a.env.Out).Encode(value); err != nil {
		return record.FatalRecordPipeline(err)
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
	if wanted != inferred {
		return "", fmt.Errorf("record type %q is incompatible with --type %q", typ, requested)
	}
	return wanted, nil
}

func resolveRecordEntity(typ string) (string, error) {
	switch typ {
	case string(target.NamespaceArtwork):
		return string(target.NamespaceArtwork), nil
	case string(target.NamespaceNovel):
		return string(target.NamespaceNovel), nil
	case string(target.NamespaceUser):
		return string(target.NamespaceUser), nil
	default:
		return "", fmt.Errorf("unsupported record type %q", typ)
	}
}

func resolveEntity(value string) (string, error) {
	switch value {
	case string(target.NamespaceArtwork), string(pixiv.ContentTypeIllust), string(pixiv.ContentTypeManga), string(pixiv.ContentTypeUgoira):
		return string(target.NamespaceArtwork), nil
	case string(target.NamespaceNovel), string(target.NamespaceUser):
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
		string(target.NamespaceArtwork): pixiv.ReferenceKindArtwork,
		string(target.NamespaceNovel):   pixiv.ReferenceKindNovel,
		string(target.NamespaceUser):    pixiv.ReferenceKindUser,
	}
	if ref.Kind != want[entity] {
		return 0, sdk.NewError("pixiv", "detail", sdk.InvalidArgument, sdk.WithDetail(fmt.Sprintf("URL does not name a supported Pixiv %s", entity)))
	}
	return ref.ID, nil
}
