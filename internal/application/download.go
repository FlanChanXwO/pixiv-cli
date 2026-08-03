package application

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// DownloadClient 是下载实现从 public SDK operation snapshot 使用的最小能力集。
// application 只编排用例，不依赖具体下载器或 Pixiv 协议实现。
type DownloadClient interface {
	Artwork(context.Context, pixiv.ArtworkRequest) (pixiv.Artwork, error)
	UgoiraMetadata(context.Context, pixiv.UgoiraMetadataRequest) (pixiv.UgoiraMetadata, error)
	ParseResourceRef(string) (sdk.ResourceRef, error)
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

// DownloadQuality 值定义静态图片的可选分辨率。
const (
	DownloadQualityOriginal DownloadQuality = "original"
	DownloadQualityRegular  DownloadQuality = "regular"
	DownloadQualitySmall    DownloadQuality = "small"
	DownloadQualityThumb    DownloadQuality = "thumb"
	DownloadQualityMini     DownloadQuality = "mini"
)

// UgoiraFormat 选择 ugoira 的最终动画容器。
type UgoiraFormat string

// UgoiraFormat 值定义 ugoira 的可选动画容器。
const (
	UgoiraFormatGIF  UgoiraFormat = "gif"
	UgoiraFormatAPNG UgoiraFormat = "apng"
)

// ParsePageSpec 解析逗号/连字符分隔的 1-based 页码选择，返回闭区间页号列表。
func ParsePageSpec(raw string) ([]int, error) {
	return parsePageSpec(raw)
}

// ValidateDownloadQuality 校验静态图片质量。
func ValidateDownloadQuality(quality DownloadQuality) error {
	switch quality {
	case "", DownloadQualityOriginal, DownloadQualityRegular, DownloadQualitySmall, DownloadQualityThumb, DownloadQualityMini:
		return nil
	default:
		return fmt.Errorf("quality must be one of original, regular, small, thumb, mini")
	}
}

// ValidateUgoiraFormat 校验 ugoira 容器格式。
func ValidateUgoiraFormat(format UgoiraFormat) error {
	switch format {
	case "", UgoiraFormatGIF, UgoiraFormatAPNG:
		return nil
	default:
		return fmt.Errorf("ugoira format must be one of gif, apng")
	}
}

// parsePageSpec 将 "1,3-5" 形式的页选择展开为去重、升序的闭区间页号。
func parsePageSpec(raw string) ([]int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var pages []int
	seen := make(map[int]struct{})
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, errors.New("page selection contains an empty entry")
		}
		var start, end int
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			if len(bounds) != 2 {
				return nil, fmt.Errorf("invalid page range %q", part)
			}
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
	// Cause 只用于 application/CLI 判断账号池的安全重试边界，输出 adapter
	// 只公开 Message，避免错误对象进入用户协议。
	Cause error
}

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

type DownloadService struct {
	NewManager DownloadManagerFactory
}

// DownloadSources 是 CLI/MCP 的新入口：作品 PID、作品 URL、受资源策略允许的直链
// 都原样交给 public SDK 解析。用户主页 URL 是唯一的应用层展开规则，因为它代表
// 一组作品而非单个 SDK 下载来源。
func (s DownloadService) DownloadSources(ctx context.Context, client DownloadTargetClient, sources []string, request DownloadRequest) (DownloadReport, error) {
	report := DownloadReport{Items: []DownloadedArtwork{}, Failures: []DownloadFailure{}}
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
		// 非 Pixiv 页面 URL 可能是裸 PID 或经 ResourcePolicy 允许的 CDN 资源。
		if id, parseErr := strconv.ParseInt(strings.TrimSpace(source), 10, 64); parseErr == nil && id > 0 {
			appendArtwork(id)
			continue
		}
		ref, refErr := client.ParseResourceRef(source)
		if refErr != nil {
			report.Failures = append(report.Failures, DownloadFailure{
				URL: source, Message: "download source is invalid", Cause: refErr,
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
		saved, err := client.SaveResource(ctx, ref, sdk.SaveOptions{Path: path})
		if err != nil {
			report.Failures = append(report.Failures, DownloadFailure{URL: ref.String(), Message: err.Error(), Cause: err})
			continue
		}
		_ = saved
		report.Items = append(report.Items, DownloadedArtwork{Files: []DownloadedFile{{Path: path, Page: 1}}})
		report.Committed = true
	}
	return report, nil
}

// directResourcePath 为资源直链生成一个安全的目标路径；无法推导时返回错误。
func directResourcePath(downloadPath string, ref sdk.ResourceRef) (string, error) {
	if strings.TrimSpace(downloadPath) == "" {
		return "", errors.New("download path is required for direct resource sources")
	}
	// 资源直链没有稳定的作品元数据；以 opaque ref 的文本摘要命名，扩展名保持未知。
	name := ref.String()
	if len(name) > 64 {
		name = name[:64]
	}
	return filepath.Join(downloadPath, "resource-"+name), nil
}

func collectUserBookmarkJobs(ctx context.Context, client DownloadTargetClient, user pixiv.Reference, appendArtwork func(int64)) error {
	_, err := TraversePages(ctx, PagePlan{}, func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
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
		_, err := TraversePages(ctx, PagePlan{}, func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
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

// referenceURL 生成 pixiv.Reference 的规范 URL；解析失败时退化为空串。
func referenceURL(reference pixiv.Reference) string {
	raw, err := reference.CanonicalURL()
	if err != nil {
		return ""
	}
	return raw
}

// isNilLike 处理 Go interface 携带 typed nil 的情况；仅对允许 IsNil 的 kind
// 调用 reflect，struct 等 value implementation 保持正常，避免反射自身触发 panic。
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
