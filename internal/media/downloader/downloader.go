package downloader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
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

// DirectResourceClient 是直链 source 使用的窄端口；作品元数据下载仍只依赖 DownloadClient。
type DirectResourceClient interface {
	SaveResourceURL(context.Context, string, sdk.SaveOptions) (sdk.SavedResource, error)
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
// 单个范围在读取作品元数据前就会被展开，因此这里拒绝跨度过大的范围，避免
// 1-1000000000 这类无界输入立即分配巨量内存或在 page++ 溢出时死循环。
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
			if parsedEnd-parsedStart > maxPageExpansion {
				return nil, fmt.Errorf("page range %q is too large", part)
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

// maxPageExpansion 限制单个范围展开的页数。真实的 Pixiv 作品页数远小于此值，
// 超过即视为用户误传的无界范围并在解析期拒绝。
const maxPageExpansion = 100000

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

// DownloadedResourceType 标记没有 artwork 元数据的直接资源产物。
const DownloadedResourceType = "resource"

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

// DownloadWarning 是成功下载结果中需要调用方关注、但不阻止产物发布的提示。
// Message 仅承接安全的本地分类文本；输出 adapter 可以据此展示 warning。
type DownloadWarning struct {
	IllustID int64
	Type     string
	Message  string
}

// DownloadBatchResult 同时保留成功产物、单项失败和非阻断提示。Failures 中的 Cause
// 保留 typed SDK error；批量级不可继续错误仍通过 Download 的 error 返回。
type DownloadBatchResult struct {
	Items    []DownloadedArtwork
	Failures []DownloadFailure
	Warnings []DownloadWarning
}

// 拒绝的 source 可能是用户误传的签名直链；失败记录只保留稳定占位符，避免
// CLI/MCP 把原始 locator 回显到 stdout、JSON 或 structured result。
const redactedDownloadSource = "[redacted source]"

// DownloadReport 同时保留成功产物和独立失败，使作者全量下载无需持久化 job 状态也能说明结果。
type DownloadReport struct {
	Items     []DownloadedArtwork
	Failures  []DownloadFailure
	Warnings  []DownloadWarning
	Committed bool
}

// DownloadManager 接收完整 DownloadRequest，避免 CLI/MCP 与实现各自解析 pages/quality。
type DownloadManager interface {
	Download(context.Context, DownloadRequest) (DownloadBatchResult, error)
}

type DownloadManagerFactory func(client DownloadClient, downloadPath, filenameTemplate string) (DownloadManager, error)

type DownloadRequest struct {
	IllustIDs        []int64
	DownloadPath     string
	FilenameTemplate string
	// DirectoryTemplate 渲染每个作品相对下载根目录的子目录层级；空表示既有
	// 扁平/多页目录布局。由 BuildRelativeDirectory 应用，仅接受相对、安全段。
	DirectoryTemplate string
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
	var directURLs []string
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
		if isDirectResourceURL(source) {
			directURLs = append(directURLs, strings.TrimSpace(source))
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
		batch, err := manager.Download(ctx, downloadRequest)
		report.Items = append(report.Items, batch.Items...)
		report.Failures = append(report.Failures, batch.Failures...)
		report.Warnings = append(report.Warnings, batch.Warnings...)
		report.Committed = report.Committed || len(batch.Items) > 0
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return report, ctxErr
			}
			// batch 中的单项业务失败已经由 Failures 表达；返回值仅保留
			// 无法继续当前 operation 的错误，不再把它伪装成单个失败项。
			return report, err
		}
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
			if ctxErr := ctx.Err(); ctxErr != nil {
				return report, ctxErr
			}
			report.Failures = append(report.Failures, DownloadFailure{URL: ref.String(), Message: err.Error(), Cause: err})
			continue
		}
		report.Items = append(report.Items, DownloadedArtwork{Files: []DownloadedFile{{Path: path, Page: 1}}})
		report.Committed = true
	}
	if len(directURLs) > 0 {
		directClient, supportsDirectURLs := client.(DirectResourceClient)
		for _, rawURL := range directURLs {
			if err := ctx.Err(); err != nil {
				return report, err
			}
			path, err := directResourcePathFromURL(request.DownloadPath, rawURL)
			if err != nil {
				report.Failures = append(report.Failures, DownloadFailure{
					URL: redactedDownloadSource, Type: DownloadedResourceType, Message: err.Error(), Cause: err,
				})
				continue
			}
			if !supportsDirectURLs {
				err := errors.New("download client does not support direct resource URLs")
				report.Failures = append(report.Failures, DownloadFailure{
					URL: redactedDownloadSource, Type: DownloadedResourceType, Message: err.Error(), Cause: err,
				})
				continue
			}
			if _, err := directClient.SaveResourceURL(ctx, rawURL, sdk.SaveOptions{Path: path}); err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return report, ctxErr
				}
				safeErr := redactDirectResourceError(rawURL, err)
				report.Failures = append(report.Failures, DownloadFailure{
					URL: redactedDownloadSource, Type: DownloadedResourceType, Message: safeErr.Error(), Cause: safeErr,
				})
				continue
			}
			report.Items = append(report.Items, DownloadedArtwork{
				Type:  DownloadedResourceType,
				Files: []DownloadedFile{{Path: path, Page: 1}},
			})
			report.Committed = true
		}
	}
	return report, nil
}

