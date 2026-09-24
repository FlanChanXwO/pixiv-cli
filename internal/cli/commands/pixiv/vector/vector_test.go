package vector_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	commands "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/vector"
	index "github.com/FlanChanXwO/pixiv-cli/internal/vector"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

// fakeEncoder 是纯内存 ImageEncoder 替身：命令装配无需 Python 或 POSIX shell。
type fakeEncoder struct {
	images []string
	closed bool
}

func (f *fakeEncoder) Image(_ context.Context, path string) ([]float32, error) {
	f.images = append(f.images, path)
	return []float32{1, 0, 0}, nil
}

func (f *fakeEncoder) Text(context.Context, string) ([]float32, error) {
	return []float32{0, 1, 0}, nil
}

func (f *fakeEncoder) Close() error { f.closed = true; return nil }

// fakeSource 只实现 BookmarkSource 的三个方法，因此结构上不可能发起 artwork detail。
type fakeSource struct {
	userID int64
	pages  [][]pixiv.Artwork
	// requests 记录 listing 调用次数（含空页）；dataPages 只统计返回了数据的调用。
	requests  int
	dataPages int
	saved     []sdk.ResourceRef
	saveFail  bool
	// listErrOn 非负时，第 listErrOn 次请求返回 listErr，模拟持久化前的可重放失败。
	listErrOn int
	listErr   error
}

func (f *fakeSource) UserID() int64 { return f.userID }

func (f *fakeSource) UserArtworkBookmarks(_ context.Context, _ pixiv.UserArtworkBookmarksRequest) (sdk.Page[pixiv.Artwork], error) {
	if f.listErr != nil && f.requests == f.listErrOn {
		f.requests++
		return sdk.Page[pixiv.Artwork]{}, f.listErr
	}
	// 按请求序号顺序消费预置页：第一个 restrict 取 pages[0]，第二个取 pages[1]……
	page, ok := f.requests, f.requests < len(f.pages)
	f.requests++
	if !ok {
		return sdk.Page[pixiv.Artwork]{}, nil
	}
	f.dataPages++
	return sdk.Page[pixiv.Artwork]{Items: f.pages[page]}, nil
}

func (f *fakeSource) SaveResource(_ context.Context, ref sdk.ResourceRef, options sdk.SaveOptions) (sdk.SavedResource, error) {
	f.saved = append(f.saved, ref)
	if f.saveFail {
		return sdk.SavedResource{}, errors.New("cover download failed")
	}
	if err := os.WriteFile(options.Path, []byte("image"), 0o600); err != nil {
		return sdk.SavedResource{}, err
	}
	return sdk.SavedResource{Path: options.Path, Size: 5}, nil
}

func coverArtwork(t *testing.T, id int64, title string, pages int) pixiv.Artwork {
	t.Helper()
	ref, err := sdk.NewResourceRef("pixiv", []byte(`{"kind":"artwork","id":1,"page":-1,"variant":"large"}`))
	if err != nil {
		t.Fatal(err)
	}
	return pixiv.Artwork{
		ID: id, Title: title, PageCount: pages,
		User:  pixiv.User{ID: 7, Name: "artist"},
		Cover: pixiv.ImageResource{Resource: sdk.Resource{Ref: ref, URL: "https://i.example/signed.jpg"}},
	}
}

