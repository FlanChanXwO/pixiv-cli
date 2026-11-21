package vector

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/searchfilter"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/target"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/vectorindex"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/vectormodel"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// OpenClient 由组合根选择固定账号；返回的关闭函数释放 Pixiv 操作通道。
type OpenClient func(context.Context, environment.PixivRequest) (int64, *pixiv.Client, func() error, error)

func syncCommand(env environment.Env, store config.Store, open OpenClient) *cobra.Command {
	return writeCommand(env, store, open, false)
}

func addCommand(env environment.Env, store config.Store, open OpenClient) *cobra.Command {
	return writeCommand(env, store, open, true)
}

func writeCommand(env environment.Env, store config.Store, open OpenClient, add bool) *cobra.Command {
	var options environment.Options
	var restrict, rating, contentType string
	var reconcile bool
	var input *environment.State
	cmd := &cobra.Command{Use: "sync", Short: "help.synchronize_bookmarks_and_embed_pending_pages", Args: env.ExactArgs(0, "pixiv vector sync [--json]"), RunE: func(cmd *cobra.Command, args []string) (err error) {
		filter, err := searchfilter.NormalizeFilter(rating, contentType)
		if err != nil {
			return env.WrapUsage(err)
		}
		switch restrict {
		case "", "all", string(pixiv.RestrictPublic), string(pixiv.RestrictPrivate):
		default:
			return env.WrapUsage(i18n.Errorf("error.vector_restrict"))
		}
		c, err := store.Current()
		if err != nil {
			return err
		}
		client, err := modelHTTP(c, cmd, options.ProxyOptions)
		if err != nil {
			return err
		}
		defer client.CloseIdleConnections()
		model, err := openModel(cmd.Context(), c.Vector, client)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, model.Close()) }()
		identity, err := configuredModel(c.Vector)
		if err != nil {
			return err
		}
		request, err := env.Request(cmd, options.ProxyOptions)
		if err != nil {
			return err
		}
		jsonOut, err := env.ResolveJSON(env.JSONOverride(cmd, options))
		if err != nil {
			return err
		}
		root, err := config.AppDataDir()
		if err != nil {
			return err
		}
		release, acquired, err := vectorindex.TrySyncLock(root)
		if err != nil {
			return err
		}
		if !acquired {
			return i18n.Errorf("error.vector_sync_is_busy")
		}
		defer func() { err = errors.Join(err, release()) }()
		db, err := vectorindex.Open(cmd.Context(), root)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, db.Close()) }()
		fixture, err := vectormodel.ProbeImage()
		if err != nil {
			return err
		}
		var progress func(int, int) error
		if file, ok := env.Err.(interface{ Fd() uintptr }); ok && term.IsTerminal(int(file.Fd())) {
			progress = func(done, total int) error {
				_, err := fmt.Fprintf(env.Err, env.Text("output.embedding_pages"), done, total)
				return err
			}
		}
		syncOptions := vectorindex.SyncOptions{Model: identity, Embedder: model, ProbeImage: fixture, OpenClient: func(ctx context.Context) (int64, *pixiv.Client, func() error, error) { return open(ctx, request) }, Progress: progress, Restrict: restrict, Filter: filter, Reconcile: reconcile}
		var report vectorindex.SyncReport
		var runErr error
		if add {
			if len(args) == 0 {
				if source, ok := env.In.(io.Closer); ok {
					// 取消时关闭输入，唤醒等待下一行的读取并释放同步锁。
					closed := make(chan error, 1)
					stop := context.AfterFunc(cmd.Context(), func() { closed <- source.Close() })
					defer func() {
						if !stop() {
							err = errors.Join(err, <-closed)
						}
					}()
				}
			}
			report, runErr = db.Add(cmd.Context(), artworkInputs(cmd.Context(), args, input.Reader(env.In)), syncOptions)
		} else {
			report, runErr = db.Sync(cmd.Context(), syncOptions)
		}
		if progress != nil {
			_, err = fmt.Fprintln(env.Err)
		}
		if jsonOut {
			err = errors.Join(err, env.PrintJSON(report))
		} else {
			err = errors.Join(err, writeSyncReport(env, report))
		}
		return errors.Join(runErr, err)
	}}
	env.BindCommonFlags(cmd, &options)
	if add {
		cmd.Use = "add [ID_OR_URL...]"
		cmd.Short = "help.vector_add"
		cmd.Args = environment.ActionOrTargetsArgs(env.In, "pixiv vector add ID_OR_URL...", env.WrapUsage)
		input = env.BindTextOrRecord(cmd, 0, -1, 0)
	} else {
		cmd.Flags().StringVar(&restrict, "restrict", "all", "flag.vector_restrict")
		cmd.Flags().StringVar(&contentType, "content-type", "all", "flag.artwork_subtype_all_illust_and_ugoira_illust_manga")
		cmd.Flags().StringVar(&rating, "rating", "all", "flag.local_rating_filter_sfw_r18_r18g_mature_all")
		cmd.Flags().BoolVar(&reconcile, "reconcile", false, "flag.vector_reconcile")
		env.BindNoInput(cmd)
	}
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func writeSyncReport(env environment.Env, r vectorindex.SyncReport) error {
	_, err := fmt.Fprintf(env.Out, env.Text("output.bookmarks_checked_new_merged_recorded_artworks_skipped_records"), r.Bookmarks.Checked, r.Bookmarks.New, r.Merged, r.SkippedRecords, r.Embedded, r.Unavailable, r.Failed, r.Index.Artworks, r.Index.Pages, r.Index.Pending, r.Model)
	_, summaryErr := fmt.Fprintf(env.Out, env.Text("output.vector_scan_summary"), r.Recorded, r.Bookmarks.Pages, r.Bookmarks.Unavailable, r.Bookmarks.Removed)
	err = errors.Join(err, summaryErr)
	for _, scope := range []string{"public", "private"} {
		if scan, ok := r.Bookmarks.Scans[scope]; ok {
			state := "vector.scan_incomplete"
			if scan.Complete {
				state = "vector.scan_complete"
			}
			_, scanErr := fmt.Fprintf(env.Out, env.Text("output.vector_scan_state"), env.Text("gallery."+scope), env.Text("vector.scan_"+scan.Phase), env.Text(state))
			err = errors.Join(err, scanErr)
		}
	}
	if r.Stopped != nil {
		_, stopErr := fmt.Fprintf(env.Out, env.Text("output.stopped"), *r.Stopped)
		err = errors.Join(err, stopErr)
	}
	return err
}

