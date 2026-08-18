package downloader

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/media/downloader/filename"
	"github.com/FlanChanXwO/pixiv-cli/internal/media/downloader/parallel"
	sharedugoira "github.com/FlanChanXwO/pixiv-cli/internal/media/ugoira"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/diagnostics"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/pagination"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/text"
	uriutil "github.com/FlanChanXwO/pixiv-cli/internal/utils/uri"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// DownloadClient 是下载实现从 public SDK operation snapshot 使用的最小能力集。
// ResourceRef 由共享 sdk core 的 ParseResourceRef 解析；下载器只消费 opaque ref。
type DownloadClient interface {
	Artwork(context.Context, pixiv.ArtworkRequest) (pixiv.Artwork, error)
	UgoiraMetadata(context.Context, pixiv.UgoiraMetadataRequest) (pixiv.UgoiraMetadata, error)
	SaveResource(context.Context, sdk.ResourceRef, sdk.SaveOptions) (sdk.SavedResource, error)
}

// DownloadTargetClient 在下载资源能力之外提供作者作品列表，用于把用户 URL 展开为视觉作品。
type DownloadTargetClient interface {
	DownloadClient
	UserArtworks(context.Context, pixiv.UserArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	UserArtworkBookmarks(context.Context, pixiv.UserArtworkBookmarksRequest) (sdk.Page[pixiv.Artwork], error)
}

// DownloadQuality 选择静态图片的下载分辨率。
type DownloadQuality string

const (
	DownloadQualityOriginal DownloadQuality = "original"
	DownloadQualityRegular  DownloadQuality = "regular"
	DownloadQualitySmall    DownloadQuality = "small"
	DownloadQualityThumb    DownloadQuality = "thumb"
	DownloadQualityMini     DownloadQuality = "mini"
)

// UgoiraFormat 选择 ugoira 的最终动画容器。
type UgoiraFormat string

const (
	UgoiraFormatGIF  UgoiraFormat = "gif"
	UgoiraFormatAPNG UgoiraFormat = "apng"
)

// ParsePageSpec 解析逗号/连字符分隔的 1-based 页码选择，返回闭区间页号列表。
func ParsePageSpec(raw string) ([]int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var pages []int
	seen := make(map[int]struct{})
	for part := range strings.SplitSeq(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, errors.New("page selection contains an empty entry")
		}
		var start, end int
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			parsedStart, err := strconv.Atoi(strings.TrimSpace(bounds[0]))
			if err != nil || parsedStart <= 0 {
				return nil, fmt.Errorf("invalid page range %q", part)
			}
			parsedEnd, err := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err != nil || parsedEnd < parsedStart {
				return nil, fmt.Errorf("invalid page range %q", part)
			}
			start, end = parsedStart, parsedEnd
		} else {
			page, err := strconv.Atoi(part)
			if err != nil || page <= 0 {
				return nil, fmt.Errorf("invalid page number %q", part)
			}
			start, end = page, page
		}
		for page := start; page <= end; page++ {
			if _, exists := seen[page]; exists {
				continue
			}
			seen[page] = struct{}{}
			pages = append(pages, page)
		}
	}
	slices.Sort(pages)
	return pages, nil
}

// ValidateDownloadQuality 校验静态图片质量。
func ValidateDownloadQuality(quality DownloadQuality) error {
	switch quality {
	case "", DownloadQualityOriginal, DownloadQualityRegular, DownloadQualitySmall, DownloadQualityThumb, DownloadQualityMini:
		return nil
	default:
		return errors.New("quality must be one of original, regular, small, thumb, mini")
	}
}

