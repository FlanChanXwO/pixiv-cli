package fanbox

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// ReferenceKind identifies the FANBOX resource a resolved URL points to.
type ReferenceKind string

// ReferenceKind values define the supported ReferenceKind constants.
const (
	ReferenceKindCreator      ReferenceKind = "creator"
	ReferenceKindCreatorPosts ReferenceKind = "creator_posts"
	ReferenceKindPost         ReferenceKind = "post"
	ReferenceKindTag          ReferenceKind = "tag"
)

// Reference is a locally parsed FANBOX resource reference. Only the fields
// relevant to Kind are set.
type Reference struct {
	Kind      ReferenceKind
	CreatorID string
	PostID    string
	Tag       string
}

// ResolveURL parses a FANBOX page URL into a typed Reference. It performs no
// network I/O and accepts HTTPS URLs on fanbox.cc hosts or the legacy Pixiv
// FANBOX host. Unknown paths and ambiguous inputs return CodeInvalidArgument.
func (c *Client) ResolveURL(ctx context.Context, request ResolveURLRequest) (Reference, error) {
	parsed, err := url.Parse(strings.TrimSpace(request.RawURL))
	if err != nil {
		return Reference{}, newError("ResolveURL", sdk.CodeInvalidArgument, errors.New("URL is not parseable"))
	}
	if parsed.Scheme != "https" || parsed.User != nil {
		return Reference{}, newError("ResolveURL", sdk.CodeInvalidArgument, errors.New("URL must be https without userinfo"))
	}
	host := strings.ToLower(parsed.Hostname())
	switch {
	case host == "www.fanbox.cc" || host == "fanbox.cc":
		return parseFanboxPath(parsed)
	case strings.HasSuffix(host, ".fanbox.cc"):
		creator := strings.TrimSuffix(host, ".fanbox.cc")
		if creator == "" || strings.Contains(creator, ".") {
			return Reference{}, newError("ResolveURL", sdk.CodeInvalidArgument, errors.New("unsupported fanbox subdomain"))
		}
		return Reference{Kind: ReferenceKindCreator, CreatorID: creator}, nil
	default:
		return Reference{}, newError("ResolveURL", sdk.CodeInvalidArgument, errors.New("URL host is not a supported FANBOX host"))
	}
}

func parseFanboxPath(parsed *url.URL) (Reference, error) {
	parts := splitPath(parsed.Path)
	if len(parts) >= 1 && parts[0] == "creators" {
		// 旧式 www.fanbox.cc/creators/{id} 重定向。
		if len(parts) == 2 && parts[1] != "" {
			return Reference{Kind: ReferenceKindCreator, CreatorID: parts[1]}, nil
		}
		return Reference{}, newError("ResolveURL", sdk.CodeInvalidArgument, errors.New("malformed creators redirect"))
	}
	if len(parts) >= 1 && strings.HasPrefix(parts[0], "@") {
		creator := strings.TrimPrefix(parts[0], "@")
		if creator == "" {
			return Reference{}, newError("ResolveURL", sdk.CodeInvalidArgument, errors.New("creator id is empty"))
		}
		switch len(parts) {
		case 1:
			return Reference{Kind: ReferenceKindCreator, CreatorID: creator}, nil
		case 2:
			if parts[1] != "posts" {
				return Reference{}, newError("ResolveURL", sdk.CodeInvalidArgument, errors.New("unsupported creator path"))
			}
			return Reference{Kind: ReferenceKindCreatorPosts, CreatorID: creator}, nil
		case 3:
			if parts[1] != "posts" {
				return Reference{}, newError("ResolveURL", sdk.CodeInvalidArgument, errors.New("unsupported creator path"))
			}
			if parts[2] == "tag" {
				return Reference{}, newError("ResolveURL", sdk.CodeInvalidArgument, errors.New("tag path is missing a tag"))
			}
			return Reference{Kind: ReferenceKindPost, CreatorID: creator, PostID: parts[2]}, nil
		case 4:
			if parts[1] != "posts" || parts[2] != "tag" {
				return Reference{}, newError("ResolveURL", sdk.CodeInvalidArgument, errors.New("unsupported tag path"))
			}
			return Reference{Kind: ReferenceKindTag, CreatorID: creator, Tag: parts[3]}, nil
		default:
			return Reference{}, newError("ResolveURL", sdk.CodeInvalidArgument, errors.New("unsupported creator path depth"))
		}
	}
	return Reference{}, newError("ResolveURL", sdk.CodeInvalidArgument, errors.New("URL does not name a supported FANBOX resource"))
}

func splitPath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}
