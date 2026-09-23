package vector

import (
	"context"

	index "github.com/FlanChanXwO/pixiv-cli/internal/vector"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// ArtworkObserver records Pixiv artworks that a command already fetched into the private
// vector index. It is a best-effort side effect: it never issues Pixiv requests, never loads
// the embedding model, and every failure is swallowed so the caller's output and exit status
// stay unchanged. Pages that are still unknown to the fetcher remain pending for an explicit
// `pixiv vector sync`.
type ArtworkObserver struct {
	open func() (*index.Store, error)
}

// NewArtworkObserver builds an observer over the given index opener. A nil opener yields an
// inert observer, so callers never need to guard the observation site.
func NewArtworkObserver(open func() (*index.Store, error)) *ArtworkObserver {
	return &ArtworkObserver{open: open}
}

// Observe records the pages the caller already has. It deliberately returns nothing: an
// observation failure must not become a command failure.
func (o *ArtworkObserver) Observe(artworks []pixiv.Artwork) {
	if o == nil || o.open == nil {
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
		return
	}
	defer store.Close()
	// 观察是短小的本地写入，没有调用方可用的取消信号，因此使用后台 context。
	_, _ = index.SyncPixivArtworks(context.Background(), store, observed)
}

// observedArtwork projects one already-fetched artwork onto page-level asset identities.
// It reports false when the artwork carries no usable image identity at all.
func observedArtwork(artwork pixiv.Artwork) (index.PixivArtwork, bool) {
	result := index.PixivArtwork{
		ID:        artwork.ID,
		Title:     artwork.Title,
		UserID:    artwork.User.ID,
		PageCount: artwork.PageCount,
	}
	if artwork.ID <= 0 {
		return index.PixivArtwork{}, false
	}
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
