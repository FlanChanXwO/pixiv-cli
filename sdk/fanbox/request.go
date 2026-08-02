package fanbox

import "github.com/FlanChanXwO/pixiv-cli/sdk"

// CreatorListKind selects the kind of creator list.
type CreatorListKind string

// CreatorListKind values define the supported CreatorListKind constants.
const (
	CreatorListSupporting CreatorListKind = "supporting"
	CreatorListFollowing  CreatorListKind = "following"
)

// CreatorRequest selects one creator profile.
type CreatorRequest struct {
	CreatorID string
}

// CreatorsRequest lists supporting or following creators. Repeat the original
// Kind when continuing with Cursor.
type CreatorsRequest struct {
	Kind   CreatorListKind
	Cursor sdk.Cursor
}

// CreatorTagsRequest lists the tags used by one creator.
type CreatorTagsRequest struct {
	CreatorID string
}

// CreatorPostsRequest lists one creator's posts. Repeat the original CreatorID
// when continuing with Cursor.
type CreatorPostsRequest struct {
	CreatorID string
	Cursor    sdk.Cursor
}

// TaggedPostsRequest lists one creator's posts for a single tag.
type TaggedPostsRequest struct {
	CreatorID string
	Tag       string
	Cursor    sdk.Cursor
}

// PostRequest selects one post by its stable ID.
type PostRequest struct {
	PostID string
}

// HomeRequest lists the current user's home feed.
type HomeRequest struct {
	Cursor sdk.Cursor
}

// SupportingRequest lists posts from the current user's supporting creators.
type SupportingRequest struct {
	Cursor sdk.Cursor
}

// CurrentUserRequest reads the current authenticated user's identity.
type CurrentUserRequest struct{}

// ResolveURLRequest resolves a FANBOX URL into a typed reference.
type ResolveURLRequest struct {
	RawURL string
}
