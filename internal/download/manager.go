package download

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/filename"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/ids"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/parallel"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/text"
	uriutil "github.com/FlanChanXwO/pixiv-cli/internal/utils/uri"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// PixivClient 保留原有名称供内部调用方源码兼容；能力边界由 application 统一拥有。
type PixivClient = application.DownloadClient

// 下载结果属于应用层稳定 DTO；alias 让现有 MCP/下载包调用方保持源码兼容。
type DownloadedArtwork = application.DownloadedArtwork
type DownloadedFile = application.DownloadedFile

type Manager struct {
	client           PixivClient
	ugoiraEncoder    UgoiraEncoder
	downloadPath     string
	filenameTemplate string
	mu               sync.RWMutex
}

func NewManager(client PixivClient, downloadPath, filenameTemplate string) *Manager {
	return &Manager{
		client:           client,
		ugoiraEncoder:    defaultUgoiraEncoder(),
		downloadPath:     downloadPath,
		filenameTemplate: filenameTemplate,
	}
}

// SetUgoiraEncoder 设置动图编码器，供启动装配和聚焦测试替换。
func (m *Manager) SetUgoiraEncoder(encoder UgoiraEncoder) {
	if encoder != nil {
		m.ugoiraEncoder = encoder
	}
}

func (m *Manager) SetDownloadPath(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.downloadPath = path
	return nil
}

func (m *Manager) DownloadPath() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.downloadPath
}

// FilenameTemplate 返回当前由运行配置提供的作品命名模板。
func (m *Manager) FilenameTemplate() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.filenameTemplate
}

func (m *Manager) Download(ctx context.Context, request application.DownloadRequest) ([]DownloadedArtwork, error) {
	unique := ids.DeduplicatePositive(request.IllustIDs)
	quality := request.Quality
	if quality == "" {
		quality = application.DownloadQualityOriginal
	}
	if err := application.ValidateDownloadQuality(quality); err != nil {
		return nil, err
	}
	ugoiraFormat := request.UgoiraFormat
	if ugoiraFormat == "" {
		ugoiraFormat = application.UgoiraFormatGIF
	}
	if err := application.ValidateUgoiraFormat(ugoiraFormat); err != nil {
		return nil, err
	}
	artworks := make([]DownloadedArtwork, len(unique))
	errs := make([]error, len(unique))
	if err := parallel.ForEach(ctx, len(unique), func(ctx context.Context, index int) {
		artworks[index], errs[index] = m.downloadArtwork(ctx, unique[index], request.Pages, quality, ugoiraFormat)
	}); err != nil {
		return nil, err
	}
	for index, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("download illust %d: %w", unique[index], err)
		}
	}
	return artworks, nil
}

