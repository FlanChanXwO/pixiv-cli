package reversesearch

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/ascii2d"
	"github.com/FlanChanXwO/pixiv-cli/sdk/saucenao"
)

const (
	operationCloudflarePrefix = "cloudflare."
	operationCloudflareClose  = "cloudflare.Close"
)

func (s *Service) searchSauceNAO(ctx context.Context, image []byte) providerOutcome {
	if err := s.sauceNAOCredential(); err != nil {
		return providerOutcome{err: err}
	}
	result, err := saucenao.Search(ctx, saucenao.Options{Image: image, APIKey: s.options.SauceNAOKey, HTTPClient: s.options.HTTPClient})
	if err != nil && result.Matches == nil {
		return providerOutcome{err: sauceNAOFailure(err)}
	}
	response := ProviderResponse{Matches: make([]Match, 0, len(result.Matches))}
	if quota := result.Quota; quota != nil {
		response.Quota = &Quota{ShortRemaining: int(quota.ShortRemaining), LongRemaining: int(quota.LongRemaining), ShortLimit: int(quota.ShortLimit), LongLimit: int(quota.LongLimit)}
	}
	for _, match := range result.Matches {
		response.Matches = append(response.Matches, Match{Rank: match.Rank, Similarity: match.Similarity, IndexID: int(match.IndexID), IndexName: match.IndexName, Title: match.Title, Author: match.Author, ArtworkID: match.ArtworkID, UserID: match.UserID, ExternalURLs: match.ExternalURLs})
	}
	outcome := providerOutcome{response: response}
	if err != nil {
		outcome.closeErr = sauceNAOFailure(err)
	}
	return outcome
}

func sauceNAOFailure(err error) error {
	var classified *sdk.Error
	errors.As(err, &classified)
	if classified.HTTPStatus >= http.StatusMultipleChoices {
		return NewError(CodeUpstreamHTTPStatus, "SauceNAO returned an unsuccessful HTTP status", err)
	}
	if classified.Reason == sdk.MalformedUpstreamResponse {
		return NewError(CodeMalformedUpstreamResponse, "SauceNAO returned a malformed response", err)
	}
	if classified.Reason == sdk.RateLimited || classified.Reason == sdk.UpstreamError {
		return NewError(CodeProviderFailed, "SauceNAO rejected the query", err)
	}
	return NewError(CodeProviderFailed, "SauceNAO request failed", err)
}

func (s *Service) ascii2dPage(page *ascii2d.Page) (ProviderResponse, error) {
	if page.Err != nil {
		return ProviderResponse{}, s.ascii2dFailure(page.Err, true)
	}
	response := ProviderResponse{Matches: make([]Match, 0, len(page.Matches))}
	for _, match := range page.Matches {
		response.Matches = append(response.Matches, Match{Rank: match.Rank, IndexName: match.IndexName, Title: match.Title, Author: match.Author, ArtworkID: match.ArtworkID, ExternalURLs: match.ExternalURLs})
	}
	return response, nil
}

func (s *Service) ascii2dFailure(err error, page bool) error {
	var classified *sdk.Error
	errors.As(err, &classified)
	// SDK 的细分错误在应用边界恢复既有的安全消息与错误分类。
	if strings.HasPrefix(classified.Operation, operationCloudflarePrefix) {
		code, message := CodeSolverFailed, "ascii2d challenge solver failed"
		switch {
		case classified.Reason == sdk.MalformedUpstreamResponse:
			code, message = CodeMalformedSolverResponse, "ascii2d challenge solver response is malformed"
		case classified.Reason == sdk.UpstreamUnavailable && classified.HTTPStatus == 0:
			code, message = CodeSolverUnavailable, "ascii2d challenge solver is unavailable"
		case classified.Reason == sdk.InvalidArgument:
			code, message = CodeSolverUnavailable, "ascii2d challenge solver is unavailable"
		}
		return NewError(code, message, err)
	}
	if classified.Reason == sdk.ChallengeRequired {
		if !page && s.options.FlareSolverrURL != "" {
			return NewError(CodeSolverFailed, "ascii2d challenge solver failed", err)
		}
		if page {
			return NewError(CodeUpstreamHTTPStatus, "ascii2d challenge detected", err)
		}
		return NewError(CodeChallengeRequired, "ascii2d challenge requires solver recovery", err)
	}
	if classified.Message == "invalid ascii2d User-Agent" {
		return NewError(CodeInvalidRequest, "ascii2d user-agent contains invalid header characters", err)
	}
	if classified.Reason == sdk.InvalidArgument {
		if classified.Message == "ascii2d image exceeds the 10 MB limit" {
			return NewError(CodeInvalidSource, classified.Message, err)
		}
		return NewError(CodeInvalidSource, "ascii2d supports only JPEG, PNG, or WEBP images", err)
	}
	if classified.Message == "ascii2d returned an unsuccessful response" {
		return NewError(CodeUpstreamHTTPStatus, "ascii2d returned an unsuccessful HTTP status", err)
	}
	if classified.Reason == sdk.MalformedUpstreamResponse {
		message := "ascii2d returned a malformed response"
		if page {
			message = "ascii2d returned a malformed result page"
		}
		if classified.Message == "ascii2d returned a malformed upload location" ||
			classified.Message == "ascii2d returned an unsafe upload location" ||
			classified.Message == "ascii2d redirected outside its origin" {
			message = classified.Message
		}
		return NewError(CodeMalformedUpstreamResponse, message, err)
	}
	message := "ascii2d session request failed"
	if classified.Message == "ascii2d upload request failed" {
		message = classified.Message
	}
	if page {
		message = "ascii2d result request failed"
	}
	return NewError(CodeProviderFailed, message, err)
}

// ascii2d SDK 会合并页面错误与会话关闭错误；只提取关闭错误，避免将页面重复报成失败。
func (s *Service) ascii2dCloseFailure(err error) error {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var failures []error
		for _, cause := range joined.Unwrap() {
			failures = append(failures, s.ascii2dCloseFailure(cause))
		}
		return errors.Join(failures...)
	}
	var classified *sdk.Error
	if errors.As(err, &classified) && classified.Operation == operationCloudflareClose {
		return s.ascii2dFailure(err, false)
	}
	return nil
}
