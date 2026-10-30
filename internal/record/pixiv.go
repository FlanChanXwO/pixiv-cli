package record

import (
	"encoding/json"
	"errors"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// RecordFromArtwork 将 public artwork 映射为管道记录。
func RecordFromArtwork(artwork pixiv.Artwork) (Record, error) {
	url := "https://www.pixiv.net/artworks/" + strconv.FormatInt(artwork.ID, 10)
	return recordFromValue(artwork, artwork.ID, "artwork", url)
}

// RecordFromNovel 将 public novel 映射为管道记录。
func RecordFromNovel(novel pixiv.Novel) (Record, error) {
	url := "https://www.pixiv.net/novel/show.php?id=" + strconv.FormatInt(novel.ID, 10)
	return recordFromValue(novel, novel.ID, "novel", url)
}

// RecordFromNovelContent 将结构化小说正文映射为可继续管道处理的小说记录。
func RecordFromNovelContent(content pixiv.NovelContent) (Record, error) {
	url := "https://www.pixiv.net/novel/show.php?id=" + strconv.FormatInt(content.NovelID, 10)
	return recordFromValue(content, content.NovelID, "novel", url)
}

// RecordFromUserPreview 将用户预览映射为管道记录。
func RecordFromUserPreview(preview pixiv.UserPreview) (Record, error) {
	id := strconv.FormatInt(preview.User.ID, 10)
	return recordFromValue(preview, preview.User.ID, "user", "https://www.pixiv.net/users/"+id)
}

// RecordFromUserDetail 将用户详情映射为管道记录。
func RecordFromUserDetail(detail pixiv.UserDetail) (Record, error) {
	id := strconv.FormatInt(detail.User.ID, 10)
	return recordFromValue(detail, detail.User.ID, "user", "https://www.pixiv.net/users/"+id)
}

// FromArtworks 批量构造 artwork records。
func FromArtworks(items []pixiv.Artwork) ([]Record, error) {
	return recordsFrom(items, RecordFromArtwork)
}

// FromNovels 批量构造 novel records。
func FromNovels(items []pixiv.Novel) ([]Record, error) {
	return recordsFrom(items, RecordFromNovel)
}

// FromUserPreviews 批量构造 user records。
func FromUserPreviews(items []pixiv.UserPreview) ([]Record, error) {
	return recordsFrom(items, RecordFromUserPreview)
}

func recordsFrom[T any](items []T, project func(T) (Record, error)) ([]Record, error) {
	records := make([]Record, 0, len(items))
	for _, item := range items {
		record, err := project(item)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func recordFromValue(value any, sourceID int64, recordType, url string) (Record, error) {
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
	return newRecord(fields), nil
}
