package vector

import (
	"context"
	"errors"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/vectorindex"
	"github.com/FlanChanXwO/pixiv-cli/internal/vectormodel"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// OpenClient 由组合根选择固定账号；返回的关闭函数释放 Pixiv 操作通道。
type OpenClient func(context.Context, environment.PixivRequest) (int64, *pixiv.Client, func() error, error)

func syncCommand(env environment.Env, store config.Store, open OpenClient) *cobra.Command {
	var options environment.Options
	cmd := &cobra.Command{Use: "sync", Short: "help.synchronize_bookmarks_and_embed_pending_pages", Args: env.ExactArgs(0, "pixiv vector sync [--json]"), RunE: func(cmd *cobra.Command, _ []string) (err error) {
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
		report, runErr := db.Sync(cmd.Context(), vectorindex.SyncOptions{Model: identity, Embedder: model, ProbeImage: fixture, OpenClient: func(ctx context.Context) (int64, *pixiv.Client, func() error, error) { return open(ctx, request) }, Progress: progress})
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
	env.BindNoInput(cmd)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func writeSyncReport(env environment.Env, r vectorindex.SyncReport) error {
	_, err := fmt.Fprintf(env.Out, env.Text("output.bookmarks_checked_new_merged_recorded_artworks_skipped_records"), r.Bookmarks.Checked, r.Bookmarks.New, r.Merged, r.SkippedRecords, r.Embedded, r.Unavailable, r.Failed, r.Index.Artworks, r.Index.Pages, r.Index.Pending, r.Model)
	if r.Stopped != nil {
		_, stopErr := fmt.Fprintf(env.Out, env.Text("output.stopped"), *r.Stopped)
		err = errors.Join(err, stopErr)
	}
	return err
}
