package pixiv

import "github.com/FlanChanXwO/pixiv-cli/sdk"

// UgoiraQuality identifies a ugoira archive quality offered by upstream.
type UgoiraQuality string

// UgoiraQuality values define the supported UgoiraQuality filesystem.
const (
	// UgoiraQualityMedium is the reduced-size archive, when upstream provides
	// one.
	UgoiraQualityMedium UgoiraQuality = "medium"
	// UgoiraQualityOriginal is the full-resolution archive. When upstream has no
	// original archive, medium is never presented as original.
	UgoiraQualityOriginal UgoiraQuality = "original"
)

// UgoiraArchive is one downloadable ugoira archive at a specific quality. The
// Resource allows direct streaming or a safe OpenResource/SaveResource read.
type UgoiraArchive struct {
	Quality  UgoiraQuality
	Resource sdk.Resource
}

// UgoiraFrame is one frame of a ugoira animation. Filename is a relative,
// non-empty archive entry name without traversal, unique within the metadata
// result. DelayMilliseconds preserves the upstream millisecond integer without
// conversion to a frame rate.
type UgoiraFrame struct {
	Filename          string
	DelayMilliseconds int
}

// UgoiraMetadata describes the playable animation of a ugoira artwork. Frame
// order is the playback and ZIP-unpack mapping order. Unknown quality strings
// are preserved as-is in UgoiraArchive.Quality so additive upstream capability
// remains representable.
type UgoiraMetadata struct {
	ArtworkID int64
	Archives  []UgoiraArchive
	Frames    []UgoiraFrame
}
