package download

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/FlanChanXwO/pixiv-mcp-server/internal/pixiv"
	"github.com/FlanChanXwO/pixiv-mcp-server/pkg/pixivutil"
)

type PixivClient interface {
	IllustDetail(context.Context, int64) (*pixiv.IllustDetail, error)
	UgoiraMetadata(context.Context, int64) (*pixiv.UgoiraMetadataResult, error)
	Download(context.Context, string, io.Writer) error
}

type Runner interface {
	Run(ctx context.Context, dir string, name string, args ...string) error
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

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, dir string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s failed: %w: %s", name, err, strings.TrimSpace(string(output)))
	}
	return nil
}

type Manager struct {
	client           PixivClient
	logger           *slog.Logger
	runner           Runner
	downloadPath     string
	filenameTemplate string
	ffmpegAvailable  bool
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
		runner:           ExecRunner{},
		downloadPath:     downloadPath,
		filenameTemplate: filenameTemplate,
		sem:              make(chan struct{}, 5),
	}
	m.ffmpegAvailable = commandExists("ffmpeg")
	return m
}

func (m *Manager) SetRunner(runner Runner) {
	if runner != nil {
		m.runner = runner
	}
}

func (m *Manager) SetFFmpegAvailable(available bool) {
	m.ffmpegAvailable = available
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
	unique := pixivutil.Deduplicate(ids)
	for _, id := range unique {
		go m.downloadOne(context.WithoutCancel(ctx), id)
	}
	return len(unique)
}

func (m *Manager) Download(ctx context.Context, ids []int64) ([]DownloadedArtwork, error) {
	unique := pixivutil.Deduplicate(ids)
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
	if illust.PageCount > 1 || illust.Type == "ugoira" {
		base = filepath.Join(base, pixivutil.SanitizeFilename(fmt.Sprintf("%d - %s", illust.ID, fallback(illust.Title, "Untitled"))))
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

	if illust.Type == "ugoira" {
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
			rawURL = firstNonEmpty(illust.ImageURLs.Original, illust.ImageURLs.Large)
		}
		if rawURL == "" {
			return DownloadedArtwork{}, fmt.Errorf("illust %d has no downloadable image url", illust.ID)
		}
		path := filepath.Join(base, pixivutil.GenerateFilename(filenameData(illust), 0, m.filenameTemplate)+filepath.Ext(pathFromURL(rawURL)))
		if err := m.downloadURL(ctx, rawURL, path); err != nil {
			return DownloadedArtwork{}, err
		}
		artwork.Files = append(artwork.Files, DownloadedFile{Path: path})
		return artwork, nil
	}

	for i, page := range illust.MetaPages {
		rawURL := firstNonEmpty(page.ImageURLs.Original, page.ImageURLs.Large)
		if rawURL == "" {
			return DownloadedArtwork{}, fmt.Errorf("illust %d page %d has no downloadable image url", illust.ID, i)
		}
		path := filepath.Join(base, pixivutil.GenerateFilename(filenameData(illust), i, m.filenameTemplate)+filepath.Ext(pathFromURL(rawURL)))
		if err := m.downloadURL(ctx, rawURL, path); err != nil {
			return DownloadedArtwork{}, err
		}
		artwork.Files = append(artwork.Files, DownloadedFile{Path: path, Page: i})
	}
	return artwork, nil
}

func (m *Manager) downloadUgoira(ctx context.Context, illust pixiv.Illust, base string) (string, error) {
	if !m.ffmpegAvailable {
		return "", fmt.Errorf("ffmpeg not found; skip ugoira %d conversion", illust.ID)
	}
	meta, err := m.client.UgoiraMetadata(ctx, illust.ID)
	if err != nil {
		return "", err
	}
	zipURL := meta.UgoiraMetadata.ZipURLs.Medium
	if zipURL == "" {
		return "", fmt.Errorf("ugoira %d has no zip url", illust.ID)
	}
	zipPath := filepath.Join(base, filepath.Base(pathFromURL(zipURL)))
	if err := m.downloadURL(ctx, zipURL, zipPath); err != nil {
		return "", err
	}
	defer os.Remove(zipPath)
	outPath := filepath.Join(base, pixivutil.GenerateFilename(filenameData(illust), 0, m.filenameTemplate)+".gif")
	if err := m.ConvertUgoira(ctx, zipPath, meta.UgoiraMetadata.Frames, base, outPath); err != nil {
		return "", err
	}
	return outPath, nil
}

func (m *Manager) downloadURL(ctx context.Context, rawURL, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return m.client.Download(ctx, rawURL, file)
}

func (m *Manager) ConvertUgoira(ctx context.Context, zipPath string, frames []pixiv.UgoiraFrame, workDir, outputGIF string) error {
	tempDir := filepath.Join(workDir, "temp_frames")
	if err := os.RemoveAll(tempDir); err != nil {
		return err
	}
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, file := range reader.File {
		if err := extractZipFile(file, tempDir); err != nil {
			return err
		}
	}

	var list strings.Builder
	for _, frame := range frames {
		list.WriteString("file '")
		list.WriteString(filepath.Base(frame.File))
		list.WriteString("'\n")
		fmt.Fprintf(&list, "duration %.3f\n", float64(frame.Delay)/1000.0)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "frame_list.txt"), []byte(list.String()), 0o644); err != nil {
		return err
	}

	return m.runner.Run(ctx, tempDir, "ffmpeg",
		"-f", "concat",
		"-safe", "0",
		"-i", "frame_list.txt",
		"-vf", "split[s0][s1];[s0]palettegen=stats_mode=single[p];[s1][p]paletteuse=new=1",
		"-y",
		outputGIF,
	)
}

func extractZipFile(file *zip.File, dstDir string) error {
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	dstPath := filepath.Join(dstDir, filepath.Base(file.Name))
	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	return err
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func pathFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Path == "" {
		return rawURL
	}
	return parsed.Path
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func filenameData(illust pixiv.Illust) pixivutil.FilenameData {
	return pixivutil.FilenameData{
		ID:        illust.ID,
		Author:    illust.User.Name,
		Title:     illust.Title,
		PageCount: illust.PageCount,
	}
}

func fallback(value, backup string) string {
	if value == "" {
		return backup
	}
	return value
}
