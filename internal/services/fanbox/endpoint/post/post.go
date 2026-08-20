// Package post owns normalized FANBOX post entities and media values.
package post

// AssetKind identifies a first-party post asset.
type AssetKind string

const (
	AssetKindImage AssetKind = "image"
	AssetKindFile  AssetKind = "file"
)

// Post is a normalized post summary. Restricted posts may have a nil Body.
type Post struct {
	ID                string
	Title             string
	PublishedDateTime string
	CreatorID         string
	FeeRequired       int
	IsRestricted      bool
	IsPinned          bool
	RestrictedFor     int
	CommentCount      int
	Body              *Body
}

// Body contains the normalized post content and ordered first-party assets.
type Body struct {
	Text   string
	Images []Image
	Files  []File
	Blocks []Block
	Assets []Asset
}

// Image is a normalized FANBOX image locator.
type Image struct {
	ID           string
	Extension    string
	OriginalURL  string
	ThumbnailURL string
}

// File is a normalized FANBOX file locator.
type File struct {
	ID        string
	Name      string
	Extension string
	URL       string
}

// Block is a normalized post body block. ImageID/FileID refer to the media
// maps carried by the same body.
type Block struct {
	Type    string
	ImageID string
	FileID  string
}

// Asset is a first-party media locator that a product resource endpoint can
// open. It contains no Cookie or request policy.
type Asset struct {
	ID           string
	Kind         AssetKind
	Name         string
	Extension    string
	URL          string
	ThumbnailURL string
}

// Page is one server-provided post page and its opaque-to-the-caller
// continuation URL. Public SDK cursors wrap this value before returning it.
type Page struct {
	Posts   []Post
	NextURL string
}
