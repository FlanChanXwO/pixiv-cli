package api

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

// Failure 表示 adapter 向 facade 交付的脱敏失败。它绝不保存响应 body、URL、
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

// TransportKind 标记传输失败的安全子类，供 facade 决定错误映射。
type TransportKind string

// TransportDNS 等值是 [TransportKind] 的全部取值，分类只依赖标准库的 typed cause。
const (
	TransportDNS               TransportKind = "dns"
	TransportTLS               TransportKind = "tls"
	TransportProxy             TransportKind = "proxy"
	TransportConnectionRefused TransportKind = "connection_refused"
	TransportConnectionReset   TransportKind = "connection_reset"
	TransportTimeout           TransportKind = "timeout"
	TransportUnknown           TransportKind = "unknown"
)

// FailureKind 枚举 [Failure] 的稳定分类。
type FailureKind string

// FailureHTTPStatus 表示上游返回非 2xx；FailureMalformed 表示响应无法解码；
// FailureResourceForbidden 表示资源 locator 未通过策略；FailureTransport 表示传输失败。
const (
	FailureHTTPStatus        FailureKind = "http_status"
	FailureMalformed         FailureKind = "malformed_response"
	FailureResourceForbidden FailureKind = "resource_forbidden"
	FailureTransport         FailureKind = "transport"
)

// ErrMalformedResponse 可由 [errors.Is] 用来识别 malformed 失败。
var ErrMalformedResponse = errors.New("pixiv upstream returned a malformed response")

// HTTPStatus 构造不带 Retry-After 的非 2xx 失败。
func HTTPStatus(status int) Failure {
	return Failure{Kind: FailureHTTPStatus, StatusCode: status}
}

// HTTPStatusWithRetryAfter 构造带可选 Retry-After 的非 2xx 失败；present 为 false 时
// RetryAfter 无意义。
func HTTPStatusWithRetryAfter(status int, retryAfter time.Duration, present bool) Failure {
	return Failure{
		Kind: FailureHTTPStatus, StatusCode: status, RetryAfter: retryAfter, HasRetryAfter: present,
	}
}

// MalformedResponse 构造响应无法解码的失败。
func MalformedResponse() Failure {
	return Failure{Kind: FailureMalformed}
}

// Transport 把传输错误转成脱敏失败；取消与 deadline 保留为可 unwrap 的原因。
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

// Error 按 FailureKind 输出固定的英文文本，不含 URL、header、body 或上游消息。
func (e Failure) Error() string {
	switch e.Kind {
	case FailureHTTPStatus:
		return fmt.Sprintf("pixiv upstream returned HTTP status %d", e.StatusCode)
	case FailureMalformed:
		return ErrMalformedResponse.Error()
	case FailureResourceForbidden:
		return "pixiv resource URL is not allowed"
	default:
		return "pixiv upstream transport failed"
	}
}

// Unwrap 返回保留的 context 取消或 deadline 原因。
func (e Failure) Unwrap() error { return e.cause }

// Is 支持用 [ErrMalformedResponse] 判断 malformed 失败。
func (e Failure) Is(target error) bool {
	return target == ErrMalformedResponse && e.Kind == FailureMalformed
}
