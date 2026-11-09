package pixiv

import (
	"context"
	"encoding/json"
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// RecommendedUsersOptions resumes the account's user recommendations.
type RecommendedUsersOptions struct{ Cursor sdk.Cursor }

// UserRecommendation contains a bound user and the sample works returned for that user.
type UserRecommendation struct {
	User     *User      `json:"user"`
	Artworks []*Artwork `json:"artworks"`
	Novels   []*Novel   `json:"novels"`
}

// UnmarshalJSON restores all recommendation data without retaining earlier bindings.
func (r *UserRecommendation) UnmarshalJSON(data []byte) error {
	type plain UserRecommendation
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*r = UserRecommendation(value)
	return nil
}

// RecommendedUsers lists users with their artwork and novel samples.
// Malformed samples fail the page so no upstream data or mapping error is silently lost.
func (c *Client) RecommendedUsers(ctx context.Context, options RecommendedUsersOptions) (sdk.Page[UserRecommendation], error) {
	const path = api.PathUserRecommended
	base := url.Values{}
	query, binding, err := c.pageQuery(ctx, path, base, options.Cursor)
	if err != nil {
		return sdk.Page[UserRecommendation]{}, err
	}
	var raw struct {
		Previews api.RequiredList[recommendationWire] `json:"user_previews"`
		NextURL  *string                              `json:"next_url"`
	}
	if err := c.getJSON(ctx, path, query, &raw); err != nil {
		return sdk.Page[UserRecommendation]{}, err
	}
	if !raw.Previews.Present || !raw.Previews.Valid {
		return sdk.Page[UserRecommendation]{}, newError(path, sdk.MalformedUpstreamResponse, "missing user recommendations", nil)
	}
	items := make([]UserRecommendation, len(raw.Previews.Items))
	for i, w := range raw.Previews.Items {
		items[i], err = c.mapRecommendation(w)
		if err != nil {
			return sdk.Page[UserRecommendation]{}, err
		}
	}
	next, err := nextCursor(raw.NextURL, path, base, binding)
	if err != nil {
		return sdk.Page[UserRecommendation]{}, err
	}
	return sdk.NewPage(items, next, func(ctx context.Context) (sdk.Page[UserRecommendation], error) {
		return c.RecommendedUsers(ctx, RecommendedUsersOptions{Cursor: next})
	}), nil
}

type recommendationWire struct {
	User     userWire      `json:"user"`
	Artworks []artworkWire `json:"illusts"`
	Novels   []novelWire   `json:"novels"`
}

func (c *Client) mapRecommendation(w recommendationWire) (UserRecommendation, error) {
	if w.User.ID <= 0 {
		return UserRecommendation{}, newError("RecommendedUsers", sdk.MalformedUpstreamResponse, "invalid user", nil)
	}
	user, err := c.mapUser(w.User)
	if err != nil {
		return UserRecommendation{}, err
	}
	result := UserRecommendation{User: user, Artworks: make([]*Artwork, len(w.Artworks)), Novels: make([]*Novel, len(w.Novels))}
	for i, item := range w.Artworks {
		result.Artworks[i], err = c.mapArtwork(item)
		if err != nil {
			return UserRecommendation{}, err
		}
	}
	for i, item := range w.Novels {
		result.Novels[i], err = c.mapNovel(item)
		if err != nil {
			return UserRecommendation{}, err
		}
	}
	return result, nil
}
