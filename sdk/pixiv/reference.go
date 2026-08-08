package pixiv

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// ReferenceKind identifies the stable Pixiv page type a parsed URL points to.
type ReferenceKind string

// ReferenceKind values define the supported ReferenceKind filesystem.
const (
	ReferenceKindArtwork       ReferenceKind = "artwork"
	ReferenceKindNovel         ReferenceKind = "novel"
	ReferenceKindUser          ReferenceKind = "user"
	ReferenceKindUserBookmarks ReferenceKind = "user_bookmarks"
	ReferenceKindArtworkSeries ReferenceKind = "artwork_series"
	ReferenceKindNovelSeries   ReferenceKind = "novel_series"
)

// Reference is a locally parsed Pixiv resource reference. ID is always the
// positive ID of the entity named by Kind; OwnerUserID is non-zero only when the
// source URL explicitly carried the author ID (artwork series pages). The
// reference never retains the original URL, query, or fragment.
type Reference struct {
	Kind        ReferenceKind
	ID          int64
	OwnerUserID int64
}

// ParseURL parses a stable Pixiv page URL into a Reference. It performs no
// network I/O, follows no redirects, and accepts only HTTPS URLs on pixiv.net
// or www.pixiv.net without userinfo or an explicit port. Unknown paths, missing
// or duplicate IDs, non-decimal or non-positive IDs, host confusion, and bare
// integers (which cannot be disambiguated between artwork, novel, user, or
// series) return InvalidArgument. Supported official forms and their locale
// path prefixes are covered by the v1 contract.
func ParseURL(rawURL string) (Reference, error) {
	value := strings.TrimSpace(rawURL)
	if value == "" {
		return Reference{}, newError("ParseURL", sdk.InvalidArgument, "empty URL")
	}
	if isBareInteger(value) {
		return Reference{}, newError("ParseURL", sdk.InvalidArgument, "bare integer cannot be resolved to a resource type")
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return Reference{}, newError("ParseURL", sdk.InvalidArgument, "URL is not parseable")
	}
	if parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" || !isPixivHost(parsed.Hostname()) {
		return Reference{}, newError("ParseURL", sdk.InvalidArgument, "URL must be https on pixiv.net without userinfo or port")
	}
	parts := splitPath(parsed.Path)
	ref, ok := parseReferencePath(parts, parsed)
	if ok {
		return ref, nil
	}
	if len(parts) > 0 && isLocaleSegment(parts[0]) {
		if ref, ok := parseReferencePath(parts[1:], parsed); ok {
			return ref, nil
		}
	}
	return Reference{}, newError("ParseURL", sdk.InvalidArgument, "URL does not name a supported Pixiv resource")
}

// CanonicalURL returns the canonical, tracking-free Pixiv page URL for the
// reference. Invalid or zero references return InvalidArgument.
func (r Reference) CanonicalURL() (string, error) {
	if r.ID <= 0 {
		return "", newError("ParseURL", sdk.InvalidArgument, "reference has no positive ID")
	}
	switch r.Kind {
	case ReferenceKindArtwork:
		return "https://www.pixiv.net/artworks/" + strconv.FormatInt(r.ID, 10), nil
	case ReferenceKindNovel:
		return "https://www.pixiv.net/novel/show.php?id=" + strconv.FormatInt(r.ID, 10), nil
	case ReferenceKindUser:
		return "https://www.pixiv.net/users/" + strconv.FormatInt(r.ID, 10), nil
	case ReferenceKindUserBookmarks:
		return "https://www.pixiv.net/users/" + strconv.FormatInt(r.ID, 10) + "/bookmarks/artworks", nil
	case ReferenceKindArtworkSeries:
		if r.OwnerUserID <= 0 {
			return "", newError("ParseURL", sdk.InvalidArgument, "artwork series reference requires an owner user ID")
		}
		return "https://www.pixiv.net/user/" + strconv.FormatInt(r.OwnerUserID, 10) + "/series/" + strconv.FormatInt(r.ID, 10), nil
	case ReferenceKindNovelSeries:
		return "https://www.pixiv.net/novel/series/" + strconv.FormatInt(r.ID, 10), nil
	default:
		return "", newError("ParseURL", sdk.InvalidArgument, "unknown reference kind")
	}
}

func parseReferencePath(parts []string, parsed *url.URL) (Reference, bool) {
	if len(parts) == 2 && parts[0] == "artworks" {
		if id, ok := parsePositiveID(parts[1]); ok {
			return Reference{Kind: ReferenceKindArtwork, ID: id}, true
		}
		return Reference{}, false
	}
	if len(parts) == 2 && parts[0] == "novel" && parts[1] == "show.php" {
		if id, ok := parseSinglePositiveQueryID(parsed, "id"); ok {
			return Reference{Kind: ReferenceKindNovel, ID: id}, true
		}
		return Reference{}, false
	}
	if len(parts) == 2 && parts[0] == "users" {
		if id, ok := parsePositiveID(parts[1]); ok {
			return Reference{Kind: ReferenceKindUser, ID: id}, true
		}
		return Reference{}, false
	}
	if len(parts) == 3 && parts[0] == "users" && parts[2] == "artworks" {
		if id, ok := parsePositiveID(parts[1]); ok {
			return Reference{Kind: ReferenceKindUser, ID: id}, true
		}
		return Reference{}, false
	}
	if len(parts) == 4 && parts[0] == "users" && parts[2] == "bookmarks" && parts[3] == "artworks" {
		if id, ok := parsePositiveID(parts[1]); ok {
			return Reference{Kind: ReferenceKindUserBookmarks, ID: id}, true
		}
		return Reference{}, false
	}
	if len(parts) == 4 && parts[0] == "user" && parts[2] == "series" {
		owner, okOwner := parsePositiveID(parts[1])
		seriesID, okSeries := parsePositiveID(parts[3])
		if okOwner && okSeries {
			return Reference{Kind: ReferenceKindArtworkSeries, ID: seriesID, OwnerUserID: owner}, true
		}
		return Reference{}, false
	}
	if len(parts) == 3 && parts[0] == "novel" && parts[1] == "series" {
		if id, ok := parsePositiveID(parts[2]); ok {
			return Reference{Kind: ReferenceKindNovelSeries, ID: id}, true
		}
		return Reference{}, false
	}
	return Reference{}, false
}

func parsePositiveID(value string) (int64, bool) {
	id, err := strconv.ParseInt(value, 10, 64)
	return id, err == nil && id > 0
}

func parseSinglePositiveQueryID(parsed *url.URL, key string) (int64, bool) {
	values := parsed.Query()[key]
	if len(values) != 1 {
		return 0, false
	}
	return parsePositiveID(values[0])
}

func splitPath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func isBareInteger(value string) bool {
	_, err := strconv.ParseInt(value, 10, 64)
	return err == nil
}

func isPixivHost(host string) bool {
	switch strings.ToLower(host) {
	case "pixiv.net", "www.pixiv.net":
		return true
	default:
		return false
	}
}

func isLocaleSegment(value string) bool {
	parts := strings.Split(value, "-")
	if len(parts) == 0 || len(parts) > 2 {
		return false
	}
	for _, part := range parts {
		if len(part) < 2 || len(part) > 8 {
			return false
		}
		for i := 0; i < len(part); i++ {
			ch := part[i]
			if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z') {
				return false
			}
		}
	}
	return true
}
