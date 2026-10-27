package pixiv

import (
	"cmp"
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
	return cmp.Or(value.Original, value.Large, value.Medium, value.SquareMedium)
}

// mapUser is the single user mapping for every Pixiv response. A profile image
// that fails resource validation returns the error with the enclosing model, so
// an upstream anomaly never becomes a default avatar.
func (c *Client) mapUser(value userWire) (User, error) {
	result := User{
		ID:         value.ID,
		Name:       value.Name,
		Account:    value.Account,
		Comment:    value.Comment,
		IsFollowed: value.IsFollowed,
	}
	if value.ProfileImageURLs.Medium != nil && *value.ProfileImageURLs.Medium != "" {
		resource, err := c.newResource("user_profile", value.ID, -1, "medium", *value.ProfileImageURLs.Medium)
		if err != nil {
			return User{}, err
		}
		result.ProfileImage = ImageResource{Resource: resource, Variant: "medium"}
	}
	return result, nil
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