func artworkInputs(ctx context.Context, args []string, input io.Reader) iter.Seq2[int64, error] {
	return func(yield func(int64, error) bool) {
		contract := target.Contract{Operation: "vector add", DefaultType: string(target.NamespaceArtwork), Types: []target.TypeSpec{{Name: string(target.NamespaceArtwork), ResultKind: target.NamespaceArtwork, BareReferenceKind: target.ReferenceKindArtwork}}, URLKinds: map[target.ReferenceKind]target.URLTypeRelation{target.ReferenceKindArtwork: target.URLTypeMustMatchResult}}
		resolve := func(source target.Input) bool {
			resolved, err := target.Resolve(ctx, source, contract)
			return yield(resolved.ID, err) && err == nil
		}
		if len(args) > 0 {
			for _, arg := range args {
				if !resolve(target.Input{Value: arg}) {
					return
				}
			}
			return
		}
		reader := bufio.NewReader(input)
		count := 0
		for line := 1; ; line++ {
			body, err := reader.ReadBytes('\n')
			if ctx.Err() != nil {
				yield(0, errors.Join(ctx.Err(), err))
				return
			}
			if err != nil && !errors.Is(err, io.EOF) {
				yield(0, err)
				return
			}
			if len(bytes.TrimSpace(body)) > 0 {
				count++
				value, parseErr := record.ParseRecordJSON(body)
				if parseErr != nil {
					yield(0, i18n.Errorf("error.vector_input_line", line, parseErr))
					return
				}
				if !resolve(target.Input{Record: &value}) {
					return
				}
			}
			if errors.Is(err, io.EOF) {
				if count == 0 {
					yield(0, i18n.Errorf("error.vector_add_input_required"))
				}
				return
			}
		}
	}
}