// Download 把 operation snapshot 与本次运行配置交给 composition root 注入的下载器。
func (s DownloadService) Download(ctx context.Context, client DownloadClient, request DownloadRequest) (DownloadBatchResult, error) {
	manager, request, err := s.newManager(client, request)
	if err != nil {
		return DownloadBatchResult{}, err
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
	// 直接 resource 源没有作品标题或页号，必须用完整 ref 的稳定摘要作为文件名。
	// 之前截断 ref 前 64 字符会让同 kind 不同 ID/page 的 ref 共享前缀并互相覆盖。
	name := ref.String()
	digest := sha256.Sum256([]byte(name))
	encoded := hex.EncodeToString(digest[:])
	return filepath.Join(downloadPath, "resource-"+encoded), nil
}

func isDirectResourceURL(source string) bool {
	trimmed := strings.TrimSpace(source)
	lower := strings.ToLower(trimmed)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func directResourcePathFromURL(downloadPath, rawURL string) (string, error) {
	if strings.TrimSpace(downloadPath) == "" {
		return "", errors.New("download path is required for direct resource sources")
	}
	name, err := directResourceBasename(rawURL)
	if err != nil {
		return "", err
	}
	return filepath.Join(downloadPath, name), nil
}

func directResourceBasename(rawURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", errors.New("direct resource URL is invalid")
	}
	escapedPath := parsed.EscapedPath()
	if escapedPath == "" || escapedPath == "/" || strings.HasSuffix(escapedPath, "/") {
		return "", errors.New("direct resource URL has no usable basename")
	}
	name, err := url.PathUnescape(path.Base(escapedPath))
	if err != nil || name == "" || name == "." || name == ".." {
		return "", errors.New("direct resource URL has no usable basename")
	}
	name = filename.Sanitize(name)
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return '_'
		}
		return r
	}, name)
	name = strings.TrimRight(name, ". ")
	if name == "" || name == "." || name == ".." {
		return "", errors.New("direct resource URL has no usable basename")
	}
	return name, nil
}