// ValidateUgoiraFormat 校验 ugoira 容器格式。
func ValidateUgoiraFormat(format UgoiraFormat) error {
	switch format {
	case "", UgoiraFormatGIF, UgoiraFormatAPNG:
		return nil
	default:
		return errors.New("ugoira format must be one of gif, apng")
	}
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

// DownloadFailure 是一次无状态批量下载中单个作品或作者列表读取的可展示失败。
// Message 仅承接 SDK/下载器的安全错误文本，不保存用户输入的原始 URL。
type DownloadFailure struct {
	URL      string
	IllustID int64
	Type     string
	Message  string
	// Cause 只用于 CLI 判断账号池的安全重试边界，输出 adapter 只公开 Message。
	Cause error
}

// 拒绝的 source 可能是用户误传的签名直链；失败记录只保留稳定占位符，避免
// CLI/MCP 把原始 locator 回显到 stdout、JSON 或 structured result。
const redactedDownloadSource = "[redacted source]"

// DownloadReport 同时保留成功产物和独立失败，使作者全量下载无需持久化 job 状态也能说明结果。
type DownloadReport struct {
	Items     []DownloadedArtwork
	Failures  []DownloadFailure
	Committed bool
}

// DownloadManager 接收完整 DownloadRequest，避免 CLI/MCP 与实现各自解析 pages/quality。
type DownloadManager interface {
	Download(context.Context, DownloadRequest) ([]DownloadedArtwork, error)
}

type DownloadManagerFactory func(client DownloadClient, downloadPath, filenameTemplate string) (DownloadManager, error)

type DownloadRequest struct {
	IllustIDs        []int64
	DownloadPath     string
	FilenameTemplate string
	// Pages 为 1-based 页码列表；空表示全部页。由 ParsePageSpec 生成。
	Pages []int
	// Quality 默认 original。
	Quality DownloadQuality
	// UgoiraFormat 默认 GIF，仅影响 ugoira 的最终动画容器。
	UgoiraFormat UgoiraFormat
}

// DownloadService 编排 source expansion、下载执行和部分成功报告；实际文件获取由 Manager 完成。
type DownloadService struct {
	NewManager DownloadManagerFactory
}

// DownloadSources 是 CLI/MCP 的入口：作品 PID、作品 URL、受资源策略允许的直链
// 都原样交给 public SDK 解析。用户主页 URL 是唯一的本地展开规则，因为它代表
// 一组作品而非单个 SDK 下载来源。
func (s DownloadService) DownloadSources(ctx context.Context, client DownloadTargetClient, sources []string, request DownloadRequest) (report DownloadReport, returnErr error) {
	startedAt := time.Now()
	report = DownloadReport{Items: []DownloadedArtwork{}, Failures: []DownloadFailure{}}
	diagnostics.Emit(ctx, diagnostics.Event{
		Module:    diagnostics.ModulePixivDownload,
		Kind:      diagnostics.EventStarted,
		Operation: fmt.Sprintf("download %d sources", len(sources)),
	})
	defer func() {
		kind := diagnostics.EventCompleted
		reason := diagnostics.ReasonNone
		if returnErr != nil {
			kind = diagnostics.EventFailed
			reason = diagnostics.ReasonCommandFailed
		}
		diagnostics.Emit(ctx, diagnostics.Event{
			Module:    diagnostics.ModulePixivDownload,
			Kind:      kind,
			Operation: "download sources",
			Reason:    reason,
			Count:     len(report.Items),
			Duration:  time.Since(startedAt),
		})
	}()
	if len(sources) == 0 {
		return report, errors.New("at least one download source is required")
	}
	artworkIDs := make([]int64, 0)
	seenArtworkIDs := make(map[int64]struct{})
	appendArtwork := func(id int64) {
		if id <= 0 {
			return
		}
		if _, seen := seenArtworkIDs[id]; seen {
			return
		}
		seenArtworkIDs[id] = struct{}{}
		artworkIDs = append(artworkIDs, id)
	}
	var directRefs []sdk.ResourceRef
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		reference, err := pixiv.ParseURL(source)
		if err == nil {
			switch reference.Kind {
			case pixiv.ReferenceKindArtwork:
				appendArtwork(reference.ID)
				continue
			case pixiv.ReferenceKindUser:
				if err := collectUserArtworkJobs(ctx, client, reference, appendArtwork, &report); err != nil {
					return report, err
				}
				continue
			case pixiv.ReferenceKindUserBookmarks:
				if err := collectUserBookmarkJobs(ctx, client, reference, appendArtwork); err != nil {
					return report, err
				}
				continue
			default:
				return report, errors.New("download source is invalid")
			}
		}
		if id, parseErr := strconv.ParseInt(strings.TrimSpace(source), 10, 64); parseErr == nil && id > 0 {
			appendArtwork(id)
			continue
		}
		ref, refErr := sdk.ParseResourceRef(source)
		if refErr != nil {
			report.Failures = append(report.Failures, DownloadFailure{
				URL: redactedDownloadSource, Message: "download source is invalid", Cause: refErr,
			})
			continue
		}
		directRefs = append(directRefs, ref)
	}
	if len(artworkIDs) > 0 {
		manager, downloadRequest, err := s.newManager(client, request)
		if err != nil {
			return report, err
		}
		downloadRequest.IllustIDs = artworkIDs
		items, err := manager.Download(ctx, downloadRequest)
		if err != nil {
			if ctx.Err() != nil {
				return report, ctx.Err()
			}
			report.Failures = append(report.Failures, DownloadFailure{Message: err.Error(), Cause: err})
			return report, nil
		}
		report.Items = append(report.Items, items...)
		report.Committed = report.Committed || len(items) > 0
	}
	for _, ref := range directRefs {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		path, err := directResourcePath(request.DownloadPath, ref)
		if err != nil {
			report.Failures = append(report.Failures, DownloadFailure{URL: ref.String(), Message: err.Error(), Cause: err})
			continue
		}
		if _, err := client.SaveResource(ctx, ref, sdk.SaveOptions{Path: path}); err != nil {
			report.Failures = append(report.Failures, DownloadFailure{URL: ref.String(), Message: err.Error(), Cause: err})
			continue
		}
		report.Items = append(report.Items, DownloadedArtwork{Files: []DownloadedFile{{Path: path, Page: 1}}})
		report.Committed = true
	}
	return report, nil
}

