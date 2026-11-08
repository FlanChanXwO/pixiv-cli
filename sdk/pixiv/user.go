package pixiv

// UserRef addresses a user on the client that created it.
type UserRef struct {
	ID     int64 `json:"id"`
	client *Client
}

// User returns a reference without network or credential I/O.
func (c *Client) User(id int64) *UserRef { return &UserRef{ID: id, client: c} }

// User carries the summary returned by artwork and user lists.
// Full profile data is loaded explicitly through the reference.
type User struct {
	*UserRef
	Name         string `json:"name"`
	Account      string `json:"account"`
	Comment      string `json:"comment"`
	IsFollowed   bool   `json:"is_followed"`
	ProfileImage Image  `json:"profile_image"`
}

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
