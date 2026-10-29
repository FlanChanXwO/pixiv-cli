package downloader

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/media/filename"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func (b *ArtworkDownloader) downloadArtwork(ctx context.Context, id int64, pages []int, quality DownloadQuality, ugoiraFormat UgoiraFormat, directoryTemplate string) (out DownloadedArtwork, warnings []DownloadWarning, err error) {
	artwork, err := b.client.Artwork(ctx, pixiv.ArtworkRequest{ArtworkID: id})
	if err != nil {
		return DownloadedArtwork{}, nil, err
	}
	base, err := filepath.Abs(b.downloadPath)
	if err != nil {
		return DownloadedArtwork{}, nil, err
	}
	data := filenameFields(artwork)
	if directoryTemplate != "" {
		relative, err := filename.BuildRelativeDirectory(directoryTemplate, data, 0)
		if err != nil {
			return DownloadedArtwork{}, nil, err
		}
		if relative != "" {
			base = filepath.Join(base, filepath.FromSlash(relative))
		}
	}
	kind := string(artwork.Kind)
	if directoryTemplate == "" && (artwork.PageCount > 1 || kind == string(pixiv.ArtworkKindUgoira)) {
		base = filepath.Join(base, filename.Sanitize(fmt.Sprintf("%d - %s", artwork.ID, cmp.Or(artwork.Title, "Untitled"))))
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return DownloadedArtwork{}, nil, err
	}

	artworkOut := DownloadedArtwork{
		IllustID: artwork.ID,
		Title:    artwork.Title,
		Author:   artwork.User.Name,
		Type:     kind,
	}

	if kind == string(pixiv.ArtworkKindUgoira) {
		// Ugoira 只支持原始 GIF/APNG 流程；派生质量或页选择显式 unsupported。
		if quality != DownloadQualityOriginal {
			return DownloadedArtwork{}, nil, fmt.Errorf("ugoira quality %q is unsupported; only original is supported", quality)
		}
		if len(pages) > 0 {
			return DownloadedArtwork{}, nil, fmt.Errorf("ugoira page selection is unsupported")
		}
		path, warning, err := b.downloadUgoira(ctx, artwork, base, ugoiraFormat)
		if warning != nil {
			warnings = append(warnings, *warning)
		}
		// 清理归档失败时，已发布的动画仍属于本次已提交结果。
		if path != "" {
			artworkOut.Files = append(artworkOut.Files, DownloadedFile{Path: path, Page: 1})
		}
		return artworkOut, warnings, err
	}

	selected, err := selectStaticPages(artwork, pages)
	if err != nil {
		return DownloadedArtwork{}, nil, err
	}
	if err := filename.ValidateTemplate(b.filenameTemplate); err != nil {
		return DownloadedArtwork{}, nil, err
	}
	for _, item := range selected {
		rawURL := item.image.Image.Resource.URL
		if rawURL == "" {
			return artworkOut, nil, fmt.Errorf("illust %d page %d has no image URL", artwork.ID, item.page1)
		}
		// GenerateChecked 在模板使用 {date} 但 CreateDate 缺失时返回错误，
		// 这样在网络请求前就把未知占位符或不匹配花括号暴露为失败，而不是
		// 写出空文件名并相互覆盖。
		basename, err := filename.GenerateChecked(data, item.page1-1, b.filenameTemplate)
		if err != nil {
			return artworkOut, nil, err
		}
		if basename == "" {
			return artworkOut, nil, fmt.Errorf("illust %d page %d produced an empty filename", artwork.ID, item.page1)
		}
		ref, err := resourceForQuality(item.image.Image, quality)
		if err != nil {
			return artworkOut, nil, err
		}
		path := filepath.Join(base, basename+downloadExtension(rawURL))
		saved, err := b.client.SaveResource(ctx, ref, sdk.SaveOptions{Path: path})
		if err != nil {
			return artworkOut, nil, err
		}
		// 所有静态 quality 都基于本次响应的实际类型发布最终扩展名，
		// 不能再让 regular/small/original 继续沿用 original URL 的后缀。
		path, err = publishDetectedImageExtension(path, saved.ContentType)
		if err != nil {
			if cleanupErr := os.Remove(saved.Path); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
				return artworkOut, nil, errors.Join(err, fmt.Errorf("remove invalid downloaded file: %w", cleanupErr))
			}
			return artworkOut, nil, err
		}
		artworkOut.Files = append(artworkOut.Files, DownloadedFile{Path: path, Page: item.page1})
	}
	return artworkOut, nil, nil
}

func filenameFields(artwork pixiv.Artwork) filename.Fields {
	tags := make([]string, 0, len(artwork.Tags))
	for _, tag := range artwork.Tags {
		if name := strings.TrimSpace(tag.Name); name != "" {
			tags = append(tags, name)
		}
	}
	createDate := ""
	if !artwork.PublishedAt.IsZero() {
		createDate = artwork.PublishedAt.Format(time.RFC3339)
	}
	return filename.Fields{
		ID:         artwork.ID,
		Author:     artwork.User.Name,
		AuthorID:   artwork.User.ID,
		Title:      artwork.Title,
		CreateDate: createDate,
		Tags:       tags,
		PageCount:  artwork.PageCount,
	}
}
