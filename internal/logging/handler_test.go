package logging_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"regexp"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
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
