package pixiv

import (
	"errors"
	"net/http"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

func newError(operation string, reason sdk.Reason, message string, cause error) *sdk.Error {
	return &sdk.Error{Operation: "pixiv." + operation, Reason: reason, Message: message, Err: cause}
}

func isUnauthorized(err error) bool {
	var failure api.Failure
	return errors.As(err, &failure) && failure.Kind == api.FailureHTTPStatus && failure.StatusCode == http.StatusUnauthorized
}

func classifyError(operation string, err error, oauth bool) *sdk.Error {
	var failure api.Failure
	if !errors.As(err, &failure) {
		return newError(operation, sdk.UpstreamError, "operation failed", err)
	}
	reason := sdk.UpstreamError
	switch failure.Kind {
	case api.FailureMalformed:
		reason = sdk.MalformedUpstreamResponse
	case api.FailureResourceForbidden:
		reason = sdk.ResourceForbidden
	case api.FailureTransport:
		reason = sdk.UpstreamUnavailable
	case api.FailureHTTPStatus:
		switch failure.StatusCode {
		case http.StatusBadRequest:
			reason = sdk.InvalidArgument
			if oauth {
				reason = sdk.CredentialsExpired
			}
		case http.StatusUnauthorized:
			reason = sdk.CredentialsExpired
		case http.StatusForbidden:
			reason = sdk.Forbidden
		case http.StatusNotFound:
			reason = sdk.NotFound
		case http.StatusGone:
			reason = sdk.ContentUnavailable
		case http.StatusTooManyRequests:
			reason = sdk.RateLimited
		}
	}
	result := newError(operation, reason, "upstream operation failed", failure)
	result.HTTPStatus = failure.StatusCode
	if failure.HasRetryAfter {
		delay := failure.RetryAfter
		result.RetryAfter = &delay
	}
	return result
}
