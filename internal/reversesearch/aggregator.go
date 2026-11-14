package reversesearch

import (
	"context"
	"errors"
	"sync"

	"github.com/FlanChanXwO/pixiv-cli/sdk/ascii2d"
)

var aggregateProviderOrder = [...]Provider{ProviderSauceNAO, ProviderASCII2DColor, ProviderASCII2DBOVW}

func (s *Service) search(ctx context.Context, image []byte, provider Provider, pixivOnly bool) (Response, error) {
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	if provider == ProviderAll {
		return s.searchAll(ctx, image, pixivOnly)
	}
	outcome := s.searchSingle(ctx, provider, image)
	s.retainCloseError(outcome.closeErr)
	if cancellation := cancellationError(ctx, errors.Join(outcome.err, outcome.closeErr)); cancellation != nil {
		return Response{}, cancellation
	}
	if outcome.err != nil {
		return responseFromFailure(provider, outcome.err)
	}
	return responseFromProvider(provider, outcome.response, pixivOnly), nil
}

type providerOutcome struct {
	response ProviderResponse
	err      error
	closeErr error
}

func (s *Service) searchAll(ctx context.Context, image []byte, pixivOnly bool) (Response, error) {
	var outcomes [len(aggregateProviderOrder)]providerOutcome
	var branches sync.WaitGroup
	// 两个服务并行；等待两边结束后再合并，避免取消时留下上传任务。
	branches.Add(2)
	go func() {
		defer branches.Done()
		outcomes[0] = s.searchSauceNAO(ctx, image)
	}()
	go func() {
		defer branches.Done()
		outcomes[1].closeErr = s.searchAllASCII2D(ctx, image, &outcomes)
	}()
	branches.Wait()

	for _, outcome := range outcomes {
		s.retainCloseError(outcome.closeErr)
	}
	for _, outcome := range outcomes {
		if cancellation := cancellationError(ctx, errors.Join(outcome.err, outcome.closeErr)); cancellation != nil {
			return Response{}, cancellation
		}
	}
	return responseFromOutcomes(outcomes, pixivOnly)
}

func (s *Service) searchAllASCII2D(ctx context.Context, image []byte, outcomes *[len(aggregateProviderOrder)]providerOutcome) error {
	result, err := ascii2d.Search(ctx, s.ascii2dOptions(image, ascii2d.Both))
	if result.Color == nil {
		outcomes[1].err = s.ascii2dFailure(err, false)
		outcomes[2].err = outcomes[1].err
		return s.ascii2dCloseFailure(err)
	}
	outcomes[1].response, outcomes[1].err = s.ascii2dPage(result.Color)
	outcomes[2].response, outcomes[2].err = s.ascii2dPage(result.BOVW)
	// 清理失败独立返回，已经成功的页面仍进入结果与摘要。
	return s.ascii2dCloseFailure(err)
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

func (s *Service) searchSingle(ctx context.Context, provider Provider, image []byte) providerOutcome {
	if provider == ProviderSauceNAO {
		return s.searchSauceNAO(ctx, image)
	}
	mode := ascii2d.Color
	if provider == ProviderASCII2DBOVW {
		mode = ascii2d.BOVW
	}
	result, err := ascii2d.Search(ctx, s.ascii2dOptions(image, mode))
	page := result.Color
	if mode == ascii2d.BOVW {
		page = result.BOVW
	}
	if page == nil {
		return providerOutcome{err: s.ascii2dFailure(err, false), closeErr: s.ascii2dCloseFailure(err)}
	}
	response, pageErr := s.ascii2dPage(page)
	return providerOutcome{response: response, err: pageErr, closeErr: s.ascii2dCloseFailure(err)}
}

func responseFromFailure(provider Provider, err error) (Response, error) {
	providerError, safeErr := safeProviderFailure(provider, err)
	return Response{Providers: []ProviderSummary{{Name: provider, Status: ProviderStatusError}},
		Results: make([]Result, 0), ProviderErrors: []ProviderError{providerError}}, safeErr
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