func redactDirectResourceError(rawURL string, err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	redacted := message
	if rawURL != "" {
		redacted = strings.ReplaceAll(redacted, rawURL, redactedDownloadSource)
		if parsed, parseErr := url.Parse(rawURL); parseErr == nil {
			if parsed.RawQuery != "" {
				redacted = strings.ReplaceAll(redacted, parsed.RawQuery, "[redacted]")
			}
			for key, values := range parsed.Query() {
				redacted = redactDirectResourceQueryParameter(redacted, key)
				for _, value := range values {
					if value != "" {
						redacted = strings.ReplaceAll(redacted, value, "[redacted]")
					}
				}
			}
		}
	}
	if redacted == message {
		return err
	}
	return errors.New(redacted)
}

func redactDirectResourceQueryParameter(message, key string) string {
	if key == "" {
		return message
	}
	marker := key + "="
	searchFrom := 0
	for searchFrom < len(message) {
		relativeIndex := strings.Index(message[searchFrom:], marker)
		if relativeIndex < 0 {
			return message
		}
		index := searchFrom + relativeIndex
		valueStart := index + len(marker)
		valueEnd := strings.IndexAny(message[valueStart:], "& \t\r\n\"'<>")
		if valueEnd < 0 {
			valueEnd = len(message)
		} else {
			valueEnd += valueStart
		}
		message = message[:valueStart] + "[redacted]" + message[valueEnd:]
		searchFrom = valueStart + len("[redacted]")
	}
	return message
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
	client            DownloadClient
	ugoiraEncoder     sharedugoira.Encoder
	downloadPath      string
	filenameTemplate  string
	directoryTemplate string
	mu                sync.RWMutex
}

func NewManager(client DownloadClient, downloadPath, filenameTemplate string) *Manager {
	return &Manager{
		client:           client,
		ugoiraEncoder:    sharedugoira.NewRustEncoder(),
		downloadPath:     downloadPath,
		filenameTemplate: filenameTemplate,
	}
}

// SetDirectoryTemplate 配置 Manager 默认目录模板；MCP 组装在启动期注入运行时值，
// CLI 通过 DownloadRequest.DirectoryTemplate 逐次覆盖。
func (m *Manager) SetDirectoryTemplate(template string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.directoryTemplate = template
}

// DirectoryTemplate 返回 Manager 配置的默认目录模板。
func (m *Manager) DirectoryTemplate() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.directoryTemplate
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

func (m *Manager) Download(ctx context.Context, request DownloadRequest) (DownloadBatchResult, error) {
	unique := deduplicatePositive(request.IllustIDs)
	quality := request.Quality
	if quality == "" {
		quality = DownloadQualityOriginal
	}
	if err := ValidateDownloadQuality(quality); err != nil {
		return DownloadBatchResult{}, err
	}
	ugoiraFormat := request.UgoiraFormat
	if ugoiraFormat == "" {
		ugoiraFormat = UgoiraFormatGIF
	}
	if err := ValidateUgoiraFormat(ugoiraFormat); err != nil {
		return DownloadBatchResult{}, err
	}
	directoryTemplate := strings.TrimSpace(request.DirectoryTemplate)
	if directoryTemplate == "" {
		directoryTemplate = strings.TrimSpace(m.DirectoryTemplate())
	}
	if err := filename.ValidateDirectoryTemplate(directoryTemplate); err != nil {
		return DownloadBatchResult{}, err
	}
	batch := DownloadBatchResult{
		Items:    []DownloadedArtwork{},
		Failures: []DownloadFailure{},
		Warnings: []DownloadWarning{},
	}
	results := make([]struct {
		artwork  DownloadedArtwork
		warnings []DownloadWarning
		err      error
		done     bool
	}, len(unique))
	parallelErr := parallel.ForEach(ctx, len(unique), func(ctx context.Context, index int) {
		results[index].artwork, results[index].warnings, results[index].err = m.downloadArtwork(ctx, unique[index], request.Pages, quality, ugoiraFormat, directoryTemplate)
		results[index].done = true
	})
	var workerContextErr error
	for index, result := range results {
		if !result.done {
			continue
		}
		batch.Warnings = append(batch.Warnings, result.warnings...)
		if result.err == nil {
			batch.Items = append(batch.Items, result.artwork)
			continue
		}
		// 作品内已有文件时，保留 partial item；失败项仍单独进入 Failures，
		// 让批量报告与磁盘上已经发布的文件保持一致。
		if len(result.artwork.Files) > 0 {
			batch.Items = append(batch.Items, result.artwork)
		}
		if errors.Is(result.err, context.Canceled) || errors.Is(result.err, context.DeadlineExceeded) {
			if workerContextErr == nil {
				workerContextErr = result.err
			}
			continue
		}
		batch.Failures = append(batch.Failures, DownloadFailure{
			IllustID: unique[index],
			Message:  result.err.Error(),
			Cause:    result.err,
		})
	}
	if parallelErr != nil {
		return batch, parallelErr
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return batch, ctxErr
	}
	if workerContextErr != nil {
		return batch, workerContextErr
	}
	return batch, nil
}