func (m *Manager) downloadArtwork(ctx context.Context, id int64, pages []int, quality application.DownloadQuality, ugoiraFormat application.UgoiraFormat) (out DownloadedArtwork, err error) {
	artwork, err := m.client.Artwork(ctx, pixiv.ArtworkRequest{ArtworkID: id})
	if err != nil {
		return DownloadedArtwork{}, err
	}
	base := m.DownloadPath()
	base, err = filepath.Abs(base)
	if err != nil {
		return DownloadedArtwork{}, err
	}
	kind := string(artwork.Kind)
	if artwork.PageCount > 1 || kind == string(pixiv.ArtworkKindUgoira) {
		base = filepath.Join(base, filename.Sanitize(fmt.Sprintf("%d - %s", artwork.ID, text.DefaultString(artwork.Title, "Untitled"))))
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return DownloadedArtwork{}, err
	}

	artworkOut := DownloadedArtwork{
		IllustID: artwork.ID,
		Title:    artwork.Title,
		Author:   artwork.User.Name,
		Type:     kind,
	}

	if kind == string(pixiv.ArtworkKindUgoira) {
		// Ugoira 只支持原始 GIF/APNG 流程；派生质量或页选择显式 unsupported。
		if quality != application.DownloadQualityOriginal {
			return DownloadedArtwork{}, fmt.Errorf("ugoira quality %q is unsupported; only original is supported", quality)
		}
		if len(pages) > 0 {
			return DownloadedArtwork{}, fmt.Errorf("ugoira page selection is unsupported")
		}
		path, err := m.downloadUgoira(ctx, artwork, base, ugoiraFormat)
		if err != nil {
			return DownloadedArtwork{}, err
		}
		artworkOut.Files = append(artworkOut.Files, DownloadedFile{Path: path, Page: 1})
		return artworkOut, nil
	}

	selected, err := selectStaticPages(artwork, pages)
	if err != nil {
		return DownloadedArtwork{}, err
	}
	for _, item := range selected {
		rawURL := item.image.Image.Resource.URL
		if rawURL == "" {
			return DownloadedArtwork{}, fmt.Errorf("illust %d page %d has no image URL", artwork.ID, item.page1)
		}
		// 文件名页索引仍用 0-based，保持既有模板语义；DownloadedFile.Page 改为 1-based 用户页号。
		path := filepath.Join(base, filename.Generate(filenameData(artwork), item.page1-1, m.filenameTemplate)+downloadExtension(rawURL))
		if err := m.saveResource(ctx, item.image.Image.Resource.Ref, path); err != nil {
			return DownloadedArtwork{}, err
		}
		artworkOut.Files = append(artworkOut.Files, DownloadedFile{Path: path, Page: item.page1})
	}
	return artworkOut, nil
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

func (m *Manager) downloadUgoira(ctx context.Context, artwork pixiv.Artwork, base string, format application.UgoiraFormat) (string, error) {
	meta, err := m.client.UgoiraMetadata(ctx, pixiv.UgoiraMetadataRequest{ArtworkID: artwork.ID})
	if err != nil {
		return "", err
	}
	archive := selectUgoiraArchive(meta)
	if archive == nil || archive.Resource.URL == "" {
		return "", fmt.Errorf("ugoira %d has no downloadable archive", artwork.ID)
	}
	zipFile, err := os.CreateTemp(base, "ugoira-*.zip")
	if err != nil {
		return "", err
	}
	zipPath := zipFile.Name()
	if err := zipFile.Close(); err != nil {
		return "", err
	}
	defer os.Remove(zipPath)
	if err := m.saveResource(ctx, archive.Resource.Ref, zipPath); err != nil {
		return "", err
	}
	outPath := filepath.Join(base, filename.Generate(filenameData(artwork), 0, m.filenameTemplate)+"."+string(format))
	if err := m.convertUgoira(ctx, zipPath, meta.Frames, base, outPath, AnimationFormat(format)); err != nil {
		return "", err
	}
	return outPath, nil
}

// selectUgoiraArchive 优先 original，缺失时退回 medium。
func selectUgoiraArchive(meta pixiv.UgoiraMetadata) *pixiv.UgoiraArchive {
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

func (m *Manager) saveResource(ctx context.Context, ref sdk.ResourceRef, path string) error {
	_, err := m.client.SaveResource(ctx, ref, sdk.SaveOptions{Path: path})
	return err
}

func (m *Manager) ConvertUgoira(ctx context.Context, zipPath string, frames []pixiv.UgoiraFrame, workDir, outputGIF string) error {
	return m.convertUgoira(ctx, zipPath, frames, workDir, outputGIF, AnimationFormatGIF)
}

// convertUgoira 保留 ConvertUgoira 的 GIF 兼容入口，同时让 DownloadRequest 显式选择 APNG。
func (m *Manager) convertUgoira(ctx context.Context, zipPath string, frames []pixiv.UgoiraFrame, workDir, outputPath string, format AnimationFormat) error {
	if m.ugoiraEncoder == nil {
		return fmt.Errorf("ugoira encoder is not configured")
	}
	return m.ugoiraEncoder.Encode(ctx, UgoiraEncodeInput{
		ZipPath:    zipPath,
		Frames:     frames,
		WorkDir:    workDir,
		OutputPath: outputPath,
		Format:     format,
	})
}

// downloadExtension 清理 URL path 推导出的扩展名，避免跨平台非法文件名字符
// 绕过作品标题和模板已有的文件名规范化边界。ASCII C0/DEL 控制字符
// 不适合作为文件名内容，Windows 还不接受尾随点或空格，因而在扩展名边界统一处理。
func downloadExtension(rawURL string) string {
	extension := filename.Sanitize(filepath.Ext(uriutil.PathFromURL(rawURL)))
	extension = strings.Map(func(character rune) rune {
		if character < 0x20 || character == 0x7f {
			return '_'
		}
		return character
	}, extension)
	return strings.TrimRight(extension, ". ")
}

func filenameData(artwork pixiv.Artwork) filename.FilenameData {
	return filename.FilenameData{
		ID:        artwork.ID,
		Author:    artwork.User.Name,
		Title:     artwork.Title,
		PageCount: artwork.PageCount,
	}
}