// Download 把 operation snapshot 与本次运行配置交给 composition root 注入的下载器。
func (s DownloadService) Download(ctx context.Context, client DownloadClient, request DownloadRequest) ([]DownloadedArtwork, error) {
	manager, request, err := s.newManager(client, request)
	if err != nil {
		return nil, err
	}
	return manager.Download(ctx, request)
}

func (s DownloadService) newManager(client DownloadClient, request DownloadRequest) (DownloadManager, DownloadRequest, error) {
	if s.NewManager == nil {
		return nil, DownloadRequest{}, errors.New("download manager factory is not configured")
	}
	if isNilLike(client) {
		return nil, DownloadRequest{}, errors.New("download operation client is not configured")
	}
	if request.Quality == "" {
		request.Quality = DownloadQualityOriginal
	}
	if err := ValidateDownloadQuality(request.Quality); err != nil {
		return nil, DownloadRequest{}, err
	}
	if request.UgoiraFormat == "" {
		request.UgoiraFormat = UgoiraFormatGIF
	}
	if err := ValidateUgoiraFormat(request.UgoiraFormat); err != nil {
		return nil, DownloadRequest{}, err
	}
	manager, err := s.NewManager(client, request.DownloadPath, request.FilenameTemplate)
	if err != nil {
		return nil, DownloadRequest{}, err
	}
	if isNilLike(manager) {
		return nil, DownloadRequest{}, errors.New("download manager factory returned nil")
	}
	return manager, request, nil
}

