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
