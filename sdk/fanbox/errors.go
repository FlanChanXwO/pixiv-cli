package fanbox

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/protocol"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

const product = "fanbox"

func newError(operation string, code sdk.Reason, err error) *sdk.Error {
	if err == nil {
		return sdk.NewError(product, operation, code)
	}
	return sdk.NewError(product, operation, code, sdk.WithCause(err))
}

// classifyError maps an adapter failure to a classified error. It preserves
// context cancellation and the distinct challenge/forbidden/expired outcomes.
func classifyError(operation string, err error) *sdk.Error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, context.Canceled):
		return sdk.NewError(product, operation, sdk.UpstreamError, sdk.WithCause(context.Canceled))
	case errors.Is(err, context.DeadlineExceeded):
		return sdk.NewError(product, operation, sdk.UpstreamError, sdk.WithCause(context.DeadlineExceeded))
	case errors.Is(err, protocol.ErrChallenge):
		return newError(operation, sdk.ChallengeRequired, err)
	case errors.Is(err, protocol.ErrSolverFailed):
		return newError(operation, sdk.ChallengeRequired, err)
	case errors.Is(err, protocol.ErrSolverUnavailable):
		return newError(operation, sdk.UpstreamUnavailable, err)
	case errors.Is(err, protocol.ErrMalformedSolverResponse):
		return newError(operation, sdk.MalformedUpstreamResponse, err)
	case errors.Is(err, protocol.ErrForbidden):
		return newError(operation, sdk.Forbidden, err)
	case errors.Is(err, protocol.ErrNotAuthenticated):
		return newError(operation, sdk.CredentialsExpired, err)
	default:
		return newError(operation, sdk.UpstreamError, err)
	}
}
