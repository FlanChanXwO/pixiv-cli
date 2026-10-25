// 跨领域共用的 Pixiv 响应结构、字段映射与值转换。
// user、tag、image_urls 等被多个领域使用的声明在这里；单领域响应在其领域文件。

package pixiv

import (
	"errors"
	"time"
)

type profileImageURLsWire struct {
	Medium *string `json:"medium"`
}

type userWire struct {
	ID               int64                `json:"id"`
	Name             string               `json:"name"`
	Account          string               `json:"account"`
	Comment          string               `json:"comment"`
	IsFollowed       bool                 `json:"is_followed"`
	ProfileImageURLs profileImageURLsWire `json:"profile_image_urls"`
}

type tagWire struct {
	Name           string `json:"name"`
	TranslatedName string `json:"translated_name"`
}

type imageURLsWire struct {
	SquareMedium string `json:"square_medium"`
	Medium       string `json:"medium"`
	Large        string `json:"large"`
	Original     string `json:"original"`
}

func firstImageURL(value imageURLsWire) string {
	for _, candidate := range []string{value.Original, value.Large, value.Medium, value.SquareMedium} {
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

// mapUser 是所有 Pixiv 响应的唯一 user 映射。
func (c *Client) mapUser(value userWire) User {
	result := User{
		ID:         value.ID,
		Name:       value.Name,
		Account:    value.Account,
		Comment:    value.Comment,
		IsFollowed: value.IsFollowed,
	}
	if value.ProfileImageURLs.Medium != nil && *value.ProfileImageURLs.Medium != "" {
		if resource, err := c.newResourceWithVariant("user_profile", value.ID, -1, "medium", *value.ProfileImageURLs.Medium); err == nil {
			result.ProfileImage = ImageResource{Resource: resource, Variant: "medium"}
		}
	}
	return result
}

func mapTags(values []tagWire) []Tag {
	result := make([]Tag, len(values))
	for index, value := range values {
		result[index] = Tag{Name: value.Name, TranslatedName: value.TranslatedName}
	}
	return result
}

func parseUTCTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, errNoPublishTime
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

var errNoPublishTime = errors.New("no publish time")
