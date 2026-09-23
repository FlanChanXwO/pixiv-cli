package vector_test

import (
	"context"
	"database/sql"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/vector"
)

func TestSyncLocalScansIncrementally(t *testing.T) {
	ctx := context.Background()
	gallery := t.TempDir()
	dir := t.TempDir()
	writePNG(t, filepath.Join(gallery, "a.png"), 0xff)
	if err := os.Mkdir(filepath.Join(gallery, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gallery, "note.txt"), []byte("not an image"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := vector.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, want := range []vector.SyncStats{{Scanned: 1, Changed: 1}, {Scanned: 1, Changed: 0}} {
		got, err := vector.SyncLocal(ctx, store, gallery)
		if err != nil || got != want {
			t.Fatalf("sync: got=%+v want=%+v err=%v", got, want, err)
		}
	}
	writePNG(t, filepath.Join(gallery, "nested", "b.png"), 0x7f)
	got, err := vector.SyncLocal(ctx, store, gallery)
	if err != nil || got != (vector.SyncStats{Scanned: 2, Changed: 1}) {
		t.Fatalf("new image: got=%+v err=%v", got, err)
	}
}

func writePNG(t *testing.T, path string, red uint8) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: red, A: 0xff})
	if err := png.Encode(file, img); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPendingLocalWorkSurvivesRestartAndContentChange(t *testing.T) {
	ctx := context.Background()
	gallery := t.TempDir()
	path := filepath.Join(gallery, "a.png")
	writePNG(t, path, 0xff)
	dir := t.TempDir()
	store, err := vector.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vector.SyncLocal(ctx, store, gallery); err != nil {
		t.Fatal(err)
	}
	canonicalPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := store.Pending(ctx, vector.ModelID, vector.Generation)
	if err != nil || len(pending) != 1 || pending[0].Key.ID != canonicalPath {
		t.Fatalf("first pending: got=%+v err=%v", pending, err)
	}
	oldFingerprint := pending[0].Fingerprint
	if err := store.PutEmbedding(ctx, pending[0].Key, pending[0].Fingerprint, vector.ModelID, vector.Generation, []float32{1, 0}); err != nil {
		t.Fatal(err)
	}
	if _, err := vector.SyncLocal(ctx, store, gallery); err != nil {
		t.Fatal(err)
	}
	pending, err = store.Pending(ctx, vector.ModelID, vector.Generation)
	if err != nil || len(pending) != 0 {
		t.Fatalf("unchanged image should be done: got=%+v err=%v", pending, err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	writePNG(t, path, 0x7f)
	if err := os.Chtimes(path, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("test requires identical file stat: before=%v after=%v err=%v", before, after, err)
	}
	stats, err := vector.SyncLocal(ctx, store, gallery)
	if err != nil || stats != (vector.SyncStats{Scanned: 1, Changed: 1}) {
		t.Fatalf("same-stat content change: stats=%+v err=%v", stats, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = vector.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	pending, err = store.Pending(ctx, vector.ModelID, vector.Generation)
	if err != nil || len(pending) != 1 || pending[0].Fingerprint == oldFingerprint {
		t.Fatalf("changed image pending after restart: got=%+v err=%v", pending, err)
	}
}

func TestStoreRejectsStaleEmbeddingAfterFileChanges(t *testing.T) {
	ctx := context.Background()
	store, err := vector.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	key := vector.Key{Source: "local", ID: "/gallery/a.png", Page: 0}
	if _, err := store.Upsert(ctx, vector.Asset{Key: key, Fingerprint: "old-hash", TargetModel: "siglip2", TargetGeneration: "one"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Upsert(ctx, vector.Asset{Key: key, Fingerprint: "new-hash", TargetModel: "siglip2", TargetGeneration: "one"}); err != nil {
		t.Fatal(err)
	}
	if err := store.PutEmbedding(ctx, key, "old-hash", "siglip2", "one", []float32{1, 0}); !errors.Is(err, vector.ErrStaleAsset) {
		t.Fatalf("stale worker overwrote changed image: %v", err)
	}
	if _, err := store.Embedding(ctx, key, "siglip2", "one"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("stale vector stored: %v", err)
	}
	if err := store.PutEmbedding(ctx, key, "new-hash", "siglip2", "one", []float32{0, 1}); err != nil {
		t.Fatal(err)
	}
}

func TestSyncLocalRejectsEmptyRoot(t *testing.T) {
	store, err := vector.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := vector.SyncLocal(context.Background(), store, ""); err == nil {
		t.Fatal("empty gallery path scanned the current working directory")
	}
}

func TestProcessLocalPendingRetriesAndRejectsStaleWork(t *testing.T) {
	ctx := context.Background()
	gallery := t.TempDir()
	path := filepath.Join(gallery, "a.png")
	writePNG(t, path, 0xff)
	store, err := vector.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := vector.SyncLocal(ctx, store, gallery); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("model failed")
	if _, err := vector.ProcessLocalPending(ctx, store, gallery, vector.ModelID, vector.Generation, func(context.Context, string) ([]float32, error) {
		return nil, failure
	}); !errors.Is(err, failure) {
		t.Fatalf("failure lost: %v", err)
	}
	pending, err := store.Pending(ctx, vector.ModelID, vector.Generation)
	if err != nil || len(pending) != 1 {
		t.Fatalf("failed work must remain pending: %v %v", pending, err)
	}
	if _, err := vector.ProcessLocalPending(ctx, store, gallery, vector.ModelID, vector.Generation, func(_ context.Context, file string) ([]float32, error) {
		if file != pending[0].Key.ID {
			t.Fatalf("wrong file: %s", file)
		}
		writePNG(t, path, 0x7f)
		return []float32{1, 0}, nil
	}); !errors.Is(err, vector.ErrStaleAsset) {
		t.Fatalf("changed file accepted: %v", err)
	}
	if _, err := store.Embedding(ctx, pending[0].Key, vector.ModelID, vector.Generation); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("stale vector stored: %v", err)
	}
	if _, err := vector.SyncLocal(ctx, store, gallery); err != nil {
		t.Fatal(err)
	}
	processed, err := vector.ProcessLocalPending(ctx, store, gallery, vector.ModelID, vector.Generation, func(context.Context, string) ([]float32, error) { return []float32{1, 0}, nil })
	if err != nil || processed != 1 {
		t.Fatalf("retry: processed=%d err=%v", processed, err)
	}
	processed, err = vector.ProcessLocalPending(ctx, store, gallery, vector.ModelID, vector.Generation, nil)
	if err != nil || processed != 0 {
		t.Fatalf("completed work reran: processed=%d err=%v", processed, err)
	}
}

func TestProcessLocalPendingCancellation(t *testing.T) {
	ctx := context.Background()
	gallery := t.TempDir()
	writePNG(t, filepath.Join(gallery, "a.png"), 0xff)
	store, err := vector.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := vector.SyncLocal(ctx, store, gallery); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := vector.ProcessLocalPending(canceled, store, gallery, vector.ModelID, vector.Generation, func(context.Context, string) ([]float32, error) {
		t.Fatal("embedding called after cancel")
		return nil, nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func TestProcessLocalPendingOnlyEmbedsRequestedGallery(t *testing.T) {
	ctx := context.Background()
	first := t.TempDir()
	second := t.TempDir()
	writePNG(t, filepath.Join(first, "a.png"), 0xff)
	writePNG(t, filepath.Join(second, "b.png"), 0x7f)
	store, err := vector.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, gallery := range []string{first, second} {
		if _, err := vector.SyncLocal(ctx, store, gallery); err != nil {
			t.Fatal(err)
		}
	}
	firstRoot, err := filepath.EvalSymlinks(first)
	if err != nil {
		t.Fatal(err)
	}
	secondRoot, err := filepath.EvalSymlinks(second)
	if err != nil {
		t.Fatal(err)
	}
	processed, err := vector.ProcessLocalPending(ctx, store, second, vector.ModelID, vector.Generation, func(_ context.Context, path string) ([]float32, error) {
		if filepath.Dir(path) != secondRoot {
			t.Fatalf("indexed outside requested gallery: %s", path)
		}
		return []float32{1, 0}, nil
	})
	if err != nil || processed != 1 {
		t.Fatalf("requested gallery: processed=%d err=%v", processed, err)
	}
	pending, err := store.Pending(ctx, vector.ModelID, vector.Generation)
	if err != nil || len(pending) != 1 || filepath.Dir(pending[0].Key.ID) != firstRoot {
		t.Fatalf("other gallery must remain pending: %v %v", pending, err)
	}
}

func TestRebuildLocalRetainsOldVectorsAndRetriesFailedWork(t *testing.T) {
	ctx := context.Background()
	gallery, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(gallery, "a.png")
	b := filepath.Join(gallery, "b.png")
	writePNG(t, a, 0xff)
	writePNG(t, b, 0x7f)
	dbDir := t.TempDir()
	store, err := vector.Open(dbDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	if _, err := vector.SyncLocal(ctx, store, gallery); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{a, b} {
		asset, err := store.Get(ctx, vector.Key{Source: "local", ID: path})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.PutEmbedding(ctx, asset.Key, asset.Fingerprint, "old", "one", []float32{1, 0}); err != nil {
			t.Fatal(err)
		}
	}
	pixivKey := vector.Key{Source: "pixiv", ID: "123", Page: 2}
	if _, err := store.Upsert(ctx, vector.Asset{Key: pixivKey, TargetModel: "old", TargetGeneration: "one"}); err != nil {
		t.Fatal(err)
	}
	if err := store.PutEmbedding(ctx, pixivKey, "", "old", "one", []float32{1, 0}); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("model failed")
	n, err := vector.RebuildLocal(ctx, store, "new", "two", func(_ context.Context, path string) ([]float32, error) {
		if path == b {
			return nil, failure
		}
		return []float32{0, 1}, nil
	})
	if n != 1 || !errors.Is(err, failure) {
		t.Fatalf("failed rebuild: n=%d err=%v", n, err)
	}
	for _, path := range []string{a, b} {
		key := vector.Key{Source: "local", ID: path}
		if got, err := store.Embedding(ctx, key, "old", "one"); err != nil || !slices.Equal(got, []float32{1, 0}) {
			t.Fatalf("old vector lost: %v %v", got, err)
		}
		asset, err := store.Get(ctx, key)
		if err != nil || asset.TargetModel != "new" || asset.TargetGeneration != "two" {
			t.Fatalf("rebuild intent: %+v %v", asset, err)
		}
	}
	pixivAsset, err := store.Get(ctx, pixivKey)
	if err != nil || pixivAsset.TargetModel != "old" || pixivAsset.TargetGeneration != "one" {
		t.Fatalf("pixiv intent changed: %+v %v", pixivAsset, err)
	}
	if _, err := store.Embedding(ctx, vector.Key{Source: "local", ID: b}, "new", "two"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("failed image got vector: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = vector.Open(dbDir)
	if err != nil {
		t.Fatal(err)
	}
	n, err = vector.RebuildLocal(ctx, store, "new", "two", func(context.Context, string) ([]float32, error) { return []float32{0, 1}, nil })
	if n != 2 || err != nil {
		t.Fatalf("retry: %d %v", n, err)
	}
	n, err = vector.RebuildLocal(ctx, store, "new", "two", func(context.Context, string) ([]float32, error) { return []float32{1, 0}, nil })
	if n != 2 || err != nil {
		t.Fatalf("repeat: %d %v", n, err)
	}
	got, err := store.Embedding(ctx, vector.Key{Source: "local", ID: a}, "new", "two")
	if err != nil || !slices.Equal(got, []float32{1, 0}) {
		t.Fatalf("repeat did not refresh: %v %v", got, err)
	}
	if err := os.Remove(b); err != nil {
		t.Fatal(err)
	}
	n, err = vector.RebuildLocal(ctx, store, "new", "two", func(context.Context, string) ([]float32, error) { return []float32{0, 1}, nil })
	if n != 1 || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing source must fail after first: %d %v", n, err)
	}
	got, err = store.Embedding(ctx, vector.Key{Source: "local", ID: b}, "new", "two")
	if err != nil || !slices.Equal(got, []float32{1, 0}) {
		t.Fatalf("missing source's last vector lost: %v %v", got, err)
	}
}
