package reversesearch

import (
	"context"
	"errors"
	"sync"
)

// PayloadQuery 是可在读取 source 前完成校验的查询元数据。
type PayloadQuery struct {
	Provider  Provider
	PixivOnly bool
}

// PayloadRequest 是 source 已固化后的内部编排请求；它不再携带原始 source。
type PayloadRequest struct {
	Snapshot  *Snapshot
	Provider  Provider
	PixivOnly bool
}

// PayloadSearcher 在同一个不可变快照上执行 provider 查询与聚合。
// Aggregator 实现该端口，Service 只拥有载荷生命周期。
type PayloadSearcher interface {
	Preflight(context.Context, PayloadQuery) error
	SearchPayload(context.Context, PayloadRequest) (Response, error)
}

// Service 将不透明 source 固化一次，并确保 provider 查询结束后清理快照。
type Service struct {
	sources   SourceLoader
	payloads  PayloadSearcher
	closeOnce sync.Once
	closeErr  error
}

func NewService(sources SourceLoader, payloads PayloadSearcher) *Service {
	return &Service{sources: sources, payloads: payloads}
}

// Close 释放 Service 所拥有的 payload 编排器。source snapshot 是每次 Search
// 的局部资源，已经在 Search 返回前关闭；长期存活的 provider/session 资源则
// 由 payloads 的可选生命周期端口释放。
func (s *Service) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		if closer, ok := s.payloads.(Closer); ok {
			s.closeErr = closer.Close()
		}
	})
	return s.closeErr
}

func (s *Service) Search(ctx context.Context, request Request) (response Response, err error) {
	if ctx == nil {
		return Response{}, NewError(CodeInvalidRequest, "reverse search context is required", nil)
	}
	if s == nil || s.sources == nil {
		return Response{}, NewError(CodeSourceLoaderNotConfigured, "reverse search source loader is not configured", nil)
	}
	if s.payloads == nil {
		return Response{}, NewError(CodeProviderNotConfigured, "reverse search payload searcher is not configured", nil)
	}
	query := PayloadQuery{Provider: request.Provider, PixivOnly: request.PixivOnly}
	if err := s.payloads.Preflight(ctx, query); err != nil {
		return Response{}, err
	}
	snapshot, err := s.sources.Load(ctx, request.Source)
	if err != nil {
		return Response{}, err
	}
	defer func() {
		if closeErr := snapshot.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()

	response, err = s.payloads.SearchPayload(ctx, PayloadRequest{
		Snapshot: snapshot, Provider: request.Provider, PixivOnly: request.PixivOnly,
	})
	response.Input = Input{Kind: snapshot.Kind(), SHA256: snapshot.SHA256()}
	return response, err
}

var _ Searcher = (*Service)(nil)
var _ Closer = (*Service)(nil)
