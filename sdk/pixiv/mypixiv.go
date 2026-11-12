package pixiv

import (
	"context"
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// MyPixivOptions resumes a MyPixiv feed for the same client account.
type MyPixivOptions struct{ Cursor sdk.Cursor }

// MyPixivArtworks lists artworks shared by the account's MyPixiv users.
func (c *Client) MyPixivArtworks(ctx context.Context, options MyPixivOptions) (sdk.Page[*Artwork], error) {
	return c.artworks(ctx, api.PathIllustMyPixiv, url.Values{}, options.Cursor)
}

// MyPixivNovels lists novels shared by the account's MyPixiv users.
func (c *Client) MyPixivNovels(ctx context.Context, options MyPixivOptions) (sdk.Page[*Novel], error) {
	return c.novels(ctx, api.PathNovelMyPixiv, url.Values{}, options.Cursor)
}

// MyPixivUsers lists MyPixiv users after establishing the client account's identity.
func (c *Client) MyPixivUsers(ctx context.Context, options MyPixivOptions) (sdk.Page[*User], error) {
	if _, err := c.credentials(ctx, Credentials{}); err != nil {
		return sdk.Page[*User]{}, err
	}
	query := url.Values{api.ParamUserID: {strconv.FormatInt(c.UserID(), 10)}, api.ParamFilter: {"for_android"}}
	return c.users(ctx, api.PathUserMyPixiv, query, options.Cursor)
}
