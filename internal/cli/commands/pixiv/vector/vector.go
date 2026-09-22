// Package vector owns the local vector CLI commands without loading Pixiv credentials.
package vector

import (
	"context"
	"errors"
	"fmt"
	"io"

	requirements "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands"
	index "github.com/FlanChanXwO/pixiv-cli/internal/vector"
	"github.com/spf13/cobra"
)

func New(out io.Writer, open func() (*index.Store, error), start func(context.Context) (*index.SigLIP2, error)) *cobra.Command {
	cmd := &cobra.Command{Use: "vector", Short: "Manage the local vector index"}
	requirements.Bind(cmd, requirements.Execution{})
	sync := &cobra.Command{Use: "sync", Short: "Synchronize explicit sources"}
	sync.AddCommand(&cobra.Command{
		Use: "local PATH", Short: "Index a local image gallery", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return syncLocal(cmd.Context(), out, open, start, args[0])
		},
	})
	cmd.AddCommand(sync)
	cmd.AddCommand(&cobra.Command{
		Use: "status", Short: "Show durable vector index counts", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) (err error) {
			store, err := open()
			if err != nil {
				return err
			}
			defer func() { err = errors.Join(err, store.Close()) }()
			assets, embeddings, err := store.Status(cmd.Context())
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(out, "assets: %d\nembeddings: %d\n", assets, embeddings)
			return err
		},
	})
	return cmd
}

func syncLocal(ctx context.Context, out io.Writer, open func() (*index.Store, error), start func(context.Context) (*index.SigLIP2, error), path string) (err error) {
	store, err := open()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, store.Close()) }()
	stats, err := index.SyncLocal(ctx, store, path)
	if err != nil {
		return err
	}
	if _, err = fmt.Fprintf(out, "scanned: %d\nchanged: %d\n", stats.Scanned, stats.Changed); err != nil {
		return err
	}
	var runtime *index.SigLIP2
	processed, processErr := index.ProcessLocalPending(ctx, store, index.ModelID, index.Generation, func(ctx context.Context, path string) ([]float32, error) {
		if runtime == nil {
			var err error
			runtime, err = start(ctx)
			if err != nil {
				return nil, err
			}
		}
		return runtime.Image(ctx, path)
	})
	if runtime != nil {
		processErr = errors.Join(processErr, runtime.Close())
	}
	_, writeErr := fmt.Fprintf(out, "embedded: %d\n", processed)
	return errors.Join(processErr, writeErr)
}
