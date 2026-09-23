package vector_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/vector"
)

// TestSyncPixivArtworksRecordsCoverAsPageZero 锁定 bookmark listing 摄取契约：
// 只用 listing 已给的 cover 建立 page 0 Asset，多页作品显式标记 cover_only。
func TestSyncPixivArtworksRecordsCoverAsPageZero(t *testing.T) {
	ctx := context.Background()
	store, err := vector.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	stats, err := vector.SyncPixivArtworks(ctx, store, []vector.PixivArtwork{
		{ID: 101, Title: "multi", UserID: 7, PageCount: 3, CoverRef: "ref-101-cover"},
		{ID: 202, Title: "single", UserID: 8, PageCount: 1, CoverRef: "ref-202-cover"},
		{ID: 303, Title: "no image", UserID: 9, PageCount: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Scanned != 3 || stats.Changed != 2 || stats.Skipped != 1 {
		t.Fatalf("stats = %+v, want scanned=3 changed=2 skipped=1", stats)
	}
	asset, err := store.Get(ctx, vector.Key{Source: "pixiv", ID: "101", Page: 0})
	if err != nil {
		t.Fatal(err)
	}
	var metadata struct {
		Title     string `json:"title"`
		UserID    int64  `json:"user_id"`
		PageCount int    `json:"page_count"`
		CoverOnly bool   `json:"cover_only"`
		URL       string `json:"url"`
	}
	if err := json.Unmarshal(asset.Metadata, &metadata); err != nil {
		t.Fatalf("metadata %s: %v", asset.Metadata, err)
	}
	if metadata.Title != "multi" || metadata.UserID != 7 || metadata.PageCount != 3 || !metadata.CoverOnly {
		t.Fatalf("multi-page metadata lost its unresolved page state: %+v", metadata)
	}
	if metadata.URL != "https://www.pixiv.net/artworks/101" {
		t.Fatalf("metadata url = %q", metadata.URL)
	}
	// 同一次 listing 的 cover 内容指纹必须稳定，重复 sync 才能幂等。
	first := asset.Fingerprint
	stats, err = vector.SyncPixivArtworks(ctx, store, []vector.PixivArtwork{
		{ID: 101, Title: "multi renamed", UserID: 7, PageCount: 3, CoverRef: "ref-101-cover"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Changed != 0 {
		t.Fatalf("unchanged cover must not report a change: %+v", stats)
	}
	reloaded, err := store.Get(ctx, vector.Key{Source: "pixiv", ID: "101", Page: 0})
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Fingerprint != first {
		t.Fatalf("fingerprint changed across idempotent sync: %q -> %q", first, reloaded.Fingerprint)
	}
	// 单一页作品的 cover 就是 page 0，不能被标成 cover_only。
	single, err := store.Get(ctx, vector.Key{Source: "pixiv", ID: "202", Page: 0})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(single.Metadata), `"cover_only":true`) {
		t.Fatalf("single-page artwork must not claim cover_only: %s", single.Metadata)
	}
	// 无 cover 的作品不得被写成无图片的假 Asset。
	if _, err := store.Get(ctx, vector.Key{Source: "pixiv", ID: "303", Page: 0}); err == nil {
		t.Fatal("artwork without an image must not create an asset")
	}
}

// TestSyncPixivArtworksRejectsInvalidInput 保证坏输入不会被静默写库。
func TestSyncPixivArtworksRejectsInvalidInput(t *testing.T) {
	ctx := context.Background()
	store, err := vector.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := vector.SyncPixivArtworks(ctx, store, []vector.PixivArtwork{{ID: 0, CoverRef: "ref-x"}}); err == nil {
		t.Fatal("non-positive artwork ID must fail")
	}
	if _, err := vector.SyncPixivArtworks(ctx, nil, nil); err == nil {
		t.Fatal("nil store must fail")
	}
}

// TestEmbedPixivArtworksEmbedsMissingCoversAndLeavesFailuresPending 锁定 bookmark
// embedding 语义：只嵌入缺向量的覆盖页，失败保持 pending，取消立即停止。
func TestEmbedPixivArtworksEmbedsMissingCoversAndLeavesFailuresPending(t *testing.T) {
	ctx := context.Background()
	store, err := vector.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	artworks := []vector.PixivArtwork{
		{ID: 101, Title: "one", UserID: 7, PageCount: 1, CoverRef: "ref-101"},
		{ID: 202, Title: "two", UserID: 8, PageCount: 2, CoverRef: "ref-202-cover"},
	}
	if _, err := vector.SyncPixivArtworks(ctx, store, artworks); err != nil {
		t.Fatal(err)
	}
	requested := make([]int64, 0, 2)
	embed := func(_ context.Context, artwork vector.PixivArtwork) ([]float32, error) {
		requested = append(requested, artwork.ID)
		if artwork.ID == 202 {
			return nil, errors.New("cover download failed")
		}
		return []float32{1, 2, 3}, nil
	}
	processed, err := vector.EmbedPixivArtworks(ctx, store, vector.ModelID, vector.Generation, artworks, embed)
	if err == nil {
		t.Fatal("a failed cover must surface an error while keeping other work durable")
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want the successful cover only", processed)
	}
	if len(requested) != 2 {
		t.Fatalf("embed calls = %v, want both pending covers", requested)
	}
	if _, err := store.Embedding(ctx, vector.Key{Source: "pixiv", ID: "101", Page: 0}, vector.ModelID, vector.Generation); err != nil {
		t.Fatalf("successful cover must persist: %v", err)
	}
	if _, err := store.Embedding(ctx, vector.Key{Source: "pixiv", ID: "202", Page: 0}, vector.ModelID, vector.Generation); err == nil {
		t.Fatal("failed cover must not persist a vector")
	}
	// 已完成的覆盖页不应被重复嵌入。
	requested = requested[:0]
	if _, err := vector.EmbedPixivArtworks(ctx, store, vector.ModelID, vector.Generation,
		[]vector.PixivArtwork{artworks[0]}, embed); err != nil {
		t.Fatal(err)
	}
	if len(requested) != 0 {
		t.Fatalf("already embedded cover was re-fetched: %v", requested)
	}
}

// TestSyncPixivArtworksRecordsEveryKnownPageIdempotently 锁定 Task 14 契约：
// 已知多页时每个页面建立独立 Asset；重复 observe 不产生新 Asset 或重复 embedding work。
func TestSyncPixivArtworksRecordsEveryKnownPageIdempotently(t *testing.T) {
	ctx := context.Background()
	store, err := vector.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	// detail 路径已取得全部三页；listing-only 作品只有 cover。
	artworks := []vector.PixivArtwork{
		{
			ID: 101, Title: "multi", UserID: 7, PageCount: 3,
			Pages: []vector.PixivPage{{Index: 0, Ref: "ref-101-p0"}, {Index: 1, Ref: "ref-101-p1"}, {Index: 2, Ref: "ref-101-p2"}},
		},
		{ID: 202, Title: "cover only", UserID: 8, PageCount: 2, CoverRef: "ref-202-cover"},
	}
	stats, err := vector.SyncPixivArtworks(ctx, store, artworks)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Scanned != 2 || stats.Changed != 4 || stats.Skipped != 0 {
		t.Fatalf("stats = %+v, want scanned=2 changed=4 (3 pages + 1 cover)", stats)
	}
	assets, embeddings, err := store.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if assets != 4 || embeddings != 0 {
		t.Fatalf("assets/embeddings = %d/%d, want 4/0 (observe records assets, never embeds)", assets, embeddings)
	}
	// 全部页面已知的作品不得被标成 cover_only，且保留真实页数。
	asset, err := store.Get(ctx, vector.Key{Source: "pixiv", ID: "101", Page: 2})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(asset.Metadata), `"cover_only":true`) {
		t.Fatalf("fully known pages must not claim cover_only: %s", asset.Metadata)
	}
	pending, err := store.Pending(ctx, vector.ModelID, vector.Generation)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 4 {
		t.Fatalf("pending = %d, want every observed page pending for a later sync", len(pending))
	}
	// 同 Asset 重复 observe 不得重复产生 Asset 或 embedding work。
	stats, err = vector.SyncPixivArtworks(ctx, store, artworks)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Changed != 0 {
		t.Fatalf("repeat observe reported changes: %+v", stats)
	}
	assets, _, err = store.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if assets != 4 {
		t.Fatalf("repeat observe duplicated assets: %d", assets)
	}
	pending, err = store.Pending(ctx, vector.ModelID, vector.Generation)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 4 {
		t.Fatalf("repeat observe changed pending work: %d", len(pending))
	}
}

// TestSyncPixivArtworksIgnoresPagesWithoutIdentity 保证缺身份的页面不会被写成假 Asset。
func TestSyncPixivArtworksIgnoresPagesWithoutIdentity(t *testing.T) {
	ctx := context.Background()
	store, err := vector.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	stats, err := vector.SyncPixivArtworks(ctx, store, []vector.PixivArtwork{{
		ID: 303, PageCount: 2,
		Pages: []vector.PixivPage{{Index: 0, Ref: "ref-p0"}, {Index: 1, Ref: ""}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Changed != 1 {
		t.Fatalf("only the page with a real identity must be recorded: %+v", stats)
	}
	if _, err := store.Get(ctx, vector.Key{Source: "pixiv", ID: "303", Page: 1}); err == nil {
		t.Fatal("a page without an image identity must not become an asset")
	}
}

// detailPageRef / listingCoverRef 模拟两条真实路径产生的身份文本（已用真实 SDK 实测确认）：
// detail 第 0 页为 page=0/variant=original，listing cover 为 page=-1/variant=large。
// 它们描述同一张图，指纹必须一致。
const (
	detailPage0Ref  = `{"k":"artwork","id":500,"v":"original"}`
	listingCoverRef = `{"k":"artwork","id":500,"p":-1,"v":"large"}`
	detailPage1Ref  = `{"k":"artwork","id":500,"p":1,"v":"original"}`
)

// TestSyncPixivArtworksFingerprintStableAcrossCoverAndDetailIdentity 锁定 Task 16A：
// 同一作品的 page 0 经 listing cover 或 detail 观察必须得到同一指纹，否则普通列表命令
// 会静默失效 detail 已存的向量。
func TestSyncPixivArtworksFingerprintStableAcrossCoverAndDetailIdentity(t *testing.T) {
	ctx := context.Background()
	store, err := vector.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	key := vector.Key{Source: "pixiv", ID: "500", Page: 0}
	// 1. detail 观察：记录 page 0 与 page 1。
	if _, err := vector.SyncPixivArtworks(ctx, store, []vector.PixivArtwork{{
		ID: 500, PageCount: 2,
		Pages: []vector.PixivPage{{Index: 0, Ref: detailPage0Ref}, {Index: 1, Ref: detailPage1Ref}},
	}}); err != nil {
		t.Fatal(err)
	}
	asset, err := store.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutEmbedding(ctx, key, asset.Fingerprint, vector.ModelID, vector.Generation, []float32{1, 0}); err != nil {
		t.Fatal(err)
	}
	// 2. 随后的普通列表命令只看到 cover：page 0 指纹必须不变。
	stats, err := vector.SyncPixivArtworks(ctx, store, []vector.PixivArtwork{{
		ID: 500, PageCount: 2, CoverRef: listingCoverRef,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Changed != 0 {
		t.Fatalf("cover observation reported changed=%d, want 0 (identity must be path-independent)", stats.Changed)
	}
	if _, err := store.Embedding(ctx, key, vector.ModelID, vector.Generation); err != nil {
		t.Fatalf("cover observation invalidated the stored page-0 vector: %v", err)
	}
}

// TestSyncPixivArtworksInterleavedObservationKeepsEmbedding 覆盖反复交错的观察序列。
func TestSyncPixivArtworksInterleavedObservationKeepsEmbedding(t *testing.T) {
	ctx := context.Background()
	store, err := vector.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	key := vector.Key{Source: "pixiv", ID: "500", Page: 0}
	detail := vector.PixivArtwork{ID: 500, PageCount: 2, Pages: []vector.PixivPage{{Index: 0, Ref: detailPage0Ref}, {Index: 1, Ref: detailPage1Ref}}}
	listing := vector.PixivArtwork{ID: 500, PageCount: 2, CoverRef: listingCoverRef}
	if _, err := vector.SyncPixivArtworks(ctx, store, []vector.PixivArtwork{detail}); err != nil {
		t.Fatal(err)
	}
	asset, err := store.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutEmbedding(ctx, key, asset.Fingerprint, vector.ModelID, vector.Generation, []float32{1, 0}); err != nil {
		t.Fatal(err)
	}
	// detail → listing → detail → listing 交错，向量必须始终保留。
	for _, artwork := range []vector.PixivArtwork{listing, detail, listing} {
		stats, err := vector.SyncPixivArtworks(ctx, store, []vector.PixivArtwork{artwork})
		if err != nil {
			t.Fatal(err)
		}
		if stats.Changed != 0 {
			t.Fatalf("interleaved observation reported changed=%d, want 0", stats.Changed)
		}
		if _, err := store.Embedding(ctx, key, vector.ModelID, vector.Generation); err != nil {
			t.Fatalf("interleaved observation dropped the stored vector: %v", err)
		}
	}
}
