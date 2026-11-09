package pixiv

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// Reference identifies a locally parsed Pixiv entity through its canonical URL.
// Use a type switch to access artwork, novel, user, bookmark, or series operations.
type Reference interface{ CanonicalURL() (string, error) }

// UserBookmarksRef identifies a user's artwork bookmark page and inherits user operations.
type UserBookmarksRef struct{ *UserRef }

// ParseURL returns a bound reference without network or credential-store I/O.
// It accepts HTTPS Pixiv artwork, novel, user, artwork-bookmark, and series pages,
// including locale prefixes.
func (c *Client) ParseURL(raw string) (Reference, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" || !isPixivHost(parsed.Hostname()) {
		return nil, newError("ParseURL", sdk.InvalidArgument, "URL must be HTTPS on pixiv.net without userinfo or port", nil)
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if ref := c.referencePath(parts, parsed); ref != nil {
		return ref, nil
	}
	if len(parts) > 0 && isLocaleSegment(parts[0]) {
		if ref := c.referencePath(parts[1:], parsed); ref != nil {
			return ref, nil
		}
	}
	return nil, newError("ParseURL", sdk.InvalidArgument, "URL does not name a supported Pixiv resource", nil)
}

func (c *Client) referencePath(parts []string, parsed *url.URL) Reference {
	if len(parts) < 2 {
		return nil
	}
	if parts[0] == "novel" {
		return c.novelReferencePath(parts, parsed)
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || id <= 0 {
		return nil
	}
	if len(parts) == 2 && parts[0] == "artworks" {
		return c.Artwork(id)
	}
	if len(parts) == 4 && parts[0] == "user" && parts[2] == "series" {
		seriesID, err := strconv.ParseInt(parts[3], 10, 64)
		if err != nil || seriesID <= 0 {
			return nil
		}
		ref := c.ArtworkSeries(seriesID)
		ref.OwnerUserID = id
		return ref
	}
	if parts[0] != "users" {
		return nil
	}
	if len(parts) == 2 || len(parts) == 3 && parts[2] == "artworks" {
		return c.User(id)
	}
	if len(parts) == 4 && parts[2] == "bookmarks" && parts[3] == "artworks" {
		return &UserBookmarksRef{UserRef: c.User(id)}
	}
	return nil
}

func (c *Client) novelReferencePath(parts []string, parsed *url.URL) Reference {
	var value string
	switch {
	case len(parts) == 2 && parts[1] == "show.php":
		query, err := url.ParseQuery(parsed.RawQuery)
		if err != nil || len(query["id"]) != 1 {
			return nil
		}
		value = query.Get("id")
	case len(parts) == 3 && parts[1] == "series":
		value = parts[2]
	default:
		return nil
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return nil
	}
	if parts[1] == "show.php" {
		return c.Novel(id)
	}
	return c.NovelSeries(id)
}

// CanonicalURL returns the tracking-free artwork URL; a positive ID is required.
func (r *ArtworkRef) CanonicalURL() (string, error) {
	if r == nil || r.ID <= 0 {
		return "", newError("CanonicalURL", sdk.InvalidArgument, "artwork ID must be positive", nil)
	}
	return "https://www.pixiv.net/artworks/" + strconv.FormatInt(r.ID, 10), nil
}

// CanonicalURL returns the tracking-free novel URL; a positive ID is required.
func (r *NovelRef) CanonicalURL() (string, error) {
	if r == nil || r.ID <= 0 {
		return "", newError("CanonicalURL", sdk.InvalidArgument, "novel ID must be positive", nil)
	}
	return "https://www.pixiv.net/novel/show.php?id=" + strconv.FormatInt(r.ID, 10), nil
}

// CanonicalURL returns the artwork series URL; positive series and owner IDs are required.
func (r *ArtworkSeriesRef) CanonicalURL() (string, error) {
	if r == nil || r.ID <= 0 || r.OwnerUserID <= 0 {
		return "", newError("CanonicalURL", sdk.InvalidArgument, "artwork series requires positive series and owner IDs", nil)
	}
	return "https://www.pixiv.net/user/" + strconv.FormatInt(r.OwnerUserID, 10) + "/series/" + strconv.FormatInt(r.ID, 10), nil
}

// CanonicalURL returns the tracking-free novel series URL; a positive ID is required.
func (r *NovelSeriesRef) CanonicalURL() (string, error) {
	if r == nil || r.ID <= 0 {
		return "", newError("CanonicalURL", sdk.InvalidArgument, "novel series ID must be positive", nil)
	}
	return "https://www.pixiv.net/novel/series/" + strconv.FormatInt(r.ID, 10), nil
}

// CanonicalURL returns the tracking-free user URL; a positive ID is required.
func (r *UserRef) CanonicalURL() (string, error) {
	if r == nil || r.ID <= 0 {
		return "", newError("CanonicalURL", sdk.InvalidArgument, "user ID must be positive", nil)
	}
	return "https://www.pixiv.net/users/" + strconv.FormatInt(r.ID, 10), nil
}

// CanonicalURL returns the user's artwork bookmark page; a positive ID is required.
func (r *UserBookmarksRef) CanonicalURL() (string, error) {
	if r == nil {
		return "", newError("CanonicalURL", sdk.InvalidArgument, "user ID must be positive", nil)
	}
	base, err := r.UserRef.CanonicalURL()
	if err != nil {
		return "", err
	}
	return base + "/bookmarks/artworks", nil
}

func isPixivHost(host string) bool {
	switch strings.ToLower(host) {
	case "pixiv.net", "www.pixiv.net":
		return true
	}
	return false
}

func isLocaleSegment(value string) bool {
	parts := strings.Split(value, "-")
	if len(parts) > 2 {
		return false
	}
	for _, part := range parts {
		if len(part) < 2 || len(part) > 8 {
			return false
		}
		for _, ch := range part {
			if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z') {
				return false
			}
		}
	}
	return true
}

// UnmarshalJSON restores the user identity without retaining an account binding.
func (r *UserBookmarksRef) UnmarshalJSON(data []byte) error {
	var user UserRef
	if err := json.Unmarshal(data, &user); err != nil {
		return err
	}
	r.UserRef = &user
	return nil
}
