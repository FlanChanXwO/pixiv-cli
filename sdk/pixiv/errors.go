package pixiv

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

const product = "pixiv"

// newError constructs a classified Pixiv error carrying a controlled, redacted
// detail string. The operation name is the public SDK operation identifier.
func newError(operation string, code sdk.Reason, detail string) *sdk.Error {
	return sdk.NewError(product, operation, code, sdk.WithDetail(detail))
}

// classifyOAuthError maps an OAuth adapter failure to a classified error. It
// never includes the refresh token, response body, or callback artifacts.
func classifyOAuthError(err error, operation string) *sdk.Error {
	var failure protocol.Failure
	if !errors.As(err, &failure) {
		if errors.Is(err, context.Canceled) {
			return sdk.NewError(product, operation, sdk.UpstreamError, sdk.WithCause(context.Canceled))
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return sdk.NewError(product, operation, sdk.UpstreamError, sdk.WithCause(context.DeadlineExceeded))
		}
		// A local rejection of the refresh token (for example an invalid
		// format) is a credential problem, not an upstream failure.
		return sdk.NewError(product, operation, sdk.CredentialsExpired, sdk.WithCause(redactedCause(err)))
	}
	switch failure.Kind {
	case protocol.FailureHTTPStatus:
		switch failure.StatusCode {
		case http.StatusBadRequest:
			return sdk.NewError(product, operation, sdk.CredentialsExpired, sdk.WithHTTPStatus(failure.StatusCode), sdk.WithRetry(retryAdvice(failure)))
		case http.StatusUnauthorized:
			return sdk.NewError(product, operation, sdk.CredentialsExpired, sdk.WithHTTPStatus(failure.StatusCode), sdk.WithRetry(retryAdvice(failure)))
		case http.StatusForbidden:
			return sdk.NewError(product, operation, sdk.Forbidden, sdk.WithHTTPStatus(failure.StatusCode))
		case http.StatusTooManyRequests:
			return sdk.NewError(product, operation, sdk.RateLimited, sdk.WithHTTPStatus(failure.StatusCode), sdk.WithRetry(retryAdvice(failure)))
		case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return sdk.NewError(product, operation, sdk.UpstreamError, sdk.WithHTTPStatus(failure.StatusCode))
		default:
			return sdk.NewError(product, operation, sdk.UpstreamError, sdk.WithHTTPStatus(failure.StatusCode))
		}
	case protocol.FailureMalformed:
		return sdk.NewError(product, operation, sdk.MalformedUpstreamResponse)
	case protocol.FailureRejected:
		return sdk.NewError(product, operation, sdk.UpstreamError)
	case protocol.FailureForbidden:
		return sdk.NewError(product, operation, sdk.Forbidden)
	default:
		return classifyTransport(failure, operation)
	}
}

// classifyAppError maps an App API adapter failure to a classified error.
// Content operations without a valid token map to CredentialsExpired;
// transport failures preserve context cancellation semantics.
func classifyAppError(err error, operation string) *sdk.Error {
	var failure protocol.Failure
	if !errors.As(err, &failure) {
		if errors.Is(err, context.Canceled) {
			return sdk.NewError(product, operation, sdk.UpstreamError, sdk.WithCause(context.Canceled))
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return sdk.NewError(product, operation, sdk.UpstreamError, sdk.WithCause(context.DeadlineExceeded))
		}
		return sdk.NewError(product, operation, sdk.UpstreamError, sdk.WithCause(redactedCause(err)))
	}
	switch failure.Kind {
	case protocol.FailureHTTPStatus:
		return classifyStatus(failure, operation)
	case protocol.FailureMalformed:
		return sdk.NewError(product, operation, sdk.MalformedUpstreamResponse)
	case protocol.FailureRejected:
		return sdk.NewError(product, operation, sdk.UpstreamError)
	case protocol.FailureForbidden:
		return sdk.NewError(product, operation, sdk.Forbidden)
	default:
		return classifyTransport(failure, operation)
	}
}

func classifyStatus(failure protocol.Failure, operation string) *sdk.Error {
	switch failure.StatusCode {
	case http.StatusBadRequest:
		return sdk.NewError(product, operation, sdk.InvalidArgument, sdk.WithHTTPStatus(failure.StatusCode))
	case http.StatusUnauthorized:
		return sdk.NewError(product, operation, sdk.CredentialsExpired, sdk.WithHTTPStatus(failure.StatusCode), sdk.WithRetry(retryAdvice(failure)))
	case http.StatusForbidden:
		return sdk.NewError(product, operation, sdk.Forbidden, sdk.WithHTTPStatus(failure.StatusCode))
	case http.StatusNotFound:
		return sdk.NewError(product, operation, sdk.NotFound, sdk.WithHTTPStatus(failure.StatusCode))
	case http.StatusGone:
		return sdk.NewError(product, operation, sdk.ContentUnavailable, sdk.WithHTTPStatus(failure.StatusCode))
	case http.StatusTooManyRequests:
		return sdk.NewError(product, operation, sdk.RateLimited, sdk.WithHTTPStatus(failure.StatusCode), sdk.WithRetry(retryAdvice(failure)))
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return sdk.NewError(product, operation, sdk.UpstreamError, sdk.WithHTTPStatus(failure.StatusCode))
	default:
		return sdk.NewError(product, operation, sdk.UpstreamError, sdk.WithHTTPStatus(failure.StatusCode))
	}
}

func classifyTransport(failure protocol.Failure, operation string) *sdk.Error {
	opts := []sdk.ErrorOption{sdk.WithTransport(sdk.TransportHTTP)}
	if failure.TransportKind != "" {
		opts = append(opts, sdk.WithDetail("transport: "+string(failure.TransportKind)))
	}
	if failure.Unwrap() != nil {
		opts = append(opts, sdk.WithCause(failure))
	}
	return sdk.NewError(product, operation, sdk.UpstreamUnavailable, opts...)
}

func retryAdvice(failure protocol.Failure) sdk.RetryAdvice {
	if !failure.HasRetryAfter {
		return sdk.RetryAdvice{}
	}
	return sdk.RetryAdvice{
		Safe:     true,
		After:    time.Now().Add(failure.RetryAfter),
		HasAfter: true,
	}
}

func redactedCause(err error) error {
	return errors.New("pixiv upstream request failed")
}
