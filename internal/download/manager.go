package download

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
		// 下载管理器可由 SDK/嵌入方单独使用；未注入时严格静默，不能落到可变全局 logger。
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
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
	_, _ = m.downloadArtwork(ctx, id)
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

// operationLog 只写稳定诊断字段；下载错误可能携带上游 URL 或文件系统路径，
// 因而不能直接作为 slog 的 error 属性输出。
func (m *Manager) operationLog(operation string, started time.Time, err error, illustID int64) {
	result := "success"
	if err != nil {
		result = "error"
	}
	m.logger.LogAttrs(nil, slog.LevelInfo, "pixiv operation",
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
	tempDir, err := os.MkdirTemp(workDir, "ugoira-frames-*")
	if err != nil {
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

	outFile, err := os.CreateTemp(filepath.Dir(outputGIF), ".ugoira-*.gif")
	if err != nil {
		return err
	}
	tmpOutput := outFile.Name()
	if err := outFile.Close(); err != nil {
		_ = os.Remove(tmpOutput)
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpOutput)
		}
	}()

	if err := m.runner.Run(ctx, tempDir, "ffmpeg",
		"-f", "concat",
		"-safe", "0",
		"-i", "frame_list.txt",
		"-vf", "split[s0][s1];[s0]palettegen=stats_mode=single[p];[s1][p]paletteuse=new=1",
		"-y",
		tmpOutput,
	); err != nil {
		return err
	}
	// GIF 转换也采用临时文件 + rename，避免 ffmpeg 失败时留下半文件或覆盖旧文件。
	if err := files.ReplaceFile(tmpOutput, outputGIF); err != nil {
		return err
	}
	cleanup = false
	return nil
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

func filenameData(illust pixiv.Illust) utils.FilenameData {
	return utils.FilenameData{
		ID:        illust.ID,
		Author:    illust.User.Name,
		Title:     illust.Title,
		PageCount: illust.PageCount,
	}
}
