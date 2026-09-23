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
	"strconv"
	"strings"

	requirements "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands"
	index "github.com/FlanChanXwO/pixiv-cli/internal/vector"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

// ImageEncoder 是本地图片编码端口。生产实现是离线 SigLIP2 子进程；测试可注入
// 纯内存替身，从而在不启动任何进程、也不依赖 POSIX shell 的前提下验证命令装配。
type ImageEncoder interface {
	Image(ctx context.Context, path string) ([]float32, error)
	Text(ctx context.Context, text string) ([]float32, error)
	Close() error
}

// BookmarkSource 是 sync bookmarks 唯一的 Pixiv 读取面。它刻意不暴露 artwork
// detail，因此实现不可能退化为“listing 后再逐个 detail”的 N+1 请求。
type BookmarkSource interface {
	UserID() int64
	UserArtworkBookmarks(ctx context.Context, request pixiv.UserArtworkBookmarksRequest) (sdk.Page[pixiv.Artwork], error)
	SaveResource(ctx context.Context, ref sdk.ResourceRef, options sdk.SaveOptions) (sdk.SavedResource, error)
}

// BookmarkPort 在账号池安全重放边界内提供一个已认证的 BookmarkSource；listing 与
// cover 取图必须落在同一个 client 实例上，否则 cover 解析会退化为 artwork detail。
type BookmarkPort func(ctx context.Context, attempt func(context.Context, BookmarkSource) (bool, error)) error

func New(out io.Writer, open func() (*index.Store, error), start func(context.Context) (ImageEncoder, error), bookmarks BookmarkPort) *cobra.Command {
	cmd := &cobra.Command{Use: "vector", Short: "Manage the local vector index"}
	requirements.Bind(cmd, requirements.Execution{})
	sync := &cobra.Command{Use: "sync", Short: "Synchronize explicit sources"}
	sync.AddCommand(&cobra.Command{
		Use: "local PATH", Short: "Index a local image gallery", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return syncLocal(cmd.Context(), out, open, start, args[0])
		},
	})
	bookmarkSync := &cobra.Command{
		Use: "bookmarks", Short: "Index the current account's bookmarked artworks", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return syncBookmarks(cmd.Context(), out, open, start, bookmarks)
		},
	}
	// bookmarks 需要本地账号与运行时配置；local/status/search/rebuild 保持无凭证、无配置副作用。
	requirements.Bind(bookmarkSync, requirements.PixivData())
	sync.AddCommand(bookmarkSync)
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

