package fanbox

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox/internal/api"
)

// Reference identifies a locally parsed FANBOX creator, post or post list.
// Use a type switch to access the corresponding bound operations.
type Reference interface{ CanonicalURL() (string, error) }

// CreatorPostsRef identifies a creator's complete or tag-filtered post list.
type CreatorPostsRef struct {
	*CreatorRef
	Tag string `json:"tag,omitempty"`
}

// ParseURL creates a bound reference without network I/O. Main-host forms are
// /creators/{id}, /@creator, /@creator/posts, /@creator/posts/{post}, and
// /@creator/posts/tag/{tag}. Creator subdomains retain the legacy interpretation
// as creator profiles regardless of path. Query and fragment are discarded.
func (c *Client) ParseURL(raw string) (Reference, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.User != nil {
		return nil, newError("ParseURL", sdk.InvalidArgument, "FANBOX URL must use HTTPS without userinfo")
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "www.fanbox.cc" && host != "fanbox.cc" {
		if strings.HasSuffix(host, ".fanbox.cc") {
			creator := strings.TrimSuffix(host, ".fanbox.cc")
			if creator != "" && !strings.Contains(creator, ".") {
				return c.Creator(creator), nil
			}
		}
		return nil, newError("ParseURL", sdk.InvalidArgument, "unsupported FANBOX URL host")
	}
	if ref := c.referencePath(strings.Split(strings.Trim(parsed.Path, "/"), "/")); ref != nil {
		return ref, nil
	}
	return nil, newError("ParseURL", sdk.InvalidArgument, "URL does not name a supported FANBOX page")
}

func (c *Client) referencePath(parts []string) Reference {
	if len(parts) == 2 && parts[0] == "creators" && strings.TrimSpace(parts[1]) != "" {
		return c.Creator(strings.TrimSpace(parts[1]))
	}
	if !strings.HasPrefix(parts[0], "@") {
		return nil
	}
	id := strings.TrimSpace(strings.TrimPrefix(parts[0], "@"))
	if id == "" {
		return nil
	}
	if len(parts) == 1 {
		return c.Creator(id)
	}
	if parts[1] != "posts" {
		return nil
	}
	switch len(parts) {
	case 2:
		return &CreatorPostsRef{CreatorRef: c.Creator(id)}
	case 3:
		postID := strings.TrimSpace(parts[2])
		if postID == "" || postID == "tag" {
			return nil
		}
		return &PostRef{ID: postID, CreatorID: id, client: c}
	case 4:
		tag := strings.TrimSpace(parts[3])
		if parts[2] == "tag" && tag != "" {
			return &CreatorPostsRef{CreatorRef: c.Creator(id), Tag: tag}
		}
	}
	return nil
}

// CanonicalURL returns the tracking-free creator URL.
func (r *CreatorRef) CanonicalURL() (string, error) {
	if r == nil {
		return "", newError("CanonicalURL", sdk.InvalidArgument, "creator ID is required")
	}
	id, err := canonicalPart(r.ID)
	if err != nil {
		return "", err
	}
	return api.WebBaseURL + "@" + id, nil
}

// CanonicalURL returns the post URL; both post ID and CreatorID are required.
func (r *PostRef) CanonicalURL() (string, error) {
	if r == nil {
		return "", newError("CanonicalURL", sdk.InvalidArgument, "post and creator IDs are required")
	}
	base, err := (&CreatorRef{ID: r.CreatorID}).CanonicalURL()
	if err != nil {
		return "", err
	}
	id, err := canonicalPart(r.ID)
	if err != nil {
		return "", err
	}
	return base + "/posts/" + id, nil
}

// CanonicalURL returns the complete or tag-filtered creator post-list URL.
func (r *CreatorPostsRef) CanonicalURL() (string, error) {
	if r == nil {
		return "", newError("CanonicalURL", sdk.InvalidArgument, "creator ID is required")
	}
	base, err := r.CreatorRef.CanonicalURL()
	if err != nil {
		return "", err
	}
	base += "/posts"
	if r.Tag == "" {
		return base, nil
	}
	tag, err := canonicalPart(r.Tag)
	if err != nil {
		return "", err
	}
	return base + "/tag/" + tag, nil
}

func canonicalPart(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "/") {
		return "", newError("CanonicalURL", sdk.InvalidArgument, "FANBOX URL identity must be a nonempty path segment")
	}
	return url.PathEscape(value), nil
}

// Posts lists this reference's posts, applying its Tag and the supplied cursor.
func (r *CreatorPostsRef) Posts(ctx context.Context, request FeedRequest) (sdk.Page[*Post], error) {
	if r == nil {
		return sdk.Page[*Post]{}, newError("CreatorPosts.Posts", sdk.InvalidArgument, "creator post list is unbound")
	}
	return r.CreatorRef.Posts(ctx, PostsRequest{Tag: r.Tag, Cursor: request.Cursor})
}

// UnmarshalJSON restores the creator and tag without retaining the earlier client.
func (r *CreatorPostsRef) UnmarshalJSON(data []byte) error {
	var value struct {
		ID  string `json:"id"`
		Tag string `json:"tag"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*r = CreatorPostsRef{CreatorRef: &CreatorRef{ID: value.ID}, Tag: value.Tag}
	return nil
}
