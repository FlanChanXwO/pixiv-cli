package vector

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
)

// PixivPage 是一个已取得身份的页面。Ref 是不含签名 URL 的稳定资源身份文本；
// 空 Ref 表示该页已知存在但没有可用图片，不能写成假 Asset。
type PixivPage struct {
	Index int
	Ref   string
}

// PixivArtwork 是一次 Pixiv 读取已取得的公开字段快照。
// Pages 非空表示已取得全部页面身份（例如 detail）；为空时只能使用 CoverRef，
// 因为 listing 只提供 cover，其余页面的 URL 需要 artwork detail。
// CoverRef 是不含签名 URL 的稳定资源身份文本；空值表示没有给出可用图片。
type PixivArtwork struct {
	ID        int64
	Title     string
	Caption   string
	UserID    int64
	UserName  string
	PageCount int
	Kind      string
	Tags      []string
	XRestrict int
	AIType    int
	CoverRef  string
	Pages     []PixivPage
}

// pixivAssetMetadata 只保存当前 Artwork 已经返回的字段，绝不为 metadata 发新请求。
type pixivAssetMetadata struct {
	Title     string   `json:"title,omitempty"`
	Caption   string   `json:"caption,omitempty"`
	UserID    int64    `json:"user_id,omitempty"`
	UserName  string   `json:"user_name,omitempty"`
	PageCount int      `json:"page_count,omitempty"`
	Kind      string   `json:"kind,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	XRestrict int      `json:"x_restrict,omitempty"`
	AIType    int      `json:"ai_type,omitempty"`
	// CoverOnly 为 true 表示该作品还有未取得的页面；不会用 cover 伪造其余页。
	CoverOnly bool   `json:"cover_only,omitempty"`
	URL       string `json:"url"`
}

// SyncPixivArtworks records every page the source actually provided, one Asset per page,
// and is idempotent: repeating an unchanged observation adds no Asset and no embedding work.
// Each page's stable resource identity is persisted alongside the Asset so a later explicit
// vector operation can re-fetch the image without re-listing. When Pages is empty only page
// 0 is known, so a multi-page artwork is stored as its cover with cover_only set; resolving
// the remaining pages would require artwork detail, which bookmark sync and the passive
// observer deliberately never issue. Missing image identity is counted as skipped rather
// than stored as an image-less asset.
func SyncPixivArtworks(ctx context.Context, store *Store, artworks []PixivArtwork) (SyncStats, error) {
	if store == nil {
		return SyncStats{}, errors.New("vector: store is required")
	}
	var stats SyncStats
	for _, artwork := range artworks {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		if artwork.ID <= 0 {
			return stats, fmt.Errorf("vector: artwork ID must be positive, got %d", artwork.ID)
		}
		stats.Scanned++
		pages := artwork.Pages
		coverOnly := false
		if len(pages) == 0 {
			if artwork.CoverRef == "" {
				// 没有图片的作品不产生 Asset；这不是失败，而是上游未提供可用图片。
				stats.Skipped++
				continue
			}
			pages = []PixivPage{{Index: 0, Ref: artwork.CoverRef}}
			coverOnly = artwork.PageCount > 1
		}
		metadata, err := json.Marshal(pixivAssetMetadata{
			Title:     artwork.Title,
			Caption:   artwork.Caption,
			UserID:    artwork.UserID,
			UserName:  artwork.UserName,
			PageCount: artwork.PageCount,
			Kind:      artwork.Kind,
			Tags:      artwork.Tags,
			XRestrict: artwork.XRestrict,
			AIType:    artwork.AIType,
			CoverOnly: coverOnly,
			URL:       "https://www.pixiv.net/artworks/" + strconv.FormatInt(artwork.ID, 10),
		})
		if err != nil {
			return stats, fmt.Errorf("vector: encode artwork metadata: %w", err)
		}
		for _, page := range pages {
			if err := ctx.Err(); err != nil {
				return stats, err
			}
			if page.Ref == "" {
				stats.Skipped++
				continue
			}
			changed, err := store.Upsert(ctx, Asset{
				Key:              Key{Source: "pixiv", ID: strconv.FormatInt(artwork.ID, 10), Page: page.Index},
				Fingerprint:      pixivPageFingerprint(artwork.ID, page.Index),
				Metadata:         metadata,
				ResourceRef:      page.Ref,
				TargetModel:      ModelID,
				TargetGeneration: Generation,
			})
			if err != nil {
				return stats, err
			}
			if changed {
				stats.Changed++
			}
		}
	}
	return stats, nil
}

// pixivPageFingerprint keys a Pixiv page by its canonical identity instead of the raw
// resource reference. A listing advertises an artwork's page 0 as its cover (page -1, the
// "large" variant) while detail advertises the same page as page 0 with the "original"
// variant; hashing the raw references would make those two views of one image look like
// different content and silently invalidate the stored vector. Pixiv image bytes are
// immutable for a given artwork ID and page index, so that identity is the content key.
// ponytail: identity is the content key; byte-hash the fetched image if Pixiv ever allows replacing a page.
func pixivPageFingerprint(artworkID int64, page int) string {
	sum := sha256.Sum256([]byte(strconv.FormatInt(artworkID, 10) + ":" + strconv.Itoa(page)))
	return hex.EncodeToString(sum[:])
}

// ResourceFetcher resolves one persisted resource identity to a readable local image path.
// It owns the file; ProcessPixivPending removes it after embedding.
type ResourceFetcher func(ctx context.Context, resourceRef string) (string, error)

// ProcessPixivPending embeds every Pixiv page that still lacks a vector for the requested
// generation, using the resource identity persisted with each Asset. This is the explicit
// processing path for Observer-created pending pages, including pages > 0: the listing
// covers that bookmark sync resolves directly stay usable without a persisted identity.
// fetch resolves and saves one page. A failed page records its error and later pages
// continue, so one bad page cannot starve the queue; failures stay pending for a retry.
func ProcessPixivPending(ctx context.Context, store *Store, model, generation string, fetch ResourceFetcher, embed func(ctx context.Context, path string) ([]float32, error)) (int, error) {
	if store == nil {
		return 0, errors.New("vector: store is required")
	}
	if embed == nil {
		return 0, errors.New("vector: embedding runtime unavailable")
	}
	pending, err := store.Pending(ctx, model, generation)
	if err != nil {
		return 0, err
	}
	processed := 0
	var assetErrs []error
	for _, asset := range pending {
		if err := ctx.Err(); err != nil {
			return processed, errors.Join(append(assetErrs, err)...)
		}
		if asset.Key.Source != "pixiv" {
			continue
		}
		if err := embedPixivAsset(ctx, store, asset, model, generation, fetch, embed); err != nil {
			// cancel 是命令层停止信号；单项失败继续后续页，保持 pending 可重试。
			if ctx.Err() != nil {
				return processed, errors.Join(append(assetErrs, err)...)
			}
			assetErrs = append(assetErrs, err)
			continue
		}
		processed++
	}
	return processed, errors.Join(assetErrs...)
}

func embedPixivAsset(ctx context.Context, store *Store, asset Asset, model, generation string, fetch ResourceFetcher, embed func(context.Context, string) ([]float32, error)) error {
	if asset.ResourceRef == "" {
		// bookmark sync 落库的 cover Asset 自带可解析引用；观察器落库的 pending 若缺失身份，
		// 只能留待一次带上 cover 引用的显式 sync，或重新观察。这是可诊断的可重试状态。
		return fmt.Errorf("vector: pixiv asset %s page %d has no persisted resource identity; re-observe it with `pixiv vector sync bookmarks` or an artwork detail read", asset.Key.ID, asset.Key.Page)
	}
	if fetch == nil {
		return errors.New("vector: pixiv resource fetcher is unavailable")
	}
	path, err := fetch(ctx, asset.ResourceRef)
	if err != nil {
		return fmt.Errorf("vector: fetch pixiv image %s page %d: %w", asset.Key.ID, asset.Key.Page, err)
	}
	defer func() { _ = os.Remove(path) }()
	values, err := embed(ctx, path)
	if err != nil {
		return fmt.Errorf("vector: embed pixiv image %s page %d: %w", asset.Key.ID, asset.Key.Page, err)
	}
	return store.PutEmbedding(ctx, asset.Key, asset.Fingerprint, model, generation, values)
}
