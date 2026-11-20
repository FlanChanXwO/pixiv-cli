package vector

import (
	"context"
	"errors"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
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
	cmd := &cobra.Command{Use: "sync", Short: "Synchronize bookmarks and embed pending pages", Args: env.ExactArgs(0, "pixiv vector sync [--json]"), RunE: func(cmd *cobra.Command, _ []string) (err error) {
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
			return errors.New("vector sync is busy")
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
				_, err := fmt.Fprintf(env.Err, "\rEmbedding %d/%d pages", done, total)
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
	_, err := fmt.Fprintf(env.Out, "Bookmarks: %d checked, %d new\nMerged: %d recorded artworks\nSkipped records: %d\nEmbedded: %d pages\nUnavailable: %d pages\nFailed: %d pages\nIndex: %d artworks, %d pages, %d pending (%s)\n", r.Bookmarks.Checked, r.Bookmarks.New, r.Merged, r.SkippedRecords, r.Embedded, r.Unavailable, r.Failed, r.Index.Artworks, r.Index.Pages, r.Index.Pending, r.Model)
	if r.Stopped != nil {
		_, stopErr := fmt.Fprintf(env.Out, "Stopped: %s\n", *r.Stopped)
		err = errors.Join(err, stopErr)
	}
	return err
}
