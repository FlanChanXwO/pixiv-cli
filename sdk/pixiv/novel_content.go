package pixiv

import "encoding/json"

// NovelBlockFormat identifies a structured novel block's representation.
type NovelBlockFormat string

const (
	// NovelBlockParagraph carries a paragraph with inline marks.
	NovelBlockParagraph NovelBlockFormat = "paragraph"
	// NovelBlockHeader carries a section heading.
	NovelBlockHeader NovelBlockFormat = "header"
	// NovelBlockImage carries a client-bound image.
	NovelBlockImage NovelBlockFormat = "image"
	// NovelBlockFile carries a client-bound attachment.
	NovelBlockFile NovelBlockFormat = "file"
	// NovelBlockUnknown retains safe scalar metadata for an unrecognized block.
	NovelBlockUnknown NovelBlockFormat = "unknown"
)

// NovelMarkStyle identifies the formatting applied to an inline text range.
type NovelMarkStyle string

const (
	// NovelMarkStrong marks bold text.
	NovelMarkStrong NovelMarkStyle = "strong"
	// NovelMarkEmphasis marks emphasized text.
	NovelMarkEmphasis NovelMarkStyle = "emphasis"
	// NovelMarkDelete marks deleted text.
	NovelMarkDelete NovelMarkStyle = "delete"
	// NovelMarkRuby carries a base text and its reading.
	NovelMarkRuby NovelMarkStyle = "ruby"
	// NovelMarkLink carries a link's text and original destination.
	NovelMarkLink NovelMarkStyle = "link"
	// NovelMarkCustom preserves Pixiv's custom inline styling.
	NovelMarkCustom NovelMarkStyle = "custom"
	// NovelMarkUnknown preserves unrecognized inline styling.
	NovelMarkUnknown NovelMarkStyle = "unknown"
)

// NovelRuby preserves the base text and its furigana reading.
type NovelRuby struct {
	Text     string `json:"text"`
	Furigana string `json:"furigana"`
}

// NovelMark records inline formatting in document order, including nested marks.
// Href retains source data; callers rendering links must apply their own URL policy.
type NovelMark struct {
	Style NovelMarkStyle `json:"style"`
	Text  string         `json:"text"`
	Start int            `json:"start"` // Inclusive UTF-8 byte offset in the containing block's Text.
	End   int            `json:"end"`   // Exclusive UTF-8 byte offset; nested marks have overlapping ranges.
	Ruby  *NovelRuby     `json:"ruby,omitempty"`
	Href  string         `json:"href,omitempty"`
	Class string         `json:"class,omitempty"`
}

// NovelImageBlock holds a body image and its caption.
type NovelImageBlock struct {
	Image   Image  `json:"image"`
	Caption string `json:"caption"`
}

// UnmarshalJSON decodes an image block without retaining an earlier media binding.
func (b *NovelImageBlock) UnmarshalJSON(data []byte) error {
	type value NovelImageBlock
	var decoded value
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*b = NovelImageBlock(decoded)
	return nil
}

// NovelFileBlock holds a body attachment and its caption.
type NovelFileBlock struct {
	File    File   `json:"file"`
	Caption string `json:"caption"`
}

// UnmarshalJSON decodes an attachment block without retaining an earlier media binding.
func (b *NovelFileBlock) UnmarshalJSON(data []byte) error {
	type value NovelFileBlock
	var decoded value
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*b = NovelFileBlock(decoded)
	return nil
}

// NovelUnknownBlock retains an upstream element identifier and otherwise unclaimed text.
type NovelUnknownBlock struct {
	Source  string            `json:"source"`
	Payload map[string]string `json:"payload"`
}

// NovelBlock carries one paragraph, heading, image, attachment, or unknown block.
type NovelBlock struct {
	Format  NovelBlockFormat   `json:"format"`
	Text    string             `json:"text"`
	Marks   []NovelMark        `json:"marks"`
	Image   *NovelImageBlock   `json:"image,omitempty"`
	File    *NovelFileBlock    `json:"file,omitempty"`
	Unknown *NovelUnknownBlock `json:"unknown,omitempty"`
}

// UnmarshalJSON replaces the block, clearing any previous images, files, and account bindings.
func (b *NovelBlock) UnmarshalJSON(data []byte) error {
	type value NovelBlock
	var decoded value
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*b = NovelBlock(decoded)
	return nil
}

// NovelContent preserves the ordered structured body without exposing raw HTML.
// A parsing error returns no partial content.
type NovelContent struct {
	NovelID int64        `json:"novel_id"`
	Title   string       `json:"title"`
	Caption string       `json:"caption"`
	Blocks  []NovelBlock `json:"blocks"`
}

// UnmarshalJSON replaces the body with unbound blocks, including when blocks are omitted.
func (c *NovelContent) UnmarshalJSON(data []byte) error {
	type value NovelContent
	var decoded value
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*c = NovelContent(decoded)
	return nil
}
