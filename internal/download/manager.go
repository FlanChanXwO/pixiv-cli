package download

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/files"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/text"
	uriutil "github.com/FlanChanXwO/pixiv-cli/internal/utils/uri"
)

type PixivClient interface {
	IllustDetail(context.Context, int64) (*pixiv.IllustDetail, error)
	UgoiraMetadata(context.Context, int64) (*pixiv.UgoiraMetadataResult, error)
	Download(context.Context, string, io.Writer) error
}

type DownloadedArtwork struct {
	IllustID int64
	Title    string
	Author   string
	Type     string
	Files    []DownloadedFile
}

type DownloadedFile struct {
	Path string
	Page int
}

type Manager struct {
	client           PixivClient
	logger           *slog.Logger
	ugoiraEncoder    UgoiraEncoder
	downloadPath     string
	filenameTemplate string
	sem              chan struct{}
	mu               sync.RWMutex
}

func NewManager(client PixivClient, logger *slog.Logger, downloadPath, filenameTemplate string) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	m := &Manager{
		client:           client,
		logger:           logger,
		ugoiraEncoder:    defaultUgoiraEncoder(),
		downloadPath:     downloadPath,
		filenameTemplate: filenameTemplate,
		sem:              make(chan struct{}, 5),
	}
	return m
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

func (m *Manager) Enqueue(ctx context.Context, ids []int64) int {
	unique := utils.Deduplicate(ids)
	for _, id := range unique {
		go m.downloadOne(context.WithoutCancel(ctx), id)
	}
	return len(unique)
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

func (m *Manager) downloadOne(ctx context.Context, id int64) {
	m.sem <- struct{}{}
	defer func() { <-m.sem }()
	if _, err := m.downloadArtwork(ctx, id); err != nil {
		m.logger.Error("download failed", "illust_id", id, "error", err)
	}
}

func (m *Manager) downloadArtwork(ctx context.Context, id int64) (DownloadedArtwork, error) {
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
	if illust.PageCount > 1 || illust.Type == string(pixiv.IllustTypeUgoira) {
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

	if illust.Type == string(pixiv.IllustTypeUgoira) {
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

func (m *Manager) downloadUgoira(ctx context.Context, illust pixiv.Illust, base string) (string, error) {
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
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".download-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := m.client.Download(ctx, rawURL, tmp); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// 只在完整下载并成功落盘后替换目标，避免网络中断留下半文件或破坏旧文件。
	if err := files.ReplaceFile(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func (m *Manager) ConvertUgoira(ctx context.Context, zipPath string, frames []pixiv.UgoiraFrame, workDir, outputGIF string) error {
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

func filenameData(illust pixiv.Illust) utils.FilenameData {
	return utils.FilenameData{
		ID:        illust.ID,
		Author:    illust.User.Name,
		Title:     illust.Title,
		PageCount: illust.PageCount,
	}
}
