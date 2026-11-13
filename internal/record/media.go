package record

import "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"

const (
	mediaResourceField  = "resource"
	mediaReferenceField = "ref"
	mediaVariantField   = "variant"
	mediaCoverField     = "cover"
	mediaUserField      = "user"
	mediaProfileField   = "profile_image"
	mediaPagesField     = "pages"
	mediaImageField     = "image"
	mediaArtworkIDField = "artwork_id"
	mediaPageIndexField = "page_index"
	mediaQualityField   = "quality"
	mediaUserIDField    = "user_id"
	mediaWidthField     = "width"
	mediaHeightField    = "height"
)

type mediaImageQuality string

const (
	mediaOriginalQuality mediaImageQuality = "original"
	mediaCoverQuality    mediaImageQuality = "cover"
)

// RecordFromMediaArtwork 保留画廊元数据的记录字段，只把图片引用投影为可读的对象身份。
// originalPages 来自可信元数据中的原图角色，缓存不依赖 SDK 的运行时标记。
// 定位 URL 与账号绑定留在 SDK 内；调用方用作品 ID 和页号调用媒体工具。
func RecordFromMediaArtwork(artwork *pixiv.Artwork, originalPages []int) (Record, error) {
	value, err := RecordFromSDKArtwork(artwork, false)
	if err != nil {
		return Record{}, err
	}
	imageReference := func(value any, identity map[string]any) {
		if value == nil {
			return
		}
		image := value.(map[string]any)
		if image[mediaResourceField] != nil {
			image[mediaResourceField] = map[string]any{mediaReferenceField: identity}
		}
		delete(image, mediaVariantField)
	}
	imageReference(value.fields[mediaCoverField], map[string]any{mediaArtworkIDField: artwork.ID, mediaQualityField: mediaCoverQuality})
	user := value.fields[mediaUserField].(map[string]any)
	imageReference(user[mediaProfileField], map[string]any{mediaUserIDField: artwork.Author.ID})
	if len(originalPages) > 0 {
		pages := make([]any, 0, len(originalPages))
		for _, index := range originalPages {
			page := artwork.Pages[index]
			pages = append(pages, map[string]any{
				mediaPageIndexField: index, mediaWidthField: page.Width, mediaHeightField: page.Height,
				mediaImageField: map[string]any{
					mediaWidthField: page.Original.Width, mediaHeightField: page.Original.Height,
					mediaResourceField: map[string]any{mediaReferenceField: map[string]any{
						mediaArtworkIDField: artwork.ID, mediaPageIndexField: index, mediaQualityField: mediaOriginalQuality,
					}},
				},
			})
		}
		value.fields[mediaPagesField] = pages
	}
	return value, nil
}
