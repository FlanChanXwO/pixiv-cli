package vector_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
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
}

func (f *fakeSource) UserID() int64 { return f.userID }

func (f *fakeSource) UserArtworkBookmarks(_ context.Context, _ pixiv.UserArtworkBookmarksRequest) (sdk.Page[pixiv.Artwork], error) {
	page, ok := 0, f.requests < len(f.pages)
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
		})
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
		})
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
		func(context.Context) (vector.ImageEncoder, error) { return nil, nil }, nil)
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
		func(context.Context) (vector.ImageEncoder, error) { return nil, nil }, nil)
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
