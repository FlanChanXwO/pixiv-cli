package download

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/filename"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/ids"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/parallel"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/text"
	uriutil "github.com/FlanChanXwO/pixiv-cli/internal/utils/uri"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
)

// PixivClient 保留原有名称供内部调用方源码兼容；能力边界由 application 统一拥有。
type PixivClient = application.DownloadClient

// 下载结果属于应用层稳定 DTO；alias 让现有 MCP/下载包调用方保持源码兼容。
type DownloadedArtwork = application.DownloadedArtwork
type DownloadedFile = application.DownloadedFile

type Manager struct {
	client           PixivClient
	logger           *slog.Logger
	ugoiraEncoder    UgoiraEncoder
	downloadPath     string
	filenameTemplate string
	mu               sync.RWMutex
}

func NewManager(client PixivClient, logger *slog.Logger, downloadPath, filenameTemplate string) *Manager {
	return &Manager{
		client:           client,
		logger:           logging.OrDiscard(logger),
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
	started := time.Now()
	defer func() { m.operationLog("download", started, err, id) }()
	detail, err := m.client.IllustDetail(ctx, id)
	if err != nil {
		return DownloadedArtwork{}, err
	}
	illust := detail.Illust
	base := m.DownloadPath()
	base, err = filepath.Abs(base)
	if err != nil {
		return DownloadedArtwork{}, err
	}
	if illust.PageCount > 1 || illust.Type == string(sdk.IllustTypeUgoira) {
		base = filepath.Join(base, filename.Sanitize(fmt.Sprintf("%d - %s", illust.ID, text.DefaultString(illust.Title, "Untitled"))))
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return DownloadedArtwork{}, err
	}

	artwork := DownloadedArtwork{
		IllustID: illust.ID,
		Title:    illust.Title,
		Author:   illust.User.Name,
		Type:     illust.Type,
	}

	if illust.Type == string(sdk.IllustTypeUgoira) {
		// Ugoira 只支持现有原始 GIF/APNG 流程；派生质量或页选择显式 unsupported。
		if quality != application.DownloadQualityOriginal {
			return DownloadedArtwork{}, fmt.Errorf("ugoira quality %q is unsupported; only original is supported", quality)
		}
		if len(pages) > 0 {
			return DownloadedArtwork{}, fmt.Errorf("ugoira page selection is unsupported")
		}
		path, err := m.downloadUgoira(ctx, illust, base, ugoiraFormat)
		if err != nil {
			return DownloadedArtwork{}, err
		}
		artwork.Files = append(artwork.Files, DownloadedFile{Path: path, Page: 1})
		return artwork, nil
	}

	selected, err := selectStaticPages(illust, pages)
	if err != nil {
		return DownloadedArtwork{}, err
	}
	for _, item := range selected {
		rawURL, err := selectImageURL(item.urls, item.singleOriginal, quality)
		if err != nil {
			return DownloadedArtwork{}, fmt.Errorf("illust %d page %d: %w", illust.ID, item.page1, err)
		}
		// 文件名页索引仍用 0-based，保持既有模板语义；DownloadedFile.Page 改为 1-based 用户页号。
		path := filepath.Join(base, filename.Generate(filenameData(illust), item.page1-1, m.filenameTemplate)+downloadExtension(rawURL))
		if err := m.downloadURL(ctx, rawURL, path); err != nil {
			return DownloadedArtwork{}, err
		}
		artwork.Files = append(artwork.Files, DownloadedFile{Path: path, Page: item.page1})
	}
	return artwork, nil
}

type staticPage struct {
	page1          int
	urls           sdk.ImageURLs
	singleOriginal string
}

// selectStaticPages 返回按自然页序排列的 1-based 页。pages 为空表示全部。
func selectStaticPages(illust sdk.Illust, pages []int) ([]staticPage, error) {
	total := illust.PageCount
	if total <= 0 {
		total = 1
	}
	if total == 1 && len(illust.MetaPages) == 0 {
		if len(pages) > 0 {
			for _, page := range pages {
				if page != 1 {
					return nil, fmt.Errorf("page %d does not exist (page_count=1)", page)
				}
			}
		}
		return []staticPage{{
			page1:          1,
			urls:           illust.ImageURLs,
			singleOriginal: illust.MetaSinglePage.OriginalImageURL,
		}}, nil
	}
	if len(illust.MetaPages) == 0 {
		return nil, fmt.Errorf("illust %d has no page metadata", illust.ID)
	}
	if total < len(illust.MetaPages) {
		total = len(illust.MetaPages)
	}
	want := map[int]struct{}{}
	if len(pages) == 0 {
		for i := 1; i <= len(illust.MetaPages); i++ {
			want[i] = struct{}{}
		}
	} else {
		for _, page := range pages {
			if page < 1 || page > total || page > len(illust.MetaPages) {
				return nil, fmt.Errorf("page %d does not exist (page_count=%d)", page, total)
			}
			want[page] = struct{}{}
		}
	}
	var selected []staticPage
	for i, page := range illust.MetaPages {
		page1 := i + 1
		if _, ok := want[page1]; !ok {
			continue
		}
		selected = append(selected, staticPage{page1: page1, urls: page.ImageURLs})
	}
	return selected, nil
}

// selectImageURL 按固定质量语义选 URL：original/regular/small/thumb/mini。
// mini 优先 SquareMedium（web thumb_mini/mini）；thumb 在 SquareMedium 更像 48 时回退 Medium。
func selectImageURL(urls sdk.ImageURLs, singleOriginal string, quality application.DownloadQuality) (string, error) {
	switch quality {
	case application.DownloadQualityOriginal:
		raw := text.FirstNonEmpty(singleOriginal, urls.Original, urls.Large)
		if raw == "" {
			return "", fmt.Errorf("no original image url")
		}
		return raw, nil
	case application.DownloadQualityRegular:
		raw := text.FirstNonEmpty(urls.Large, urls.Medium, urls.Original, singleOriginal)
		if raw == "" {
			return "", fmt.Errorf("no regular image url")
		}
		return raw, nil
	case application.DownloadQualitySmall:
		raw := text.FirstNonEmpty(urls.Medium, urls.Large, urls.Original, singleOriginal)
		if raw == "" {
			return "", fmt.Errorf("no small image url")
		}
		return raw, nil
	case application.DownloadQualityThumb:
		// 250x250 居中裁剪：优先 SquareMedium，缺失时 Medium。
		raw := text.FirstNonEmpty(urls.SquareMedium, urls.Medium, urls.Large, urls.Original, singleOriginal)
		if raw == "" {
			return "", fmt.Errorf("no thumb image url")
		}
		return raw, nil
	case application.DownloadQualityMini:
		// 48x48 居中裁剪：优先 SquareMedium（web mini/thumb_mini）。
		raw := text.FirstNonEmpty(urls.SquareMedium, urls.Medium, urls.Large, urls.Original, singleOriginal)
		if raw == "" {
			return "", fmt.Errorf("no mini image url")
		}
		return raw, nil
	default:
		return "", fmt.Errorf("quality must be one of original, regular, small, thumb, mini")
	}
}

// operationLog 只写稳定诊断字段；下载错误可能携带上游 URL 或文件系统路径，
// 因而不能直接作为 slog 的 error 属性输出。
func (m *Manager) operationLog(operation string, started time.Time, err error, illustID int64) {
	result := logging.ResultSuccess
	if err != nil {
		result = logging.ResultError
	}
	logging.LogOperation(m.logger, logging.OperationEvent{
		Component: "download",
		Operation: operation,
		Backend:   logging.BackendLocal,
		Duration:  time.Since(started),
		Result:    result,
		IllustID:  illustID,
	})
}

func (m *Manager) downloadUgoira(ctx context.Context, illust sdk.Illust, base string, format application.UgoiraFormat) (string, error) {
	meta, err := m.client.UgoiraMetadata(ctx, illust.ID)
	if err != nil {
		return "", err
	}
	zipURL := meta.UgoiraMetadata.DownloadURL
	if zipURL == "" {
		return "", fmt.Errorf("ugoira %d has no zip url", illust.ID)
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
	if err := m.downloadURL(ctx, zipURL, zipPath); err != nil {
		return "", err
	}
	outPath := filepath.Join(base, filename.Generate(filenameData(illust), 0, m.filenameTemplate)+"."+string(format))
	if err := m.convertUgoira(ctx, zipPath, meta.UgoiraMetadata.Frames, base, outPath, AnimationFormat(format)); err != nil {
		return "", err
	}
	return outPath, nil
}

func (m *Manager) downloadURL(ctx context.Context, rawURL, path string) error {
	ref, err := m.client.ParseResourceRef(rawURL)
	if err != nil {
		return err
	}
	_, err = m.client.DownloadResource(ctx, ref, path)
	return err
}

func (m *Manager) ConvertUgoira(ctx context.Context, zipPath string, frames []sdk.UgoiraFrame, workDir, outputGIF string) error {
	return m.convertUgoira(ctx, zipPath, frames, workDir, outputGIF, AnimationFormatGIF)
}

// convertUgoira 保留 ConvertUgoira 的 GIF 兼容入口，同时让 DownloadRequest 显式选择 APNG。
func (m *Manager) convertUgoira(ctx context.Context, zipPath string, frames []sdk.UgoiraFrame, workDir, outputPath string, format AnimationFormat) error {
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

func filenameData(illust sdk.Illust) filename.FilenameData {
	return filename.FilenameData{
		ID:        illust.ID,
		Author:    illust.User.Name,
		Title:     illust.Title,
		PageCount: illust.PageCount,
	}
}
