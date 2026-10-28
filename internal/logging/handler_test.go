package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"syscall"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

func TestTextHandlerResolvesPrivateLogValues(t *testing.T) {
	var output bytes.Buffer
	logger, err := logging.New(&output, "info", logging.FormatText)
	require.NoError(t, err)
	logger.Info("request", slog.Any("token", privateLogValue("credential-canary")))
	require.Contains(t, output.String(), "<redacted>")
	require.NotContains(t, output.String(), "credential-canary")
}

func TestTextHandlerFormatAndCallLabel(t *testing.T) {
	unsetNoColor(t)
	var output bytes.Buffer
	logger, err := logging.New(&output, "debug", logging.FormatText)
	require.NoError(t, err)
	ctx := logging.WithCall(context.Background(), logger, "pixiv search", "7")
	logger.WarnContext(ctx, "Request rejected", slog.Int("status", 429))
	require.Regexp(t, `^\[[0-9]{2}:[0-9]{2}:[0-9]{2}\.[0-9]{3}\] \[pixiv search\] \[WARN\] \[logging/handler_test\.go:[0-9]+\]: Request rejected status=429 call_id=7\n$`, output.String())
	require.NotContains(t, output.String(), "operation=")
	require.NotContains(t, output.String(), "\x1b[")
}

func TestTextHandlerOmitsGroupedOperation(t *testing.T) {
	var output bytes.Buffer
	logger, err := logging.New(&output, "info", logging.FormatText)
	require.NoError(t, err)
	ctx := logging.WithCall(context.Background(), logger, "pixiv search", "")
	logger.WithGroup("request").InfoContext(ctx, "request", slog.String("host", "pixiv.net"))
	require.Regexp(t, `\[pixiv search\] \[INFO\]`, output.String())
	require.NotContains(t, output.String(), "operation=")
	require.Contains(t, output.String(), "request.host=pixiv.net")
}

func TestTextHandlerQuotesUnprintableTextAndValues(t *testing.T) {
	unsetNoColor(t)
	var output bytes.Buffer
	logger, err := logging.New(&output, "info", logging.FormatText)
	require.NoError(t, err)
	logger.Info("bad\x1b[31m", slog.String("key\nx", "\x00value"), slog.String("raw", "two words"))
	require.Contains(t, output.String(), `]: "bad\x1b[31m" "key\nx"="\x00value" raw="two words"`)
}

func TestTextHandlerIndentsMultilineValues(t *testing.T) {
	unsetNoColor(t)
	var output bytes.Buffer
	logger, err := logging.New(&output, "info", logging.FormatText)
	require.NoError(t, err)
	logger.Info("failed", slog.String(logging.AttrError, "first line\nsecond line"), slog.Int("count", 2))
	require.Contains(t, output.String(), " count=2 error=\"first line\n    second line\"\n")
}

func TestTextHandlerStopsAtFirstJoinLevel(t *testing.T) {
	unsetNoColor(t)
	var output bytes.Buffer
	logger, err := logging.New(&output, "info", logging.FormatText)
	require.NoError(t, err)
	logger.Error("failed", slog.Any(logging.AttrError, errors.Join(wrappedFailure{}, errors.New("second"))))
	require.Contains(t, output.String(), "public failure")
	require.Contains(t, output.String(), "second")
	require.NotContains(t, output.String(), "private cause")
}

type wrappedFailure struct{}

func (wrappedFailure) Error() string { return "public failure" }

func (wrappedFailure) Unwrap() error { return errors.New("private cause") }

func TestJSONHandlerWritesSource(t *testing.T) {
	var output bytes.Buffer
	logger, err := logging.New(&output, "info", logging.FormatJSON)
	require.NoError(t, err)
	logger.Info("request")
	require.Regexp(t, regexp.MustCompile(`"source":\{"function":"[^"]*TestJSONHandlerWritesSource"`), output.String())
}

func unsetNoColor(t *testing.T) {
	t.Helper()
	value, set := os.LookupEnv("NO_COLOR")
	require.NoError(t, os.Unsetenv("NO_COLOR"))
	t.Cleanup(func() {
		if set {
			require.NoError(t, os.Setenv("NO_COLOR", value))
		}
	})
}

type privateLogValue string

func (privateLogValue) LogValue() slog.Value { return slog.StringValue("<redacted>") }

func TestSDKErrorOriginPreservesPresentation(t *testing.T) {
	err := sdk.NewError("pixiv", "Open", sdk.UpstreamUnavailable)
	plain := &sdk.Error{Product: "pixiv", Operation: "Open", Reason: sdk.UpstreamUnavailable}
	require.Equal(t, plain.Error(), err.Error())
	actual, marshalErr := json.Marshal(err)
	require.NoError(t, marshalErr)
	expected, marshalErr := json.Marshal(plain)
	require.NoError(t, marshalErr)
	require.JSONEq(t, string(expected), string(actual))
	require.Regexp(t, `logging/handler_test.go:[0-9]+`, logging.OriginAttr(fmt.Errorf("safe: %w", err))[0].Value.String())
	require.Empty(t, logging.OriginAttr(errors.New("local failure")))
	require.Empty(t, logging.OriginAttr(plain))
}

func TestTransportDetailsExcludeURLCredentials(t *testing.T) {
	var output bytes.Buffer
	logger, err := logging.New(&output, "debug", logging.FormatJSON)
	require.NoError(t, err)
	cause := &url.Error{Op: "Get", URL: "https://canary-user:canary-password@example.test/private?token=secret", Err: &net.OpError{Op: "dial", Net: "tcp", Addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1}, Err: &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED}}}
	logger.LogAttrs(t.Context(), slog.LevelDebug, "Request failed", logging.TransportAttrs(cause)...)
	require.Contains(t, output.String(), `"op":"dial"`)
	require.Contains(t, output.String(), `"net":"tcp"`)
	require.Contains(t, output.String(), `"address":"127.0.0.1:1"`)
	require.Contains(t, output.String(), `"errno":"ECONNREFUSED"`)
	for _, private := range []string{"canary-user", "canary-password", "example.test", "/private", "secret"} {
		require.NotContains(t, output.String(), private)
	}
}

func TestAuthorizationExchangeUsesCallLogger(t *testing.T) {
	var output bytes.Buffer
	logger, err := logging.New(&output, "debug", logging.FormatJSON)
	require.NoError(t, err)
	proxy, err := url.Parse("http://user-canary:password-canary@127.0.0.1:1")
	require.NoError(t, err)
	transport := &http.Transport{Proxy: http.ProxyURL(proxy)}
	defer transport.CloseIdleConnections()
	session, err := pixiv.BeginLogin(pixiv.LoginOptions{HTTPClient: &http.Client{Transport: transport}})
	require.NoError(t, err)
	ctx := logging.WithCall(t.Context(), logger, "pixiv auth login", "login")
	_, err = session.Complete(ctx, "code-canary")
	require.Error(t, err)
	require.Contains(t, output.String(), "Request failed")
	require.Contains(t, output.String(), `"sdk_operation":"Complete"`)
	for _, secret := range []string{"user-canary", "password-canary", "code-canary"} {
		require.NotContains(t, output.String(), secret)
	}
}
