package pixiv

import "github.com/FlanChanXwO/pixiv-cli/sdk"

// NovelBlockKind is the structured kind of a novel content block.
type NovelBlockKind string

// NovelBlockKind values define the supported NovelBlockKind filesystem.
const (
	NovelBlockParagraph NovelBlockKind = "paragraph"
	NovelBlockHeader    NovelBlockKind = "header"
	NovelBlockImage     NovelBlockKind = "image"
	NovelBlockFile      NovelBlockKind = "file"
	// NovelBlockUnknown preserves a block whose type the SDK does not yet
	// recognize; its raw type identifier and safe payload are retained.
	NovelBlockUnknown NovelBlockKind = "unknown"
)

// NovelMarkKind is the structured kind of an inline mark within a block.
type NovelMarkKind string

// NovelMarkKind values define the supported NovelMarkKind filesystem.
const (
	NovelMarkStrong   NovelMarkKind = "strong"
	NovelMarkEmphasis NovelMarkKind = "emphasis"
	NovelMarkDelete   NovelMarkKind = "delete"
	NovelMarkRuby     NovelMarkKind = "ruby"
	NovelMarkLink     NovelMarkKind = "link"
	NovelMarkCustom   NovelMarkKind = "custom"
	NovelMarkUnknown  NovelMarkKind = "unknown"
)

// NovelRuby is a ruby annotation with its base text and furigana reading.
type NovelRuby struct {
	Text     string
	Furigana string
}

// NovelMark is one inline mark within a block. Depending on Kind, only the
// relevant fields are set: Ruby for ruby, Href for link, Class for custom.
type NovelMark struct {
	Kind  NovelMarkKind
	Text  string
	Ruby  *NovelRuby
	Href  string
	Class string
}

// NovelImageBlock is a first-party image block.
type NovelImageBlock struct {
	Resource sdk.Resource
	Caption  string
	Width    int
	Height   int
}

// NovelFileBlock is a first-party file block.
type NovelFileBlock struct {
	Resource sdk.Resource
	Filename string
	Caption  string
	Size     int64
}

// NovelUnknownBlock retains a block whose type is not yet recognized. RawType
// preserves the upstream type identifier and Payload carries only safe, scalar
// metadata; it never carries raw HTML or executable content.
type NovelUnknownBlock struct {
	RawType string
	Payload map[string]string
}

// NovelBlock is one structured block of novel content. Text blocks carry inline
// Marks; image and file blocks carry a resource plus safe metadata. Unknown
// blocks are preserved so no body is lost.
type NovelBlock struct {
	Kind    NovelBlockKind
	Text    string
	Marks   []NovelMark
	Image   *NovelImageBlock
	File    *NovelFileBlock
	Unknown *NovelUnknownBlock
}

// NovelContent is the structured body of a novel. It never exposes raw HTML.
// Blocks preserve the upstream order and full semantics; unparseable content
// fails explicitly with MalformedUpstreamResponse instead of returning a
// partial body.
type NovelContent struct {
	NovelID int64
	Title   string
	Caption string
	Blocks  []NovelBlock
}
