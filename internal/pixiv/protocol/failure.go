package protocol

import (
	"context"
	"errors"
	"fmt"
)

// Failure 是 adapter 向 facade 交付的脱敏失败。它绝不保存响应 body、URL、
// header、token、cookie 或 Web envelope message；只有取消和 deadline 可作为
// 可 unwrap 的底层原因，以保留 Go 的 context 语义。
type Failure struct {
	Kind       FailureKind
	StatusCode int
	cause      error
}

type FailureKind string

const (
	FailureHTTPStatus FailureKind = "http_status"
	FailureMalformed  FailureKind = "malformed_response"
	FailureRejected   FailureKind = "upstream_rejected"
	FailureForbidden  FailureKind = "forbidden"
	FailureTransport  FailureKind = "transport"
)

var ErrMalformedResponse = errors.New("pixiv upstream returned a malformed response")

func HTTPStatus(status int) Failure { return Failure{Kind: FailureHTTPStatus, StatusCode: status} }
func MalformedResponse() Failure    { return Failure{Kind: FailureMalformed} }
func UpstreamRejected() Failure     { return Failure{Kind: FailureRejected} }
func Forbidden() Failure            { return Failure{Kind: FailureForbidden} }
func Transport(err error) Failure {
	var failure Failure
	if errors.As(err, &failure) {
		return failure
	}
	if errors.Is(err, context.Canceled) {
		return Failure{Kind: FailureTransport, cause: context.Canceled}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return Failure{Kind: FailureTransport, cause: context.DeadlineExceeded}
	}
	return Failure{Kind: FailureTransport}
}

func (e Failure) Error() string {
	switch e.Kind {
	case FailureHTTPStatus:
		return fmt.Sprintf("pixiv upstream returned HTTP status %d", e.StatusCode)
	case FailureMalformed:
		return ErrMalformedResponse.Error()
	case FailureRejected:
		return "pixiv upstream rejected the request"
	case FailureForbidden:
		return "pixiv request was forbidden by policy"
	default:
		return "pixiv upstream transport failed"
	}
}
func (e Failure) Unwrap() error { return e.cause }
func (e Failure) Is(target error) bool {
	return target == ErrMalformedResponse && e.Kind == FailureMalformed
}