// runBookmarkSync 执行真实的 vector 命令树，仅替换模型运行时与 Pixiv 读取面。
func runBookmarkSync(t *testing.T, source *fakeSource, encoder *fakeEncoder) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd := vector.New(&out,
		func() (*index.Store, error) { return index.Open(t.TempDir()) },
		func(context.Context) (vector.ImageEncoder, error) { return encoder, nil },
		func(ctx context.Context, attempt func(context.Context, vector.BookmarkSource) (bool, error)) error {
			_, err := attempt(ctx, source)
			return err
		}, nil)
	cmd.SetArgs([]string{"sync", "bookmarks"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.ExecuteContext(context.Background())
	return out.String(), err
}

// TestSyncBookmarksIndexesListingCoversWithoutArtworkDetail 锁定 Task 13 契约：
// 只用 listing 已取得的字段建立 page 0 Asset，多页作品显式标记 cover_only，
// 且整条路径不依赖 POSIX shell。
func TestSyncBookmarksIndexesListingCoversWithoutArtworkDetail(t *testing.T) {
	source := &fakeSource{userID: 7, pages: [][]pixiv.Artwork{{
		coverArtwork(t, 101, "multi", 3),
		coverArtwork(t, 202, "single", 1),
	}}}
	encoder := &fakeEncoder{}
	out, err := runBookmarkSync(t, source, encoder)
	if err != nil {
		t.Fatalf("sync bookmarks: %v\n%s", err, out)
	}
	if !strings.Contains(out, "scanned: 2\n") || !strings.Contains(out, "embedded: 2\n") {
		t.Fatalf("unexpected summary: %q", out)
	}
	if len(source.saved) != 2 || len(encoder.images) != 2 {
		t.Fatalf("covers saved=%d embedded=%d, want 2 each", len(source.saved), len(encoder.images))
	}
	// 两个 restrict（public/private）各遍历一次，未发生任何 detail 请求。
	if source.requests != 2 || source.dataPages != 1 {
		t.Fatalf("listing requests = %d (data pages %d), want the public/private passes", source.requests, source.dataPages)
	}
	if !encoder.closed {
		t.Fatal("encoder must be closed once the command finishes")
	}
	want := "scanned: 2\nchanged: 2\nskipped: 0\nembedded: 2\n"
	if out != want {
		t.Fatalf("summary = %q, want %q", out, want)
	}
}

// TestSyncBookmarksMarksMultiPageArtworkAsCoverOnly 证明多页作品不会被伪造成完整页集。
func TestSyncBookmarksMarksMultiPageArtworkAsCoverOnly(t *testing.T) {
	source := &fakeSource{userID: 7, pages: [][]pixiv.Artwork{{
		coverArtwork(t, 101, "multi", 3),
	}}}
	encoder := &fakeEncoder{}
	// 用同一个 store 观察落盘结果：这里直接复用命令内部的 store 语义。
	var out bytes.Buffer
	dir := t.TempDir()
	cmd := vector.New(&out,
		func() (*index.Store, error) { return index.Open(dir) },
		func(context.Context) (vector.ImageEncoder, error) { return encoder, nil },
		func(ctx context.Context, attempt func(context.Context, vector.BookmarkSource) (bool, error)) error {
			_, err := attempt(ctx, source)
			return err
		}, nil)
	cmd.SetArgs([]string{"sync", "bookmarks"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("sync bookmarks: %v\n%s", err, out.String())
	}
	store, err := index.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	asset, err := store.Get(context.Background(), index.Key{Source: "pixiv", ID: "101", Page: 0})
	if err != nil {
		t.Fatal(err)
	}
	var metadata struct {
		PageCount int  `json:"page_count"`
		CoverOnly bool `json:"cover_only"`
	}
	if err := json.Unmarshal(asset.Metadata, &metadata); err != nil {
		t.Fatalf("metadata %s: %v", asset.Metadata, err)
	}
	if metadata.PageCount != 3 || !metadata.CoverOnly {
		t.Fatalf("multi-page artwork must be cover_only with its real page count: %s", asset.Metadata)
	}
	// 除 cover 外不得出现其它 page 的 Asset。
	if _, err := store.Get(context.Background(), index.Key{Source: "pixiv", ID: "101", Page: 1}); err == nil {
		t.Fatal("listing sync must not fabricate page 1")
	}
}

// TestSyncBookmarksKeepsFailedCoverPending 证明单张 cover 失败不会丢已完成的向量。
func TestSyncBookmarksKeepsFailedCoverPending(t *testing.T) {
	source := &fakeSource{userID: 7, saveFail: true, pages: [][]pixiv.Artwork{{
		coverArtwork(t, 101, "multi", 1),
	}}}
	encoder := &fakeEncoder{}
	out, err := runBookmarkSync(t, source, encoder)
	if err == nil {
		t.Fatalf("a failed cover must surface an error: %q", out)
	}
	if !strings.Contains(out, "changed: 1\n") {
		t.Fatalf("asset must stay recorded for retry: %q", out)
	}
	if len(encoder.images) != 0 {
		t.Fatalf("no image should be embedded after a failed cover: %v", encoder.images)
	}
}

// TestSyncBookmarksRejectsMissingAccount 证明没有可用账号身份时命令明确失败。
func TestSyncBookmarksRejectsMissingAccount(t *testing.T) {
	out, err := runBookmarkSync(t, &fakeSource{userID: 0}, &fakeEncoder{})
	if err == nil || !strings.Contains(err.Error(), "current user ID") {
		t.Fatalf("missing account identity must fail: err=%v out=%q", err, out)
	}
}

// TestVectorCommandLifecycleKeepsLocalLeavesCredentialFree 锁定文档声明的边界：
// 只有 sync bookmarks 进入普通 Pixiv 数据命令的启动/config 生命周期，本地命令不需要凭证。
func TestVectorCommandLifecycleKeepsLocalLeavesCredentialFree(t *testing.T) {
	cmd := vector.New(io.Discard, func() (*index.Store, error) { return nil, nil },
		func(context.Context) (vector.ImageEncoder, error) { return nil, nil }, nil, nil)
	wanted := map[string]bool{"bookmarks": true, "local": false, "status": false, "search": false, "rebuild": false}
	seen := make(map[string]bool)
	for _, top := range cmd.Commands() {
		leaves := []*cobra.Command{top}
		if top.Name() == "sync" {
			leaves = top.Commands()
		}
		for _, leaf := range leaves {
			want, ok := wanted[leaf.Name()]
			if !ok {
				continue
			}
			seen[leaf.Name()] = true
			if got := commands.For(leaf).EnsureConfig; got != want {
				t.Fatalf("%s EnsureConfig = %v, want %v", leaf.Name(), got, want)
			}
		}
	}
	for name := range wanted {
		if !seen[name] {
			t.Fatalf("vector command %q was not found", name)
		}
	}
}

// TestVectorCommandSurfaceStaysInsideV1NonGoals 锁定原方案 §26 的非目标：
// v1 不提供 seed crawler，也不把 vector 扩展成额外子命令面（无 daemon/watcher 入口）。
func TestVectorCommandSurfaceStaysInsideV1NonGoals(t *testing.T) {
	cmd := vector.New(io.Discard, func() (*index.Store, error) { return nil, nil },
		func(context.Context) (vector.ImageEncoder, error) { return nil, nil }, nil, nil)
	want := map[string][]string{
		"sync":    {"bookmarks", "local"}, // cobra sorts children by name
		"search":  nil,
		"status":  nil,
		"rebuild": nil,
	}
	got := make(map[string][]string, len(cmd.Commands()))
	for _, child := range cmd.Commands() {
		names := make([]string, 0, len(child.Commands()))
		for _, leaf := range child.Commands() {
			names = append(names, leaf.Name())
		}
		got[child.Name()] = names
	}
	if len(got) != len(want) {
		t.Fatalf("vector surface = %v, want exactly %v (no seed/daemon extras)", got, want)
	}
	for name, children := range want {
		actual, ok := got[name]
		if !ok {
			t.Fatalf("missing vector subcommand %q", name)
		}
		if len(actual) != len(children) {
			t.Fatalf("%s children = %v, want %v", name, actual, children)
		}
		for i := range children {
			if actual[i] != children[i] {
				t.Fatalf("%s children = %v, want %v", name, actual, children)
			}
		}
	}
}

func TestSyncBookmarksProcessesObservedPagesAfterRestart(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	open := func() (*index.Store, error) { return index.Open(dir) }
	observer := vector.NewArtworkObserver(open, io.Discard)
	pages := []pixiv.ArtworkPage{
		{PageIndex: 0, Image: pixiv.ImageResource{Resource: artworkRef(t, 900, 0)}},
		{PageIndex: 1, Image: pixiv.ImageResource{Resource: artworkRef(t, 900, 1)}},
		{PageIndex: 2, Image: pixiv.ImageResource{Resource: artworkRef(t, 900, 2)}},
	}
	observer.Observe(ctx, []pixiv.Artwork{{ID: 900, PageCount: 3, Pages: pages}, {ID: 901, PageCount: 1, Cover: pixiv.ImageResource{Resource: artworkRef(t, 901, -1)}}})
	// The observer has closed its store; the next invocation only has persisted identities.
	source := &fakeSource{userID: 7}
	encoder := &fakeEncoder{}
	var out bytes.Buffer
	cmd := vector.New(&out, open, func(context.Context) (vector.ImageEncoder, error) { return encoder, nil },
		func(ctx context.Context, attempt func(context.Context, vector.BookmarkSource) (bool, error)) error {
			_, err := attempt(ctx, source)
			return err
		}, nil)
	cmd.SetArgs([]string{"sync", "bookmarks"})
	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatal(err)
	}
	if len(source.saved) != 4 {
		t.Fatalf("downloaded %d persisted pages, want 4; output=%s", len(source.saved), out.String())
	}
	for i, page := range pages {
		if source.saved[i] != page.Image.Resource.Ref {
			t.Fatalf("page %d reference lost", i)
		}
	}
	store, err := open()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	pending, err := store.Pending(ctx, index.ModelID, index.Generation)
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending=%v err=%v", pending, err)
	}
	matches, err := store.Search(ctx, index.ModelID, index.Generation, []float32{1, 0, 0})
	if err != nil || len(matches) != 4 {
		t.Fatalf("search results=%d err=%v", len(matches), err)
	}
	for _, path := range encoder.images {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("temporary image remains: %v", err)
		}
	}
}

