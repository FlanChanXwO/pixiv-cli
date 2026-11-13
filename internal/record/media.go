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
)

type mediaImageQuality string

const (
	mediaOriginalQuality mediaImageQuality = "original"
	mediaCoverQuality    mediaImageQuality = "cover"
)

// RecordFromMediaArtwork 保留画廊元数据的记录字段，只把图片引用投影为可读的对象身份。
// 定位 URL 与账号绑定留在 SDK 内；调用方用作品 ID 和页号调用媒体工具。
func RecordFromMediaArtwork(artwork *pixiv.Artwork) (Record, error) {
	value, err := RecordFromSDKArtwork(artwork, true)
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
	if pages, ok := value.fields[mediaPagesField].([]any); ok {
		for _, page := range pages {
			image := page.(map[string]any)
			imageReference(image[mediaImageField], map[string]any{mediaArtworkIDField: artwork.ID, mediaPageIndexField: image[mediaPageIndexField], mediaQualityField: mediaOriginalQuality})
		}
	}
	return value, nil
}
