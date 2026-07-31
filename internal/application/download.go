package application

import (
	"context"
	"errors"
	"reflect"

	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
)

// DownloadClient 是下载实现从 public SDK operation snapshot 使用的最小能力集。
// application 只编排用例，不依赖具体下载器或 Pixiv 协议实现。
type DownloadClient interface {
	IllustDetail(context.Context, int64) (*sdk.IllustDetail, error)
	UgoiraMetadata(context.Context, int64) (*sdk.UgoiraMetadataResult, error)
	ParseResourceRef(string) (sdk.ResourceRef, error)
	DownloadResource(context.Context, sdk.ResourceRef, string) (sdk.ResourceDownloadResult, error)
}

// DownloadTargetClient 在下载资源能力之外提供作者作品列表，用于把用户 URL 展开为视觉作品。
// 该接口仍只依赖顶层 public SDK 的规范化能力。
type DownloadTargetClient interface {
	DownloadClient
	UserArtworks(context.Context, sdk.UserArtworksRequest) (*sdk.IllustListResult, error)
}

// 质量与页选择契约由 public SDK 拥有，application 仅 alias 以便 CLI/MCP 共用。
type DownloadQuality = sdk.DownloadQuality
type UgoiraFormat = sdk.UgoiraFormat

const (
	DownloadQualityOriginal = sdk.DownloadQualityOriginal
	DownloadQualityRegular  = sdk.DownloadQualityRegular
	DownloadQualitySmall    = sdk.DownloadQualitySmall
	DownloadQualityThumb    = sdk.DownloadQualityThumb
	DownloadQualityMini     = sdk.DownloadQualityMini
	UgoiraFormatGIF         = sdk.UgoiraFormatGIF
	UgoiraFormatAPNG        = sdk.UgoiraFormatAPNG
)

