package reversesearch

import (
	"context"
	"errors"
	"sync"
)

var aggregateProviderOrder = [...]Provider{ProviderSauceNAO, ProviderASCII2DColor, ProviderASCII2DBOVW}

// Aggregator 在同一快照上执行 provider 并构造稳定领域 envelope。
type Aggregator struct {
	sauceNAO ProviderClient
	ascii2d  ASCII2DClient
}

// NewAggregator 组合 SauceNAO 与 ascii2d 端口；Close 按相反顺序释放两者。
func NewAggregator(sauceNAO ProviderClient, ascii2d ASCII2DClient) *Aggregator {
	return &Aggregator{sauceNAO: sauceNAO, ascii2d: ascii2d}
}

// Close 以与 provider 构造相反的顺序释放长期存活的 provider 资源。
func (a *Aggregator) Close() error {
	return errors.Join(a.ascii2d.Close(), a.sauceNAO.Close())
}

func (a *Aggregator) preflight(ctx context.Context, provider Provider) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch provider {
	case ProviderSauceNAO:
		return a.sauceNAO.Preflight(ctx)
	case ProviderASCII2DColor, ProviderASCII2DBOVW:
		return nil
	case ProviderAll:
		// all 必须允许单个 provider 的 preflight 失败转为 partial；具体错误在
		// search 中按固定 provider 顺序进入 envelope。
		return nil
	default:
		return NewError(CodeInvalidRequest, "reverse search provider is invalid", nil)
	}
}

func (a *Aggregator) search(ctx context.Context, snapshot *Snapshot, provider Provider, pixivOnly bool) (Response, error) {
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	if provider == ProviderAll {
		return a.searchAll(ctx, snapshot, pixivOnly)
	}
	providerResponse, err := a.searchSingle(ctx, provider, snapshot)
	if err != nil {
		if cancellation := cancellationError(ctx, err); cancellation != nil {
			return Response{}, cancellation
		}
		providerError, safeErr := safeProviderFailure(provider, err)
		return Response{
			Providers:      []ProviderSummary{{Name: provider, Status: ProviderStatusError}},
			Results:        make([]Result, 0),
			ProviderErrors: []ProviderError{providerError},
		}, safeErr
	}
	return responseFromProvider(provider, providerResponse, pixivOnly), nil
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
	var failures []error
	for index, outcome := range outcomes {
		provider := aggregateProviderOrder[index]
		if outcome.err != nil {
			failures = append(failures, outcome.err)
			providerError, _ := safeProviderFailure(provider, outcome.err)
			response.Providers = append(response.Providers, ProviderSummary{Name: provider, Status: ProviderStatusError})
			response.ProviderErrors = append(response.ProviderErrors, providerError)
			continue
		}
		successes++
		response.Providers = append(response.Providers, ProviderSummary{
			Name: provider, Status: ProviderStatusSuccess, ResultCount: len(outcome.response.Matches), Quota: outcome.response.Quota,
		})
		appendProviderMatches(&response.Results, canonical, provider, outcome.response.Matches, pixivOnly)
	}
	if successes == 0 {
		return response, NewError(CodeAllProvidersFailed, "all reverse search providers failed", errors.Join(failures...))
	}
	response.Partial = successes != len(outcomes)
	return response, nil
}

func (a *Aggregator) searchSingle(ctx context.Context, provider Provider, snapshot *Snapshot) (ProviderResponse, error) {
	switch provider {
	case ProviderSauceNAO:
		return a.sauceNAO.Search(ctx, snapshot)
	case ProviderASCII2DColor, ProviderASCII2DBOVW:
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
		// envelope 只取稳定字段；原始错误留在 cause 中供 errors.Is/As 使用。
		safeErr := NewError(classified.Code(), classified.Error(), err)
		return ProviderError{Provider: provider, Code: safeErr.Code(), Message: safeErr.Error()}, safeErr
	}
	safeErr := NewError(CodeProviderFailed, "reverse search provider failed", err)
	return ProviderError{Provider: provider, Code: CodeProviderFailed, Message: safeErr.Error()}, safeErr
}

func cancellationError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return nil
}

func responseFromProvider(provider Provider, providerResponse ProviderResponse, pixivOnly bool) Response {
	response := Response{
		Providers: []ProviderSummary{{
			Name: provider, Status: ProviderStatusSuccess, ResultCount: len(providerResponse.Matches), Quota: providerResponse.Quota,
		}},
		Results:        make([]Result, 0, len(providerResponse.Matches)),
		ProviderErrors: make([]ProviderError, 0),
	}
	appendProviderMatches(&response.Results, make(map[pixivRefKey]int), provider, providerResponse.Matches, pixivOnly)
	return response
}

var _ Closer = (*Aggregator)(nil)
