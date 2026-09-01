package reversesearch

import (
	"context"
	"errors"
	"sync"
)

var aggregateProviderOrder = [...]Provider{ProviderSauceNAO, ProviderASCII2DColor, ProviderASCII2DBOVW}

// AggregatorDependencies 是 provider 编排器的构造端口。
type AggregatorDependencies struct {
	SauceNAO ProviderClient
	ASCII2D  ASCII2DClient
}

// Aggregator 在同一快照上执行 provider 并构造稳定领域 envelope。
type Aggregator struct {
	sauceNAO  ProviderClient
	ascii2d   ASCII2DClient
	closeOnce sync.Once
	closeErr  error
}

func NewAggregator(dependencies AggregatorDependencies) *Aggregator {
	return &Aggregator{sauceNAO: dependencies.SauceNAO, ascii2d: dependencies.ASCII2D}
}

// Close 以与 provider 构造相反的顺序释放长期存活的 provider 资源；每个
// provider 都是可选的 lifecycle port，未实现 Close 的测试/嵌入 provider 不受影响。
func (a *Aggregator) Close() error {
	if a == nil {
		return nil
	}
	a.closeOnce.Do(func() {
		if closer, ok := a.ascii2d.(Closer); ok {
			a.closeErr = errors.Join(a.closeErr, closer.Close())
		}
		if closer, ok := a.sauceNAO.(Closer); ok {
			a.closeErr = errors.Join(a.closeErr, closer.Close())
		}
	})
	return a.closeErr
}