func collectUserArtworkJobs(ctx context.Context, client DownloadTargetClient, user pixiv.Reference, appendArtwork func(int64), report *DownloadReport) error {
	for _, kind := range []pixiv.ArtworkKind{pixiv.ArtworkKindIllustration, pixiv.ArtworkKindManga, pixiv.ArtworkKindUgoira} {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := pagination.TraversePages(ctx, pagination.PagePlan{}, func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
			result, err := client.UserArtworks(ctx, pixiv.UserArtworksRequest{UserID: user.ID, Kind: kind, Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}, func(items []pixiv.Artwork) error {
			for _, artwork := range items {
				appendArtwork(artwork.ID)
			}
			return nil
		})
		if err == nil {
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		report.Failures = append(report.Failures, DownloadFailure{
			URL: referenceURL(user), Type: string(kind), Message: err.Error(), Cause: err,
		})
	}
	return nil
}

func collectUserBookmarkJobs(ctx context.Context, client DownloadTargetClient, user pixiv.Reference, appendArtwork func(int64)) error {
	_, err := pagination.TraversePages(ctx, pagination.PagePlan{}, func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.UserArtworkBookmarks(ctx, pixiv.UserArtworkBookmarksRequest{UserID: user.ID, Restrict: pixiv.RestrictPublic, Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}, func(items []pixiv.Artwork) error {
		for _, artwork := range items {
			appendArtwork(artwork.ID)
		}
		return nil
	})
	return err
}

func directResourcePath(downloadPath string, ref sdk.ResourceRef) (string, error) {
	if strings.TrimSpace(downloadPath) == "" {
		return "", errors.New("download path is required for direct resource sources")
	}
	name := ref.String()
	if len(name) > 64 {
		name = name[:64]
	}
	return filepath.Join(downloadPath, "resource-"+name), nil
}

func referenceURL(reference pixiv.Reference) string {
	raw, err := reference.CanonicalURL()
	if err != nil {
		return ""
	}
	return raw
}

func isNilLike(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

type Manager struct {
	client           DownloadClient
	ugoiraEncoder    sharedugoira.Encoder
	downloadPath     string
	filenameTemplate string
	mu               sync.RWMutex
}

func NewManager(client DownloadClient, downloadPath, filenameTemplate string) *Manager {
	return &Manager{
		client:           client,
		ugoiraEncoder:    sharedugoira.NewRustEncoder(),
		downloadPath:     downloadPath,
		filenameTemplate: filenameTemplate,
	}
}

// SetUgoiraEncoder 设置动图编码器，供启动装配和聚焦测试替换。
func (m *Manager) SetUgoiraEncoder(encoder sharedugoira.Encoder) {
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

func (m *Manager) Download(ctx context.Context, request DownloadRequest) ([]DownloadedArtwork, error) {
	unique := deduplicatePositive(request.IllustIDs)
	quality := request.Quality
	if quality == "" {
		quality = DownloadQualityOriginal
	}
	if err := ValidateDownloadQuality(quality); err != nil {
		return nil, err
	}
	ugoiraFormat := request.UgoiraFormat
	if ugoiraFormat == "" {
		ugoiraFormat = UgoiraFormatGIF
	}
	if err := ValidateUgoiraFormat(ugoiraFormat); err != nil {
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

func (m *Manager) downloadArtwork(ctx context.Context, id int64, pages []int, quality DownloadQuality, ugoiraFormat UgoiraFormat) (out DownloadedArtwork, err error) {
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
		if quality != DownloadQualityOriginal {
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

func (m *Manager) downloadUgoira(ctx context.Context, artwork pixiv.Artwork, base string, format UgoiraFormat) (string, error) {
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
	if err := m.convertUgoira(ctx, zipPath, meta.Frames, base, outPath, sharedugoira.Format(format)); err != nil {
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
	return m.convertUgoira(ctx, zipPath, frames, workDir, outputGIF, sharedugoira.FormatGIF)
}

// convertUgoira 保留 ConvertUgoira 的 GIF 兼容入口，同时让 DownloadRequest 显式选择 APNG。
func (m *Manager) convertUgoira(ctx context.Context, zipPath string, frames []pixiv.UgoiraFrame, workDir, outputPath string, format sharedugoira.Format) error {
	if m.ugoiraEncoder == nil {
		return fmt.Errorf("ugoira encoder is not configured")
	}
	ugoiraFrames := make([]sharedugoira.Frame, len(frames))
	for index, frame := range frames {
		ugoiraFrames[index] = sharedugoira.Frame{File: frame.Filename, Delay: frame.DelayMilliseconds}
	}
	return m.ugoiraEncoder.Encode(ctx, sharedugoira.Input{
		ZipPath:    zipPath,
		Frames:     ugoiraFrames,
		WorkDir:    workDir,
		OutputPath: outputPath,
		Format:     format,
	})
}

func deduplicatePositive(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	unique := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	slices.Sort(unique)
	return unique
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
