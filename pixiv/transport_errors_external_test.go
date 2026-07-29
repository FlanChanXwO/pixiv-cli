package pixiv_test

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pixiv"
)

// TestTransportFailureDNSClassification 验证公开 SDK 只暴露稳定、安全的 DNS 子类。
func TestTransportFailureDNSClassification(t *testing.T) {
	t.Parallel()

	client, err := pixiv.NewClient(pixiv.NewClientOptions{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, &net.DNSError{Name: "dns-canary.invalid", Err: "lookup canary"}
		})},
		AppAPIBaseURL: "https://app.invalid",
		AccessToken:   "test-token",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.TrendingTagsIllust(context.Background())
	if err == nil {
		t.Fatal("TrendingTagsIllust() error = nil, want DNS transport failure")
	}
	var typed *pixiv.Error
	if !errors.As(err, &typed) {
		t.Fatalf("errors.As(%T) = false, want *pixiv.Error", err)
	}
	if typed.TransportKind != pixiv.TransportKindDNS {
		t.Errorf("TransportKind = %q, want %q", typed.TransportKind, pixiv.TransportKindDNS)
	}
	if typed.Code != pixiv.CodeUpstreamUnavailable || typed.Operation != pixiv.OperationTrendingTagsIllust || typed.Backend != pixiv.BackendAppAPI || !typed.Retryable {
		t.Errorf("typed error = %#v, want stable upstream-unavailable metadata", typed)
	}
}

// TestDownloadPreservesOpenResourceTransportKind 验证 operation remap 不丢失安全 transport 子类。
func TestDownloadPreservesOpenResourceTransportKind(t *testing.T) {
	t.Parallel()

	client, err := pixiv.NewClient(pixiv.NewClientOptions{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, &net.DNSError{Name: "download-dns-canary.invalid", Err: "lookup canary"}
		})},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.DownloadResource(
		context.Background(),
		pixiv.ResourceRef{URL: "https://i.pximg.net/img-original/a.jpg"},
		filepath.Join(t.TempDir(), "a.jpg"),
	)
	var typed *pixiv.Error
	if !errors.As(err, &typed) {
		t.Fatalf("errors.As(%T) = false, want *pixiv.Error", err)
	}
	if typed.Operation != pixiv.OperationDownload || typed.Backend != pixiv.BackendResource || typed.TransportKind != pixiv.TransportKindDNS {
		t.Errorf("typed error = %#v, want download/resource/DNS metadata", typed)
	}
}

// TestTransportFailureUnknownIsClassifiedWithoutLeakingCause 验证未知失败可识别且安全诊断不回显原始 cause。
func TestTransportFailureUnknownIsClassifiedWithoutLeakingCause(t *testing.T) {
	t.Parallel()

	const rawCanary = "transport-raw-canary"
	unknown := errors.New(rawCanary + " https://user:password@unknown-host.invalid/path?token=query-secret Cookie: session-secret certificate-canary")
	client, err := pixiv.NewClient(pixiv.NewClientOptions{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("wrapped unknown: %w", unknown)
		})},
		AppAPIBaseURL: "https://app.invalid",
		AccessToken:   "test-token",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.TrendingTagsIllust(context.Background())
	var typed *pixiv.Error
	if !errors.As(err, &typed) {
		t.Fatalf("errors.As(%T) = false, want *pixiv.Error", err)
	}
	if typed.TransportKind != pixiv.TransportKindUnknown {
		t.Errorf("TransportKind = %q, want %q", typed.TransportKind, pixiv.TransportKindUnknown)
	}
	if !strings.Contains(err.Error(), "transport_kind=unknown") {
		t.Errorf("Error() = %q, want stable transport_kind diagnostic", err.Error())
	}
	if !errors.Is(err, pixiv.ErrUpstreamUnavailable) || errors.Is(err, unknown) {
		t.Errorf("errors.Is() lost stable sentinel or exposed raw cause: %v", err)
	}
	diagnostics := []string{err.Error(), fmt.Sprint(errors.Unwrap(err)), fmt.Sprintf("%+v", typed)}
	for _, diagnostic := range diagnostics {
		for _, forbidden := range []string{rawCanary, "unknown-host.invalid", "user:password", "query-secret", "session-secret", "certificate-canary"} {
			if strings.Contains(diagnostic, forbidden) {
				t.Errorf("diagnostic %q leaked %q", diagnostic, forbidden)
			}
		}
	}
}

// TestTransportFailureConnectionResetClassification 验证包装后的连接重置 errno 可稳定识别。
func TestTransportFailureConnectionResetClassification(t *testing.T) {
	t.Parallel()

	reset := &url.Error{
		Op:  "Get",
		URL: "https://reset-canary.invalid/path?token=query-secret",
		Err: fmt.Errorf("reset wrapper: %w", &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}),
	}
	client, err := pixiv.NewClient(pixiv.NewClientOptions{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, reset
		})},
		AppAPIBaseURL: "https://app.invalid",
		AccessToken:   "test-token",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.TrendingTagsIllust(context.Background())
	var typed *pixiv.Error
	if !errors.As(err, &typed) {
		t.Fatalf("errors.As(%T) = false, want *pixiv.Error", err)
	}
	if typed.TransportKind != pixiv.TransportKindConnectionReset {
		t.Errorf("TransportKind = %q, want %q", typed.TransportKind, pixiv.TransportKindConnectionReset)
	}
}

