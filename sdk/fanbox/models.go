package fanbox

import (
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// AssetKind distinguishes first-party image and file assets.
type AssetKind string

// AssetKindImage and AssetKindFile distinguish the two media kinds a post body
// can carry.
const (
	AssetKindImage AssetKind = "image"
	AssetKindFile  AssetKind = "file"
)

// ImageResource exposes a first-party FANBOX image through the shared resource
// contract.
type ImageResource struct {
	Resource sdk.Resource `json:"resource"`
	Variant  string       `json:"variant"`
	Width    int          `json:"width"`
	Height   int          `json:"height"`
}

// FileResource exposes a first-party FANBOX file through the shared resource
// contract.
type FileResource struct {
	Resource sdk.Resource `json:"resource"`
	Name     string       `json:"name"`
}

// CreatorSummary identifies a creator in list results without the full
// profile.
type CreatorSummary struct {
	ID   string        `json:"id"`
	Name string        `json:"name"`
	Icon ImageResource `json:"icon"`
}

// Creator is a creator profile; optional fields stay zero when upstream omits
// them. PlanFee carries the upstream plan fee value unchanged.
type Creator struct {
	CreatorSummary
	HasAdultContent   bool          `json:"has_adult_content"`
	IsFollowing       bool          `json:"is_following"`
	Cover             ImageResource `json:"cover"`
	PlanFee           int           `json:"plan_fee"`
	HasSupportingPlan bool          `json:"has_supporting_plan"`
}

// PostBlockKind classifies a structured block of a post body.
type PostBlockKind string

// PostBlockImage, PostBlockFile, PostBlockArticle, and PostBlockVideo name the
// block shapes the SDK decodes.
const (
	PostBlockImage   PostBlockKind = "image"
	PostBlockFile    PostBlockKind = "file"
	PostBlockArticle PostBlockKind = "article"
	PostBlockVideo   PostBlockKind = "video"
	// PostBlockUnknown preserves a block the SDK does not recognize; its raw type
	// and safe payload are retained.
	PostBlockUnknown PostBlockKind = "unknown"
)

// PostImageBlock carries a first-party image inside a post body.
type PostImageBlock struct {
	Resource sdk.Resource `json:"resource"`
	Caption  string       `json:"caption"`
}

// PostFileBlock carries a first-party file attachment inside a post body.
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

// Asset is one first-party media item of a post, including images inside
// article blocks. Assets lists every image before every file, so it does not
// preserve the cross-type order of Blocks.
type Asset struct {
	ID        string        `json:"id"`
	Kind      AssetKind     `json:"kind"`
	Name      string        `json:"name"`
	Resource  sdk.Resource  `json:"resource"`
	Thumbnail ImageResource `json:"thumbnail"`
}

// PostBody carries a post's caption text, its ordered Blocks, and every
// first-party media item flattened into Assets. Assets groups images before
// files; Blocks is the source of cross-type display order. Restricted posts have
// a nil Body and only summary fields.
type PostBody struct {
	Text   string      `json:"text"`
	Blocks []PostBlock `json:"blocks"`
	Assets []Asset     `json:"assets"`
}

// Post is a FANBOX post. PublishedAt is UTC. A restricted post carries only
// summary fields and a nil Body.
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

// User is the identity of the session that opened the client.
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
