package download

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils"
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
	if logger == nil {
		// 下载管理器可由 SDK/嵌入方单独使用；未注入时严格静默，不能落到可变全局 logger。
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Manager{
		client:           client,
		logger:           logger,
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

func (m *Manager) Download(ctx context.Context, ids []int64) ([]DownloadedArtwork, error) {
	unique := utils.Deduplicate(ids)
	artworks := make([]DownloadedArtwork, 0, len(unique))
	for _, id := range unique {
		artwork, err := m.downloadArtwork(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("download illust %d: %w", id, err)
		}
		artworks = append(artworks, artwork)
	}
	return artworks, nil
}

func (m *Manager) downloadArtwork(ctx context.Context, id int64) (out DownloadedArtwork, err error) {
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
		base = filepath.Join(base, utils.SanitizeFilename(fmt.Sprintf("%d - %s", illust.ID, text.DefaultString(illust.Title, "Untitled"))))
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
		path, err := m.downloadUgoira(ctx, illust, base)
		if err != nil {
			return DownloadedArtwork{}, err
		}
		artwork.Files = append(artwork.Files, DownloadedFile{Path: path})
		return artwork, nil
	}
	if illust.PageCount <= 1 {
		rawURL := illust.MetaSinglePage.OriginalImageURL
		if rawURL == "" {
			rawURL = text.FirstNonEmpty(illust.ImageURLs.Original, illust.ImageURLs.Large)
		}
		if rawURL == "" {
			return DownloadedArtwork{}, fmt.Errorf("illust %d has no downloadable image url", illust.ID)
		}
		path := filepath.Join(base, utils.GenerateFilename(filenameData(illust), 0, m.filenameTemplate)+filepath.Ext(uriutil.PathFromURL(rawURL)))
		if err := m.downloadURL(ctx, rawURL, path); err != nil {
			return DownloadedArtwork{}, err
		}
		artwork.Files = append(artwork.Files, DownloadedFile{Path: path})
		return artwork, nil
	}

	for i, page := range illust.MetaPages {
		rawURL := text.FirstNonEmpty(page.ImageURLs.Original, page.ImageURLs.Large)
		if rawURL == "" {
			return DownloadedArtwork{}, fmt.Errorf("illust %d page %d has no downloadable image url", illust.ID, i)
		}
		path := filepath.Join(base, utils.GenerateFilename(filenameData(illust), i, m.filenameTemplate)+filepath.Ext(uriutil.PathFromURL(rawURL)))
		if err := m.downloadURL(ctx, rawURL, path); err != nil {
			return DownloadedArtwork{}, err
		}
		artwork.Files = append(artwork.Files, DownloadedFile{Path: path, Page: i})
	}
	return artwork, nil
}

// operationLog 只写稳定诊断字段；下载错误可能携带上游 URL 或文件系统路径，
// 因而不能直接作为 slog 的 error 属性输出。
func (m *Manager) operationLog(operation string, started time.Time, err error, illustID int64) {
	result := "success"
	level := slog.LevelInfo
	if err != nil {
		result = "error"
		level = slog.LevelError
	}
	m.logger.LogAttrs(nil, level, "pixiv operation",
		slog.String("component", "download"),
		slog.String("operation", operation),
		slog.String("backend", "local"),
		slog.Duration("duration", time.Since(started)),
		slog.String("result", result),
		slog.String("error_code", ""),
		slog.Int("status", 0),
		slog.Int64("illust_id", illustID),
	)
}

func (m *Manager) downloadUgoira(ctx context.Context, illust sdk.Illust, base string) (string, error) {
	meta, err := m.client.UgoiraMetadata(ctx, illust.ID)
	if err != nil {
		return "", err
	}
	zipURL := meta.UgoiraMetadata.ZipURLs.Medium
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
	outPath := filepath.Join(base, utils.GenerateFilename(filenameData(illust), 0, m.filenameTemplate)+".gif")
	if err := m.ConvertUgoira(ctx, zipPath, meta.UgoiraMetadata.Frames, base, outPath); err != nil {
		return "", err
	}
	return outPath, nil
}

func (m *Manager) downloadURL(ctx context.Context, rawURL, path string) error {
	ref, err := m.client.ParseResourceRef(rawURL)
	if err != nil {
		return err
	}
	return m.client.Download(ctx, ref, path)
}

func (m *Manager) ConvertUgoira(ctx context.Context, zipPath string, frames []sdk.UgoiraFrame, workDir, outputGIF string) error {
	if m.ugoiraEncoder == nil {
		return fmt.Errorf("ugoira encoder is not configured")
	}
	return m.ugoiraEncoder.Encode(ctx, UgoiraEncodeInput{
		ZipPath:    zipPath,
		Frames:     frames,
		WorkDir:    workDir,
		OutputPath: outputGIF,
		Format:     AnimationFormatGIF,
	})
}

func filenameData(illust sdk.Illust) utils.FilenameData {
	return utils.FilenameData{
		ID:        illust.ID,
		Author:    illust.User.Name,
		Title:     illust.Title,
		PageCount: illust.PageCount,
	}
}
