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
	Resource sdk.Resource `json:"resource"`
	Variant  string       `json:"variant"`
	Width    int          `json:"width"`
	Height   int          `json:"height"`
}

// FileResource is a first-party FANBOX file exposed through the shared resource
// contract.
type FileResource struct {
	Resource sdk.Resource `json:"resource"`
	Name     string       `json:"name"`
}

// CreatorSummary is a creator identity used in list results.
type CreatorSummary struct {
	ID   string        `json:"id"`
	Name string        `json:"name"`
	Icon ImageResource `json:"icon"`
}

// Creator is a creator profile. Optional fields remain zero when upstream does
// not provide them.
type Creator struct {
	CreatorSummary
	HasAdultContent   bool          `json:"has_adult_content"`
	IsFollowing       bool          `json:"is_following"`
	Cover             ImageResource `json:"cover"`
	PlanFee           int           `json:"plan_fee"`
	HasSupportingPlan bool          `json:"has_supporting_plan"`
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
	Resource sdk.Resource `json:"resource"`
	Caption  string       `json:"caption"`
}

// PostFileBlock is a first-party file block in a post body.
type PostFileBlock struct {
	Resource sdk.Resource `json:"resource"`
	Name     string       `json:"name"`
	Caption  string       `json:"caption"`
}

// PostArticleBlock is a text article block. Image blocks referenced within it
// are flattened into the post's asset list in display order.
type PostArticleBlock struct {
	Text string `json:"text"`
}

// PostVideoEmbed is a third-party embed. Only the provider, content identity,
// canonical link, and safe metadata are exposed; the embed is never fetched or
// downloaded.
type PostVideoEmbed struct {
	Provider     string            `json:"provider"`
	ContentID    string            `json:"content_id"`
	CanonicalURL string            `json:"canonical_url"`
	Title        string            `json:"title"`
	ThumbnailURL string            `json:"thumbnail_url"`
	VideoID      string            `json:"video_id"`
	EmbeddedData map[string]string `json:"embedded_data"`
}

// PostUnknownBlock preserves an unrecognized block's type and safe payload.
type PostUnknownBlock struct {
	RawType string            `json:"raw_type"`
	Payload map[string]string `json:"payload"`
}

// PostBlock is one structured block of a post body.
type PostBlock struct {
	Kind    PostBlockKind     `json:"kind"`
	Image   *PostImageBlock   `json:"image"`
	File    *PostFileBlock    `json:"file"`
	Article *PostArticleBlock `json:"article"`
	Video   *PostVideoEmbed   `json:"video"`
	Unknown *PostUnknownBlock `json:"unknown"`
}

// Asset is one first-party media asset of a post. Assets are flattened in
// upstream display order, including article image blocks.
type Asset struct {
	ID        string        `json:"id"`
	Kind      AssetKind     `json:"kind"`
	Name      string        `json:"name"`
	Resource  sdk.Resource  `json:"resource"`
	Thumbnail ImageResource `json:"thumbnail"`
}

// PostBody is the structured body of a post. Text carries the caption text;
// Blocks preserve the ordered block structure; Assets flatten every first-party
// media item in display order. Restricted posts have a nil Body and only a
// summary.
type PostBody struct {
	Text   string      `json:"text"`
	Blocks []PostBlock `json:"blocks"`
	Assets []Asset     `json:"assets"`
}

// Post is a FANBOX post. PublishedAt is the UTC publication time. A restricted
// post carries only the summary fields and a nil Body.
type Post struct {
	ID            string        `json:"id"`
	Title         string        `json:"title"`
	PublishedAt   time.Time     `json:"published_at"`
	CreatorID     string        `json:"creator_id"`
	FeeRequired   int           `json:"fee_required"`
	IsRestricted  bool          `json:"is_restricted"`
	IsPinned      bool          `json:"is_pinned"`
	RestrictedFor int           `json:"restricted_for"`
	CommentCount  int           `json:"comment_count"`
	Cover         ImageResource `json:"cover"`
	Body          *PostBody     `json:"body"`
}

// User is the current authenticated FANBOX identity.
type User struct {
	UserID        int64  `json:"user_id"`
	DisplayName   string `json:"display_name"`
	CreatorID     string `json:"creator_id"`
	CreatorStatus string `json:"creator_status"`
	IsCreator     bool   `json:"is_creator"`
}

// CreatorTag is one tag used by a creator's posts.
type CreatorTag struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}
