package reversesearch

import (
	"context"
	"errors"
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

// PayloadSearcher 在同一个不可变快照上执行 provider 查询与聚合。Task 5 的
// aggregator 实现该端口，Facade 只拥有载荷生命周期。
type PayloadSearcher interface {
	Preflight(context.Context, PayloadQuery) error
	SearchPayload(context.Context, PayloadRequest) (Response, error)
}

// Dependencies 是 Facade 的构造端口。
type Dependencies struct {
	Sources  SourceLoader
	Payloads PayloadSearcher
}

// Facade 将不透明 source 固化一次，并确保 provider 查询结束后清理快照。
type Facade struct {
	sources  SourceLoader
	payloads PayloadSearcher
}

func NewFacade(dependencies Dependencies) *Facade {
	return &Facade{sources: dependencies.Sources, payloads: dependencies.Payloads}
}

// New 是 NewFacade 的简洁别名。
func New(dependencies Dependencies) *Facade { return NewFacade(dependencies) }

func (f *Facade) Search(ctx context.Context, request Request) (response Response, err error) {
	if ctx == nil {
		return Response{}, NewError(CodeInvalidRequest, "reverse search context is required", nil)
	}
	if f == nil || f.sources == nil {
		return Response{}, NewError(CodeSourceLoaderNotConfigured, "reverse search source loader is not configured", nil)
	}
	if f.payloads == nil {
		return Response{}, NewError(CodeProviderNotConfigured, "reverse search payload searcher is not configured", nil)
	}
	query := PayloadQuery{Provider: request.Provider, PixivOnly: request.PixivOnly}
	if err := f.payloads.Preflight(ctx, query); err != nil {
		return Response{}, err
	}
	snapshot, err := f.sources.Load(ctx, request.Source)
	if err != nil {
		return Response{}, err
	}
	defer func() {
		if closeErr := snapshot.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()

	response, err = f.payloads.SearchPayload(ctx, PayloadRequest{
		Snapshot: snapshot, Provider: request.Provider, PixivOnly: request.PixivOnly,
	})
	response.Input = Input{Kind: snapshot.Kind(), SHA256: snapshot.SHA256()}
	return response, err
}

var _ Searcher = (*Facade)(nil)