var (
	ParsePageSpec           = sdk.ParsePageSpec
	ValidateDownloadQuality = sdk.ValidateDownloadQuality
	ValidateUgoiraFormat    = sdk.ValidateUgoiraFormat
)

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
func (s DownloadService) DownloadSources(ctx context.Context, client DownloadTargetClient, sources []string, options sdk.DownloadOptions) (DownloadReport, error) {
	report := DownloadReport{Items: []DownloadedArtwork{}, Failures: []DownloadFailure{}}
	if len(sources) == 0 {
		return report, errors.New("at least one download source is required")
	}
	downloadSources := make([]string, 0, len(sources))
	sourceInputIndices := make([]int, 0, len(sources))
	seenArtworkIDs := make(map[int64]struct{})
	appendArtwork := func(id int64, inputIndex int) {
		if id <= 0 {
			return
		}
		if _, seen := seenArtworkIDs[id]; seen {
			return
		}
		seenArtworkIDs[id] = struct{}{}
		downloadSources = append(downloadSources, sdk.Reference{Kind: sdk.ReferenceKindArtwork, ID: id}.URL())
		sourceInputIndices = append(sourceInputIndices, inputIndex)
	}
	for inputIndex, source := range sources {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		reference, err := sdk.ParseReference(source)
		if err != nil {
			// 非 Pixiv 页面 URL 可能是经 ResourcePolicy 允许的 CDN 资源；SDK 会在
			// ParseResourceRef 中统一验证，application 不复制该安全边界。
			downloadSources = append(downloadSources, source)
			sourceInputIndices = append(sourceInputIndices, inputIndex)
			continue
		}
		if reference.Kind == sdk.ReferenceKindArtwork {
			appendArtwork(reference.ID, inputIndex)
			continue
		}
		if reference.Kind != sdk.ReferenceKindUser {
			return report, errors.New("download source is invalid")
		}
		jobs := make([]downloadTargetJob, 0)
		if err := collectUserArtworkJobs(ctx, client, reference, &jobs, &report); err != nil {
			return report, err
		}
		for _, job := range jobs {
			appendArtwork(job.target.ID, inputIndex)
		}
	}
	if len(downloadSources) == 0 {
		return report, nil
	}
	downloader, ok := client.(interface {
		DownloadAllWith(context.Context, []string, sdk.DownloadOptions) (sdk.DownloadAllResult, error)
	})
	if !ok {
		// 仅保留给旧嵌入方的兼容适配；生产 public SDK 必实现上述高层 API。
		targets := make([]sdk.Reference, 0, len(downloadSources))
		for _, source := range downloadSources {
			reference, err := sdk.ParseReference(source)
			if err != nil || reference.Kind != sdk.ReferenceKindArtwork {
				return report, errors.New("download source requires a public SDK supporting DownloadAllWith")
			}
			targets = append(targets, reference)
		}
		return s.DownloadTargets(ctx, client, targets, DownloadRequest{
			DownloadPath: options.DownloadPath, FilenameTemplate: options.FilenameTemplate,
			Pages: options.Pages, Quality: options.Quality, UgoiraFormat: options.UgoiraFormat,
		})
	}
	if progress := options.Progress; progress != nil {
		// application 在展开用户页、并按 canonical artwork ID 去重后才调用 SDK。
		// SDK 的 SourceIndex 指向展开后的输入；这里映射回调用方原始 sources 下标，
		// 使进度事件始终能定位到用户提供的来源位置。
		options.Progress = func(event sdk.DownloadProgress) {
			if event.SourceIndex >= 0 && event.SourceIndex < len(sourceInputIndices) {
				event.SourceIndex = sourceInputIndices[event.SourceIndex]
			}
			progress(event)
		}
	}
	result, err := downloader.DownloadAllWith(ctx, downloadSources, options)
	if err != nil {
		return report, err
	}
	for index, item := range result.Items {
		if item.Committed {
			report.Committed = true
		}
		if item.Result != nil {
			artwork := DownloadedArtwork{
				IllustID: item.Result.IllustID, Title: item.Result.Title,
				Author: item.Result.Author, Type: item.Result.Type,
				Files: make([]DownloadedFile, 0, len(item.Result.Files)),
			}
			for _, file := range item.Result.Files {
				artwork.Files = append(artwork.Files, DownloadedFile{Path: file.Path, Page: file.Page})
			}
			report.Items = append(report.Items, artwork)
		}
		if item.Err != nil {
			report.Failures = append(report.Failures, DownloadFailure{URL: downloadSources[index], Message: item.Err.Error(), Cause: item.Err})
		}
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

// DownloadTargets 是旧嵌入下载器的兼容路径。新 CLI/MCP 使用 DownloadSources 并由
// public SDK 统一调度并发；这里保留输入顺序逐项调用，避免旧 DownloadManager 的
// 路径和模板可变状态在并发下交叉。
func (s DownloadService) DownloadTargets(ctx context.Context, client DownloadTargetClient, targets []sdk.Reference, request DownloadRequest) (DownloadReport, error) {
	report := DownloadReport{Items: []DownloadedArtwork{}, Failures: []DownloadFailure{}}
	if len(targets) == 0 {
		return report, errors.New("at least one download target is required")
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	manager, request, err := s.newManager(client, request)
	if err != nil {
		return report, err
	}
	jobs := make([]downloadTargetJob, 0, len(targets))
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		if target.ID <= 0 || target.URL() == "" {
			return report, errors.New("download target is invalid")
		}
		switch target.Kind {
		case sdk.ReferenceKindArtwork:
			jobs = append(jobs, downloadTargetJob{target: target})
		case sdk.ReferenceKindUser:
			if err := collectUserArtworkJobs(ctx, client, target, &jobs, &report); err != nil {
				return report, err
			}
		default:
			return report, errors.New("download target is invalid")
		}
	}
	if err := runDownloadTargetJobs(ctx, manager, request, jobs, &report); err != nil {
		return report, err
	}
	return report, nil
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

type downloadTargetJob struct {
	target     sdk.Reference
	illustType string
}

type downloadTargetJobResult struct {
	items []DownloadedArtwork
	err   error
}

func collectUserArtworkJobs(ctx context.Context, client DownloadTargetClient, user sdk.Reference, jobs *[]downloadTargetJob, report *DownloadReport) error {
	for _, illustType := range []sdk.IllustType{sdk.IllustTypeIllust, sdk.IllustTypeManga, sdk.IllustTypeUgoira} {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := TraversePages(ctx, PagePlan{}, func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
			result, err := client.UserArtworks(ctx, sdk.UserArtworksRequest{UserID: user.ID, Type: illustType, Cursor: cursor})
			if err != nil {
				return nil, "", err
			}
			if result == nil {
				return nil, "", errors.New("pixiv sdk returned an empty user artworks result")
			}
			return result.Illusts, result.NextCursor, nil
		}, func(items []sdk.Illust) error {
			for _, illust := range items {
				target := sdk.Reference{Kind: sdk.ReferenceKindArtwork, ID: illust.ID}
				*jobs = append(*jobs, downloadTargetJob{target: target, illustType: illust.Type})
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
			URL: user.URL(), Type: string(illustType), Message: err.Error(), Cause: err,
		})
	}
	return nil
}

func runDownloadTargetJobs(ctx context.Context, manager DownloadManager, request DownloadRequest, jobs []downloadTargetJob, report *DownloadReport) error {
	for _, job := range jobs {
		if err := ctx.Err(); err != nil {
			return err
		}
		one := request
		one.IllustIDs = []int64{job.target.ID}
		items, err := manager.Download(ctx, one)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			report.Failures = append(report.Failures, DownloadFailure{
				URL: job.target.URL(), IllustID: job.target.ID, Type: job.illustType, Message: err.Error(), Cause: err,
			})
			continue
		}
		report.Items = append(report.Items, items...)
	}
	return nil
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
