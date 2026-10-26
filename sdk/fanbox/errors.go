package fanbox

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/fanboxapi"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

const product = "fanbox"

func newError(operation string, code sdk.Reason, err error) *sdk.Error {
	return sdk.NewError(product, operation, code, sdk.WithCause(err))
}

// classifyResponseError classifies a wire-decoding failure, preserving a reason
// the mapping already assigned (for example ResourceForbidden).
func classifyResponseError(operation string, err error) error {
	var classified *sdk.Error
	if errors.As(err, &classified) {
		return err
	}
	return newError(operation, sdk.MalformedUpstreamResponse, err)
}

// classifyError maps an adapter failure to a classified error. It preserves
// context cancellation and the distinct challenge/forbidden/expired outcomes.
func classifyError(operation string, err error) *sdk.Error {
	switch {
	case errors.Is(err, context.Canceled):
		return sdk.NewError(product, operation, sdk.UpstreamError, sdk.WithCause(context.Canceled))
	case errors.Is(err, context.DeadlineExceeded):
		return sdk.NewError(product, operation, sdk.UpstreamError, sdk.WithCause(context.DeadlineExceeded))
	case errors.Is(err, fanboxapi.ErrChallenge):
		return newError(operation, sdk.ChallengeRequired, err)
	case errors.Is(err, fanboxapi.ErrSolverFailed):
		return newError(operation, sdk.ChallengeRequired, err)
	case errors.Is(err, fanboxapi.ErrSolverUnavailable):
		return newError(operation, sdk.UpstreamUnavailable, err)
	case errors.Is(err, fanboxapi.ErrMalformedSolverResponse), errors.Is(err, fanboxapi.ErrMalformedResponse):
		return newError(operation, sdk.MalformedUpstreamResponse, err)
	case errors.Is(err, fanboxapi.ErrForbidden):
		return newError(operation, sdk.Forbidden, err)
	case errors.Is(err, fanboxapi.ErrNotAuthenticated):
		return newError(operation, sdk.CredentialsExpired, err)
	default:
		return newError(operation, sdk.UpstreamError, err)
	}
}