// TestTransportFailureConnectionRefusedClassification 验证包装后的拒连 errno 可稳定识别。
func TestTransportFailureConnectionRefusedClassification(t *testing.T) {
	t.Parallel()

	refused := fmt.Errorf("dial wrapper with canary: %w", &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED})
	client, err := pixiv.NewClient(pixiv.NewClientOptions{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, refused
		})},
		AppAPIBaseURL: "https://app.invalid",
		AccessToken:   "test-token",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.TrendingTagsIllust(context.Background())
	var typed *pixiv.Error
	if !errors.As(err, &typed) {
		t.Fatalf("errors.As(%T) = false, want *pixiv.Error", err)
	}
	if typed.TransportKind != pixiv.TransportKindConnectionRefused {
		t.Errorf("TransportKind = %q, want %q", typed.TransportKind, pixiv.TransportKindConnectionRefused)
	}
}

// TestTransportFailureProxyClassification 验证 proxyconnect 优先于内层拒连分类。
func TestTransportFailureProxyClassification(t *testing.T) {
	t.Parallel()

	proxyFailure := &url.Error{
		Op:  "Get",
		URL: "https://proxy-user:proxy-pass@proxy.invalid/path?token=query-secret",
		Err: &net.OpError{Op: "proxyconnect", Net: "tcp", Err: syscall.ECONNREFUSED},
	}
	client, err := pixiv.NewClient(pixiv.NewClientOptions{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, proxyFailure
		})},
		AppAPIBaseURL: "https://app.invalid",
		AccessToken:   "test-token",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.TrendingTagsIllust(context.Background())
	var typed *pixiv.Error
	if !errors.As(err, &typed) {
		t.Fatalf("errors.As(%T) = false, want *pixiv.Error", err)
	}
	if typed.TransportKind != pixiv.TransportKindProxy {
		t.Errorf("TransportKind = %q, want %q", typed.TransportKind, pixiv.TransportKindProxy)
	}
}

// TestTransportFailureTLSClassification 验证证书校验失败映射为 TLS 子类。
func TestTransportFailureTLSClassification(t *testing.T) {
	t.Parallel()

	client, err := pixiv.NewClient(pixiv.NewClientOptions{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, x509.UnknownAuthorityError{}
		})},
		AppAPIBaseURL: "https://app.invalid",
		AccessToken:   "test-token",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.TrendingTagsIllust(context.Background())
	var typed *pixiv.Error
	if !errors.As(err, &typed) {
		t.Fatalf("errors.As(%T) = false, want *pixiv.Error", err)
	}
	if typed.TransportKind != pixiv.TransportKindTLS {
		t.Errorf("TransportKind = %q, want %q", typed.TransportKind, pixiv.TransportKindTLS)
	}
}

// TestTransportFailureContextIdentityIsPreserved 验证公开错误链仍支持标准 context 判断。
func TestTransportFailureContextIdentityIsPreserved(t *testing.T) {
	t.Parallel()

	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		cause := cause
		t.Run(cause.Error(), func(t *testing.T) {
			client, err := pixiv.NewClient(pixiv.NewClientOptions{
				HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					return nil, fmt.Errorf("context wrapper: %w", cause)
				})},
				AppAPIBaseURL: "https://app.invalid",
				AccessToken:   "test-token",
			})
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}

			_, err = client.TrendingTagsIllust(context.Background())
			if !errors.Is(err, cause) {
				t.Errorf("errors.Is(err, %v) = false", cause)
			}
			var typed *pixiv.Error
			if !errors.As(err, &typed) {
				t.Fatalf("errors.As(%T) = false, want *pixiv.Error", err)
			}
			if typed.TransportKind != "" || typed.Retryable {
				t.Errorf("typed error = %#v, want non-retryable context failure without transport subtype", typed)
			}
		})
	}
}

// TestErrorNormalizesUnrecognizedTransportKind 验证公开可写字段不能把任意内容带入诊断。
func TestErrorNormalizesUnrecognizedTransportKind(t *testing.T) {
	t.Parallel()

	const raw = "https://host.invalid/?token=secret"
	typed := &pixiv.Error{
		Code:          pixiv.CodeUpstreamUnavailable,
		TransportKind: pixiv.TransportKind(raw),
	}
	diagnostic := typed.Error()
	for _, forbidden := range []string{raw, "host.invalid", "token=secret"} {
		if strings.Contains(diagnostic, forbidden) {
			t.Errorf("Error() = %q, leaked unrecognized transport kind %q", diagnostic, forbidden)
		}
	}
	if strings.Count(diagnostic, "transport_kind=unknown") != 1 {
		t.Errorf("Error() = %q, want exactly one stable transport_kind=unknown", diagnostic)
	}
}

// TestErrorPreservesDeclaredTransportKinds 验证 whitelist 不改变稳定常量，空值仍省略。
func TestErrorPreservesDeclaredTransportKinds(t *testing.T) {
	t.Parallel()

	for _, kind := range []pixiv.TransportKind{
		pixiv.TransportKindDNS,
		pixiv.TransportKindTLS,
		pixiv.TransportKindProxy,
		pixiv.TransportKindConnectionRefused,
		pixiv.TransportKindConnectionReset,
		pixiv.TransportKindTimeout,
		pixiv.TransportKindUnknown,
	} {
		diagnostic := (&pixiv.Error{Code: pixiv.CodeUpstreamUnavailable, TransportKind: kind}).Error()
		if !strings.Contains(diagnostic, "transport_kind="+string(kind)) {
			t.Errorf("Error() = %q, want declared kind %q unchanged", diagnostic, kind)
		}
	}
	if diagnostic := (&pixiv.Error{Code: pixiv.CodeUpstreamUnavailable}).Error(); strings.Contains(diagnostic, "transport_kind=") {
		t.Errorf("Error() = %q, want empty transport kind omitted", diagnostic)
	}
}
