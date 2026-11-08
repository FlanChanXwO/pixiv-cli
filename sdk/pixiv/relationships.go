package pixiv

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// Restrict selects the visibility of bookmarks and following relationships.
type Restrict string

const (
	// RestrictPublic makes an item visible to other users.
	RestrictPublic Restrict = "public"
	// RestrictPrivate makes an item visible only to its owner.
	RestrictPrivate Restrict = "private"
)

func normalizeRestrict(value Restrict) (Restrict, error) {
	switch value {
	case "", RestrictPublic:
		return RestrictPublic, nil
	case RestrictPrivate:
		return value, nil
	}
	return "", newError("restrict", sdk.InvalidArgument, "unsupported visibility", nil)
}

// RelationshipOptions selects visibility and continuation; zero visibility means public.
type RelationshipOptions struct {
	Restrict Restrict
	Cursor   sdk.Cursor
}

// RelatedUsersOptions resumes a related-user query.
type RelatedUsersOptions struct{ Cursor sdk.Cursor }

// BlockedUsersOptions resumes a blocked-user query.
type BlockedUsersOptions struct{ Cursor sdk.Cursor }

// FollowOptions selects visibility; the zero value means public.
type FollowOptions struct{ Restrict Restrict }

// Related lists users related to this user.
func (r *UserRef) Related(ctx context.Context, options RelatedUsersOptions) (sdk.Page[*User], error) {
	if err := r.bound("User.Related"); err != nil {
		return sdk.Page[*User]{}, err
	}
	return r.client.users(ctx, api.PathUserRelated, url.Values{api.ParamSeedUserID: {strconv.FormatInt(r.ID, 10)}}, options.Cursor)
}

// Following lists users followed by this user.
func (r *UserRef) Following(ctx context.Context, options RelationshipOptions) (sdk.Page[*User], error) {
	return r.relationship(ctx, api.PathUserFollowing, options)
}

// Followers lists users following this user.
func (r *UserRef) Followers(ctx context.Context, options RelationshipOptions) (sdk.Page[*User], error) {
	return r.relationship(ctx, api.PathUserFollower, options)
}

func (r *UserRef) relationship(ctx context.Context, path string, options RelationshipOptions) (sdk.Page[*User], error) {
	if err := r.bound(path); err != nil {
		return sdk.Page[*User]{}, err
	}
	restrict, err := normalizeRestrict(options.Restrict)
	if err != nil {
		return sdk.Page[*User]{}, err
	}
	return r.client.users(ctx, path, url.Values{api.ParamUserID: {strconv.FormatInt(r.ID, 10)}, api.ParamRestrict: {string(restrict)}}, options.Cursor)
}

// BlockedUsers lists this user's blocked users with the App filter.
func (r *UserRef) BlockedUsers(ctx context.Context, options BlockedUsersOptions) (sdk.Page[*User], error) {
	if err := r.bound("User.BlockedUsers"); err != nil {
		return sdk.Page[*User]{}, err
	}
	return r.client.users(ctx, api.PathUserBlocked, url.Values{api.ParamUserID: {strconv.FormatInt(r.ID, 10)}, api.ParamFilter: {"for_android"}}, options.Cursor)
}

func (c *Client) users(ctx context.Context, path string, base url.Values, token sdk.Cursor) (sdk.Page[*User], error) {
	query, binding, err := c.pageQuery(ctx, path, base, token)
	if err != nil {
		return sdk.Page[*User]{}, err
	}

	var raw struct {
		Previews api.RequiredList[json.RawMessage] `json:"user_previews"`
		Users    api.RequiredList[json.RawMessage] `json:"users"`
		NextURL  *string                           `json:"next_url"`
	}
	if err := c.getJSON(ctx, path, query, &raw); err != nil {
		return sdk.Page[*User]{}, err
	}
	list := raw.Previews
	if path == api.PathUserBlocked && raw.Users.Present {
		list = raw.Users
	}
	if !list.Present || !list.Valid {
		return sdk.Page[*User]{}, newError(path, sdk.MalformedUpstreamResponse, "missing users", nil)
	}
	items := make([]*User, len(list.Items))
	for i, data := range list.Items {
		var w blockedUserWire
		if path == api.PathUserBlocked {
			err = json.Unmarshal(data, &w)
		} else {
			var preview struct {
				User userWire `json:"user"`
			}
			err = json.Unmarshal(data, &preview)
			w.User = preview.User
		}
		if err != nil || w.User.ID <= 0 {
			return sdk.Page[*User]{}, newError(path, sdk.MalformedUpstreamResponse, "invalid user", err)
		}
		items[i], err = c.mapUser(w.User)
		if err != nil {
			return sdk.Page[*User]{}, err
		}
	}
	next, err := nextCursor(raw.NextURL, path, base, binding)
	if err != nil {
		return sdk.Page[*User]{}, err
	}
	return sdk.NewPage(items, next, func(ctx context.Context) (sdk.Page[*User], error) { return c.users(ctx, path, base, next) }), nil
}

// Follow follows this user for the client account, sending the write once.
func (r *UserRef) Follow(ctx context.Context, options FollowOptions) error {
	if err := r.bound("User.Follow"); err != nil {
		return err
	}
	restrict, err := normalizeRestrict(options.Restrict)
	if err != nil {
		return err
	}
	return r.client.postForm(ctx, api.PathFollowAdd, url.Values{api.ParamUserID: {strconv.FormatInt(r.ID, 10)}, api.ParamRestrict: {string(restrict)}})
}

// Unfollow removes this user's following relationship for the client account.
func (r *UserRef) Unfollow(ctx context.Context) error {
	if err := r.bound("User.Unfollow"); err != nil {
		return err
	}
	return r.client.postForm(ctx, api.PathFollowDelete, url.Values{api.ParamUserID: {strconv.FormatInt(r.ID, 10)}})
}

func (c *Client) postForm(ctx context.Context, path string, form url.Values) error {
	value, err := c.credentials(ctx, Credentials{})
	if err != nil {
		return err
	}
	if err := c.app(value).PostForm(ctx, path, form); err != nil {
		return classifyError(path, err, false)
	}
	return nil
}
