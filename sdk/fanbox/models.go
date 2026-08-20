package fanbox

import (
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// AssetKind distinguishes first-party image and file assets.
type AssetKind string

// AssetKind values define the supported AssetKind filesystem.
const (
	AssetKindImage AssetKind = "image"
	AssetKindFile  AssetKind = "file"
)

// ImageResource is a first-party FANBOX image exposed through the shared
// resource contract.
type ImageResource struct {
	Resource sdk.Resource
	Variant  string
	Width    int
	Height   int
}

// FileResource is a first-party FANBOX file exposed through the shared resource
// contract.
type FileResource struct {
	Resource sdk.Resource
	Name     string
}

// CreatorSummary is a creator identity used in list results.
type CreatorSummary struct {
	ID   string
	Name string
	Icon ImageResource
}

// Creator is a creator profile. Optional fields remain zero when upstream does
// not provide them.
type Creator struct {
	CreatorSummary
	HasAdultContent   bool
	IsFollowing       bool
	Cover             ImageResource
	PlanFee           int
	HasSupportingPlan bool
}

// PostBlockKind is the structured kind of a post body block.
type PostBlockKind string

// PostBlockKind values define the supported PostBlockKind filesystem.
const (
	PostBlockImage   PostBlockKind = "image"
	PostBlockFile    PostBlockKind = "file"
	PostBlockArticle PostBlockKind = "article"
	PostBlockVideo   PostBlockKind = "video"
	// PostBlockUnknown preserves a block the SDK does not recognize; its raw type
	// and safe payload are retained.
	PostBlockUnknown PostBlockKind = "unknown"
)

// PostImageBlock is a first-party image block in a post body.
type PostImageBlock struct {
	Resource sdk.Resource
	Caption  string
}

// PostFileBlock is a first-party file block in a post body.
type PostFileBlock struct {
	Resource sdk.Resource
	Name     string
	Caption  string
}

// PostArticleBlock is a text article block. Image blocks referenced within it
// are flattened into the post's asset list in display order.
type PostArticleBlock struct {
	Text string
}

// PostVideoEmbed is a third-party embed. Only the provider, content identity,
// canonical link, and safe metadata are exposed; the embed is never fetched or
// downloaded.
type PostVideoEmbed struct {
	Provider     string
	ContentID    string
	CanonicalURL string
	Title        string
	ThumbnailURL string
	VideoID      string
	EmbeddedData map[string]string
}

// PostUnknownBlock preserves an unrecognized block's type and safe payload.
type PostUnknownBlock struct {
	RawType string
	Payload map[string]string
}

// PostBlock is one structured block of a post body.
type PostBlock struct {
	Kind    PostBlockKind
	Image   *PostImageBlock
	File    *PostFileBlock
	Article *PostArticleBlock
	Video   *PostVideoEmbed
	Unknown *PostUnknownBlock
}

// Asset is one first-party media asset of a post. Assets are flattened in
// upstream display order, including article image blocks.
type Asset struct {
	ID        string
	Kind      AssetKind
	Name      string
	Resource  sdk.Resource
	Thumbnail ImageResource
}

// PostBody is the structured body of a post. Text carries the caption text;
// Blocks preserve the ordered block structure; Assets flatten every first-party
// media item in display order. Restricted posts have a nil Body and only a
// summary.
type PostBody struct {
	Text   string
	Blocks []PostBlock
	Assets []Asset
}

// Post is a FANBOX post. PublishedAt is the UTC publication time. A restricted
// post carries only the summary fields and a nil Body.
type Post struct {
	ID            string
	Title         string
	PublishedAt   time.Time
	CreatorID     string
	FeeRequired   int
	IsRestricted  bool
	IsPinned      bool
	RestrictedFor int
	CommentCount  int
	Cover         ImageResource
	Body          *PostBody
}

// User is the current authenticated FANBOX identity.
type User struct {
	UserID        int64
	DisplayName   string
	CreatorID     string
	CreatorStatus string
	IsCreator     bool
}

// CreatorTag is one tag used by a creator's posts.
type CreatorTag struct {
	Name string
	URL  string
}
