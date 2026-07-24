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
	Download(context.Context, sdk.ResourceRef, string) error
}

// DownloadTargetClient 在下载资源能力之外提供作者作品列表，用于把用户 URL 展开为视觉作品。
// 该接口仍只依赖顶层 public SDK 的规范化能力。
type DownloadTargetClient interface {
	DownloadClient
	UserArtworks(context.Context, sdk.UserArtworksRequest) (*sdk.IllustListResult, error)
}

// 质量与页选择契约由 public SDK 拥有，application 仅 alias 以便 CLI/MCP 共用。
type DownloadQuality = sdk.DownloadQuality

const (
	DownloadQualityOriginal = sdk.DownloadQualityOriginal
	DownloadQualityRegular  = sdk.DownloadQualityRegular
	DownloadQualitySmall    = sdk.DownloadQualitySmall
	DownloadQualityThumb    = sdk.DownloadQualityThumb
	DownloadQualityMini     = sdk.DownloadQualityMini
)

var (
	ParsePageSpec           = sdk.ParsePageSpec
	ValidateDownloadQuality = sdk.ValidateDownloadQuality
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
}

// DownloadReport 同时保留成功产物和独立失败，使作者全量下载无需持久化 job 状态也能说明结果。
type DownloadReport struct {
	Items    []DownloadedArtwork
	Failures []DownloadFailure
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
}

type DownloadService struct {
	NewManager DownloadManagerFactory
}

// Download 把 operation snapshot 与本次运行配置交给 composition root 注入的下载器。
func (s DownloadService) Download(ctx context.Context, client DownloadClient, request DownloadRequest) ([]DownloadedArtwork, error) {
	manager, request, err := s.newManager(client, request)
	if err != nil {
		return nil, err
	}
	return manager.Download(ctx, request)
}

// DownloadTargets 依输入顺序下载作品，或展开作者的全部 illust、manga、ugoira。
// 单件失败会进入 report 并继续；用户取消则立即返回 context 错误，不伪装成普通部分失败。
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
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		if target.ID <= 0 || target.URL() == "" {
			return report, errors.New("download target is invalid")
		}
		switch target.Kind {
		case sdk.ReferenceKindArtwork:
			if err := downloadOne(ctx, manager, request, target, "", &report); err != nil {
				return report, err
			}
		case sdk.ReferenceKindUser:
			if err := downloadUserArtworks(ctx, client, manager, request, target, &report); err != nil {
				return report, err
			}
		default:
			return report, errors.New("download target is invalid")
		}
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
	manager, err := s.NewManager(client, request.DownloadPath, request.FilenameTemplate)
	if err != nil {
		return nil, DownloadRequest{}, err
	}
	if isNilLike(manager) {
		return nil, DownloadRequest{}, errors.New("download manager factory returned nil")
	}
	return manager, request, nil
}

func downloadUserArtworks(ctx context.Context, client DownloadTargetClient, manager DownloadManager, request DownloadRequest, user sdk.Reference, report *DownloadReport) error {
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
				if err := downloadOne(ctx, manager, request, target, illust.Type, report); err != nil {
					return err
				}
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
			URL: user.URL(), Type: string(illustType), Message: err.Error(),
		})
	}
	return nil
}

func downloadOne(ctx context.Context, manager DownloadManager, request DownloadRequest, target sdk.Reference, illustType string, report *DownloadReport) error {
	one := request
	one.IllustIDs = []int64{target.ID}
	items, err := manager.Download(ctx, one)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		report.Failures = append(report.Failures, DownloadFailure{
			URL: target.URL(), IllustID: target.ID, Type: illustType, Message: err.Error(),
		})
		return nil
	}
	if len(items) == 0 {
		return nil
	}
	report.Items = append(report.Items, items...)
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
