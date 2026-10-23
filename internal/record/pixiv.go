package record

import (
	"encoding/json"
	"errors"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// RecordFromArtworkDTO 将显式 DTO 映射为管道记录。运行时 SDK 模型必须先在
// 产品包内完成字段级转换，记录层不再通过反射猜测或脱敏运行时字段。
func RecordFromArtworkDTO(artwork pixiv.ArtworkDTO) (Record, error) {
	url := "https://www.pixiv.net/artworks/" + strconv.FormatInt(artwork.ID, 10)
	recordType, err := artworkRecordType(artwork.Kind)
	if err != nil {
		return Record{}, err
	}
	return recordFromDTO(artwork, artwork.ID, recordType, url)
}

func artworkRecordType(kind pixiv.ArtworkKind) (string, error) {
	switch kind {
	case pixiv.ArtworkKindIllustration:
		return "illust", nil
	case pixiv.ArtworkKindManga:
		return "manga", nil
	case pixiv.ArtworkKindUgoira:
		return "ugoira", nil
	default:
		return "", errors.New("unsupported artwork kind for record")
	}
}

// RecordFromNovelDTO 将小说 DTO 映射为管道记录。
func RecordFromNovelDTO(novel pixiv.NovelDTO) (Record, error) {
	url := "https://www.pixiv.net/novel/show.php?id=" + strconv.FormatInt(novel.ID, 10)
	return recordFromDTO(novel, novel.ID, "novel", url)
}

// RecordFromUserPreviewDTO 将用户预览 DTO 映射为管道记录。
// RecordFromNovelContentDTO 将结构化小说正文映射为可继续管道处理的小说记录。
func RecordFromNovelContentDTO(content pixiv.NovelContentDTO) (Record, error) {
	url := "https://www.pixiv.net/novel/show.php?id=" + strconv.FormatInt(content.NovelID, 10)
	return recordFromDTO(content, content.NovelID, "novel", url)
}

func RecordFromUserPreviewDTO(preview pixiv.UserPreviewDTO) (Record, error) {
	id := strconv.FormatInt(preview.User.ID, 10)
	return recordFromDTO(preview, preview.User.ID, "user", "https://www.pixiv.net/users/"+id)
}

// RecordFromUserDetailDTO 将用户详情 DTO 映射为管道记录。
func RecordFromUserDetailDTO(detail pixiv.UserDetailDTO) (Record, error) {
	id := strconv.FormatInt(detail.User.ID, 10)
	return recordFromDTO(detail, detail.User.ID, "user", "https://www.pixiv.net/users/"+id)
}
func recordFromDTO(value any, sourceID int64, recordType, url string) (Record, error) {
	if sourceID <= 0 {
		return Record{}, errors.New("record id must be positive")
	}
	body, err := json.Marshal(value)
	if err != nil {
		return Record{}, err
	}
	fields, err := decodeRecordObject(body)
	if err != nil {
		return Record{}, err
	}
	fields["id"] = strconv.FormatInt(sourceID, 10)
	fields["type"] = recordType
	fields["url"] = url
	return newRecord(fields)
}
