package download

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/FlanChanXwO/pixiv-cli/internal/media/ugoira"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/atomicfile"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

const archiveFilename = "frames.zip"
const archiveDirectoryPattern = ".ugoira-archive-*"
const directoryPermissions = 0o755

func saveUgoira(ctx context.Context, art *pixiv.Artwork, base, name string, format UgoiraFormat) (output string, returnErr error) {
	metadata, err := art.UgoiraMetadata(ctx)
	if err != nil {
		return "", err
	}
	var archive *pixiv.UgoiraArchive
	for i := range metadata.Archives {
		candidate := &metadata.Archives[i]
		if archive == nil || candidate.Quality == pixiv.UgoiraQualityOriginal {
			archive = candidate
		}
		if candidate.Quality == pixiv.UgoiraQualityOriginal {
			break
		}
	}
	if archive == nil {
		return "", invalidArgument("ugoira has no downloadable archive")
	}
	body, err := archive.Open(ctx)
	if err != nil {
		return "", err
	}
	defer func() { returnErr = errors.Join(returnErr, body.Close()) }()
	if err := os.MkdirAll(base, directoryPermissions); err != nil {
		return "", err
	}
	work, err := os.MkdirTemp(base, archiveDirectoryPattern)
	if err != nil {
		return "", err
	}
	defer func() { returnErr = errors.Join(returnErr, os.RemoveAll(work)) }()
	zipPath := filepath.Join(work, archiveFilename)
	if _, err := atomicfile.AtomicWrite(ctx, zipPath, body); err != nil {
		return "", err
	}
	frames := make([]ugoira.Frame, len(metadata.Frames))
	for i, frame := range metadata.Frames {
		frames[i] = ugoira.Frame{File: frame.Filename, Delay: frame.DelayMilliseconds}
	}
	output = filepath.Join(base, name+"."+string(format))
	if err := ugoira.NewEncoder().Encode(ctx, ugoira.Input{ZipPath: zipPath, Frames: frames, WorkDir: work, OutputPath: output, Format: ugoira.Format(format)}); err != nil {
		return "", err
	}
	return output, nil
}
