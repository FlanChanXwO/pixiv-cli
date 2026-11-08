package pixiv

import (
	"context"
	"encoding/json"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// UgoiraQuality identifies the upstream archive resolution.
type UgoiraQuality string

const (
	// UgoiraQualityMedium selects the medium archive.
	UgoiraQualityMedium UgoiraQuality = "medium"
	// UgoiraQualityOriginal selects the original archive.
	UgoiraQualityOriginal UgoiraQuality = "original"
)

// UgoiraArchive exposes a ZIP through the client-bound Open and Save operations.
type UgoiraArchive struct {
	Image
	Quality UgoiraQuality `json:"quality"`
}

// UgoiraFrame preserves archive order and the upstream delay in milliseconds.
type UgoiraFrame struct {
	Filename          string `json:"filename"`
	DelayMilliseconds int    `json:"delay_milliseconds"`
}

// UgoiraMetadata carries the archives and ordered frames needed for playback.
type UgoiraMetadata struct {
	ArtworkID int64           `json:"artwork_id"`
	Archives  []UgoiraArchive `json:"archives"`
	Frames    []UgoiraFrame   `json:"frames"`
}

// UgoiraMetadata loads the animation metadata for this artwork.
func (r *ArtworkRef) UgoiraMetadata(ctx context.Context) (UgoiraMetadata, error) {
	if err := r.bound("UgoiraMetadata"); err != nil {
		return UgoiraMetadata{}, err
	}
	var raw ugoiraResponseWire
	if err := r.client.getJSON(ctx, api.PathUgoiraMetadata, url.Values{api.ParamIllustID: {strconv.FormatInt(r.ID, 10)}}, &raw); err != nil {
		return UgoiraMetadata{}, err
	}
	w := raw.Metadata.Value
	if !raw.Metadata.Present || !raw.Metadata.Valid || !w.ZipURLs.Present || !w.ZipURLs.Valid ||
		!w.Frames.Present || !w.Frames.Valid || len(w.Frames.Items) == 0 || w.ZipURLs.Value.Medium == "" && w.ZipURLs.Value.Original == "" {
		return UgoiraMetadata{}, newError("UgoiraMetadata", sdk.MalformedUpstreamResponse, "missing animation metadata", nil)
	}
	result := UgoiraMetadata{ArtworkID: r.ID, Archives: []UgoiraArchive{}, Frames: make([]UgoiraFrame, len(w.Frames.Items))}
	for _, candidate := range []struct {
		quality UgoiraQuality
		raw     string
	}{{UgoiraQualityMedium, w.ZipURLs.Value.Medium}, {UgoiraQualityOriginal, w.ZipURLs.Value.Original}} {
		if candidate.raw == "" {
			continue
		}
		image, err := r.client.image(candidate.raw, 0, 0)
		if err != nil {
			return UgoiraMetadata{}, err
		}
		result.Archives = append(result.Archives, UgoiraArchive{Image: image, Quality: candidate.quality})
	}
	seen := map[string]bool{}
	for i, frame := range w.Frames.Items {
		if frame.Delay < 0 {
			return UgoiraMetadata{}, newError("UgoiraMetadata", sdk.MalformedUpstreamResponse, "negative animation frame delay", nil)
		}
		if !safeArchiveFilename(frame.File) || seen[frame.File] {
			return UgoiraMetadata{}, newError("UgoiraMetadata", sdk.MalformedUpstreamResponse, "invalid animation frame filename", nil)
		}
		seen[frame.File] = true
		result.Frames[i] = UgoiraFrame{Filename: frame.File, DelayMilliseconds: frame.Delay}
	}
	return result, nil
}

func safeArchiveFilename(name string) bool {
	if name == "" || strings.ContainsAny(name, "\\\x00") || path.IsAbs(name) || len(name) >= 2 && name[1] == ':' {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

// UnmarshalJSON restores archive data without retaining a previous client binding.
func (a *UgoiraArchive) UnmarshalJSON(data []byte) error {
	var value struct {
		URL     string        `json:"url"`
		Width   int           `json:"width"`
		Height  int           `json:"height"`
		Quality UgoiraQuality `json:"quality"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*a = UgoiraArchive{Image: Image{URL: value.URL, Width: value.Width, Height: value.Height}, Quality: value.Quality}
	return nil
}
