package vector

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// PixivArtwork 是一次 bookmark listing 已取得的公开字段快照。
// listing 只提供 cover，其余页面的 URL 需要 artwork detail，因此这里不假设多页图片可得。
// CoverRef 是不含签名 URL 的稳定资源身份文本；空值表示 listing 没有给出可用图片。
type PixivArtwork struct {
	ID        int64
	Title     string
	UserID    int64
	PageCount int
	CoverRef  string
}

// pixivAssetMetadata 是写入 Asset 的稳定最小身份，便于搜索时恢复作品 URL。
type pixivAssetMetadata struct {
	Title     string `json:"title,omitempty"`
	UserID    int64  `json:"user_id,omitempty"`
	PageCount int    `json:"page_count,omitempty"`
	// CoverOnly 为 true 表示该作品还有未取得的页面；不会用 cover 伪造其余页。
	CoverOnly bool   `json:"cover_only,omitempty"`
	URL       string `json:"url"`
}

// SyncPixivArtworks records the pages a bookmark listing actually provided.
// Only page 0 is known from a listing: multi-page artworks are stored as their cover with
// cover_only set, because resolving the remaining pages requires artwork detail, which
// bookmark sync deliberately never issues. Artworks without an image are skipped, not faked.
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
		if artwork.CoverRef == "" {
			// 没有图片的作品不产生 Asset；这不是失败，而是 listing 未提供可用图片。
			stats.Skipped++
			continue
		}
		metadata, err := json.Marshal(pixivAssetMetadata{
			Title:     artwork.Title,
			UserID:    artwork.UserID,
			PageCount: artwork.PageCount,
			CoverOnly: artwork.PageCount > 1,
			URL:       "https://www.pixiv.net/artworks/" + strconv.FormatInt(artwork.ID, 10),
		})
		if err != nil {
			return stats, fmt.Errorf("vector: encode artwork metadata: %w", err)
		}
		changed, err := store.Upsert(ctx, Asset{
			Key:              Key{Source: "pixiv", ID: strconv.FormatInt(artwork.ID, 10), Page: 0},
			Fingerprint:      pixivCoverFingerprint(artwork.CoverRef),
			Metadata:         metadata,
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
	return stats, nil
}

// pixivCoverFingerprint keys on the stable resource identity, not the signed CDN URL, so a
// refreshed signature does not invalidate an unchanged cover while a genuinely different
// cover still retargets the asset.
// ponytail: identity is a proxy for content; hash the fetched bytes if a same-identity cover ever changes.
func pixivCoverFingerprint(coverRef string) string {
	sum := sha256.Sum256([]byte(coverRef))
	return hex.EncodeToString(sum[:])
}

// EmbedPixivArtworks embeds the cover of every synced Pixiv artwork that still lacks a
// vector for the requested generation. embed resolves and encodes one artwork cover; it owns
// any temporary file. The first failure stops the pass and is returned with the completed
// count, so successful covers stay durable and the rest remain pending for a later sync.
func EmbedPixivArtworks(ctx context.Context, store *Store, model, generation string, artworks []PixivArtwork, embed func(context.Context, PixivArtwork) ([]float32, error)) (int, error) {
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
	// listing 只可能提供 page 0；按 ID 索引以免对同一作品重复取图。
	wanted := make(map[string]PixivArtwork, len(artworks))
	for _, artwork := range artworks {
		if artwork.ID > 0 && artwork.CoverRef != "" {
			wanted[strconv.FormatInt(artwork.ID, 10)] = artwork
		}
	}
	processed := 0
	for _, asset := range pending {
		if err := ctx.Err(); err != nil {
			return processed, err
		}
		if asset.Key.Source != "pixiv" || asset.Key.Page != 0 {
			continue
		}
		artwork, ok := wanted[asset.Key.ID]
		if !ok {
			continue
		}
		values, err := embed(ctx, artwork)
		if err != nil {
			return processed, fmt.Errorf("vector: embed artwork cover: %w", err)
		}
		if err := store.PutEmbedding(ctx, asset.Key, asset.Fingerprint, model, generation, values); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}