// fakePixivPort 是 rebuild 的 Pixiv 资源端口替身，记录是否被进入以及每次取图。
type fakePixivPort struct {
	entered int
	saver   *fakeSource
}

func (f *fakePixivPort) port(ctx context.Context, attempt func(context.Context, vector.ResourceSaver) error) error {
	f.entered++
	return attempt(ctx, f.saver)
}

// runRebuild 执行 rebuild，可注入 Pixiv 端口。
func runRebuild(t *testing.T, dir string, encoder *fakeEncoder, pixivPort func(context.Context, func(context.Context, vector.ResourceSaver) error) error) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd := vector.New(&out, func() (*index.Store, error) { return index.Open(dir) },
		func(context.Context) (vector.ImageEncoder, error) { return encoder, nil }, nil, pixivPort)
	cmd.SetArgs([]string{"rebuild"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.ExecuteContext(context.Background())
	return out.String(), err
}

// TestRebuildMigratesLocalAndPixivTogether 锁定 Workstream C：rebuild 迁移全部
// Asset 来源——本地文件与持久化 ResourceRef 的 Pixiv 页面（含 page>0）。
func TestRebuildMigratesLocalAndPixivTogether(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	gallery := t.TempDir()
	if err := os.WriteFile(filepath.Join(gallery, "a.png"), []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := index.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := index.SyncLocal(ctx, store, gallery); err != nil {
		t.Fatal(err)
	}
	// 一个 observer 风格的多页 Pixiv Asset：身份持久化，向量缺失。
	if _, err := index.SyncPixivArtworks(ctx, store, []index.PixivArtwork{{
		ID: 900, PageCount: 3,
		Pages: []index.PixivPage{
			{Index: 0, Ref: artworkRef(t, 900, 0).Ref.String()},
			{Index: 1, Ref: artworkRef(t, 900, 1).Ref.String()},
			{Index: 2, Ref: artworkRef(t, 900, 2).Ref.String()},
		},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	pixivPort := &fakePixivPort{saver: &fakeSource{userID: 7}}
	encoder := &fakeEncoder{}
	out, err := runRebuild(t, dir, encoder, pixivPort.port)
	if err != nil {
		t.Fatalf("rebuild: %v\n%s", err, out)
	}
	if !strings.Contains(out, "embedded: 4\n") {
		t.Fatalf("summary = %q, want 4 embeddings (1 local + 3 pixiv pages)", out)
	}
	if pixivPort.entered == 0 {
		t.Fatal("pixiv port must be entered when pixiv assets exist")
	}
	if len(pixivPort.saver.saved) != 3 {
		t.Fatalf("pixiv pages fetched = %d, want 3 (all pages incl. page>0)", len(pixivPort.saver.saved))
	}
	store, err = index.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	pending, err := store.Pending(ctx, index.ModelID, index.Generation)
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending after rebuild = %v (err=%v), want none", pending, err)
	}
	matches, err := store.Search(ctx, index.ModelID, index.Generation, []float32{1, 0, 0})
	if err != nil || len(matches) != 4 {
		t.Fatalf("search after rebuild = %d (err=%v), want 4", len(matches), err)
	}
}

// TestRebuildKeepsOldGenerationOnPixivFailure 锁定：Pixiv 页面 rebuild 失败时，
// 已完成的本地迁移保留、旧 generation 向量保留、失败页可重试、命令非零退出。
func TestRebuildKeepsOldGenerationOnPixivFailure(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := index.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := index.SyncPixivArtworks(ctx, store, []index.PixivArtwork{{
		ID: 900, PageCount: 2,
		Pages: []index.PixivPage{{Index: 0, Ref: artworkRef(t, 900, 0).Ref.String()}, {Index: 1, Ref: artworkRef(t, 900, 1).Ref.String()}},
	}}); err != nil {
		t.Fatal(err)
	}
	// 旧 generation 已有向量：rebuild 失败后必须保留。
	if err := store.PutEmbedding(ctx, index.Key{Source: "pixiv", ID: "900", Page: 0}, storeFingerprint(t, store, index.Key{Source: "pixiv", ID: "900", Page: 0}), "old-model", "old-gen", []float32{1, 0}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	pixivPort := &fakePixivPort{saver: &fakeSource{userID: 7, saveFail: true}}
	out, err := runRebuild(t, dir, &fakeEncoder{}, pixivPort.port)
	if err == nil {
		t.Fatalf("failed pixiv rebuild must exit non-zero: %q", out)
	}
	store, err = index.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.Embedding(ctx, index.Key{Source: "pixiv", ID: "900", Page: 0}, "old-model", "old-gen"); err != nil {
		t.Fatalf("old generation vector lost on failed rebuild: %v", err)
	}
	pending, err := store.Pending(ctx, index.ModelID, index.Generation)
	if err != nil || len(pending) != 2 {
		t.Fatalf("failed pages must stay retryable: %v (err=%v)", pending, err)
	}
}

// TestRebuildLocalOnlyNeverEntersPixivPort 锁定：索引只有本地 Asset 时，
// rebuild 不初始化 Pixiv 账号端口。
func TestRebuildLocalOnlyNeverEntersPixivPort(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	gallery := t.TempDir()
	if err := os.WriteFile(filepath.Join(gallery, "a.png"), []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := index.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := index.SyncLocal(ctx, store, gallery); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	pixivPort := &fakePixivPort{saver: &fakeSource{userID: 7}}
	out, err := runRebuild(t, dir, &fakeEncoder{}, pixivPort.port)
	if err != nil {
		t.Fatalf("rebuild: %v\n%s", err, out)
	}
	if pixivPort.entered != 0 {
		t.Fatalf("local-only rebuild entered the pixiv port %d times", pixivPort.entered)
	}
}

// storeFingerprint 读取一个已落库 Asset 的指纹。
func storeFingerprint(t *testing.T, store *index.Store, key index.Key) string {
	t.Helper()
	asset, err := store.Get(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	return asset.Fingerprint
}

// TestSyncBookmarksReplayStatsReflectFinalAttempt 锁定二次审查修复：账号池重放时，
// 统计只代表最终 committed attempt，失败 attempt 的 scanned/skipped 不重复累计。
func TestSyncBookmarksReplayStatsReflectFinalAttempt(t *testing.T) {
	// attempt 1: public listing 成功（scanned 2）后 private listing 失败——持久化之前，
	// 账号池会安全重放。attempt 2: public 2 项 + private 1 项，成功提交。
	firstSource := &fakeSource{userID: 7, pages: [][]pixiv.Artwork{
		{coverArtwork(t, 101, "a", 1), coverArtwork(t, 202, "b", 1)},
	}, listErrOn: 1, listErr: errors.New("account replay required")}
	secondSource := &fakeSource{userID: 7, pages: [][]pixiv.Artwork{
		{coverArtwork(t, 101, "a", 1), coverArtwork(t, 202, "b", 1)},
		{coverArtwork(t, 303, "c", 1)},
	}}
	var out bytes.Buffer
	cmd := vector.New(&out,
		func() (*index.Store, error) { return index.Open(t.TempDir()) },
		func(context.Context) (vector.ImageEncoder, error) { return &fakeEncoder{}, nil },
		func(ctx context.Context, attempt func(context.Context, vector.BookmarkSource) (bool, error)) error {
			if _, err := attempt(ctx, firstSource); err == nil {
				return errors.New("first attempt must fail to trigger replay")
			}
			_, err := attempt(ctx, secondSource)
			return err
		}, nil)
	cmd.SetArgs([]string{"sync", "bookmarks"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	// 最终 attempt 只有 3 项 listing；失败 attempt 的 2 项不得重复累计（bug 时为 5）。
	if !strings.Contains(out.String(), "scanned: 3\n") {
		t.Fatalf("stats must reflect only the final attempt: %q", out.String())
	}
}
