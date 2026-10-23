// artwork 领域：公开模型映射、资源引用解析，以及分页游标构建。
// wire 与响应结构在 wire.go；公开类型在 models.go。

package pixiv

import (
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// mapArtworkEntity 把 internal artwork 端点（novel/user 尚未并入）的规范化实体
// 转成公开模型；wire 映射与资源注册由 mapArtwork 统一完成。
func (c *Client) mapArtworkEntity(value artwork.Artwork) (Artwork, error) {
	return c.mapArtwork(artworkWireFromEntity(value))
}

func (c *Client) artworkPage(op string, query url.Values, key string, values []artworkWire, nextValue int64, hasNext bool) (sdk.Page[Artwork], error) {
	items := make([]Artwork, len(values))
	for index, value := range values {
		mapped, err := c.mapArtwork(value)
		if err != nil {
			return sdk.Page[Artwork]{}, err
		}
		items[index] = mapped
	}
	next, err := c.buildCursor(op, query, key, nextValue, hasNext)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	return sdk.Page[Artwork]{Items: items, Next: next}, nil
}

// artworkParamsPage 以多参数 continuation 构建 cursor（recommended）。
func (c *Client) artworkParamsPage(op string, query url.Values, values []artworkWire, nextParams url.Values, hasNext bool) (sdk.Page[Artwork], error) {
	items := make([]Artwork, len(values))
	for index, value := range values {
		mapped, err := c.mapArtwork(value)
		if err != nil {
			return sdk.Page[Artwork]{}, err
		}
		items[index] = mapped
	}
	var next sdk.Cursor
	if hasNext {
		built, err := c.buildContinuationCursor(op, query, continuationEnvelope{Params: nextParams})
		if err != nil {
			return sdk.Page[Artwork]{}, err
		}
		next = built
	}
	return sdk.Page[Artwork]{Items: items, Next: next}, nil
}

// commentPage 映射 comment 列表，并把 offset continuation 编码为游标。
func (c *Client) commentPage(op string, query url.Values, values []commentWire, nextOffset int, hasNext bool, total *int64, access *CommentAccessControl) (CommentPage, error) {
	items := make([]Comment, len(values))
	for index, value := range values {
		mapped, err := c.mapComment(value)
		if err != nil {
			return CommentPage{}, err
		}
		items[index] = mapped
	}
	next, err := c.buildCursor(op, query, "offset", int64(nextOffset), hasNext)
	if err != nil {
		return CommentPage{}, err
	}
	page := CommentPage{Page: sdk.Page[Comment]{Items: items, Next: next}}
	if total != nil {
		page.Total = cloneValue(total)
	}
	page.AccessControl = cloneValue(access)
	return page, nil
}
