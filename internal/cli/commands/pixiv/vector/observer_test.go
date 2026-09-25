package vector_test

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/vector"
	index "github.com/FlanChanXwO/pixiv-cli/internal/vector"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// artworkRef 构造一个真实的资源引用；page 为 -1 表示 listing 的 cover 身份。
func artworkRef(t *testing.T, artworkID int64, page int) sdk.Resource {
	t.Helper()
	payload := `{"kind":"artwork","id":` + strconv.FormatInt(artworkID, 10) + `,"page":` + strconv.Itoa(page) + `}`
	ref, err := sdk.NewResourceRef("pixiv", []byte(payload))
	if err != nil {
		t.Fatal(err)
	}
	return sdk.Resource{Ref: ref, URL: "https://i.example/signed.jpg"}
}

// TestArtworkObserverRecordsAlreadyFetchedPagesIdempotently 锁定 Task 14 的观察端口：
// 只消费调用方已经取得的 Artwork，逐页记录 Asset，且重复观察不产生重复工作。
func TestArtworkObserverRecordsAlreadyFetchedPagesIdempotently(t *testing.T) {
	dir := t.TempDir()
	observer := vector.NewArtworkObserver(func() (*index.Store, error) { return index.Open(dir) }, io.Discard)
	detail := pixiv.Artwork{
		ID: 900, Title: "detail pages", PageCount: 2,
		Pages: []pixiv.ArtworkPage{
			{PageIndex: 0, Image: pixiv.ImageResource{Resource: artworkRef(t, 900, 0)}},
			{PageIndex: 1, Image: pixiv.ImageResource{Resource: artworkRef(t, 900, 1)}},
		},
	}
	listing := pixiv.Artwork{
		ID: 901, Title: "listing only", PageCount: 3,
		Cover: pixiv.ImageResource{Resource: artworkRef(t, 901, -1)},
	}
	observer.Observe(context.Background(), []pixiv.Artwork{detail, listing})

	store, err := index.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	assets, embeddings, err := store.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// 详情作品的 2 页 + listing 作品的 cover = 3 个 page-level Asset；观察不嵌入。
	if assets != 3 || embeddings != 0 {
		t.Fatalf("assets/embeddings = %d/%d, want 3/0", assets, embeddings)
	}
	asset, err := store.Get(context.Background(), index.Key{Source: "pixiv", ID: "901", Page: 0})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(asset.Metadata), `"cover_only":true`) {
		t.Fatalf("listing-only multi-page artwork must stay cover_only: %s", asset.Metadata)
	}
	// 重复观察同一批作品不得新增 Asset。
	observer.Observe(context.Background(), []pixiv.Artwork{detail, listing})
	assets, _, err = store.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if assets != 3 {
		t.Fatalf("repeat observation duplicated assets: %d", assets)
	}
}

// TestArtworkObserverDiagnosticsIsolatedToSink 锁定 Workstream E：观察失败写一行诊断到
// 指定 sink，而不是完全吞掉；诊断不进入 stdout，也不合 token/路径。
func TestArtworkObserverDiagnosticsIsolatedToSink(t *testing.T) {
	var diags strings.Builder
	observer := vector.NewArtworkObserver(func() (*index.Store, error) { return nil, errors.New("index unavailable") }, &diags)
	observer.Observe(context.Background(), []pixiv.Artwork{{ID: 1, PageCount: 1, Cover: pixiv.ImageResource{Resource: artworkRef(t, 1, -1)}}})
	if !strings.Contains(diags.String(), "vector observer unavailable") {
		t.Fatalf("missing diagnostic: %q", diags.String())
	}
}

// TestArtworkObserverStopOnCancelledContext 锁定 ctx 语义：调用方取消后观察不再写库。
func TestArtworkObserverStopOnCancelledContext(t *testing.T) {
	dir := t.TempDir()
	observer := vector.NewArtworkObserver(func() (*index.Store, error) { return index.Open(dir) }, io.Discard)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	observer.Observe(ctx, []pixiv.Artwork{{ID: 900, PageCount: 1, Cover: pixiv.ImageResource{Resource: artworkRef(t, 900, -1)}}})
	store, err := index.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	assets, _, err := store.Status(context.Background())
	if err != nil || assets != 0 {
		t.Fatalf("cancelled observation wrote %d assets (err=%v), want 0", assets, err)
	}
}

// TestArtworkObserverFailureIsIsolated 锁定 best-effort 契约：索引失败不得影响调用方，
// 也不得 panic，因为观察是普通 Pixiv 命令的副作用。
func TestArtworkObserverFailureIsIsolated(t *testing.T) {
	failing := vector.NewArtworkObserver(func() (*index.Store, error) { return nil, errors.New("index unavailable") }, io.Discard)
	failing.Observe(context.Background(), []pixiv.Artwork{{ID: 1, PageCount: 1, Cover: pixiv.ImageResource{Resource: artworkRef(t, 1, -1)}}})
	unconfigured := vector.NewArtworkObserver(nil, nil)
	unconfigured.Observe(context.Background(), []pixiv.Artwork{{ID: 1, PageCount: 1, Cover: pixiv.ImageResource{Resource: artworkRef(t, 1, -1)}}})
}

// TestArtworkObserverRecordsNothingWithoutImageIdentity 保证缺少图片身份的作品
// 不写入假 Asset，也不触发取图或推理。
func TestArtworkObserverRecordsNothingWithoutImageIdentity(t *testing.T) {
	dir := t.TempDir()
	observer := vector.NewArtworkObserver(func() (*index.Store, error) { return index.Open(dir) }, io.Discard)
	observer.Observe(context.Background(), []pixiv.Artwork{{ID: 5, PageCount: 2}})
	store, err := index.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	assets, embeddings, err := store.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if assets != 0 || embeddings != 0 {
		t.Fatalf("artwork without image identity must record nothing: %d/%d", assets, embeddings)
	}
}
