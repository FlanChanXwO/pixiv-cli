package protocol

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"syscall"
	"time"
)

// Failure 是 adapter 向 facade 交付的脱敏失败。它绝不保存响应 body、URL、
// header、token、cookie 或 Web envelope message；只有取消和 deadline 可作为
// 可 unwrap 的底层原因，以保留 Go 的 context 语义。
type Failure struct {
	Kind          FailureKind
	TransportKind TransportKind
	StatusCode    int
	RetryAfter    time.Duration
	HasRetryAfter bool
	cause         error
}

// TransportKind 是 adapter 交付给 facade 的安全传输失败子类。
type TransportKind string

const (
	TransportDNS               TransportKind = "dns"
	TransportTLS               TransportKind = "tls"
	TransportProxy             TransportKind = "proxy"
	TransportConnectionRefused TransportKind = "connection_refused"
	TransportConnectionReset   TransportKind = "connection_reset"
	TransportTimeout           TransportKind = "timeout"
	TransportUnknown           TransportKind = "unknown"
)

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
func HTTPStatusWithRetryAfter(status int, retryAfter time.Duration, present bool) Failure {
	return Failure{Kind: FailureHTTPStatus, StatusCode: status, RetryAfter: retryAfter, HasRetryAfter: present}
}
func MalformedResponse() Failure { return Failure{Kind: FailureMalformed} }
func UpstreamRejected() Failure  { return Failure{Kind: FailureRejected} }
func Forbidden() Failure         { return Failure{Kind: FailureForbidden} }
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
	return Failure{Kind: FailureTransport, TransportKind: classifyTransportKind(err)}
}

// classifyTransportKind 只依赖标准库的 typed cause；错误文本可能包含敏感地址或
// 凭据，不得作为分类输入。proxyconnect 必须先于其常见的内层 ECONNREFUSED。
func classifyTransportKind(err error) TransportKind {
	var operationError *net.OpError
	if errors.As(err, &operationError) && operationError.Op == "proxyconnect" {
		return TransportProxy
	}
	var dnsError *net.DNSError
	if errors.As(err, &dnsError) {
		return TransportDNS
	}
	if isTLSTransportError(err) {
		return TransportTLS
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return TransportConnectionRefused
	}
	if errors.Is(err, syscall.ECONNRESET) {
		return TransportConnectionReset
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return TransportTimeout
	}
	return TransportUnknown
}

func isTLSTransportError(err error) bool {
	var certificateVerificationError *tls.CertificateVerificationError
	if errors.As(err, &certificateVerificationError) {
		return true
	}
	var unknownAuthorityError x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthorityError) {
		return true
	}
	var recordHeaderError tls.RecordHeaderError
	if errors.As(err, &recordHeaderError) {
		return true
	}
	var alertError tls.AlertError
	return errors.As(err, &alertError)
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
