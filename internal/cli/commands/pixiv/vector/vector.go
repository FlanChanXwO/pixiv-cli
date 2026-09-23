// Package vector owns the local vector CLI commands without loading Pixiv credentials.
package vector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

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
		Use: "search QUERY_OR_IMAGE", Short: "Search the persistent local vector index", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return search(cmd.Context(), out, open, start, args[0])
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use: "rebuild", Short: "Explicitly re-embed recorded local images", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return rebuild(cmd.Context(), out, open, start)
		},
	})
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
	processed, processErr := index.ProcessLocalPending(ctx, store, path, index.ModelID, index.Generation, func(ctx context.Context, path string) ([]float32, error) {
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

func rebuild(ctx context.Context, out io.Writer, open func() (*index.Store, error), start func(context.Context) (*index.SigLIP2, error)) (err error) {
	store, err := open()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, store.Close()) }()
	var runtime *index.SigLIP2
	processed, processErr := index.RebuildLocal(ctx, store, index.ModelID, index.Generation, func(ctx context.Context, path string) ([]float32, error) {
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

// search has no Pixiv SDK or reverse-search dependency; it only reads the private index.
func search(ctx context.Context, out io.Writer, open func() (*index.Store, error), start func(context.Context) (*index.SigLIP2, error), input string) (err error) {
	if strings.TrimSpace(input) == "" {
		return errors.New("vector: query is required")
	}
	file, statErr := os.Stat(input)
	image := statErr == nil
	if image && !file.Mode().IsRegular() {
		return errors.New("vector: image must be a regular file")
	}
	if statErr != nil {
		if !errors.Is(statErr, os.ErrNotExist) {
			return fmt.Errorf("vector: inspect query: %w", statErr)
		}
		if filepath.IsAbs(input) || strings.HasPrefix(input, "./") || strings.HasPrefix(input, "../") || index.IsImageExtension(filepath.Ext(input)) {
			return errors.New("vector: image file does not exist")
		}
	}
	store, err := open()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, store.Close()) }()
	runtime, err := start(ctx)
	if err != nil {
		return err
	}
	var query []float32
	if image {
		query, err = runtime.Image(ctx, input)
	} else {
		query, err = runtime.Text(ctx, input)
	}
	err = errors.Join(err, runtime.Close())
	if err != nil {
		return err
	}
	matches, err := store.Search(ctx, index.ModelID, index.Generation, query)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(out)
	// Store.Search is already score-sorted; the first page is the best hit for each artwork.
	seenPixiv := make(map[string]bool)
	for _, match := range matches {
		if match.Asset.Key.Source == "pixiv" {
			if seenPixiv[match.Asset.Key.ID] {
				continue
			}
			seenPixiv[match.Asset.Key.ID] = true
		}
		result := struct {
			Source   string          `json:"source"`
			SourceID string          `json:"source_id"`
			Page     int             `json:"page_index"`
			Score    float64         `json:"score"`
			Metadata json.RawMessage `json:"metadata"`
			URL      string          `json:"url,omitempty"`
		}{match.Asset.Key.Source, match.Asset.Key.ID, match.Asset.Key.Page, match.Score, match.Asset.Metadata, ""}
		if result.Source == "pixiv" {
			result.URL = "https://www.pixiv.net/artworks/" + url.PathEscape(result.SourceID)
		}
		if err := encoder.Encode(result); err != nil {
			return err
		}
	}
	return nil
}
