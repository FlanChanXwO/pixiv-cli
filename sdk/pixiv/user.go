package pixiv

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

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
	userData
}

type userData struct {
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

func (r *UserRef) bound(operation string) error {
	if r == nil || r.client == nil {
		return newError(operation, sdk.InvalidArgument, "user is unbound; create a reference through Client.User", nil)
	}
	if r.ID <= 0 {
		return newError(operation, sdk.InvalidArgument, "user ID must be positive", nil)
	}
	return nil
}

// UnmarshalJSON clears any earlier client binding.
func (r *UserRef) UnmarshalJSON(data []byte) error {
	var value struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*r = UserRef{ID: value.ID}
	return nil
}

// UnmarshalJSON restores a summary without retaining its earlier account binding.
func (u *User) UnmarshalJSON(data []byte) error {
	var value struct {
		userData
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*u = User{UserRef: &UserRef{ID: value.ID}, userData: value.userData}
	return nil
}

// Get loads a user summary; Profile explicitly loads the complete profile.
func (r *UserRef) Get(ctx context.Context) (*User, error) {
	p, err := r.Profile(ctx)
	if err != nil {
		return nil, err
	}
	return p.User, nil
}

// Profile loads public counters, visibility settings, and workspace details.
func (r *UserRef) Profile(ctx context.Context) (*Profile, error) {
	if err := r.bound("User.Profile"); err != nil {
		return nil, err
	}
	return r.client.profile(ctx, r.ID, false)
}

// CurrentUser loads the authenticated account's full profile with the App filter.
func (c *Client) CurrentUser(ctx context.Context) (*Profile, error) {
	if _, err := c.credentials(ctx, Credentials{}); err != nil {
		return nil, err
	}
	if c.UserID() <= 0 {
		return nil, newError("CurrentUser", sdk.Unauthorized, "current user identity is unknown", nil)
	}
	return c.profile(ctx, c.UserID(), true)
}

func (c *Client) profile(ctx context.Context, id int64, filter bool) (*Profile, error) {
	query := url.Values{api.ParamUserID: {strconv.FormatInt(id, 10)}}
	if filter {
		query.Set(api.ParamFilter, "for_android")
	}
	var raw userDetailWire
	if err := c.getJSON(ctx, api.PathUserDetail, query, &raw); err != nil {
		return nil, err
	}
	if !raw.User.Present || !raw.User.Valid || raw.User.Value.ID <= 0 || !raw.Profile.Present || !raw.Profile.Valid ||
		!raw.ProfilePublicity.Present || !raw.ProfilePublicity.Valid || !raw.ProfilePublicity.Value.valid() || !raw.Workspace.Present || !raw.Workspace.Valid {
		return nil, newError("User.Profile", sdk.MalformedUpstreamResponse, "incomplete user profile", nil)
	}
	return c.mapProfile(raw)
}

type blockedUserWire struct{ User userWire }

// UnmarshalJSON accepts the wrapped and bare user forms returned by blocked-user lists.
func (w *blockedUserWire) UnmarshalJSON(data []byte) error {
	var wrapped struct {
		User *userWire `json:"user"`
	}
	if err := json.Unmarshal(data, &wrapped); err != nil {
		return err
	}
	if wrapped.User != nil {
		w.User = *wrapped.User
		return nil
	}
	return json.Unmarshal(data, &w.User)
}