func (m *Manager) downloadArtwork(ctx context.Context, id int64, pages []int, quality DownloadQuality, ugoiraFormat UgoiraFormat, directoryTemplate string) (out DownloadedArtwork, warnings []DownloadWarning, err error) {
	artwork, err := m.client.Artwork(ctx, pixiv.ArtworkRequest{ArtworkID: id})
	if err != nil {
		return DownloadedArtwork{}, nil, err
	}
	base := m.DownloadPath()
	base, err = filepath.Abs(base)
	if err != nil {
		return DownloadedArtwork{}, nil, err
	}
	data := filenameData(artwork)
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
		base = filepath.Join(base, filename.Sanitize(fmt.Sprintf("%d - %s", artwork.ID, text.DefaultString(artwork.Title, "Untitled"))))
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
		path, warning, err := m.downloadUgoira(ctx, artwork, base, ugoiraFormat)
		if warning != nil {
			warnings = append(warnings, *warning)
		}
		if err != nil {
			return artworkOut, warnings, err
		}
		artworkOut.Files = append(artworkOut.Files, DownloadedFile{Path: path, Page: 1})
		return artworkOut, warnings, nil
	}

	selected, err := selectStaticPages(artwork, pages)
	if err != nil {
		return DownloadedArtwork{}, nil, err
	}
	if err := filename.ValidateTemplate(m.filenameTemplate); err != nil {
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
		basename, err := filename.GenerateChecked(data, item.page1-1, m.filenameTemplate)
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
		saved, err := m.saveResource(ctx, ref, path)
		if err != nil {
			return artworkOut, nil, err
		}
		// 所有静态 quality 都基于本次响应的实际类型发布最终扩展名，
		// 不能再让 regular/small/original 继续沿用 original URL 的后缀。
		path, err = publishDetectedImageExtension(path, saved.ContentType)
		if err != nil {
			if cleanupErr := os.Remove(saved.Path); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
				return artworkOut, nil, fmt.Errorf("%w; remove invalid downloaded file: %v", err, cleanupErr)
			}
			return artworkOut, nil, err
		}
		artworkOut.Files = append(artworkOut.Files, DownloadedFile{Path: path, Page: item.page1})
	}
	return artworkOut, nil, nil
}

// resourceForQuality 把公开 DownloadQuality 映射到 SDK 的 artwork variant。
// original 复用页面既有的 Resource；其余质量请求对应变体的 ResourceRef，
// SaveResource 会在 revalidate 路径上按 variant 重新解析 locator。
func resourceForQuality(image pixiv.ImageResource, quality DownloadQuality) (sdk.ResourceRef, error) {
	variant := qualityVariant(quality)
	if variant == "" {
		return image.Resource.Ref, nil
	}
	return pixiv.ArtworkVariantResource(image.Resource, variant)
}

