package downloader

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/FlanChanXwO/pixiv-cli/internal/media/filename"
	"github.com/FlanChanXwO/pixiv-cli/internal/media/ugoira"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func (b *ArtworkDownloader) downloadUgoira(ctx context.Context, artwork pixiv.Artwork, base string, format UgoiraFormat) (outputPath string, warning *DownloadWarning, returnErr error) {
	data := filenameFields(artwork)
	basename, generationErr := filename.GenerateChecked(data, 0, b.filenameTemplate)
	if generationErr != nil || basename == "" {
		fallback, fallbackErr := filename.GenerateChecked(data, 0, "")
		if fallbackErr != nil {
			return "", nil, fmt.Errorf("ugoira filename fallback failed: %w", fallbackErr)
		}
		if fallback == "" {
			return "", nil, errors.New("ugoira filename fallback produced an empty name")
		}
		message := "ugoira filename template failed; using default filename"
		if generationErr == nil {
			message = "ugoira filename template produced an empty name; using default filename"
		}
		warning = &DownloadWarning{
			IllustID: artwork.ID,
			Type:     string(pixiv.ContentTypeUgoira),
			Message:  message,
		}
		basename = fallback
	}

	meta, err := b.client.UgoiraMetadata(ctx, pixiv.UgoiraMetadataRequest{ArtworkID: artwork.ID})
	if err != nil {
		return "", warning, err
	}
	archive := SelectUgoiraArchive(meta)
	if archive == nil || archive.Resource.URL == "" {
		return "", warning, fmt.Errorf("ugoira %d has no downloadable archive", artwork.ID)
	}
	zipFile, err := os.CreateTemp(base, "ugoira-*.zip")
	if err != nil {
		return "", warning, err
	}
	zipPath := zipFile.Name()
	if err := zipFile.Close(); err != nil {
		return "", warning, errors.Join(err, os.Remove(zipPath))
	}
	defer func() {
		if err := os.Remove(zipPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			returnErr = errors.Join(returnErr, err)
		}
	}()
	if _, err := b.client.SaveResource(ctx, archive.Resource.Ref, sdk.SaveOptions{Path: zipPath}); err != nil {
		return "", warning, err
	}
	outPath := filepath.Join(base, basename+"."+string(format))
	if err := encodeUgoira(ctx, zipPath, meta.Frames, base, outPath, ugoira.Format(format)); err != nil {
		return "", warning, err
	}
	return outPath, warning, nil
}

// SelectUgoiraArchive 优先 original，缺失时退回 medium。
func SelectUgoiraArchive(meta pixiv.UgoiraMetadata) *pixiv.UgoiraArchive {
	var fallback *pixiv.UgoiraArchive
	for index := range meta.Archives {
		archive := &meta.Archives[index]
		if archive.Quality == pixiv.UgoiraQualityOriginal {
			return archive
		}
		if fallback == nil {
			fallback = archive
		}
	}
	return fallback
}

func encodeUgoira(ctx context.Context, zipPath string, frames []pixiv.UgoiraFrame, workDir, outputPath string, format ugoira.Format) error {
	ugoiraFrames := make([]ugoira.Frame, len(frames))
	for index, frame := range frames {
		ugoiraFrames[index] = ugoira.Frame{File: frame.Filename, Delay: frame.DelayMilliseconds}
	}
	return ugoira.NewEncoder().Encode(ctx, ugoira.Input{
		ZipPath:    zipPath,
		Frames:     ugoiraFrames,
		WorkDir:    workDir,
		OutputPath: outputPath,
		Format:     format,
	})
}

// resourceForQuality 把公开 DownloadQuality 映射到 SDK 的 artwork variant。
// original 复用页面既有的 Resource；其余质量请求对应变体的 ResourceRef，
// SaveResource 会在 revalidate 路径上按 variant 重新解析 locator。
// DownloadQuality 的取值本身就是上游 variant 名。
func resourceForQuality(image pixiv.ImageResource, quality DownloadQuality) (sdk.ResourceRef, error) {
	if quality == "" || quality == DownloadQualityOriginal {
		return image.Resource.Ref, nil
	}
	return pixiv.ArtworkVariantResource(image.Resource, string(quality))
}

type staticPage struct {
	page1 int
	image pixiv.ArtworkPage
}

// selectStaticPages 返回按自然页序排列的 1-based 页。pages 为空表示全部。
func selectStaticPages(artwork pixiv.Artwork, pages []int) ([]staticPage, error) {
	total := artwork.PageCount
	if total <= 0 {
		total = len(artwork.Pages)
	}
	if total <= 0 || len(artwork.Pages) == 0 {
		return nil, fmt.Errorf("illust %d has no page metadata", artwork.ID)
	}
	want := map[int]struct{}{}
	if len(pages) == 0 {
		for i := 1; i <= len(artwork.Pages); i++ {
			want[i] = struct{}{}
		}
	} else {
		for _, page := range pages {
			if page < 1 || page > total || page > len(artwork.Pages) {
				return nil, fmt.Errorf("page %d does not exist (page_count=%d)", page, total)
			}
			want[page] = struct{}{}
		}
	}
	var selected []staticPage
	for i, page := range artwork.Pages {
		page1 := i + 1
		if _, ok := want[page1]; !ok {
			continue
		}
		selected = append(selected, staticPage{page1: page1, image: page})
	}
	return selected, nil
}
