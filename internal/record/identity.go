package record

import (
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
)

// NewIdentityRecord 构造只包含 Pixiv canonical identity 的管道记录。
// artwork 表示尚无法可靠判断 illust/manga/ugoira 子类型的作品身份。
func NewIdentityRecord(id int64, recordType string) (Record, error) {
	if id <= 0 {
		return Record{}, i18n.Errorf("error.record_id_must_be_positive")
	}
	idString := strconv.FormatInt(id, 10)
	var canonicalURL string
	switch recordType {
	case "artwork":
		canonicalURL = "https://www.pixiv.net/artworks/" + idString
	case "user":
		canonicalURL = "https://www.pixiv.net/users/" + idString
	default:
		return Record{}, i18n.Errorf("error.identity_record_type_must_be_artwork_or_user")
	}
	return newRecord(map[string]any{
		"id":   idString,
		"type": recordType,
		"url":  canonicalURL,
	}), nil
}
