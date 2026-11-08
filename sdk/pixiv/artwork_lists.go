package pixiv

import (
	"context"
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// UserArtworksOptions selects artwork kind and continuation; zero kind means illustration.
type UserArtworksOptions struct {
	Kind   ContentType
	Cursor sdk.Cursor
}

// RelatedArtworksOptions resumes the same artwork's related query.
type RelatedArtworksOptions struct{ Cursor sdk.Cursor }

// Artworks lists this user's works with complete page data.
func (r *UserRef) Artworks(ctx context.Context, options UserArtworksOptions) (sdk.Page[*Artwork], error) {
	if err := r.bound("User.Artworks"); err != nil {
		return sdk.Page[*Artwork]{}, err
	}
	kind := options.Kind
	if kind == "" {
		kind = ContentTypeIllust
	}
	switch kind {
	case ContentTypeIllust, ContentTypeManga, ContentTypeUgoira:
	default:
		return sdk.Page[*Artwork]{}, newError("User.Artworks", sdk.InvalidArgument, "unsupported artwork kind", nil)
	}
	return r.client.artworks(ctx, api.PathUserIllusts, url.Values{api.ParamUserID: {strconv.FormatInt(r.ID, 10)}, api.ParamType: {string(kind)}}, options.Cursor)
}

// Related lists artworks related to this artwork, preserving multi-parameter continuations.
func (r *ArtworkRef) Related(ctx context.Context, options RelatedArtworksOptions) (sdk.Page[*Artwork], error) {
	if err := r.bound("Artwork.Related"); err != nil {
		return sdk.Page[*Artwork]{}, err
	}
	return r.client.artworks(ctx, api.PathIllustRelated, url.Values{api.ParamIllustID: {strconv.FormatInt(r.ID, 10)}}, options.Cursor)
}

func (c *Client) artworks(ctx context.Context, path string, base url.Values, token sdk.Cursor) (sdk.Page[*Artwork], error) {
	query, binding, err := c.pageQuery(ctx, path, base, token)
	if err != nil {
		return sdk.Page[*Artwork]{}, err
	}
	var raw artworkListWire
	if err := c.getJSON(ctx, path, query, &raw); err != nil {
		return sdk.Page[*Artwork]{}, err
	}
	if !raw.Illusts.Present || !raw.Illusts.Valid {
		return sdk.Page[*Artwork]{}, newError(path, sdk.MalformedUpstreamResponse, "missing artworks", nil)
	}
	items := make([]*Artwork, len(raw.Illusts.Items))
	for i, w := range raw.Illusts.Items {
		items[i], err = c.mapArtwork(w)
		if err != nil {
			return sdk.Page[*Artwork]{}, err
		}
	}
	next, err := nextCursor(raw.NextURL, path, base, binding)
	if err != nil {
		return sdk.Page[*Artwork]{}, err
	}
	return sdk.NewPage(items, next, func(ctx context.Context) (sdk.Page[*Artwork], error) { return c.artworks(ctx, path, base, next) }), nil
}