func (a *Aggregator) Preflight(ctx context.Context, query PayloadQuery) error {
	if ctx == nil {
		return NewError(CodeInvalidRequest, "reverse search context is required", nil)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	switch query.Provider {
	case ProviderSauceNAO:
		if a == nil || a.sauceNAO == nil {
			return NewError(CodeProviderNotConfigured, "SauceNAO provider is not configured", nil)
		}
		return a.sauceNAO.Preflight(ctx)
	case ProviderASCII2DColor, ProviderASCII2DBOVW:
		if a == nil || a.ascii2d == nil {
			return NewError(CodeProviderNotConfigured, "ascii2d provider is not configured", nil)
		}
		return a.ascii2d.Preflight(ctx)
	case ProviderAll:
		// all 必须允许单个 provider 的 preflight 失败转为 partial；具体错误在
		// SearchPayload 中按固定 provider 顺序进入 envelope。
		return nil
	default:
		return NewError(CodeInvalidRequest, "reverse search provider is invalid", nil)
	}
}

func (a *Aggregator) SearchPayload(ctx context.Context, request PayloadRequest) (Response, error) {
	if ctx == nil {
		return Response{}, NewError(CodeInvalidRequest, "reverse search context is required", nil)
	}
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	if request.Snapshot == nil {
		return Response{}, NewError(CodeInvalidRequest, "image snapshot is required", nil)
	}
	if request.Provider == ProviderAll {
		return a.searchAll(ctx, request.Snapshot, request.PixivOnly)
	}
	providerResponse, err := a.searchSingle(ctx, request.Provider, request.Snapshot)
	if err != nil {
		if cancellation := cancellationError(ctx, err); cancellation != nil {
			return Response{}, cancellation
		}
		providerError, safeErr := safeProviderFailure(request.Provider, err)
		return Response{
			Providers:      []ProviderSummary{{Name: request.Provider, Status: ProviderStatusError}},
			Results:        make([]Result, 0),
			ProviderErrors: []ProviderError{providerError},
		}, safeErr
	}
	return responseFromProvider(request.Provider, providerResponse, request.PixivOnly), nil
}

type providerOutcome struct {
	response ProviderResponse
	err      error
}

func (a *Aggregator) searchAll(ctx context.Context, snapshot *Snapshot, pixivOnly bool) (Response, error) {
	var outcomes [len(aggregateProviderOrder)]providerOutcome
	var branches sync.WaitGroup
	branches.Add(2)
	go func() {
		defer branches.Done()
		if a == nil || a.sauceNAO == nil {
			outcomes[0].err = NewError(CodeProviderNotConfigured, "SauceNAO provider is not configured", nil)
			return
		}
		if err := a.sauceNAO.Preflight(ctx); err != nil {
			outcomes[0].err = err
			return
		}
		outcomes[0].response, outcomes[0].err = a.sauceNAO.Search(ctx, snapshot)
	}()
	go func() {
		defer branches.Done()
		a.searchAllASCII2D(ctx, snapshot, &outcomes)
	}()
	branches.Wait()

	for _, outcome := range outcomes {
		if cancellation := cancellationError(ctx, outcome.err); cancellation != nil {
			return Response{}, cancellation
		}
	}
	return responseFromOutcomes(outcomes, pixivOnly)
}

func (a *Aggregator) searchAllASCII2D(ctx context.Context, snapshot *Snapshot, outcomes *[len(aggregateProviderOrder)]providerOutcome) {
	if a == nil || a.ascii2d == nil {
		err := NewError(CodeProviderNotConfigured, "ascii2d provider is not configured", nil)
		outcomes[1].err, outcomes[2].err = err, err
		return
	}
	if err := a.ascii2d.Preflight(ctx); err != nil {
		outcomes[1].err, outcomes[2].err = err, err
		return
	}
	session, err := a.ascii2d.Upload(ctx, snapshot)
	if err != nil {
		outcomes[1].err, outcomes[2].err = err, err
		return
	}
	var searches sync.WaitGroup
	for index := 1; index < len(outcomes); index++ {
		searches.Add(1)
		go func(index int) {
			defer searches.Done()
			outcomes[index].response, outcomes[index].err = session.Search(ctx, aggregateProviderOrder[index])
		}(index)
	}
	searches.Wait()
}

func responseFromOutcomes(outcomes [len(aggregateProviderOrder)]providerOutcome, pixivOnly bool) (Response, error) {
	response := Response{
		Providers:      make([]ProviderSummary, 0, len(outcomes)),
		Results:        make([]Result, 0),
		ProviderErrors: make([]ProviderError, 0),
	}
	canonical := make(map[pixivRefKey]int)
	successes := 0
	for index, outcome := range outcomes {
		provider := aggregateProviderOrder[index]
		if outcome.err != nil {
			providerError, _ := safeProviderFailure(provider, outcome.err)
			response.Providers = append(response.Providers, ProviderSummary{Name: provider, Status: ProviderStatusError})
			response.ProviderErrors = append(response.ProviderErrors, providerError)
			continue
		}
		successes++
		response.Providers = append(response.Providers, ProviderSummary{
			Name: provider, Status: ProviderStatusSuccess, ResultCount: len(outcome.response.Matches), Quota: cloneQuota(outcome.response.Quota),
		})
		appendProviderMatches(&response.Results, canonical, provider, outcome.response.Matches, pixivOnly)
	}
	if successes == 0 {
		return response, NewError(CodeAllProvidersFailed, "all reverse search providers failed", nil)
	}
	response.Partial = successes != len(outcomes)
	return response, nil
}

func (a *Aggregator) searchSingle(ctx context.Context, provider Provider, snapshot *Snapshot) (ProviderResponse, error) {
	switch provider {
	case ProviderSauceNAO:
		if a == nil || a.sauceNAO == nil {
			return ProviderResponse{}, NewError(CodeProviderNotConfigured, "SauceNAO provider is not configured", nil)
		}
		return a.sauceNAO.Search(ctx, snapshot)
	case ProviderASCII2DColor, ProviderASCII2DBOVW:
		if a == nil || a.ascii2d == nil {
			return ProviderResponse{}, NewError(CodeProviderNotConfigured, "ascii2d provider is not configured", nil)
		}
		session, err := a.ascii2d.Upload(ctx, snapshot)
		if err != nil {
			return ProviderResponse{}, err
		}
		return session.Search(ctx, provider)
	default:
		return ProviderResponse{}, NewError(CodeInvalidRequest, "reverse search provider is invalid", nil)
	}
}

func safeProviderFailure(provider Provider, err error) (ProviderError, error) {
	var classified *Error
	if errors.As(err, &classified) {
		// 只复制已审查的稳定字段；不能把 provider 的 cause 或 errors.Join 诊断带过边界。
		safeErr := NewError(classified.Code(), classified.Error(), nil)
		return ProviderError{Provider: provider, Code: safeErr.Code(), Message: safeErr.Error()}, safeErr
	}
	safeErr := NewError(CodeProviderFailed, "reverse search provider failed", nil)
	return ProviderError{Provider: provider, Code: CodeProviderFailed, Message: safeErr.Error()}, safeErr
}

func cancellationError(ctx context.Context, err error) error {
	if ctx != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return nil
}

func responseFromProvider(provider Provider, providerResponse ProviderResponse, pixivOnly bool) Response {
	response := Response{
		Providers: []ProviderSummary{{
			Name: provider, Status: ProviderStatusSuccess, ResultCount: len(providerResponse.Matches), Quota: cloneQuota(providerResponse.Quota),
		}},
		Results:        make([]Result, 0, len(providerResponse.Matches)),
		ProviderErrors: make([]ProviderError, 0),
	}
	appendProviderMatches(&response.Results, make(map[pixivRefKey]int), provider, providerResponse.Matches, pixivOnly)
	return response
}

var _ PayloadSearcher = (*Aggregator)(nil)
var _ Closer = (*Aggregator)(nil)
