package record

import (
	"errors"
	"strconv"
)

// NewIdentityRecord 构造只包含 Pixiv canonical identity 的管道记录。
// artwork 表示尚无法可靠判断 illust/manga/ugoira 子类型的作品身份。
func NewIdentityRecord(id int64, recordType string) (Record, error) {
	if id <= 0 {
		return Record{}, errors.New("record id must be positive")
	}
	idString := strconv.FormatInt(id, 10)
	var canonicalURL string
	switch recordType {
	case "artwork":
		canonicalURL = "https://www.pixiv.net/artworks/" + idString
	case "user":
		canonicalURL = "https://www.pixiv.net/users/" + idString
	default:
		return Record{}, errors.New("identity record type must be artwork or user")
	}
	return newRecord(map[string]any{
		"id":   idString,
		"type": recordType,
		"url":  canonicalURL,
	})
}
