package vector_test

import (
	"context"
	"math"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/vector"
)

func TestSearchRanksPagesAcrossRestartWithoutMixingGenerations(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := vector.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		key        vector.Key
		generation string
		values     []float32
		metadata   string
	}{
		{vector.Key{Source: "pixiv", ID: "123", Page: 0}, "one", []float32{1, 0}, `{"title":"first"}`},
		{vector.Key{Source: "pixiv", ID: "123", Page: 1}, "one", []float32{0, 1}, `{"title":"second"}`},
		{vector.Key{Source: "local", ID: "/gallery/a.png"}, "two", []float32{1, 0}, `{}`},
	} {
		if _, err := store.Upsert(ctx, vector.Asset{Key: item.key, Fingerprint: "same", Metadata: []byte(item.metadata), TargetModel: "model", TargetGeneration: item.generation}); err != nil {
			t.Fatal(err)
		}
		if err := store.PutEmbedding(ctx, item.key, "same", "model", item.generation, item.values); err != nil {
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
	matches, err := store.Search(ctx, "model", "one", []float32{2, 1})
	if err != nil || len(matches) != 2 {
		t.Fatalf("search after reopen: %v %v", matches, err)
	}
	if matches[0].Asset.Key.Page != 0 || matches[1].Asset.Key.Page != 1 || string(matches[1].Asset.Metadata) != `{"title":"second"}` {
		t.Fatalf("page identity/order/metadata lost: %+v", matches)
	}
	if math.Abs(matches[0].Score-2/math.Sqrt(5)) > 1e-6 || math.Abs(matches[1].Score-1/math.Sqrt(5)) > 1e-6 {
		t.Fatalf("expected exact cosine, not dot product: %+v", matches)
	}
}

// TestCollapsePixivPagesKeepsBestPagePerArtwork pins the display-time grouping without a
// CLI process or shell fixture, so it also runs on Windows.
func TestCollapsePixivPagesKeepsBestPagePerArtwork(t *testing.T) {
	matches := []vector.Match{
		{Asset: vector.Asset{Key: vector.Key{Source: "pixiv", ID: "123", Page: 1}, Metadata: []byte(`{"title":"best"}`)}, Score: 0.9},
		{Asset: vector.Asset{Key: vector.Key{Source: "pixiv", ID: "456", Page: 0}}, Score: 0.8},
		{Asset: vector.Asset{Key: vector.Key{Source: "pixiv", ID: "123", Page: 0}, Metadata: []byte(`{"title":"weaker"}`)}, Score: 0.7},
		{Asset: vector.Asset{Key: vector.Key{Source: "local", ID: "123"}}, Score: 0.6},
		{Asset: vector.Asset{Key: vector.Key{Source: "local", ID: "/g/a.png"}}, Score: 0.5},
	}
	got := vector.CollapsePixivPages(matches)
	if len(got) != 4 {
		t.Fatalf("want one result per Pixiv artwork plus every local asset, got %+v", got)
	}
	for i, want := range []vector.Key{
		{Source: "pixiv", ID: "123", Page: 1},
		{Source: "pixiv", ID: "456", Page: 0},
		{Source: "local", ID: "123"},
		{Source: "local", ID: "/g/a.png"},
	} {
		if got[i].Asset.Key != want {
			t.Fatalf("result %d = %+v, want %+v (score order must survive grouping)", i, got[i].Asset.Key, want)
		}
	}
	if string(got[0].Asset.Metadata) != `{"title":"best"}` || got[0].Score != 0.9 {
		t.Fatalf("best page lost its own metadata/score: %+v", got[0])
	}
	if len(vector.CollapsePixivPages(nil)) != 0 {
		t.Fatal("empty input must stay empty")
	}
}