func syncLocal(ctx context.Context, out io.Writer, open func() (*index.Store, error), start func(context.Context) (ImageEncoder, error), path string) (err error) {
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
	var runtime ImageEncoder
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

// syncBookmarks records page 0 of every bookmarked artwork the account can list.
// It never issues artwork detail, so a multi-page artwork is stored as its listing cover
// with cover_only set rather than fabricating the pages the listing did not provide.
func syncBookmarks(ctx context.Context, out io.Writer, open func() (*index.Store, error), start func(context.Context) (ImageEncoder, error), bookmarks BookmarkPort) (err error) {
	if bookmarks == nil {
		return errors.New("vector: bookmark sync is not configured")
	}
	store, err := open()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, store.Close()) }()
	var runtime ImageEncoder
	var stats index.SyncStats
	processed := 0
	runErr := bookmarks(ctx, func(ctx context.Context, source BookmarkSource) (bool, error) {
		userID := source.UserID()
		if userID <= 0 {
			return false, errors.New("vector: cannot determine the current user ID")
		}
		// 1. 只用 listing 已返回的数据收集作品与其 cover 引用，不发起 detail 请求。
		artworks := make([]index.PixivArtwork, 0, 64)
		covers := make(map[string]sdk.ResourceRef)
		for _, restrict := range []pixiv.Restrict{pixiv.RestrictPublic, pixiv.RestrictPrivate} {
			page, err := collectBookmarks(ctx, source, userID, restrict, &artworks, covers)
			if err != nil {
				return false, err
			}
			stats.Scanned += page.Scanned
			stats.Skipped += page.Skipped
		}
		// 2. 记录 Asset；此处开始改动本地索引，因此之后不再让账号池重放已持久化的工作。
		pageStats, err := index.SyncPixivArtworks(ctx, store, artworks)
		if err != nil {
			return true, err
		}
		stats.Changed += pageStats.Changed
		// 3. 用同一个 client 取回缺向量的 cover，绝不请求 artwork detail。
		count, err := index.EmbedPixivArtworks(ctx, store, index.ModelID, index.Generation, artworks, func(ctx context.Context, artwork index.PixivArtwork) ([]float32, error) {
			ref, ok := covers[strconv.FormatInt(artwork.ID, 10)]
			if !ok {
				return nil, errors.New("vector: bookmark cover reference is unavailable")
			}
			if runtime == nil {
				runtime, err = start(ctx)
				if err != nil {
					return nil, err
				}
			}
			temp, err := os.CreateTemp("", "pixiv-vector-cover-*")
			if err != nil {
				return nil, err
			}
			path := temp.Name()
			if closeErr := temp.Close(); closeErr != nil {
				return nil, closeErr
			}
			defer func() { _ = os.Remove(path) }()
			if _, err := source.SaveResource(ctx, ref, sdk.SaveOptions{Path: path}); err != nil {
				return nil, err
			}
			return runtime.Image(ctx, path)
		})
		processed += count
		return true, err
	})
	if runtime != nil {
		runErr = errors.Join(runErr, runtime.Close())
	}
	_, writeErr := fmt.Fprintf(out, "scanned: %d\nchanged: %d\nskipped: %d\nembedded: %d\n", stats.Scanned, stats.Changed, stats.Skipped, processed)
	return errors.Join(runErr, writeErr)
}

// collectBookmarks 遍历一个 restrict 下的全部 bookmark listing 页，把 listing 已取得的
// 字段追加到 artworks，并记录每个作品的 cover 引用。它不设结果条数上限。
func collectBookmarks(ctx context.Context, source BookmarkSource, userID int64, restrict pixiv.Restrict, artworks *[]index.PixivArtwork, covers map[string]sdk.ResourceRef) (index.SyncStats, error) {
	var stats index.SyncStats
	cursor := sdk.Cursor{}
	for {
		page, err := source.UserArtworkBookmarks(ctx, pixiv.UserArtworkBookmarksRequest{UserID: userID, Restrict: restrict, Cursor: cursor})
		if err != nil {
			return stats, err
		}
		for _, item := range page.Items {
			stats.Scanned++
			artwork := index.PixivArtwork{ID: item.ID, Title: item.Title, UserID: item.User.ID, PageCount: item.PageCount}
			ref := item.Cover.Resource.Ref
			if item.ID > 0 && !ref.IsZero() {
				covers[strconv.FormatInt(item.ID, 10)] = ref
				// 用不含签名 URL 的稳定身份作为指纹键，避免 CDN 换签导致重复嵌入。
				artwork.CoverRef = ref.String()
			} else {
				// listing 没有给出可用 cover；记为 skipped，不写入无图片的假 Asset。
				stats.Skipped++
			}
			*artworks = append(*artworks, artwork)
		}
		if page.Next.IsZero() {
			return stats, nil
		}
		cursor = page.Next
	}
}

func rebuild(ctx context.Context, out io.Writer, open func() (*index.Store, error), start func(context.Context) (ImageEncoder, error)) (err error) {
	store, err := open()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, store.Close()) }()
	var runtime ImageEncoder
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
func search(ctx context.Context, out io.Writer, open func() (*index.Store, error), start func(context.Context) (ImageEncoder, error), input string) (err error) {
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
	// Grouping is shared with the index package so its behavior is verifiable without a CLI process.
	for _, match := range index.CollapsePixivPages(matches) {
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
