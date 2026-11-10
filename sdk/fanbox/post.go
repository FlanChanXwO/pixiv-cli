package fanbox

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox/internal/api"
)

// PostRef addresses a post on the client that created it.
type PostRef struct {
	ID     string `json:"id"`
	client *Client
}

// Post returns a reference without network I/O.
func (c *Client) Post(id string) *PostRef { return &PostRef{ID: id, client: c} }

// Post carries the same metadata and available body in details and lists.
// PublishedAt is UTC; Body remains nil when upstream withholds it.
// Creator is a bound ID summary when the response supplies a creator ID.
type Post struct {
	*PostRef
	postData
}

type postData struct {
	Title         string    `json:"title"`
	PublishedAt   time.Time `json:"published_at"`
	Creator       *Creator  `json:"creator"`
	FeeRequired   int       `json:"fee_required"`
	IsRestricted  bool      `json:"is_restricted"`
	IsPinned      bool      `json:"is_pinned"`
	RestrictedFor int       `json:"restricted_for"`
	CommentCount  int       `json:"comment_count"`
	Cover         Image     `json:"cover"`
	Body          *PostBody `json:"body"`
}

// Get loads this post's details and available body.
func (r *PostRef) Get(ctx context.Context) (*Post, error) {
	const operation = "Post.Get"
	if err := r.bound(operation); err != nil {
		return nil, err
	}
	id := strings.TrimSpace(r.ID)
	body, err := r.client.getBody(ctx, operation, api.PathPostInfo, url.Values{api.ParamPostID: {id}})
	if err != nil {
		return nil, err
	}
	var source struct {
		Post *postWire `json:"post"`
	}
	if json.Unmarshal(body, &source) != nil || source.Post == nil || strings.TrimSpace(source.Post.ID) != id {
		return nil, newError(operation, sdk.MalformedUpstreamResponse, "invalid FANBOX post detail")
	}
	return r.client.mapPost(operation, *source.Post)
}

func (r *PostRef) bound(operation string) error {
	if r == nil || r.client == nil {
		return newError(operation, sdk.InvalidArgument, "post is unbound; create a reference through Client.Post")
	}
	if strings.TrimSpace(r.ID) == "" {
		return newError(operation, sdk.InvalidArgument, "post ID is required")
	}
	return nil
}

// UnmarshalJSON restores the ID and clears the earlier client binding.
func (r *PostRef) UnmarshalJSON(data []byte) error {
	var value struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*r = PostRef{ID: value.ID}
	return nil
}

// UnmarshalJSON restores metadata without retaining object or media bindings.
func (p *Post) UnmarshalJSON(data []byte) error {
	var value struct {
		postData
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*p = Post{PostRef: &PostRef{ID: value.ID}, postData: value.postData}
	return nil
}

// PostBody preserves text, ordered blocks and first-party media.
// Explicit media lists retain duplicates; block media are deduplicated within
// each class in first-use order. Payload retains the complete upstream body,
// including embed maps and fields with no typed representation.
type PostBody struct {
	Text    string          `json:"text"`
	Images  []Image         `json:"images"`
	Files   []File          `json:"files"`
	Blocks  []PostBlock     `json:"blocks"`
	Payload json.RawMessage `json:"payload"`
}

// BlockStyle is the upstream block discriminator; unknown values are retained.
type BlockStyle string

const (
	// BlockParagraph carries a paragraph and its text styles.
	BlockParagraph BlockStyle = "p"
	// BlockHeader carries a heading.
	BlockHeader BlockStyle = "header"
	// BlockImage references a first-party image.
	BlockImage BlockStyle = "image"
	// BlockFile references a first-party attachment.
	BlockFile BlockStyle = "file"
	// BlockArticle carries article text.
	BlockArticle BlockStyle = "article"
	// BlockVideo carries third-party video metadata.
	BlockVideo BlockStyle = "video"
	// BlockEmbed references third-party embed metadata.
	BlockEmbed BlockStyle = "embed"
	// BlockURLEmbed references a link preview.
	BlockURLEmbed BlockStyle = "url_embed"
)

// PostBlock preserves display order and the complete upstream block in Payload.
// Image and File are first-party references, including on unknown block styles.
// Third-party embeds remain metadata in Payload and PostBody.Payload.
type PostBlock struct {
	Style   BlockStyle      `json:"style"`
	Text    string          `json:"text"`
	Caption string          `json:"caption"`
	Image   *Image          `json:"image"`
	File    *File           `json:"file"`
	Payload json.RawMessage `json:"payload"`
}