func qualityVariant(quality DownloadQuality) string {
	switch quality {
	case "", DownloadQualityOriginal:
		return "original"
	case DownloadQualityRegular:
		return "regular"
	case DownloadQualitySmall:
		return "small"
	case DownloadQualityThumb:
		return "thumb"
	case DownloadQualityMini:
		return "mini"
	default:
		return "original"
	}
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

func (m *Manager) downloadUgoira(ctx context.Context, artwork pixiv.Artwork, base string, format UgoiraFormat) (string, *DownloadWarning, error) {
	data := filenameData(artwork)
	basename, generationErr := filename.GenerateChecked(data, 0, m.filenameTemplate)
	var warning *DownloadWarning
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
			Type:     string(pixiv.ArtworkKindUgoira),
			Message:  message,
		}
		basename = fallback
	}

	meta, err := m.client.UgoiraMetadata(ctx, pixiv.UgoiraMetadataRequest{ArtworkID: artwork.ID})
	if err != nil {
		return "", warning, err
	}
	archive := selectUgoiraArchive(meta)
	if archive == nil || archive.Resource.URL == "" {
		return "", warning, fmt.Errorf("ugoira %d has no downloadable archive", artwork.ID)
	}
	zipFile, err := os.CreateTemp(base, "ugoira-*.zip")
	if err != nil {
		return "", warning, err
	}
	zipPath := zipFile.Name()
	if err := zipFile.Close(); err != nil {
		return "", warning, err
	}
	defer os.Remove(zipPath)
	if _, err := m.saveResource(ctx, archive.Resource.Ref, zipPath); err != nil {
		return "", warning, err
	}
	outPath := filepath.Join(base, basename+"."+string(format))
	if err := m.convertUgoira(ctx, zipPath, meta.Frames, base, outPath, sharedugoira.Format(format)); err != nil {
		return "", warning, err
	}
	return outPath, warning, nil
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

func (m *Manager) saveResource(ctx context.Context, ref sdk.ResourceRef, path string) (sdk.SavedResource, error) {
	return m.client.SaveResource(ctx, ref, sdk.SaveOptions{Path: path})
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

// publishDetectedImageExtension 统一修正静态质量 URL 后缀与实际实体格式不一致的情况。
// 响应 MIME 优先，文件签名次之，只有上游没有返回 MIME 时才使用 URL 后缀兜底。
// hard link 发布不会覆盖同名目标；删除旧路径失败时撤销新 link，保留原文件并暴露错误。
func publishDetectedImageExtension(path, contentType string) (string, error) {
	reportedType := normalizeMediaType(contentType)
	mediaType := reportedType
	extension, ok := imageExtensionForMediaType(mediaType)
	if !ok {
		mediaType = detectImageMimeType(path)
		extension, ok = imageExtensionForMediaType(mediaType)
	}
	if !ok && reportedType == "" {
		mediaType = MimeTypeForPath(path)
		extension, ok = imageExtensionForMediaType(mediaType)
	}
	if !ok {
		if reportedType == "" {
			reportedType = mediaType
		}
		return path, fmt.Errorf("unsupported image content type %q", reportedType)
	}

	current := filepath.Ext(path)
	if strings.EqualFold(current, extension) || (extension == ".jpg" && strings.EqualFold(current, ".jpeg")) {
		return path, nil
	}
	target := strings.TrimSuffix(path, current) + extension
	if err := os.Link(path, target); err != nil {
		return "", fmt.Errorf("publish detected image extension: %w", err)
	}
	if err := os.Remove(path); err != nil {
		_ = os.Remove(target)
		return "", fmt.Errorf("remove mismatched image path: %w", err)
	}
	return target, nil
}

func filenameData(artwork pixiv.Artwork) filename.FilenameData {
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
	return filename.FilenameData{
		ID:         artwork.ID,
		Author:     artwork.User.Name,
		AuthorID:   artwork.User.ID,
		Title:      artwork.Title,
		CreateDate: createDate,
		Tags:       tags,
		PageCount:  artwork.PageCount,
	}
}
