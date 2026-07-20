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
	if s.NewManager == nil {
		return nil, errors.New("download manager factory is not configured")
	}
	if isNilLike(client) {
		return nil, errors.New("download operation client is not configured")
	}
	if request.Quality == "" {
		request.Quality = DownloadQualityOriginal
	}
	if err := ValidateDownloadQuality(request.Quality); err != nil {
		return nil, err
	}
	manager, err := s.NewManager(client, request.DownloadPath, request.FilenameTemplate)
	if err != nil {
		return nil, err
	}
	if isNilLike(manager) {
		return nil, errors.New("download manager factory returned nil")
	}
	return manager.Download(ctx, request)
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
