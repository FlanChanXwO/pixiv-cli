package fanbox

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

const product = "fanbox"

func newError(operation string, code sdk.Code, err error) *sdk.Error {
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
		return sdk.NewError(product, operation, sdk.CodeUpstreamError, sdk.WithCause(context.Canceled))
	case errors.Is(err, context.DeadlineExceeded):
		return sdk.NewError(product, operation, sdk.CodeUpstreamError, sdk.WithCause(context.DeadlineExceeded))
	case errors.Is(err, fanbox.ErrChallenge):
		return newError(operation, sdk.CodeChallengeRequired, err)
	case errors.Is(err, fanbox.ErrForbidden):
		return newError(operation, sdk.CodeForbidden, err)
	case errors.Is(err, fanbox.ErrNotAuthenticated):
		return newError(operation, sdk.CodeCredentialsExpired, err)
	default:
		return newError(operation, sdk.CodeUpstreamError, err)
	}
}
