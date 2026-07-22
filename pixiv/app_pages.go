package pixiv

import (
	"net/url"
	"path"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/model"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/protocol"
)

// appDetailMetaPages 将 App detail 的单页字段规范化为现有的 pages 公开形状。
func appDetailMetaPages(illust model.Illust) ([]MetaPage, error) {
	pages := mapMetaPages(illust.MetaPages)
	if illust.PageCount <= 0 {
		return pages, nil
	}
	if illust.PageCount > 1 {
		if len(pages) != illust.PageCount {
			return nil, protocol.MalformedResponse()
		}
		return pages, nil
	}
	if len(pages) == 1 {
		return pages, nil
	}
	if len(pages) != 0 {
		return nil, protocol.MalformedResponse()
	}
	original := illust.MetaSinglePage.OriginalImageURL
	if original == "" {
		original = illust.ImageURLs.Original
	}
	if original == "" {
		return nil, protocol.MalformedResponse()
	}
	return []MetaPage{{
		PageIndex: 0,
		Width:     illust.Width,
		Height:    illust.Height,
		Extension: imageExtension(original),
		ImageURLs: ImageURLs{
			SquareMedium: illust.ImageURLs.SquareMedium,
			Medium:       illust.ImageURLs.Medium,
			Large:        illust.ImageURLs.Large,
			Original:     original,
		},
	}}, nil
}

func imageExtension(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(path.Ext(parsed.Path), ".")
}
