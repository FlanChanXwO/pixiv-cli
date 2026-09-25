package vector

import (
	"context"
	"fmt"
	"io"

	index "github.com/FlanChanXwO/pixiv-cli/internal/vector"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// ArtworkObserver records Pixiv artworks that a command already fetched into the private
// vector index. It is a best-effort side effect: it never issues Pixiv requests, never loads
// the embedding model, and every failure only writes one diagnostic line to the given sink
// so the caller's stdout and exit status stay unchanged. Pages that are still unknown to
// the fetcher remain pending for an explicit `pixiv vector sync`.
type ArtworkObserver struct {
	open  func() (*index.Store, error)
	diags io.Writer
}

// NewArtworkObserver builds an observer over the given index opener. A nil opener yields an
// inert observer, so callers never need to guard the observation site. diags receives one
// short diagnostic line per failure; nil silences them without changing the best-effort
// contract.
func NewArtworkObserver(open func() (*index.Store, error), diags io.Writer) *ArtworkObserver {
	return &ArtworkObserver{open: open, diags: diags}
}

// Observe records the pages the caller already has. It deliberately returns nothing: an
// observation failure must not become a command failure. Cancelling ctx stops this
// observation's local writes without affecting the caller.
func (o *ArtworkObserver) Observe(ctx context.Context, artworks []pixiv.Artwork) {
	if o == nil || o.open == nil {
		return
	}
	if err := ctx.Err(); err != nil {
		return
	}
	observed := make([]index.PixivArtwork, 0, len(artworks))
	for _, artwork := range artworks {
		converted, ok := observedArtwork(artwork)
		if ok {
			observed = append(observed, converted)
		}
	}
	// 没有可记录身份时不开库，避免只读命令因观察而产生空的 vector.db。
	if len(observed) == 0 {
		return
	}
	store, err := o.open()
	if err != nil {
		o.diagnostic("vector observer unavailable: %v", err)
		return
	}
	defer func() {
		if err := store.Close(); err != nil {
			o.diagnostic("vector observer close failed: %v", err)
		}
	}()
	if _, err := index.SyncPixivArtworks(ctx, store, observed); err != nil {
		o.diagnostic("vector observer persist failed: %v", err)
	}
}

// diagnostic 输出一行可诊断信息到诊断 sink；错误里不含 token、签名 URL 或本地图片路径。
func (o *ArtworkObserver) diagnostic(format string, args ...any) {
	if o.diags == nil {
		return
	}
	fmt.Fprintf(o.diags, format+"\n", args...)
}

// ObserveWithContext 适配命令层端口：把调用方的 ctx 一并传入观察。观察仍不返回错误。
func (o *ArtworkObserver) ObserveWithContext(ctx context.Context, artworks []pixiv.Artwork) {
	o.Observe(ctx, artworks)
}

// observedArtwork projects one already-fetched artwork onto page-level asset identities.
// It reports false when the artwork carries no usable image identity at all.
func observedArtwork(artwork pixiv.Artwork) (index.PixivArtwork, bool) {
	result := index.PixivArtwork{
		ID:        artwork.ID,
		Title:     artwork.Title,
		Caption:   artwork.Caption,
		UserID:    artwork.User.ID,
		UserName:  artwork.User.Name,
		PageCount: artwork.PageCount,
		Kind:      string(artwork.Kind),
		XRestrict: artwork.XRestrict,
		AIType:    artwork.AIType,
	}
	if artwork.ID <= 0 {
		return index.PixivArtwork{}, false
	}
	tags := make([]string, 0, len(artwork.Tags))
	for _, tag := range artwork.Tags {
		tags = append(tags, tag.Name)
	}
	result.Tags = tags
	// detail 已取得全部页面：每页建立独立 Asset。
	pages := make([]index.PixivPage, 0, len(artwork.Pages))
	for _, page := range artwork.Pages {
		pages = append(pages, index.PixivPage{Index: page.PageIndex, Ref: page.Image.Resource.Ref.String()})
	}
	if len(pages) > 0 {
		result.Pages = pages
		return result, true
	}
	// listing 只有 cover：其余页面需要 artwork detail，观察不请求它。
	if artwork.Cover.Resource.Ref.IsZero() {
		return index.PixivArtwork{}, false
	}
	result.CoverRef = artwork.Cover.Resource.Ref.String()
	return result, true
}
