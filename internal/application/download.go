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

type DownloadManager interface {
	Download(context.Context, []int64) ([]DownloadedArtwork, error)
}

type DownloadManagerFactory func(client DownloadClient, downloadPath, filenameTemplate string) (DownloadManager, error)

type DownloadRequest struct {
	IllustIDs        []int64
	DownloadPath     string
	FilenameTemplate string
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
	manager, err := s.NewManager(client, request.DownloadPath, request.FilenameTemplate)
	if err != nil {
		return nil, err
	}
	if isNilLike(manager) {
		return nil, errors.New("download manager factory returned nil")
	}
	return manager.Download(ctx, request.IllustIDs)
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
