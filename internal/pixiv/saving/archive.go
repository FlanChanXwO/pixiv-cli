package saving

import "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"

// SelectUgoiraArchive 优先原质量，未提供时选用上游第一个可用归档。
func SelectUgoiraArchive(meta pixiv.UgoiraMetadata) *pixiv.UgoiraArchive {
	var fallback *pixiv.UgoiraArchive
	for i := range meta.Archives {
		archive := &meta.Archives[i]
		if archive.Quality == pixiv.UgoiraQualityOriginal {
			return archive
		}
		if fallback == nil {
			fallback = archive
		}
	}
	return fallback
}
