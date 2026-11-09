package pixiv

import (
	"context"
	"math"
	"net/url"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/internal/cursor"
)

// ArtworkSearchOptions controls upstream filters, local AI selection, and continuation.
// Repeat these options and the search word when restoring a saved Cursor.
type ArtworkSearchOptions struct {
	Target        SearchTarget
	Sort          SortMode
	Duration      DurationFilter
	StartDate     string // Inclusive YYYY-MM-DD; empty leaves the lower bound to upstream.
	EndDate       string // Inclusive YYYY-MM-DD; empty leaves the upper bound to upstream.
	Category      ArtworkCategory
	AIMode        SearchAIMode
	AspectRatio   SearchAspectRatio
	Resolution    SearchResolution
	Tool          string
	BookmarkMin   *int   // Inclusive bookmark count; nil omits the lower bound, zero is valid.
	BookmarkMax   *int   // Inclusive bookmark count; nil omits the upper bound, zero is valid.
	Offset        int    // Nonnegative starting position in the raw upstream stream; repeat when resuming.
	CursorContext string // Caller-side filter semantics; included in the binding, never sent upstream.
	Cursor        sdk.Cursor
}

// NovelSearchOptions selects matching fields, ordering, publication window, and continuation.
type NovelSearchOptions struct {
	Target   SearchTarget
	Sort     SortMode
	Duration DurationFilter
	Cursor   sdk.Cursor
}

// UserSearchOptions resumes the same user search.
type UserSearchOptions struct{ Cursor sdk.Cursor }

// SearchArtworks searches complete bound artwork objects; AI-only filtering is local.
func (c *Client) SearchArtworks(ctx context.Context, word string, options ArtworkSearchOptions) (sdk.Page[*Artwork], error) {
	base, err := artworkSearchQuery(word, options)
	if err != nil {
		return sdk.Page[*Artwork]{}, err
	}
	return c.searchArtworks(ctx, base, options.Cursor)
}

// SearchNovels searches bound novel objects, requiring the upstream search metadata.
func (c *Client) SearchNovels(ctx context.Context, word string, options NovelSearchOptions) (sdk.Page[*Novel], error) {
	base, err := searchQuery(word, options.Target, options.Sort, options.Duration)
	if err != nil {
		return sdk.Page[*Novel]{}, err
	}
	return c.novels(ctx, api.PathSearchNovel, base, options.Cursor)
}

// SearchUsers searches bound user summaries; Profile explicitly loads full profiles.
func (c *Client) SearchUsers(ctx context.Context, word string, options UserSearchOptions) (sdk.Page[*User], error) {
	if strings.TrimSpace(word) == "" {
		return sdk.Page[*User]{}, newError("SearchUsers", sdk.InvalidArgument, "search word is required", nil)
	}
	return c.users(ctx, api.PathSearchUser, url.Values{api.ParamWord: {word}}, options.Cursor)
}

// CheckpointSearchArtworks records consumed SDK items within the requested batch,
// after AI filtering. It accumulates repeated checkpoints and binds the account
// through saved credentials, refreshing them if needed. Resume with the same word
// and options; the batch is fetched again from the live upstream data.
// options.Cursor must be the cursor used to fetch that batch, not its returned next-page cursor.
func (c *Client) CheckpointSearchArtworks(ctx context.Context, word string, options ArtworkSearchOptions, consumed int) (sdk.Cursor, error) {
	base, err := artworkSearchQuery(word, options)
	if err != nil {
		return sdk.Cursor{}, err
	}
	if consumed <= 0 {
		return sdk.Cursor{}, newError("CheckpointSearchArtworks", sdk.InvalidArgument, "consumed must be positive", nil)
	}
	query, binding, err := c.pageQuery(ctx, api.PathSearchIllust, base, options.Cursor)
	if err != nil {
		return sdk.Cursor{}, err
	}
	offset, previous := artworkSearchPosition(query)
	if consumed > math.MaxInt-previous {
		return sdk.Cursor{}, newError("CheckpointSearchArtworks", sdk.InvalidArgument, "consumed position overflows", nil)
	}
	return cursor.Encode(binding, url.Values{api.ParamOffset: {strconv.Itoa(offset)}, searchConsumedKey: {strconv.Itoa(previous + consumed)}})
}
