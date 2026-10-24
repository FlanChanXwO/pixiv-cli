// 共享测试夹具：伪造的 SDK client、资源/作品构造函数与 zip 工具。

package downloader_test

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"sync"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func ugoiraMetadata(zipURL string) pixiv.UgoiraMetadata {
	return pixiv.UgoiraMetadata{
		Archives: []pixiv.UgoiraArchive{{Quality: pixiv.UgoiraQualityOriginal, Resource: testResource(zipURL)}},
		Frames:   []pixiv.UgoiraFrame{{Filename: "000000.jpg", DelayMilliseconds: 80}},
	}
}

func createZip(t *testing.T, path, name string, body []byte) {
	t.Helper()
	if err := os.WriteFile(path, makeZip(t, name, body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func makeZip(t *testing.T, name string, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// testResource 构造一个 Ref 编码了 URL 的资源，使 fake SaveResource 能还原 URL。
func testResource(rawURL string) sdk.Resource {
	ref, err := sdk.NewResourceRef("test", []byte(rawURL))
	if err != nil {
		panic(err)
	}
	return sdk.Resource{URL: rawURL, Ref: ref}
}

func artworkPage(rawURL string, index int) pixiv.ArtworkPage {
	return pixiv.ArtworkPage{PageIndex: index, Image: pixiv.ImageResource{Resource: testResource(rawURL)}}
}

func artworkPageWithArtworkRef(rawURL string) pixiv.ArtworkPage {
	ref, err := sdk.NewResourceRef("pixiv", []byte(`{"k":"artwork","id":42,"p":0}`))
	if err != nil {
		panic(err)
	}
	return pixiv.ArtworkPage{PageIndex: 0, Image: pixiv.ImageResource{Resource: sdk.Resource{URL: rawURL, Ref: ref}}}
}

type fakePixivClient struct {
	details              map[int64]pixiv.Artwork
	ugoira               map[int64]pixiv.UgoiraMetadata
	downloads            map[string][]byte
	downloadErr          error
	savedURLs            []string
	destinations         []string
	saveResourceOverride func(context.Context, sdk.ResourceRef, sdk.SaveOptions) (sdk.SavedResource, error)
}

func (c *fakePixivClient) Artwork(_ context.Context, request pixiv.ArtworkRequest) (pixiv.Artwork, error) {
	artwork, ok := c.details[request.ArtworkID]
	if !ok {
		return pixiv.Artwork{}, os.ErrNotExist
	}
	return artwork, nil
}

func (c *fakePixivClient) UgoiraMetadata(_ context.Context, request pixiv.UgoiraMetadataRequest) (pixiv.UgoiraMetadata, error) {
	meta, ok := c.ugoira[request.ArtworkID]
	if !ok {
		return pixiv.UgoiraMetadata{}, os.ErrNotExist
	}
	return meta, nil
}

func (c *fakePixivClient) ParseResourceRef(string) (sdk.ResourceRef, error) {
	return sdk.ResourceRef{}, nil
}

func (c *fakePixivClient) SaveResource(ctx context.Context, ref sdk.ResourceRef, options sdk.SaveOptions) (sdk.SavedResource, error) {
	if c.saveResourceOverride != nil {
		return c.saveResourceOverride(ctx, ref, options)
	}
	if c.downloadErr != nil {
		return sdk.SavedResource{}, c.downloadErr
	}
	payload, err := sdk.ResourceRefPayload(ref)
	if err != nil {
		return sdk.SavedResource{}, err
	}
	rawURL := string(payload)
	body, ok := c.downloads[rawURL]
	if !ok {
		return sdk.SavedResource{}, os.ErrNotExist
	}
	c.savedURLs = append(c.savedURLs, rawURL)
	c.destinations = append(c.destinations, options.Path)
	if err := os.WriteFile(options.Path, body, 0o644); err != nil {
		return sdk.SavedResource{}, err
	}
	return sdk.SavedResource{Path: options.Path, Size: int64(len(body))}, nil
}

func assertFileBody(t *testing.T, path, want string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != want {
		t.Fatalf("%s body = %q, want %q", path, string(body), want)
	}
}

type directResourceErrorContext struct {
	context.Context
	wantErr   error
	done      chan struct{}
	mu        sync.RWMutex
	triggered bool
}

func (c *directResourceErrorContext) Done() <-chan struct{} {
	return c.done
}

func (c *directResourceErrorContext) Err() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.triggered {
		return c.wantErr
	}
	return c.Context.Err()
}

func (c *directResourceErrorContext) trigger() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.triggered {
		return
	}
	c.triggered = true
	close(c.done)
}

type downloadManagerStub struct {
	download func(context.Context, downloader.DownloadRequest) (downloader.DownloadReport, error)
}

func (m *downloadManagerStub) Download(ctx context.Context, request downloader.DownloadRequest) (downloader.DownloadReport, error) {
	return m.download(ctx, request)
}

type downloadSourcesStub struct {
	userArtworks         func(context.Context, pixiv.UserArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	userArtworkBookmarks func(context.Context, pixiv.UserArtworkBookmarksRequest) (sdk.Page[pixiv.Artwork], error)
	saveResource         func(context.Context, sdk.ResourceRef, sdk.SaveOptions) (sdk.SavedResource, error)
	saveResourceURL      func(context.Context, string, sdk.SaveOptions) (sdk.SavedResource, error)
}

func (c *downloadSourcesStub) UserArtworks(ctx context.Context, request pixiv.UserArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
	if c.userArtworks != nil {
		return c.userArtworks(ctx, request)
	}
	return sdk.Page[pixiv.Artwork]{}, nil
}

func (c *downloadSourcesStub) UserArtworkBookmarks(ctx context.Context, request pixiv.UserArtworkBookmarksRequest) (sdk.Page[pixiv.Artwork], error) {
	if c.userArtworkBookmarks != nil {
		return c.userArtworkBookmarks(ctx, request)
	}
	return sdk.Page[pixiv.Artwork]{}, nil
}

func (c *downloadSourcesStub) SaveResource(ctx context.Context, ref sdk.ResourceRef, options sdk.SaveOptions) (sdk.SavedResource, error) {
	if c.saveResource != nil {
		return c.saveResource(ctx, ref, options)
	}
	return sdk.SavedResource{}, nil
}

func (c *downloadSourcesStub) SaveResourceURL(ctx context.Context, rawURL string, options sdk.SaveOptions) (sdk.SavedResource, error) {
	if c.saveResourceURL != nil {
		return c.saveResourceURL(ctx, rawURL, options)
	}
	return sdk.SavedResource{}, nil
}

func (c *downloadSourcesStub) Artwork(context.Context, pixiv.ArtworkRequest) (pixiv.Artwork, error) {
	panic("downloadSourcesStub.Artwork must not be called by source expansion")
}

func (c *downloadSourcesStub) UgoiraMetadata(context.Context, pixiv.UgoiraMetadataRequest) (pixiv.UgoiraMetadata, error) {
	panic("downloadSourcesStub.UgoiraMetadata must not be called by source expansion")
}

var _ downloader.DownloadTargetClient = (*downloadSourcesStub)(nil)
