package vector_test

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
	"github.com/FlanChanXwO/pixiv-cli/internal/vector"
)

func TestStoreKeepsDistinctPagesAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	store, err := vector.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	key0 := vector.Key{Source: "pixiv", ID: "123", Page: 0}
	key1 := vector.Key{Source: "pixiv", ID: "123", Page: 1}
	for _, tc := range []struct {
		key     vector.Key
		created bool
	}{
		{key0, true}, {key0, false}, {key1, true},
	} {
		created, err := store.Upsert(context.Background(), vector.Asset{Key: tc.key, Metadata: []byte(`{"title":"test"}`), TargetModel: "siglip2", TargetGeneration: "one"})
		if err != nil || created != tc.created {
			t.Fatalf("upsert %+v: created=%v err=%v", tc.key, created, err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = vector.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, key := range []vector.Key{key0, key1} {
		got, err := store.Get(context.Background(), key)
		if err != nil || string(got.Metadata) != `{"title":"test"}` {
			t.Fatalf("get %+v after reopen: got=%+v err=%v", key, got, err)
		}
	}
}

func TestStorePersistsEmbeddingsByPageAndGeneration(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := vector.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	first := vector.Key{Source: "pixiv", ID: "123", Page: 0}
	second := vector.Key{Source: "pixiv", ID: "123", Page: 1}
	for _, key := range []vector.Key{first, second} {
		if _, err := store.Upsert(ctx, vector.Asset{Key: key, TargetModel: "siglip2", TargetGeneration: "one"}); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		key        vector.Key
		generation string
		value      []float32
	}{
		{first, "one", []float32{1, 0}},
		{second, "one", []float32{0, 1}},
		{first, "two", []float32{0.5, 0.5}},
	} {
		if err := store.PutEmbedding(ctx, tc.key, "", "siglip2", tc.generation, tc.value); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = vector.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, tc := range []struct {
		key        vector.Key
		generation string
		want       []float32
	}{
		{first, "one", []float32{1, 0}},
		{second, "one", []float32{0, 1}},
		{first, "two", []float32{0.5, 0.5}},
	} {
		got, err := store.Embedding(ctx, tc.key, "siglip2", tc.generation)
		if err != nil || !slices.Equal(got, tc.want) {
			t.Fatalf("embedding %+v/%s: got=%v err=%v", tc.key, tc.generation, got, err)
		}
	}
	if _, err := store.Embedding(ctx, second, "siglip2", "two"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing generation: %v", err)
	}
}

func TestStoreInvalidatesEmbeddingOnlyWhenContentChanges(t *testing.T) {
	ctx := context.Background()
	store, err := vector.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	key := vector.Key{Source: "local", ID: "/gallery/image.jpg", Page: 0}
	initial := vector.Asset{Key: key, Fingerprint: "size:1", Metadata: []byte(`{"title":"old"}`), TargetModel: "siglip2", TargetGeneration: "one"}
	if changed, err := store.Upsert(ctx, initial); err != nil || !changed {
		t.Fatalf("first observe: changed=%v err=%v", changed, err)
	}
	if err := store.PutEmbedding(ctx, key, initial.Fingerprint, "siglip2", "one", []float32{1, 0}); err != nil {
		t.Fatal(err)
	}
	initial.Metadata = []byte(`{"title":"new"}`)
	if changed, err := store.Upsert(ctx, initial); err != nil || changed {
		t.Fatalf("metadata-only observe: changed=%v err=%v", changed, err)
	}
	if got, err := store.Get(ctx, key); err != nil || string(got.Metadata) != `{"title":"new"}` {
		t.Fatalf("metadata refresh: got=%+v err=%v", got, err)
	}
	if _, err := store.Embedding(ctx, key, "siglip2", "one"); err != nil {
		t.Fatalf("metadata change removed embedding: %v", err)
	}
	initial.Fingerprint = "size:2"
	initial.TargetGeneration = "two"
	if changed, err := store.Upsert(ctx, initial); err != nil || !changed {
		t.Fatalf("content change: changed=%v err=%v", changed, err)
	}
	if _, err := store.Embedding(ctx, key, "siglip2", "one"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("old embedding survived content change: %v", err)
	}
	pending, err := store.Pending(ctx, "siglip2", "two")
	if err != nil || len(pending) != 1 || pending[0].Key != key {
		t.Fatalf("changed content must target the new generation: %v %v", pending, err)
	}
}

func TestStoreRejectsInvalidAssetsAndEmbeddings(t *testing.T) {
	ctx := context.Background()
	store, err := vector.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	key := vector.Key{Source: "local", ID: "/gallery/image.jpg", Page: 0}
	for _, asset := range []vector.Asset{
		{Key: vector.Key{Source: "unknown", ID: "x"}},
		{Key: vector.Key{Source: "local", ID: " "}},
		{Key: vector.Key{Source: "local", ID: "x", Page: -1}},
		{Key: key, Metadata: []byte(`[]`), TargetModel: "siglip2", TargetGeneration: "one"},
	} {
		if _, err := store.Upsert(ctx, asset); err == nil {
			t.Fatalf("accepted invalid asset: %+v", asset)
		}
	}
	if err := store.PutEmbedding(ctx, key, "", "siglip2", "one", []float32{1}); err == nil {
		t.Fatal("stored orphan embedding")
	}
	if _, err := store.Upsert(ctx, vector.Asset{Key: key, TargetModel: "siglip2", TargetGeneration: "one"}); err != nil {
		t.Fatal(err)
	}
	if err := store.PutEmbedding(ctx, key, "", "siglip2", "one", []float32{float32(math.NaN())}); err == nil {
		t.Fatal("stored non-finite embedding")
	}
}

func TestStoreProtectsAccountDatabaseBoundary(t *testing.T) {
	dir := t.TempDir()
	store, err := vector.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "pixiv-cli.db")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("vector open touched account database: %v", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dir, "vector.db"))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("vector database mode: info=%v err=%v", info, err)
		}
	}
	if err := os.Rename(filepath.Join(dir, "vector.db"), filepath.Join(dir, "old-vector.db")); err != nil {
		t.Fatal(err)
	}
	auth, err := database.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(dir, "pixiv-cli.db"), filepath.Join(dir, "vector.db")); err != nil {
		t.Fatal(err)
	}
	if mistaken, err := vector.Open(dir); err == nil {
		_ = mistaken.Close()
		t.Fatal("accepted account database as vector index")
	}
}

func TestStoreStatusCountsAssetsAndEmbeddings(t *testing.T) {
	ctx := context.Background()
	store, err := vector.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	key := vector.Key{Source: "local", ID: "/a.png"}
	if _, err := store.Upsert(ctx, vector.Asset{Key: key, Fingerprint: "a", TargetModel: "model", TargetGeneration: "one"}); err != nil {
		t.Fatal(err)
	}
	if err := store.PutEmbedding(ctx, key, "a", "model", "one", []float32{1}); err != nil {
		t.Fatal(err)
	}
	assets, embeddings, err := store.Status(ctx)
	if err != nil || assets != 1 || embeddings != 1 {
		t.Fatalf("status: assets=%d embeddings=%d err=%v", assets, embeddings, err)
	}
	if _, err := store.Upsert(ctx, vector.Asset{Key: key, Fingerprint: "b", TargetModel: "model", TargetGeneration: "one"}); err != nil {
		t.Fatal(err)
	}
	assets, embeddings, err = store.Status(ctx)
	if err != nil || assets != 1 || embeddings != 0 {
		t.Fatalf("invalidated status: assets=%d embeddings=%d err=%v", assets, embeddings, err)
	}
}

func TestOldGenerationIsNotImplicitlyReembeddedAfterRestart(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := vector.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	key := vector.Key{Source: "local", ID: "/gallery/old.png"}
	if _, err := store.Upsert(ctx, vector.Asset{Key: key, Fingerprint: "same", TargetModel: "model", TargetGeneration: "old"}); err != nil {
		t.Fatal(err)
	}
	if err := store.PutEmbedding(ctx, key, "same", "model", "old", []float32{1, 0}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = vector.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	pending, err := store.Pending(ctx, "model", "new")
	if err != nil || len(pending) != 0 {
		t.Fatalf("old generation must not migrate implicitly: %v %v", pending, err)
	}
	old, err := store.Search(ctx, "model", "old", []float32{1, 0})
	if err != nil || len(old) != 1 || old[0].Asset.Key != key {
		t.Fatalf("old generation must remain readable: %v %v", old, err)
	}
	newer, err := store.Search(ctx, "model", "new", []float32{1, 0})
	if err != nil || len(newer) != 0 {
		t.Fatalf("new generation compared old vector: %v %v", newer, err)
	}
}

func TestPendingTargetsOnlyNewlyObservedGeneration(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := vector.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	old := vector.Asset{Key: vector.Key{Source: "local", ID: "/gallery/old.png"}, Fingerprint: "same", TargetModel: "model", TargetGeneration: "old"}
	if _, err := store.Upsert(ctx, old); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = vector.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	// Reobserving unchanged content under the new model must not migrate history.
	old.TargetGeneration = "new"
	if changed, err := store.Upsert(ctx, old); err != nil || changed {
		t.Fatalf("unchanged old asset: %v %v", changed, err)
	}
	fresh := vector.Asset{Key: vector.Key{Source: "local", ID: "/gallery/new.png"}, Fingerprint: "fresh", TargetModel: "model", TargetGeneration: "new"}
	if _, err := store.Upsert(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	pending, err := store.Pending(ctx, "model", "new")
	if err != nil || len(pending) != 1 || pending[0].Key != fresh.Key {
		t.Fatalf("new generation should only enqueue new observation: %v %v", pending, err)
	}
	pending, err = store.Pending(ctx, "model", "old")
	if err != nil || len(pending) != 1 || pending[0].Key != old.Key {
		t.Fatalf("old pending intent must survive restart: %v %v", pending, err)
	}
}

// TestStoreWaitsForCrossProcessWriteLock 锁定二次审查修复：两个独立 Store
// （等价于两个进程）短暂竞争同一 vector.db 写锁时，后到者等待而不是立即失败。
func TestStoreWaitsForCrossProcessWriteLock(t *testing.T) {
	dir := t.TempDir()
	first, err := vector.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := vector.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	done := make(chan error, 1)
	// Connection A 用同驱动独立连接（等价于另一进程）持有写锁 1 秒后主动释放。
	holder, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(dir, "vector.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	go func() {
		tx, err := holder.BeginTx(context.Background(), nil)
		if err != nil {
			done <- err
			return
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(context.Background(), `INSERT INTO asset (source,source_id,page_index,fingerprint,metadata)
			VALUES ('local','holder.png',0,'fp','{}')`); err != nil {
			done <- err
			return
		}
		time.Sleep(1 * time.Second)
		done <- nil
	}()
	// 等 A 拿到写锁再让 B 写同一张表：B 应等待而不是立即 SQLITE_BUSY。
	time.Sleep(200 * time.Millisecond)
	_, err = second.Upsert(context.Background(), vector.Asset{
		Key: vector.Key{Source: "local", ID: "waiter.png"}, Fingerprint: "fp",
		Metadata: []byte(`{}`), TargetModel: "m", TargetGeneration: "g",
	})
	if err != nil {
		t.Fatalf("second writer failed under brief cross-store lock contention: %v", err)
	}
	if holderErr := <-done; holderErr != nil {
		t.Fatal(holderErr)
	}
}
